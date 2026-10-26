package adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/czifrushanna/sleeko/pkg/api/v1"
	"github.com/czifrushanna/sleeko/pkg/registry"
)

// fakeRegistryClient is a hand-written stub of registry.Client, keyed by
// reference/digest (the tests never care which repository was requested).
type fakeRegistryClient struct {
	manifests   map[string][]byte
	manifestErr map[string]error
	blobs       map[string][]byte
}

func newFakeRegistryClient() *fakeRegistryClient {
	return &fakeRegistryClient{
		manifests:   map[string][]byte{},
		manifestErr: map[string]error{},
		blobs:       map[string][]byte{},
	}
}

func (f *fakeRegistryClient) PullManifest(_, reference string) ([]byte, error) {
	if err, ok := f.manifestErr[reference]; ok {
		return nil, err
	}
	if data, ok := f.manifests[reference]; ok {
		return data, nil
	}
	return nil, errors.New("manifest not found")
}

func (f *fakeRegistryClient) PullBlob(_, digest string) (io.ReadCloser, error) {
	if data, ok := f.blobs[digest]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return nil, errors.New("blob not found")
}

// withFakeRegistryClient overrides newRegistryClient for the duration of the test
// so run() talks to a fake registry.Client instead of a real HTTP endpoint.
func withFakeRegistryClient(t *testing.T, cli registry.Client) {
	t.Helper()
	orig := newRegistryClient
	newRegistryClient = func(_, _ string, _ bool) registry.Client { return cli }
	t.Cleanup(func() { newRegistryClient = orig })
}

// sampleOptimizeRequest carries a non-empty ScanReport by default so extraction-success
// tests reach the LLM step; TestRun_NoScanReport covers the empty-ScanReport gate.
func sampleOptimizeRequest() *v1.OptimizeRequest {
	return &v1.OptimizeRequest{
		Registry:   &v1.Registry{URL: "http://registry.invalid", Authorization: "Basic dXNlcjpwYXNz"},
		Artifact:   &v1.Artifact{Repository: "library/photon", Digest: "sha256:artifact"},
		ScanReport: `{"vulnerabilities":[]}`,
	}
}

func TestFailedReport(t *testing.T) {
	report := failedReport(v1.ErrorCodeLLMFailed, "boom")
	require.Equal(t, v1.ReportStatusFailed, report.Status)
	require.NotNil(t, report.Error)
	require.Equal(t, v1.ErrorCodeLLMFailed, report.Error.Code)
	require.Equal(t, "boom", report.Error.Message)
}

func TestRun_NoAttestation(t *testing.T) {
	cli := newFakeRegistryClient()
	cli.manifests["sha256:artifact"] = []byte(`{"manifests":[]}`)
	withFakeRegistryClient(t, cli)

	s := testServer()
	report := s.run(context.Background(), sampleOptimizeRequest())

	require.Equal(t, v1.ReportStatusFailed, report.Status)
	require.NotNil(t, report.Error)
	require.Equal(t, v1.ErrorCodeNoAttestation, report.Error.Code)
}

func TestRun_ExtractionFailedOnRegistryError(t *testing.T) {
	cli := newFakeRegistryClient()
	cli.manifestErr["sha256:artifact"] = errors.New("registry unavailable")
	withFakeRegistryClient(t, cli)

	s := testServer()
	report := s.run(context.Background(), sampleOptimizeRequest())

	require.Equal(t, v1.ReportStatusFailed, report.Status)
	require.NotNil(t, report.Error)
	require.Equal(t, v1.ErrorCodeExtractionFailed, report.Error.Code)
}

// buildValidChain returns index/attestation/statement bytes for a full
// extraction success, following the same shape used by pkg/dockerfile's own
// workflow tests.
func buildValidChain(t *testing.T, dockerfile string) (idx, attestation, statement []byte) {
	t.Helper()

	encoded := base64.StdEncoding.EncodeToString([]byte(dockerfile))

	indexJSON := map[string]any{
		"manifests": []map[string]any{{
			"digest": "sha256:attestation",
			"annotations": map[string]string{
				"vnd.docker.reference.type": "attestation-manifest",
			},
		}},
	}
	attestationJSON := map[string]any{
		"layers": []map[string]string{{"digest": "sha256:statement"}},
	}
	statementJSON := map[string]any{
		"predicate": map[string]any{
			"runDetails": map[string]any{
				"metadata": map[string]any{
					"buildkit_metadata": map[string]any{
						"source": map[string]any{
							"infos": []map[string]string{{"data": encoded}},
						},
					},
				},
			},
		},
	}

	var err error
	idx, err = json.Marshal(indexJSON)
	require.NoError(t, err)
	attestation, err = json.Marshal(attestationJSON)
	require.NoError(t, err)
	statement, err = json.Marshal(statementJSON)
	require.NoError(t, err)
	return idx, attestation, statement
}

