package adapter

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	v1 "github.com/czifrushanna/sleeko/pkg/api/v1"
	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/registry"
)

// workerTimeout bounds one optimization end to end (registry pulls + LLM call).
const workerTimeout = 5 * time.Minute

// newRegistryClient constructs the registry client used to pull artifact blobs.
// Extracted as a var (rather than calling registry.NewClient directly in run())
// so tests can substitute a fake registry.Client instead of standing up a real
// HTTP registry to exercise run()'s error-classification branches.
var newRegistryClient = func(url, authorization string, insecure bool) registry.Client {
	return registry.NewClient(url, authorization, insecure)
}

// process runs the optimization pipeline for one accepted request and stores the
// terminal report. All pipeline failures become Failed reports, never HTTP errors,
// so the polling Harbor job can always persist an outcome.
func (s *Server) process(id string, req *v1.OptimizeRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), workerTimeout)
	defer cancel()

	report := s.run(ctx, req)
	s.store.complete(id, report)
	log.Printf("optimization %s finished with status %s", id, report.Status)
}

func (s *Server) run(ctx context.Context, req *v1.OptimizeRequest) *v1.OptimizationReport {
	// 1. Pull the artifact blobs from the Harbor registry with the handed credential.
	cli := newRegistryClient(req.Registry.URL, req.Registry.Authorization, req.Registry.Insecure)

	src := &dockerfile.RegistryBlobSource{
		Client:     cli,
		Repository: req.Artifact.Repository,
		Reference:  req.Artifact.Digest,
	}

	// 2. Extract the Dockerfile from the BuildKit provenance attestation.
	result, err := dockerfile.NewWorkflow().ExtractDockerfileFromSource(ctx, src)
	if err != nil {
		if !strings.Contains(err.Error(), "attestation manifest not found") {
			return failedReport(v1.ErrorCodeExtractionFailed, err.Error())
		}

		// No BuildKit provenance attestation on this artifact (e.g. it wasn't
		// built with BuildKit, or was pulled/re-pushed without one): fall back
		// to reconstructing a best-effort Dockerfile from the image's own
		// config history rather than failing outright.
		generated, genErr := dockerfile.GenerateDockerfileFromSource(ctx, src)
		if genErr != nil {
			return failedReport(v1.ErrorCodeNoAttestation,
				fmt.Sprintf("no provenance attestation, and Dockerfile generation fallback also failed: %s", genErr.Error()))
		}
		result = generated
	}

	// 3. Require a completed vulnerability scan report before calling the LLM: the
	// optimization prompt is meant to factor in known vulnerabilities, so optimizing
	// without one would silently skip that. Harbor's core attaches the report (if
	// any) to the request; an empty ScanReport means the artifact has not been
	// scanned (or the scan hasn't finished) yet.
	if req.ScanReport == "" {
		return failedReport(v1.ErrorCodeNoScanReport,
			"no vulnerability scan report found for this artifact; trigger a scan and try again")
	}

	// 4. Optimize via the LLM gateway. The scan report is condensed and fitted to
	// the model's context window on the way into the prompt.
	budget := dockerfile.Budget{MaxTokens: s.cfg.MaxTokens, ContextWindow: s.cfg.ContextWindow}
	optimized, err := dockerfile.OptimizeDockerfile(ctx, s.cfg.APIBaseURL, s.cfg.APIKey, s.cfg.Model, s.cfg.Prompt, result.Dockerfile, req.ScanReport, budget)
	if err != nil {
		return failedReport(v1.ErrorCodeLLMFailed, err.Error())
	}

	return &v1.OptimizationReport{
		Status:                    v1.ReportStatusSuccess,
		Dockerfile:                result.Dockerfile,
		OptimizedDockerfile:       stripCodeFences(optimized),
		AttestationManifestDigest: result.AttestationManifestDigest,
		StatementDigest:           result.StatementDigest,
		Generated:                 result.Generated,
	}
}

func failedReport(code, message string) *v1.OptimizationReport {
	return &v1.OptimizationReport{
		Status: v1.ReportStatusFailed,
		Error: &v1.ReportError{
			Code:    code,
			Message: message,
		},
	}
}

// stripCodeFences removes a leading ``` / ```dockerfile line and a trailing ```
// line that LLM gateways commonly wrap around the returned Dockerfile.
func stripCodeFences(s string) string {
	out := strings.TrimSpace(s)

	if strings.HasPrefix(out, "```") {
		if idx := strings.IndexByte(out, '\n'); idx >= 0 {
			out = out[idx+1:]
		} else {
			// A lone fence line with no content
			return ""
		}
	}

	if strings.HasSuffix(strings.TrimSpace(out), "```") {
		trimmed := strings.TrimSpace(out)
		trimmed = strings.TrimSuffix(trimmed, "```")
		out = strings.TrimRight(trimmed, " \t\n")
	}

	return strings.TrimSpace(out) + "\n"
}
