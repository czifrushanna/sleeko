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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/pipeline"
	"github.com/czifrushanna/sleeko/pkg/verify"
)

// attempts makes one record per outcome ("s" success, "f" failure), with ΔV
// and ΔS on the successes.
func attempts(item, pipelineName string, outcomes string, dv, ds float64) []Record {
	var out []Record
	for i, o := range outcomes {
		r := Record{Item: item, Pipeline: pipelineName, Repetition: i + 1, Status: StatusMeasured, Success: o == 's'}
		if o == 's' {
			r.Components = &Components{DeltaV: dv, DeltaS: ds}
		}
		out = append(out, r)
	}
	return out
}

func TestCompare(t *testing.T) {
	var recs []Record
	add := func(rs ...[]Record) {
		for _, r := range rs {
			recs = append(recs, r...)
		}
	}
	// Five images only W0+W1 solves.
	for _, item := range []string{"b1", "b2", "b3", "b4", "b5"} {
		add(attempts(item, "W0", "fff", 0, 0), attempts(item, "W0+W1", "ssf", 0.4, 0.1))
	}
	// Both solve a1, but only W0 succeeds in most repetitions.
	add(attempts("a1", "W0", "ssf", 0.2, 0), attempts("a1", "W0+W1", "sff", 0.3, 0))
	// Both succeed on three images; W0+W1 reduces vulnerabilities more.
	add(attempts("both1", "W0", "sss", 0.10, 0.05), attempts("both1", "W0+W1", "sss", 0.30, 0.05))
	add(attempts("both2", "W0", "ssf", 0.20, 0.00), attempts("both2", "W0+W1", "sss", 0.25, 0.10))
	add(attempts("both3", "W0", "sss", 0.50, 0.00), attempts("both3", "W0+W1", "sss", 0.45, 0.00))
	// Neither, and an image only one pipeline measured.
	add(attempts("none", "W0", "fff", 0, 0), attempts("none", "W0+W1", "fff", 0, 0))
	add(attempts("only-w0", "W0", "sss", 0.9, 0))
	// Unmeasured attempts do not count.
	recs = append(recs, Record{Item: "none", Pipeline: "W0+W1", Repetition: 4, Status: StatusInfraError})

	c := Compare(recs, "W0", "W0+W1")
	require.Equal(t, 10, c.Images)
	// Solved (≥ 1 success): a1 and both1–3 by both, b1–b5 only by W0+W1.
	require.Equal(t, PairedOutcomes{BothSucceed: 4, OnlyA: 0, OnlyB: 5, Neither: 1, McNemarP: 0.0625}, c.Solved, "exact: 2 × (1/2)^5")
	// Majority: a1 becomes W0-only.
	require.Equal(t, 3, c.Majority.BothSucceed)
	require.Equal(t, 1, c.Majority.OnlyA)
	require.Equal(t, 5, c.Majority.OnlyB)
	require.InDelta(t, 0.21875, c.Majority.McNemarP, 1e-12, "the SciPy reference for 1 vs 5")

	// Pairs: a1 (B 0.3 − A 0.2), both1 (+0.2), both2 (+0.05), both3 (−0.05):
	// |0.05| twice ties at rank 1.5, so W+ = 3 + 4 + 1.5.
	require.Equal(t, 4, c.DeltaV.Pairs)
	require.InDelta(t, 0.075, c.DeltaV.MedianDiff, 1e-12)
	require.Equal(t, 4, c.DeltaV.Test.N)
	require.InDelta(t, 8.5, c.DeltaV.Test.WPlus, 1e-9)
	require.Equal(t, 1, c.DeltaS.Test.N, "zero ΔS differences are dropped: only both2 differs")

	var buf bytes.Buffer
	require.NoError(t, WriteComparison(&buf, c))
	md := buf.String()
	require.Contains(t, md, "## `W0+W1` vs `W0`")
	require.Contains(t, md, "An image is solved when at least one of its repetitions succeeds (pass@k).")
	require.Contains(t, md, "| **`W0` solves** | 4 | 0 |")
	require.Contains(t, md, "| **`W0` does not** | 5 | 1 |")
	require.Contains(t, md, "McNemar (exact, two-sided): discordant images 5 vs 0, p = 0.0625")
	require.Contains(t, md, "Sensitivity, an image succeeding when most repetitions do: discordant images 5 vs 1, p = 0.2188")
	require.Contains(t, md, "| ΔV | 4 | 7.5% | 8.5 |")
}

func round(n int, failed ...string) pipeline.VerifyRound {
	var r verify.Result
	for _, c := range failed {
		r.Findings = append(r.Findings, verify.Finding{Check: c, Severity: verify.Fail})
	}
	return pipeline.VerifyRound{Round: n, Result: r}
}

func TestVerifyStats(t *testing.T) {
	recs := []Record{
		{Verification: []pipeline.VerifyRound{round(0)}},
		{Verification: []pipeline.VerifyRound{round(0, verify.CheckContext), round(1)}},
		{Verification: []pipeline.VerifyRound{round(0, verify.CheckBaseTag, verify.CheckFixVersion), round(1, verify.CheckFixVersion), round(2, verify.CheckParse)}},
		{}, // no verify stage
	}
	vs := verifyStats(recs)
	require.Equal(t, 3, vs.Attempts)
	require.Equal(t, 2, vs.NeededRepair)
	require.Equal(t, 1, vs.Repaired)
	require.Equal(t, map[string]int{verify.CheckContext: 1, verify.CheckBaseTag: 1, verify.CheckFixVersion: 1}, vs.InitialFailures)
	require.Equal(t, map[string]int{verify.CheckFixVersion: 1}, vs.FinalFailures, "the kept candidate is round 1, not the unparseable round 2")
	require.Equal(t, map[int]int{0: 1, 1: 1, 2: 1}, vs.RepairRounds)

	require.Nil(t, verifyStats([]Record{{}}))

	var b bytes.Buffer
	reps := BuildReport([]Record{{Item: "x", Pipeline: "W0+W1", Repetition: 1, Status: StatusMeasured, Verification: recs[2].Verification}})
	require.NoError(t, WriteMarkdown(&b, reps))
	require.Contains(t, b.String(), "### Verify and repair (W1)")
	require.Contains(t, b.String(), "| fix_version | 1 (100.0%) | 1 (100.0%) |")
	require.Contains(t, b.String(), "Repair rounds used: 2 round(s): 1.")
}
