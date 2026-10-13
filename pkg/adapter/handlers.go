package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"

	v1 "github.com/czifrushanna/sleeko/pkg/api/v1"
	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/pipeline"
)

const (
	// adapterName/adapterVendor identify this adapter in its metadata document.
	adapterName   = "sleeko"
	adapterVendor = "sleeko"

	// refreshAfterSeconds is the poll interval hint returned while a job is running.
	refreshAfterSeconds = 5
)

// Version of the adapter, injected at build time via
// -ldflags "-X github.com/czifrushanna/sleeko/pkg/adapter.Version=...".
var Version = "dev"

// Server is the optimizer adapter HTTP service.
type Server struct {
	cfg      *Config
	store    *store
	pipeline *pipeline.Pipeline
	// sem bounds the number of concurrently running optimizations.
	sem chan struct{}
}

// NewServer creates the adapter server. Call Handler() to obtain the http.Handler
// and StartSweeper to enable TTL eviction of finished jobs. An empty
// cfg.Pipeline runs pipeline.DefaultNames(). It panics on an unknown stage name:
// LoadConfig rejects those, so one reaching here is a programming error.
func NewServer(cfg *Config) *Server {
	names := cfg.Pipeline
	if len(names) == 0 {
		names = pipeline.DefaultNames()
	}
	p, err := pipeline.New(names, pipeline.Deps{
		LLM: pipeline.LLM{
			APIBaseURL: cfg.APIBaseURL,
			APIKey:     cfg.APIKey,
			Model:      cfg.Model,
			Prompt:     cfg.Prompt,
			Budget:     dockerfile.Budget{MaxTokens: cfg.MaxTokens, ContextWindow: cfg.ContextWindow},
		},
	})
	if err != nil {
		panic(fmt.Sprintf("sleeko: build pipeline: %v", err))
	}

	return &Server{
		cfg:      cfg,
		store:    newStore(cfg.JobTTL),
		pipeline: p,
		sem:      make(chan struct{}, cfg.MaxConcurrency),
	}
}

// StartSweeper launches the background TTL sweeper until ctx is done.
func (s *Server) StartSweeper(ctx context.Context) {
	go s.store.sweep(ctx)
}

// Handler returns the HTTP handler implementing the optimizer adapter REST contract.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/metadata", s.getMetadata)
	mux.HandleFunc("POST /api/v1/optimize", s.submitOptimize)
	mux.HandleFunc("GET /api/v1/optimize/{id}/report", s.getReport)
	mux.HandleFunc("GET /probe/healthy", s.healthy)
	return mux
}

// metadata returns the adapter metadata document used for registration validation
// and capability negotiation.
func (s *Server) metadata() *v1.OptimizerAdapterMetadata {
	return &v1.OptimizerAdapterMetadata{
		Optimizer: &v1.Optimizer{
			Name:    adapterName,
			Vendor:  adapterVendor,
			Version: Version,
		},
		Capabilities: []*v1.OptimizerCapability{
			{
				// BuildKit provenance attestations hang off the image index, so the
				// index/list mime types are the primary targets; plain manifests are
				// accepted too (they fall back to the image-history Dockerfile
				// generator and only terminate with a NO_ATTESTATION report if that
				// fallback also fails).
				ConsumesMimeTypes: []string{
					v1.MimeTypeOCIIndex,
					v1.MimeTypeDockerManifestList,
					v1.MimeTypeOCIArtifact,
					v1.MimeTypeDockerArtifact,
				},
				ProducesMimeTypes: []string{
					v1.MimeTypeOptimizationReport,
				},
			},
		},
		Properties: v1.OptimizerProperties{
			"harbor.optimizer-adapter/registry-authorization-type": "Basic",
		},
	}
}

func (s *Server) getMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, v1.MimeTypeAdapterMeta, s.metadata())
}

func (s *Server) submitOptimize(w http.ResponseWriter, r *http.Request) {
	req := &v1.OptimizeRequest{}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode optimize request: %v", err))
		return
	}

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	id := uuid.NewString()
	s.store.put(id)

	go func() {
		s.sem <- struct{}{}
		defer func() { <-s.sem }()
		s.process(id, req)
	}()

	log.Printf("accepted optimize request %s for %s@%s", id, req.Artifact.Repository, req.Artifact.Digest)
	writeJSON(w, http.StatusAccepted, v1.MimeTypeOptimizeResponse, &v1.OptimizeResponse{ID: id})
}

func (s *Server) getReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	entry := s.store.get(id)
	if entry == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("optimize request %s not found", id))
		return
	}

	if entry.report == nil {
		// Still running: the client translates 302 + Refresh-After into a retry.
		w.Header().Set("Refresh-After", fmt.Sprintf("%d", refreshAfterSeconds))
		w.WriteHeader(http.StatusFound)
		return
	}

	writeJSON(w, http.StatusOK, v1.MimeTypeOptimizationReport, entry.report)
}

func (s *Server) healthy(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func writeJSON(w http.ResponseWriter, status int, contentType string, v any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, "application/json", &v1.ErrorResponse{
		Err: &v1.Error{Message: message},
	})
}
