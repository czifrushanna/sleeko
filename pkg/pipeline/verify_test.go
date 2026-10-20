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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/verify"
)

const verifyOriginal = "FROM alpine:3.20\nCOPY app /app\nCMD [\"/app\"]\n"

// scriptedGateway answers each request with the next reply, recording the
// conversations it was sent; a reply of "ERROR" answers 500.
type scriptedGateway struct {
	mu      sync.Mutex
	replies []string
	sent    [][]dockerfile.Message
}

func (g *scriptedGateway) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []dockerfile.Message `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		g.mu.Lock()
		g.sent = append(g.sent, req.Messages)
		reply := g.replies[0]
		g.replies = g.replies[1:]
		g.mu.Unlock()
		if reply == "ERROR" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		payload, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"delta": map[string]string{"content": reply}}}})
		fmt.Fprintf(w, "data: %s\ndata: [DONE]\n", payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// run runs generate then verify with the given replies.
func runVerify(t *testing.T, maxRounds int, replies ...string) (*State, *scriptedGateway) {
	t.Helper()
	gw := &scriptedGateway{replies: replies}
	srv := gw.serve(t)
	llm := LLM{APIBaseURL: srv.URL, APIKey: "k", Model: "m", Prompt: "optimize:\n"}
	p, err := New([]string{GenerateStageName, VerifyStageName}, Deps{
		LLM:             llm,
		Verifier:        &verify.Verifier{}, // no lookups: the structural checks only
		MaxRepairRounds: maxRounds,
	})
	require.NoError(t, err)
	st := &State{Original: verifyOriginal, ScanReport: `{"vulnerabilities": []}`}
	require.NoError(t, p.Run(context.Background(), st))
	return st, gw
}

const (
	good     = "FROM alpine:3.21\nCOPY app /app\nCMD [\"/app\"]\n"
	invented = "FROM alpine:3.21\nCOPY app /app\nCOPY config.yaml /etc/\nCMD [\"/app\"]\n"
	unparsed = "FROM alpine:3.21\nCOPYY app /app\n"
	twoBad   = "FROM alpine:3.21\nCOPY app /app\nCOPY x /x\nEXPOSE 9\nCMD [\"/app\"]\n"
)

func failed(st *State) [][]string {
	var out [][]string
	for _, r := range st.Verification {
		out = append(out, r.Result.FailedChecks())
	}
	return out
}

func TestVerify_PassesFirstTime(t *testing.T) {
	st, gw := runVerify(t, 2, good)
	require.Equal(t, good, st.Candidate)
	require.Equal(t, 1, st.LLMRounds, "no repair needed")
	require.Len(t, gw.sent, 1)
	require.Equal(t, [][]string{nil}, failed(st))
}

func TestVerify_RepairedAndConverges(t *testing.T) {
	st, gw := runVerify(t, 2, "```dockerfile\n"+invented+"```", good)
	require.Equal(t, good, st.Candidate)
	require.Equal(t, 2, st.LLMRounds)
	require.Equal(t, [][]string{{verify.CheckContext}, nil}, failed(st))

	// The repair request carries the whole conversation and the finding.
	repair := gw.sent[1]
	require.Len(t, repair, 3)
	require.Equal(t, "user", repair[0].Role)
	require.Equal(t, "assistant", repair[1].Role)
	require.Contains(t, repair[1].Content, "COPY config.yaml")
	require.Equal(t, "user", repair[2].Role)
	require.Contains(t, repair[2].Content, "config.yaml (line 3)")
	require.Contains(t, repair[2].Content, "Return the complete corrected Dockerfile")
	require.Len(t, st.Messages, 4)
}

func TestVerify_NeverConvergesKeepsTheBest(t *testing.T) {
	// Round 0 fails two checks, round 1 one, round 2 does not parse: the
	// round-1 candidate, with the fewest failures, is returned.
	st, _ := runVerify(t, 2, twoBad, invented, unparsed)
	require.Equal(t, invented, st.Candidate)
	require.Equal(t, 3, st.LLMRounds)
	require.Equal(t, [][]string{
		{verify.CheckContext, verify.CheckContract},
		{verify.CheckContext},
		{verify.CheckParse},
	}, failed(st))
	require.NotEmpty(t, st.Diagnostics, "remaining failures are reported")
	require.Contains(t, st.Diagnostics[len(st.Diagnostics)-1].Message, "fail context_copies")
}

func TestVerify_TieTakesTheLaterCandidate(t *testing.T) {
	other := strings.Replace(invented, "config.yaml", "other.yaml", 1)
	st, _ := runVerify(t, 1, invented, other)
	require.Equal(t, other, st.Candidate)
}

func TestVerify_ZeroRoundsOnlyChecks(t *testing.T) {
	st, gw := runVerify(t, 0, invented)
	require.Equal(t, invented, st.Candidate)
	require.Len(t, gw.sent, 1)
	require.Equal(t, [][]string{{verify.CheckContext}}, failed(st))
}

func TestVerify_FailedRepairCallKeepsTheCandidate(t *testing.T) {
	st, _ := runVerify(t, 2, invented, "ERROR")
	require.Equal(t, invented, st.Candidate)
	require.Equal(t, 2, st.LLMRounds, "the failed call still counts")
	require.Contains(t, st.Diagnostics[0].Message, "repair round 1 failed")
}

func TestVerify_NeedsACandidateAndAVerifier(t *testing.T) {
	v := &Verify{verifier: &verify.Verifier{}}
	require.ErrorContains(t, v.Run(context.Background(), &State{}), "run generate first")
	v = &Verify{}
	require.ErrorContains(t, v.Run(context.Background(), &State{Candidate: "FROM x\n"}), "no verifier")
}

func TestRepairMessage(t *testing.T) {
	msg := RepairMessage(verify.Result{Findings: []verify.Finding{
		{Check: verify.CheckBaseTag, Severity: verify.Fail, Line: 1, Message: "the base image python:3.99 does not exist"},
		{Check: verify.CheckDroppedRun, Severity: verify.Warn, Message: "the original's RUN at line 5 is gone: make"},
	}})
	require.Equal(t, "The Dockerfile you returned has problems that must be fixed before it can be used:\n"+
		"- line 1: the base image python:3.99 does not exist\n"+
		"\nAlso check these; keep them only if they were intended:\n"+
		"- the original's RUN at line 5 is gone: make\n"+
		"\nReturn the complete corrected Dockerfile and nothing else. Keep the improvements that were not part of these problems.\n", msg)
}

// TestSampling: every LLM call of an optimization, repairs included, sends
// the temperature and the base seed plus the state's offset; unset sampling
// sends neither field, leaving the gateway's defaults.
func TestSampling(t *testing.T) {
	run := func(sampling dockerfile.Sampling, offset int64) []map[string]any {
		var mu sync.Mutex
		var bodies []map[string]any
		replies := []string{invented, good}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			mu.Lock()
			bodies = append(bodies, body)
			reply := replies[0]
			replies = replies[1:]
			mu.Unlock()
			payload, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"delta": map[string]string{"content": reply}}}})
			fmt.Fprintf(w, "data: %s\ndata: [DONE]\n", payload)
		}))
		defer srv.Close()
		p, err := New([]string{GenerateStageName, VerifyStageName}, Deps{
			LLM:             LLM{APIBaseURL: srv.URL, APIKey: "k", Model: "m", Prompt: "optimize:\n", Sampling: sampling},
			Verifier:        &verify.Verifier{},
			MaxRepairRounds: 2,
		})
		require.NoError(t, err)
		st := &State{Original: verifyOriginal, ScanReport: `{"vulnerabilities": []}`, SeedOffset: offset}
		require.NoError(t, p.Run(context.Background(), st))
		return bodies
	}

	temp, seed := 0.7, int64(100)
	bodies := run(dockerfile.Sampling{Temperature: &temp, Seed: &seed}, 2)
	require.Len(t, bodies, 2, "generate and one repair")
	for _, b := range bodies {
		require.Equal(t, 0.7, b["temperature"])
		require.Equal(t, 102.0, b["seed"], "base seed + offset, the same for the repair")
	}
	require.Equal(t, int64(100), seed, "the configured base seed is not changed")

	for _, b := range run(dockerfile.Sampling{}, 2) {
		require.NotContains(t, b, "temperature")
		require.NotContains(t, b, "seed")
	}

	zero := 0.0
	bodies = run(dockerfile.Sampling{Temperature: &zero}, 0)
	require.Equal(t, 0.0, bodies[0]["temperature"], "a temperature of 0 is sent, not omitted")
	require.NotContains(t, bodies[0], "seed")
}
