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

package eval

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/score"
	"github.com/czifrushanna/sleeko/pkg/verify"
)

func TestContractChanges(t *testing.T) {
	// The reason as CheckBehaviour writes it.
	o := score.Contract{Cmd: []string{"/bin/sh", "-c", "python  app.py"}, Workdir: "/app", Expose: []string{"8000"}}
	c := score.Contract{Cmd: []string{"python", "app.py"}, Expose: []string{"9000"}}
	g := score.CheckBehaviour(nil, o, c)
	got, ok := contractChanges(g.Reason)
	require.True(t, ok)
	require.Equal(t, []ContractChange{
		{Setting: "CMD", From: `["/bin/sh" "-c" "python  app.py"]`, To: `["python" "app.py"]`, Kind: kindShellExec},
		{Setting: "EXPOSE", From: "[8000/tcp]", To: "[9000/tcp]", Kind: kindChanged},
		{Setting: "WORKDIR", From: `"/app"`, To: `""`, Kind: kindChanged},
	}, got)

	got, _ = contractChanges(score.CheckBehaviour(nil, score.Contract{Entrypoint: []string{"node"}}, score.Contract{}).Reason)
	require.Equal(t, kindUnset, got[0].Kind)

	_, ok = contractChanges("smoke test: exited with code 1")
	require.False(t, ok)
}

func TestSameCommandForms(t *testing.T) {
	require.True(t, sameCommandForms([]string{"/bin/sh", "-c", "npm start"}, []string{"npm", "start"}))
	require.True(t, sameCommandForms([]string{"npm", "start"}, []string{"sh", "-c", "npm  start"}))
	require.False(t, sameCommandForms([]string{"/bin/sh", "-c", "npm start"}, []string{"node", "server.js"}))
	require.False(t, sameCommandForms([]string{"npm", "start"}, []string{"npm", "start"}), "identical is not a form change")
	require.False(t, sameCommandForms([]string{"/bin/sh", "-c", "a"}, []string{"/bin/sh", "-c", "b"}))
}

func TestSoftG6(t *testing.T) {
	r := Record{Original: &ImageInfo{Severities: map[string]int{"Critical": 1, "High": 2}}, Candidate: &ImageInfo{Severities: map[string]int{"High": 3}}}
	require.Equal(t, score.Pass, softG6(r), "3 ≤ 3, new IDs or not")
	r.Candidate.Severities["Critical"] = 1
	require.Equal(t, score.Fail, softG6(r))
	require.Equal(t, score.NotChecked, softG6(Record{}))
}

// analysisLinter flags DL3008 as an error in Dockerfiles containing "apt-get".
type analysisLinter struct{ calls int }

func (l *analysisLinter) Lint(_ context.Context, df string) ([]verify.LintFinding, error) {
	l.calls++
	if strings.Contains(df, "apt-get") {
		return []verify.LintFinding{{Code: "DL3008", Level: "error", Line: 2}}, nil
	}
	return nil, nil
}

// measured is a measured attempt with every gate passed and the given ΔV;
// fail names gates to fail instead.
func measured(item, pipelineName string, rep int, dv float64, fail ...score.GateID) Record {
	r := Record{Item: item, Pipeline: pipelineName, Repetition: rep, Status: StatusMeasured,
		OriginalDockerfile: "FROM a\n", CandidateDockerfile: "FROM b\n",
		Original:   &ImageInfo{Severities: map[string]int{"High": 1}},
		Candidate:  &ImageInfo{Severities: map[string]int{"High": 1}},
		Components: &Components{DeltaV: dv}}
	failed := map[score.GateID]bool{}
	for _, id := range fail {
		failed[id] = true
	}
	for _, id := range score.AllGates {
		g := score.Gate{ID: id, Status: score.Pass}
		if failed[id] {
			g.Status = score.Fail
			if id == score.G5Behaviour {
				g.Reason = `runtime contract changed: CMD ["/bin/sh" "-c" "npm start"] -> ["npm" "start"]`
			}
		}
		r.Gates = append(r.Gates, g)
	}
	r.Success = score.Success(r.Gates, dv, 0, score.DefaultSizeTolerance)
	return r
}

