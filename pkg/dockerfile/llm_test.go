// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dockerfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptimizeDockerfile_StreamingDelta(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM "}}]}`)
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"alpine:3.21"}}]}`)
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer ts.Close()

	result, err := OptimizeDockerfile(context.Background(), ts.URL, "test-key", "test-model", "test-prompt:\n", "FROM scratch\n", "test-report", Budget{})
	require.NoError(t, err)
	require.Equal(t, "FROM alpine:3.21", result)
}

func TestOptimizeDockerfile_NonStreamingMessage(t *testing.T) {
	// Some gateway implementations return message.content instead of delta.content.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `data: {"choices":[{"message":{"content":"FROM debian:12"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer ts.Close()

	result, err := OptimizeDockerfile(context.Background(), ts.URL, "key", "model", "test-prompt:\n", "FROM scratch\n", "test-report", Budget{})
	require.NoError(t, err)
	require.Equal(t, "FROM debian:12", result)
}

func TestOptimizeDockerfile_Non2xxStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error from gateway")
	}))
	defer ts.Close()

	_, err := OptimizeDockerfile(context.Background(), ts.URL, "key", "model", "test-prompt:\n", "FROM scratch\n", "test-report", Budget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
	require.Contains(t, err.Error(), "internal error from gateway")
}

func TestOptimizeDockerfile_EmptyContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":""}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer ts.Close()

	_, err := OptimizeDockerfile(context.Background(), ts.URL, "key", "model", "test-prompt:\n", "FROM scratch\n", "test-report", Budget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no optimized content")
}

func TestOptimizeDockerfile_ScanReportIncludedInPrompt(t *testing.T) {
	var receivedBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		receivedBody = string(body)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM scratch"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer ts.Close()

	_, err := OptimizeDockerfile(context.Background(), ts.URL, "key", "model", "test-prompt:\n", "FROM scratch\n", `{"vulnerabilities":["CVE-2024-1234"]}`, Budget{})
	require.NoError(t, err)
	require.Contains(t, receivedBody, `CVE-2024-1234`)
}

// captureRequest runs one OptimizeDockerfile call against a stub gateway and
// returns the decoded request body it received.
func captureRequest(t *testing.T, scanReport string, budget Budget, promptPrefix, dockerfileText string) chatRequest {
	t.Helper()

	var got chatRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM scratch"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer ts.Close()

	_, err := OptimizeDockerfile(context.Background(), ts.URL, "key", "model", promptPrefix, dockerfileText, scanReport, budget)
	require.NoError(t, err)

	return got
}

// scanReportJSON builds a report in Harbor's normalized shape, including the
// bulky fields that must not reach the gateway.
func scanReportJSON(t *testing.T, findings int) string {
	t.Helper()

	items := make([]map[string]any, 0, findings)
	for i := 0; i < findings; i++ {
		items = append(items, map[string]any{
			"id":          fmt.Sprintf("CVE-2026-%04d", i),
			"package":     fmt.Sprintf("pkg-%03d", i%40),
			"version":     "1.0",
			"fix_version": "1.1",
			"severity":    "High",
			"description": strings.Repeat("prose the model cannot act on. ", 20),
			"links":       []string{"https://nvd.nist.gov/vuln/detail/CVE-2026-0001"},
			"vendor_attributes": map[string]any{
				"CVSS": map[string]any{"nvd": map[string]any{"V3Score": 9.8}},
			},
		})
	}

	raw, err := json.Marshal(map[string]any{"vulnerabilities": items})
	require.NoError(t, err)

	return string(raw)
}

func TestOptimizeDockerfile_SendsMaxTokens(t *testing.T) {
	got := captureRequest(t, "test-report", Budget{MaxTokens: 4096, ContextWindow: 32768}, "test-prompt:\n", "FROM scratch\n")
	require.Equal(t, 4096, got.MaxTokens)

	// With no configured budget the field is omitted rather than sent as 0,
	// which a gateway would reject outright.
	body, err := json.Marshal(chatRequest{Model: "model", Stream: true})
	require.NoError(t, err)
	require.NotContains(t, string(body), "max_tokens")
}

func TestOptimizeDockerfile_ScanReportIsCondensed(t *testing.T) {
	raw := scanReportJSON(t, 200)

	got := captureRequest(t, raw, Budget{MaxTokens: 4096, ContextWindow: 32768}, "test-prompt:\n", "FROM alpine:3.20\n")
	prompt := got.Messages[0].Content

	require.Contains(t, prompt, "test-prompt:")
	require.Contains(t, prompt, "FROM alpine:3.20")
	require.Contains(t, prompt, "Vulnerability scan summary:")
	require.Contains(t, prompt, "200 findings in 40 packages")
	require.Contains(t, prompt, "pkg-000 1.0 -> 1.1 (5 High)")

	require.NotContains(t, prompt, "prose the model cannot act on")
	require.NotContains(t, prompt, "nvd.nist.gov")
	require.Less(t, len(prompt), len(raw)/10)
}

