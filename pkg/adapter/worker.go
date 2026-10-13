package adapter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	v1 "github.com/czifrushanna/sleeko/pkg/api/v1"
	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/pipeline"
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
		if !errors.Is(err, dockerfile.ErrNoProvenance) {
			return failedReport(v1.ErrorCodeExtractionFailed, err.Error())
		}

		// No usable BuildKit provenance attestation on this artifact (e.g. it
		// wasn't built with BuildKit, was pulled/re-pushed without one, or was
		// built with --sbom but without --provenance): fall back to
		// reconstructing a best-effort Dockerfile from the image's own config
		// history rather than failing outright.
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

	// 4. Run the configured optimization pipeline (by default the single
	// "generate" LLM call). The scan report is condensed and fitted to the
	// model's context window on the way into the prompt.
	st := &pipeline.State{
		Original:   result.Dockerfile,
		Generated:  result.Generated,
		ScanReport: req.ScanReport,
	}
	if err := s.pipeline.Run(ctx, st); err != nil {
		// The Harbor contract has no failure code beyond LLM_FAILED for this part
		// of the run, so every terminal stage error is reported under it, with the
		// stage's own message so the report reads exactly as it did before stages
		// existed.
		var stageErr *pipeline.StageError
		if errors.As(err, &stageErr) {
			err = stageErr.Err
		}
		return failedReport(v1.ErrorCodeLLMFailed, err.Error())
	}
	for _, d := range st.Diagnostics {
		log.Printf("%s: %s", d.Stage, d.Message)
	}

	return &v1.OptimizationReport{
		Status:                    v1.ReportStatusSuccess,
		Dockerfile:                result.Dockerfile,
		OptimizedDockerfile:       st.Candidate,
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
