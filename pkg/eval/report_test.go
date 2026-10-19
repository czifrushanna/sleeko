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
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/score"
)

// testdata/results.jsonl, pipeline W0:
//
//	a (debian): success, success, G6 fails          -> 2/3
//	b (alpine): optimizer fails, G4 fails, success  -> 1/3
//	            (rep 2 was first an infra error, retried: the later record counts)
//	c (debian): O′ fails its smoke test three times -> unmeasured
//
// and one no-op attempt on a.
func loadFixture(t *testing.T) []PipelineReport {
	t.Helper()
	recs, err := ReadRecords(filepath.Join("testdata", "results.jsonl"))
	require.NoError(t, err)
	require.Len(t, recs, 11)
	return BuildReport(recs)
}

func TestBuildReport(t *testing.T) {
	reports := loadFixture(t)
	require.Len(t, reports, 2)
	require.Equal(t, "W0", reports[0].Pipeline)
	require.Equal(t, "noop", reports[1].Pipeline)

	w0 := reports[0]
	require.Equal(t, 2, w0.All.Items, "c is unmeasured")
	require.Equal(t, 6, w0.All.Attempts)
	require.InDelta(t, 0.5, w0.All.SuccessRate, 1e-12, "mean of per-image rates 2/3 and 1/3")
	lo, hi := score.Wilson(1, 2)
	require.Equal(t, lo, w0.All.WilsonLow)
	require.Equal(t, hi, w0.All.WilsonHigh)

	// Components of the three successes only: ΔV 0.5, 0.6, 0.2; ΔS 0.1, 0, −0.05.
	require.Equal(t, 3, w0.All.DeltaV.N)
	require.InDelta(t, 0.5, w0.All.DeltaV.Median, 1e-12)
	require.InDelta(t, 0.35, w0.All.DeltaV.Q1, 1e-12)
	require.InDelta(t, 0.55, w0.All.DeltaV.Q3, 1e-12)
	require.InDelta(t, 0, w0.All.DeltaS.Median, 1e-12)
	require.InDelta(t, -0.025, w0.All.DeltaS.Q1, 1e-12)
	// Fix rates 1/2 and 4/4; a#2 had nothing to fix.
	require.Equal(t, 2, w0.All.FixRate.N)
	require.InDelta(t, 0.75, w0.All.FixRate.Median, 1e-12)
	require.Equal(t, 1, w0.All.FixRate.NothingToFix)

	require.Equal(t, map[score.GateID]int{score.G6NoNewSevere: 1, score.G4Build: 1}, w0.GateFailures)
	require.Equal(t, 1, w0.OptimizerFailures)
	require.Equal(t, 3, w0.Unmeasured, "the retried infra error is superseded, c's three are not")

	require.Len(t, w0.ByDistro, 2)
	require.Equal(t, 1, w0.ByDistro["debian"].Items)
	require.InDelta(t, 2.0/3, w0.ByDistro["debian"].SuccessRate, 1e-12)
	require.InDelta(t, 1.0/3, w0.ByDistro["alpine"].SuccessRate, 1e-12)
	require.Equal(t, 1, w0.ByDistro["alpine"].DeltaV.N)

	noop := reports[1]
	require.Equal(t, 1, noop.All.Items)
	require.Zero(t, noop.All.SuccessRate)
	require.Zero(t, noop.All.DeltaV.N, "no successes, no component summary")
}

func TestBuildReport_Empty(t *testing.T) {
	require.Empty(t, BuildReport(nil))

	only := BuildReport([]Record{{Item: "x", Pipeline: "W0", Repetition: 1, Status: StatusInfraError}})
	require.Len(t, only, 1)
	require.Zero(t, only[0].All.Items)
	require.Equal(t, 1, only[0].Unmeasured)
	require.Equal(t, 0.0, only[0].All.WilsonLow)
	require.Equal(t, 1.0, only[0].All.WilsonHigh)
}