func TestOptimizeDockerfile_PromptStaysInsideTheContextWindow(t *testing.T) {
	// 4000 findings across 2000 packages: condensing alone is not enough here,
	// so the package list has to be cut as well.
	items := make([]map[string]any, 0, 4000)
	for i := 0; i < 4000; i++ {
		items = append(items, map[string]any{
			"id":          fmt.Sprintf("CVE-2026-%04d", i),
			"package":     fmt.Sprintf("package-with-a-realistic-name-%04d", i%2000),
			"version":     "1.0.0-1~deb12u1",
			"fix_version": "1.0.1-1~deb12u1",
			"severity":    []string{"Critical", "High", "Medium", "Low"}[i%4],
		})
	}
	raw, err := json.Marshal(map[string]any{"vulnerabilities": items})
	require.NoError(t, err)

	budget := Budget{MaxTokens: 4096, ContextWindow: 32768}
	got := captureRequest(t, string(raw), budget, "test-prompt:\n", "FROM alpine:3.20\n")
	prompt := got.Messages[0].Content

	limit := (budget.ContextWindow - budget.MaxTokens - promptOverheadTokens) * charsPerToken
	require.Less(t, len(prompt), limit, "prompt must fit the window with room for the completion")
	require.Contains(t, prompt, "4000 findings in 2000 packages")
	require.Contains(t, prompt, "with a fix omitted for length")
	// Truncation drops the least severe packages, so the Criticals survive
	// (each package here carries two findings of one severity).
	require.Contains(t, prompt, "(2 Critical)")
	require.NotContains(t, prompt, "(2 Low)")
}

func TestBudget_ScanReportBudget(t *testing.T) {
	t.Run("no context window means no trimming", func(t *testing.T) {
		require.Equal(t, 0, Budget{MaxTokens: 4096}.scanReportBudget("prefix", "FROM scratch"))
	})

	t.Run("room left over for the report", func(t *testing.T) {
		b := Budget{MaxTokens: 1000, ContextWindow: 10000}
		want := (10000-1000-promptOverheadTokens)*charsPerToken - len("prefix") - len("FROM scratch") - len(scanReportHeading)
		require.Equal(t, want, b.scanReportBudget("prefix", "FROM scratch"))
	})

	t.Run("oversized prefix squeezes the report to the floor", func(t *testing.T) {
		b := Budget{MaxTokens: 1000, ContextWindow: 2000}
		require.Equal(t, minScanReportChars, b.scanReportBudget(strings.Repeat("x", 100000), "FROM scratch"))
	})
}

func TestOptimizeDockerfile_MalformedSSELinesIgnored(t *testing.T) {
	// Lines that are not prefixed with "data:" or are blank should be skipped.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, ": keep-alive")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "event: message")
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM scratch"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	defer ts.Close()

	result, err := OptimizeDockerfile(context.Background(), ts.URL, "key", "model", "test-prompt:\n", "FROM scratch\n", "test-report", Budget{})
	require.NoError(t, err)
	require.Equal(t, "FROM scratch", result)
}

// A gateway may interleave chunks the client cannot parse — a truncated JSON
// payload, or a keep-alive chunk carrying no choices. Those must be skipped so
// the surrounding content still assembles into a Dockerfile.
func TestOptimizeDockerfile_UnparseableDataChunksSkipped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM "}}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":`)
		fmt.Fprintln(w, `data: {"choices":[]}`)
		fmt.Fprintln(w, `data: {"id":"chunk-without-choices"}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":""}}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"alpine:3.21"}}]}`)
		fmt.Fprintln(w, "data: [DONE]")
		// Anything after [DONE] is not read.
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"IGNORED"}}]}`)
	}))
	defer ts.Close()

	result, err := OptimizeDockerfile(context.Background(), ts.URL, "key", "model", "p:\n", "FROM scratch\n", "report", Budget{})
	require.NoError(t, err)
	require.Equal(t, "FROM alpine:3.21", result)
}

func TestOptimizeDockerfile_UnreachableGateway(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := ts.URL
	ts.Close() // nothing is listening any more

	_, err := OptimizeDockerfile(context.Background(), url, "key", "model", "p:\n", "FROM scratch\n", "report", Budget{})
	require.Error(t, err)
}

func TestOptimizeDockerfile_MalformedGatewayURL(t *testing.T) {
	// A control character makes http.NewRequestWithContext itself fail.
	_, err := OptimizeDockerfile(context.Background(), "http://llm.invalid\x7f", "key", "model", "p:\n", "FROM scratch\n", "report", Budget{})
	require.Error(t, err)
}

func TestOptimizeDockerfile_ContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM alpine"}}]}`)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := OptimizeDockerfile(ctx, ts.URL, "key", "model", "p:\n", "FROM scratch\n", "report", Budget{})
	require.ErrorIs(t, err, context.Canceled)
}

// A stream that dies mid-response must surface as an error rather than a
// silently truncated Dockerfile.
func TestOptimizeDockerfile_StreamAbortedMidResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"FROM alpine:3.21"}}]}`)
		w.(http.Flusher).Flush()
		// Break the connection without sending [DONE].
		if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
			_ = conn.Close()
		}
	}))
	defer ts.Close()

	_, err := OptimizeDockerfile(context.Background(), ts.URL, "key", "model", "p:\n", "FROM scratch\n", "report", Budget{})
	require.Error(t, err)
}

func TestUnreachable(t *testing.T) {
	// A listener closed before the call: the connection is refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	_, err = Chat(context.Background(), "http://"+addr, "k", "m", []Message{{Role: "user", Content: "x"}}, 0, Sampling{})
	require.Error(t, err)
	require.True(t, Unreachable(err), "%v", err)
	require.True(t, Unreachable(fmt.Errorf("stage generate: %w", err)), "wrapped")

	_, err = Chat(context.Background(), "http://no-such-host.invalid", "k", "m", nil, 0, Sampling{})
	require.True(t, Unreachable(err), "DNS failure: %v", err)

	// The gateway answered: not unreachable, whatever it said.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	_, err = Chat(context.Background(), ts.URL, "k", "m", nil, 0, Sampling{})
	require.Error(t, err)
	require.False(t, Unreachable(err))
	require.False(t, Unreachable(nil))
	require.False(t, Unreachable(errors.New("context deadline exceeded")))
}