func TestRun_SuccessAndLLMFailedShareExtraction(t *testing.T) {
	idx, attestation, statement := buildValidChain(t, "FROM alpine:3.20\nRUN echo hello\n")

	newCli := func() *fakeRegistryClient {
		cli := newFakeRegistryClient()
		cli.manifests["sha256:artifact"] = idx
		cli.manifests["sha256:attestation"] = attestation
		cli.manifests["sha256:statement"] = statement
		return cli
	}

	t.Run("LLM failure surfaces as LLM_FAILED", func(t *testing.T) {
		withFakeRegistryClient(t, newCli())

		llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "gateway error")
		}))
		defer llm.Close()

		s := NewServer(&Config{APIKey: "key", APIBaseURL: llm.URL, Model: "test-model", MaxConcurrency: 1})
		report := s.run(context.Background(), sampleOptimizeRequest())

		require.Equal(t, v1.ReportStatusFailed, report.Status)
		require.NotNil(t, report.Error)
		require.Equal(t, v1.ErrorCodeLLMFailed, report.Error.Code)
	})

	t.Run("success end to end, including code-fence stripping", func(t *testing.T) {
		withFakeRegistryClient(t, newCli())

		llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"`+"```dockerfile\\nFROM alpine:3.21\\n```"+`"}}]}`)
			fmt.Fprintln(w, "data: [DONE]")
		}))
		defer llm.Close()

		s := NewServer(&Config{APIKey: "key", APIBaseURL: llm.URL, Model: "test-model", MaxConcurrency: 1})
		report := s.run(context.Background(), sampleOptimizeRequest())

		require.Equal(t, v1.ReportStatusSuccess, report.Status)
		require.Equal(t, "FROM alpine:3.20\nRUN echo hello\n", report.Dockerfile)
		require.Equal(t, "FROM alpine:3.21\n", report.OptimizedDockerfile)
		require.Equal(t, "sha256:attestation", report.AttestationManifestDigest)
		require.Equal(t, "sha256:statement", report.StatementDigest)
	})
}

func TestRun_CondensesScanReportAndBoundsThePrompt(t *testing.T) {
	idx, attestation, statement := buildValidChain(t, "FROM alpine:3.20\nRUN echo hello\n")

	cli := newFakeRegistryClient()
	cli.manifests["sha256:artifact"] = idx
	cli.manifests["sha256:attestation"] = attestation
	cli.manifests["sha256:statement"] = statement
	withFakeRegistryClient(t, cli)

	// A report the size of a real base image's: too large to send as it stands.
	items := make([]map[string]any, 0, 3000)
	for i := 0; i < 3000; i++ {
		items = append(items, map[string]any{
			"id":          fmt.Sprintf("CVE-2026-%04d", i),
			"package":     fmt.Sprintf("package-%04d", i%1500),
			"version":     "1.0.0-1~deb12u1",
			"fix_version": "1.0.1-1~deb12u1",
			"severity":    []string{"Critical", "High", "Medium", "Low"}[i%4],
			"description": "prose the model cannot act on. ",
			"links":       []string{"https://nvd.nist.gov/vuln/detail/CVE-2026-0001"},
		})
	}
	rawReport, err := json.Marshal(map[string]any{"vulnerabilities": items})
	require.NoError(t, err)

	var sent struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
		MaxTokens int `json:"max_tokens"`
	}
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM alpine:3.21"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer llm.Close()

	s := NewServer(&Config{
		APIKey: "key", APIBaseURL: llm.URL, Model: "test-model", MaxConcurrency: 1,
		Prompt: "optimize this:\n", MaxTokens: 4096, ContextWindow: 32768,
	})

	req := sampleOptimizeRequest()
	req.ScanReport = string(rawReport)
	report := s.run(context.Background(), req)

	require.Equal(t, v1.ReportStatusSuccess, report.Status)
	require.Equal(t, 4096, sent.MaxTokens)

	prompt := sent.Messages[0].Content
	require.Contains(t, prompt, "optimize this:")
	require.Contains(t, prompt, "FROM alpine:3.20")
	require.Contains(t, prompt, "3000 findings in 1500 packages")
	require.NotContains(t, prompt, "prose the model cannot act on")
	// Inside the window with the completion budget still reserved.
	require.Less(t, len(prompt), (32768-4096-512)*2)
	require.Less(t, len(prompt), len(rawReport)/5)
}

func TestRun_NoScanReport(t *testing.T) {
	idx, attestation, statement := buildValidChain(t, "FROM alpine:3.20\nRUN echo hello\n")

	cli := newFakeRegistryClient()
	cli.manifests["sha256:artifact"] = idx
	cli.manifests["sha256:attestation"] = attestation
	cli.manifests["sha256:statement"] = statement
	withFakeRegistryClient(t, cli)

	// No LLM server is started: if the pipeline called the LLM despite the missing
	// scan report, the request would fail with a connection error, not this code.
	s := NewServer(&Config{APIKey: "key", APIBaseURL: "http://llm.invalid", Model: "test-model", MaxConcurrency: 1})

	req := sampleOptimizeRequest()
	req.ScanReport = ""
	report := s.run(context.Background(), req)

	require.Equal(t, v1.ReportStatusFailed, report.Status)
	require.NotNil(t, report.Error)
	require.Equal(t, v1.ErrorCodeNoScanReport, report.Error.Code)
}

func TestRun_GeneratesDockerfileWhenNoAttestation(t *testing.T) {
	idx := []byte(`{"config":{"digest":"sha256:cfg"},"layers":[{"digest":"sha256:layer1"}]}`)
	cfg := []byte(`{
		"config": {"Env": ["PATH=/usr/bin"], "Cmd": ["/bin/sh"]},
		"history": [
			{"created_by": "/bin/sh -c #(nop)  ENV PATH=/usr/bin", "empty_layer": true},
			{"created_by": "/bin/sh -c apk add --no-cache curl", "empty_layer": false},
			{"created_by": "/bin/sh -c #(nop)  CMD [\"/bin/sh\"]", "empty_layer": true}
		]
	}`)

	cli := newFakeRegistryClient()
	cli.manifests["sha256:artifact"] = idx
	cli.manifests["sha256:cfg"] = cfg
	withFakeRegistryClient(t, cli)

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM alpine:3.21\n"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer llm.Close()

	s := NewServer(&Config{APIKey: "key", APIBaseURL: llm.URL, Model: "test-model", MaxConcurrency: 1})
	report := s.run(context.Background(), sampleOptimizeRequest())

	require.Equal(t, v1.ReportStatusSuccess, report.Status)
	require.True(t, report.Generated)
	require.Contains(t, report.Dockerfile, "FROM scratch")
	require.Contains(t, report.Dockerfile, "ENV PATH=/usr/bin")
	require.Contains(t, report.Dockerfile, "RUN apk add --no-cache curl")
	require.Contains(t, report.Dockerfile, `CMD ["/bin/sh"]`)
	require.Equal(t, "FROM alpine:3.21\n", report.OptimizedDockerfile)
	require.Empty(t, report.AttestationManifestDigest)
}

func TestRun_NoAttestationAndGenerationFailsStillNoAttestation(t *testing.T) {
	// No manifests at all, and no config digest either: the generation
	// fallback can't find anything to work with, so the report stays
	// classified as NO_ATTESTATION (nothing usable was found either way).
	cli := newFakeRegistryClient()
	cli.manifests["sha256:artifact"] = []byte(`{"manifests":[]}`)
	withFakeRegistryClient(t, cli)

	s := testServer()
	report := s.run(context.Background(), sampleOptimizeRequest())

	require.Equal(t, v1.ReportStatusFailed, report.Status)
	require.NotNil(t, report.Error)
	require.Equal(t, v1.ErrorCodeNoAttestation, report.Error.Code)
	require.Contains(t, report.Error.Message, "generation fallback also failed")
	require.False(t, report.Generated)
}

func TestProcess_StoresTerminalReport(t *testing.T) {
	cli := newFakeRegistryClient()
	cli.manifests["sha256:artifact"] = []byte(`{"manifests":[]}`)
	withFakeRegistryClient(t, cli)

	s := testServer()
	s.store.put("job-1")

	s.process("job-1", sampleOptimizeRequest())

	entry := s.store.get("job-1")
	require.NotNil(t, entry)
	require.NotNil(t, entry.report)
	require.Equal(t, v1.ReportStatusFailed, entry.report.Status)
	require.Equal(t, v1.ErrorCodeNoAttestation, entry.report.Error.Code)
}

// TestRun_PicksProvenanceLayerWhenSBOMComesFirst covers an image pushed with both
// --sbom and --provenance=mode=max: BuildKit orders the SPDX document ahead of
// the SLSA provenance in the attestation manifest, and reading the first layer
// yields a statement that parses but carries no Dockerfile.
func TestRun_PicksProvenanceLayerWhenSBOMComesFirst(t *testing.T) {
	const expected = "FROM python:3.11-slim\nRUN apt-get update\n"

	idx, _ := json.Marshal(map[string]any{
		"manifests": []map[string]any{{
			"digest": "sha256:attestation",
			"annotations": map[string]string{
				"vnd.docker.reference.type": "attestation-manifest",
			},
		}},
	})
	attestation, _ := json.Marshal(map[string]any{
		"layers": []map[string]any{
			{
				"digest":      "sha256:sbom",
				"annotations": map[string]string{"in-toto.io/predicate-type": "https://spdx.dev/Document"},
			},
			{
				"digest":      "sha256:provenance",
				"annotations": map[string]string{"in-toto.io/predicate-type": "https://slsa.dev/provenance/v1"},
			},
		},
	})
	statement, _ := json.Marshal(map[string]any{
		"predicate": map[string]any{
			"runDetails": map[string]any{
				"metadata": map[string]any{
					"buildkit_metadata": map[string]any{
						"source": map[string]any{
							"infos": []map[string]string{
								{"data": base64.StdEncoding.EncodeToString([]byte(expected))},
							},
						},
					},
				},
			},
		},
	})

	cli := newFakeRegistryClient()
	cli.manifests["sha256:artifact"] = idx
	cli.manifests["sha256:attestation"] = attestation
	cli.blobs["sha256:sbom"] = []byte(`{"predicateType":"https://spdx.dev/Document","predicate":{"name":"pyapi"}}`)
	cli.blobs["sha256:provenance"] = statement
	withFakeRegistryClient(t, cli)

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM python:3.11-alpine\n"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer llm.Close()

	s := NewServer(&Config{APIKey: "key", APIBaseURL: llm.URL, Model: "test-model", MaxConcurrency: 1})
	report := s.run(context.Background(), sampleOptimizeRequest())

	require.Equal(t, v1.ReportStatusSuccess, report.Status)
	require.False(t, report.Generated)
	require.Equal(t, expected, report.Dockerfile)
	require.Equal(t, "sha256:provenance", report.StatementDigest)
}

// TestRun_SBOMOnlyAttestationFallsBackToGeneration covers --sbom without
// --provenance: an attestation manifest exists but holds nothing extractable, so
// the pipeline should reconstruct from image history rather than report
// EXTRACTION_FAILED.
func TestRun_SBOMOnlyAttestationFallsBackToGeneration(t *testing.T) {
	idx, _ := json.Marshal(map[string]any{
		"manifests": []map[string]any{
			{"digest": "sha256:image"},
			{
				"digest": "sha256:attestation",
				"annotations": map[string]string{
					"vnd.docker.reference.type": "attestation-manifest",
				},
			},
		},
	})
	attestation, _ := json.Marshal(map[string]any{
		"layers": []map[string]any{{
			"digest":      "sha256:sbom",
			"annotations": map[string]string{"in-toto.io/predicate-type": "https://spdx.dev/Document"},
		}},
	})
	image := []byte(`{"config":{"digest":"sha256:cfg"},"layers":[{"digest":"sha256:layer1"}]}`)
	cfg := []byte(`{
		"config": {"Cmd": ["/bin/sh"]},
		"history": [
			{"created_by": "/bin/sh -c apk add --no-cache curl", "empty_layer": false}
		]
	}`)

	cli := newFakeRegistryClient()
	cli.manifests["sha256:artifact"] = idx
	cli.manifests["sha256:attestation"] = attestation
	cli.manifests["sha256:image"] = image
	cli.manifests["sha256:cfg"] = cfg
	cli.blobs["sha256:sbom"] = []byte(`{"predicateType":"https://spdx.dev/Document"}`)
	withFakeRegistryClient(t, cli)

	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM alpine:3.21\n"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer llm.Close()

	s := NewServer(&Config{APIKey: "key", APIBaseURL: llm.URL, Model: "test-model", MaxConcurrency: 1})
	report := s.run(context.Background(), sampleOptimizeRequest())

	require.Equal(t, v1.ReportStatusSuccess, report.Status)
	require.True(t, report.Generated)
	require.Contains(t, report.Dockerfile, "RUN apk add --no-cache curl")
}