func TestWriteMarkdown(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteMarkdown(&buf, loadFixture(t)))
	md := buf.String()

	for _, want := range []string{
		"## Pipeline `W0`",
		"| Group | Images | Attempts | Success rate | 95% CI (Wilson) | ΔV median (IQR) | Fix rate median (IQR) | Nothing to fix | ΔS median (IQR) |",
		"| all | 2 | 6 | 50.0% | [9.5%, 90.5%] | 50.0% (35.0%–55.0%) | 75.0% (62.5%–87.5%) | 1 | 0.0% (-2.5%–5.0%) |",
		"| alpine | 1 | 3 | 33.3% |",
		"| debian | 1 | 3 | 66.7% |",
		"| G4_build | 1 | 16.7% |",
		"| G6_no_new_severe | 1 | 16.7% |",
		"| G1_parse | 0 | 0.0% |",
		"| optimizer produced no candidate | 1 | 16.7% |",
		"Not measured (harness errors or invalid items, excluded above): 3 attempts.",
		"## Pipeline `noop`",
		"| all | 1 | 1 | 0.0% | [0.0%, 79.3%] | – | – | 0 | – |",
	} {
		require.Contains(t, md, want)
	}
	require.Less(t, strings.Index(md, "`W0`"), strings.Index(md, "`noop`"))
}

func TestRecordWriterAndReader(t *testing.T) {
	var buf bytes.Buffer
	w := NewRecordWriter(&buf)
	require.NoError(t, w.Write(Record{Item: "a", Pipeline: "W0", Repetition: 1, Status: StatusMeasured,
		Components: &Components{FixRate: score.FixRate{}}}))
	require.NoError(t, w.Write(Record{Item: "a", Pipeline: "W0", Repetition: 2, Status: StatusInfraError}))
	require.Equal(t, 2, strings.Count(buf.String(), "\n"))
	require.Contains(t, buf.String(), `"nothing_to_fix":true`)

	recs, err := DecodeRecords(&buf)
	require.NoError(t, err)
	require.Len(t, recs, 2)
	require.True(t, recs[0].final())
	require.False(t, recs[1].final())

	_, err = DecodeRecords(strings.NewReader("{\"item\":\"a\"}\nnot json\n"))
	require.ErrorContains(t, err, "line 2")

	missing, err := ReadRecords(filepath.Join(t.TempDir(), "none.jsonl"))
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestReport_EndOfLife(t *testing.T) {
	yes, no := true, false
	rec := func(item string, rep int, o, c *bool, success bool) Record {
		r := Record{Item: item, Pipeline: "W2", Repetition: rep, Status: StatusMeasured, Success: success,
			Original: &ImageInfo{EOSL: o}}
		if c != nil {
			r.Candidate = &ImageInfo{EOSL: c}
		}
		return r
	}
	recs := []Record{
		rec("a", 1, &yes, &no, true),   // moved off
		rec("a", 2, &yes, &yes, false), // stayed
		rec("b", 1, &no, &yes, false),  // moved onto
		rec("c", 1, &no, nil, false),   // C not built: not counted in moves
		rec("d", 1, nil, &no, true),    // record without the flag
	}
	pr := BuildReport(recs)[0]
	require.Equal(t, EOSLStats{Scanned: 3, OriginalEOSL: 2, MovedOff: 1, MovedOnto: 1}, pr.EOSL)
	require.Equal(t, 2, pr.ByOriginalEOSL[eoslYes].Attempts)
	require.Equal(t, 0.5, pr.ByOriginalEOSL[eoslYes].SuccessRate)
	require.Equal(t, 2, pr.ByOriginalEOSL[eoslNo].Items)
	require.Equal(t, 1, pr.ByOriginalEOSL[eoslUnknown].Attempts)

	var buf bytes.Buffer
	require.NoError(t, WriteMarkdown(&buf, []PipelineReport{pr}))
	out := buf.String()
	require.Contains(t, out, "| O′ end of life | 1 | 2 | 50.0% |")
	require.Contains(t, out, "| O′ supported | 2 | 2 | 0.0% |")
	require.Contains(t, out, "| O′ unknown | 1 | 1 | 100.0% |")
	require.Contains(t, out, "O′ is past its end of life in 2 of 3 attempts with C scanned; C moved off it in 1 (50.0% of those), and onto it from a supported O′ in 1.")

	buf.Reset()
	require.NoError(t, WriteMarkdown(&buf, BuildReport([]Record{rec("x", 1, nil, nil, false)})))
	require.NotContains(t, buf.String(), "End of life", "nothing to say without flags")
}