func TestAnalyze(t *testing.T) {
	recs := []Record{
		// x: A fails on G6 only, B succeeds.
		measured("x", "A", 1, 0.5, score.G6NoNewSevere), measured("x", "B", 1, 0.5),
		// y: A succeeds; B succeeds but adds a lint error.
		measured("y", "A", 1, 0.5), measured("y", "B", 1, 0.5),
		// z: B fails G5 on a shell/exec form change only; A fails G3.
		measured("z", "A", 1, 0, score.G3Versions), measured("z", "B", 1, 0.5, score.G5Behaviour),
		{Item: "w", Pipeline: "B", Repetition: 1, Status: StatusInfraError},
	}
	recs[3].CandidateDockerfile = "FROM b\nRUN apt-get install -y curl\n"

	lint := &analysisLinter{}
	a, err := Analyze(context.Background(), recs, []string{"A", "B"}, lint, score.DefaultSizeTolerance)
	require.NoError(t, err)
	require.Zero(t, a.Mismatches)

	require.Equal(t, RuleResult{Images: 3, Solved: 1, Attempts: 3, Successes: 1}, a.Rules["as measured (G1–G6)"]["A"])
	require.Equal(t, RuleResult{Images: 3, Solved: 2, Attempts: 3, Successes: 2}, a.Rules["as measured (G1–G6)"]["B"])
	require.Equal(t, 1, a.Rules["with G7 (lint)"]["B"].Solved, "y's lint error counts")
	require.Equal(t, 2, a.Rules["without G6"]["A"].Solved, "x succeeds without G6")
	require.Equal(t, 2, a.Rules["softer G6 (no more critical + high than O′)"]["A"].Solved, "x has no more high findings")
	require.Equal(t, 3, a.Rules["G5 accepting shell/exec forms of the same command"]["B"].Solved)
	require.Equal(t, []PairResult{{A: "A", B: "B", OnlyA: 0, OnlyB: 1, P: 1}}, a.Pairs["as measured (G1–G6)"])

	require.Equal(t, 1, a.LintFailed["B"])
	require.Equal(t, G3Result{Checked: 3, Failed: 1, ImagesFailing: 1}, a.G3["A"])
	require.Equal(t, 1, a.G3Pairs[0].OnlyA)
	require.Equal(t, 1, a.ContractFailures["B"])
	require.Equal(t, 1, a.Contract["B"]["CMD: "+kindShellExec])

	before := lint.calls
	_, err = Analyze(context.Background(), recs, []string{"A", "B"}, &CachedLinter{Linter: lint, Version: "v", Cache: &Cache{Dir: t.TempDir()}}, 0.1)
	require.NoError(t, err)
	require.Equal(t, before+3, lint.calls, "the cache lints each distinct Dockerfile once")

	var buf bytes.Buffer
	require.NoError(t, WriteAnalysis(&buf, a))
	md := buf.String()
	require.Contains(t, md, "0 mismatches")
	require.Contains(t, md, "| as measured (G1–G6) | 1/3 (33.3%) | 33.3% | 2/3 (66.7%) | 66.7% | 1 vs 0, p = 1 |")
	require.Contains(t, md, "| `B` | 1 | 0 |")
	require.Contains(t, md, "| CMD: same command, shell vs exec form | 0 | 1 |")
}

// TestPrefetchLint: after a prefetch, Analyze finds every Dockerfile in the
// cache.
func TestPrefetchLint(t *testing.T) {
	recs := []Record{measured("x", "A", 1, 0.5), measured("y", "A", 1, 0.5)}
	recs[1].CandidateDockerfile = "FROM c\nRUN apt-get install -y curl\n"
	lint := &lockedLinter{}
	cached := &CachedLinter{Linter: lint, Version: "v", Cache: &Cache{Dir: t.TempDir()}}
	require.NoError(t, PrefetchLint(context.Background(), recs, cached, 3))
	require.Equal(t, 3, lint.n(), "FROM a, FROM b and the apt-get candidate")
	_, err := Analyze(context.Background(), recs, []string{"A"}, cached, 0.1)
	require.NoError(t, err)
	require.Equal(t, 3, lint.n(), "all from the cache")
}

// lockedLinter is analysisLinter safe for concurrent use.
type lockedLinter struct {
	mu sync.Mutex
	l  analysisLinter
}

func (l *lockedLinter) Lint(ctx context.Context, df string) ([]verify.LintFinding, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.l.Lint(ctx, df)
}

func (l *lockedLinter) n() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.l.calls
}
