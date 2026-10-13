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

package pipeline

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/dockerfile"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/")

// sentRequest is the part of the chat-completions request the tests inspect.
type sentRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Stream    bool `json:"stream"`
	MaxTokens int  `json:"max_tokens"`
}

// fakeGateway serves one streamed completion with the given content and records
// the request it received.
func fakeGateway(t *testing.T, content string, sent *sentRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(sent))
		payload, err := json.Marshal(map[string]any{
			"choices": []map[string]any{{"delta": map[string]string{"content": content}}},
		})
		require.NoError(t, err)
		fmt.Fprintf(w, "data: %s\n", payload)
		fmt.Fprintln(w, "data: [DONE]")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(data)
}

// TestGenerate_PromptGolden pins the exact prompt the default pipeline sends:
// the built-in prompt, the Dockerfile and the condensed scan report, under the
// adapter's default budget. Later stages add their own prompt sections; this
// keeps them from silently changing the baseline's. Run with -update to accept
// a deliberate change.
func TestGenerate_PromptGolden(t *testing.T) {
	var sent sentRequest
	gw := fakeGateway(t, "FROM python:3.12-slim\n", &sent)

	g := NewGenerate(LLM{
		APIBaseURL: gw.URL,
		APIKey:     "key",
		Model:      "test-model",
		Prompt:     dockerfile.DefaultPrompt,
		Budget:     dockerfile.Budget{MaxTokens: 4096, ContextWindow: 32768},
	})
	st := &State{
		Original:   readTestdata(t, "generate_input.Dockerfile"),
		ScanReport: readTestdata(t, "generate_scanreport.json"),
	}
	require.NoError(t, g.Run(context.Background(), st))

	require.Equal(t, "test-model", sent.Model)
	require.True(t, sent.Stream)
	require.Equal(t, 4096, sent.MaxTokens)
	require.Len(t, sent.Messages, 1)
	require.Equal(t, "user", sent.Messages[0].Role)

	golden := filepath.Join("testdata", "generate_prompt.golden")
	if *update {
		require.NoError(t, os.WriteFile(golden, []byte(sent.Messages[0].Content), 0o644))
	}
	require.Equal(t, readTestdata(t, "generate_prompt.golden"), sent.Messages[0].Content)
}

func TestGenerate_SetsCandidateAndCountsTheRound(t *testing.T) {
	var sent sentRequest
	gw := fakeGateway(t, "```dockerfile\nFROM alpine:3.21\n```", &sent)

	g := NewGenerate(LLM{APIBaseURL: gw.URL, APIKey: "key", Model: "m", Prompt: "p:\n"})
	st := &State{Original: "FROM alpine:3.20\n", ScanReport: `{"vulnerabilities":[]}`}
	require.NoError(t, g.Run(context.Background(), st))

	require.Equal(t, "FROM alpine:3.21\n", st.Candidate)
	require.Equal(t, 1, st.LLMRounds)
	require.Equal(t, "FROM alpine:3.20\n", st.Original, "the original must not be touched")
}

func TestGenerate_GatewayErrorLeavesCandidateEmpty(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "upstream down")
	}))
	defer gw.Close()

	g := NewGenerate(LLM{APIBaseURL: gw.URL, APIKey: "key", Model: "m", Prompt: "p:\n"})
	st := &State{Original: "FROM alpine:3.20\n", ScanReport: `{"vulnerabilities":[]}`}
	err := g.Run(context.Background(), st)

	require.EqualError(t, err, "llm gateway returned 502 Bad Gateway: upstream down")
	require.Empty(t, st.Candidate)
	require.Equal(t, 1, st.LLMRounds, "a failed call is still a round")
}

func TestStripCodeFences(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "FROM alpine\nRUN true", "FROM alpine\nRUN true\n"},
		{"bare fences", "```\nFROM alpine\n```", "FROM alpine\n"},
		{"dockerfile fences", "```dockerfile\nFROM alpine\nRUN true\n```", "FROM alpine\nRUN true\n"},
		{"leading whitespace", "\n\n```dockerfile\nFROM alpine\n```\n\n", "FROM alpine\n"},
		{"only fence", "```", ""},
		{"no trailing fence", "```dockerfile\nFROM alpine", "FROM alpine\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, stripCodeFences(c.in))
		})
	}
}
