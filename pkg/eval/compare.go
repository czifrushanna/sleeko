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
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/czifrushanna/sleeko/pkg/pipeline"
	"github.com/czifrushanna/sleeko/pkg/score"
	"github.com/czifrushanna/sleeko/pkg/verify"
)

// itemOutcome is one pipeline's result on one image across its repetitions.
type itemOutcome struct {
	attempts, successes int
	deltaV, deltaS      []float64 // of the successful attempts
}

func (o itemOutcome) succeeded() bool { return 2*o.successes > o.attempts }

func mean(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// outcomes collects each image's outcome for a pipeline, from the last
// record of each attempt, leaving out attempts that could not be measured.
func outcomes(records []Record, pipelineName string) map[string]*itemOutcome {
	last := map[attemptKey]Record{}
	for _, r := range records {
		if r.Pipeline == pipelineName {
			last[r.key()] = r
		}
	}
	out := map[string]*itemOutcome{}
	for _, r := range last {
		if r.Status == StatusInfraError || r.Status == StatusItemInvalid {
			continue
		}
		o := out[r.Item]
		if o == nil {
			o = &itemOutcome{}
			out[r.Item] = o
		}
		o.attempts++
		if r.Success && r.Components != nil {
			o.successes++
			o.deltaV = append(o.deltaV, r.Components.DeltaV)
			o.deltaS = append(o.deltaS, r.Components.DeltaS)
		}
	}
	return out
}

// PairedComponent compares a component between two pipelines over the
// images where both succeeded at least once, each image's value being the
// mean over its successful attempts.
type PairedComponent struct {
	Pairs int `json:"pairs"`
	// MedianDiff is the median of B − A.
	MedianDiff float64              `json:"median_diff"`
	Test       score.WilcoxonResult `json:"wilcoxon"`
}

// Comparison is the schedule's paired comparison of pipeline B against A.
type Comparison struct {
	A, B string
	// Images is the number measured under both.
	Images int
	// An image succeeds under a pipeline when most of its repetitions do.
	BothSucceed, OnlyA, OnlyB, Neither int
	McNemarP                           float64
	DeltaV, DeltaS                     PairedComponent
}

// Compare compares pipeline b against pipeline a on the images both
// measured: McNemar on per-image success, Wilcoxon signed-rank on ΔV and ΔS.
// Repetitions of an image are not independent samples, so each image
// contributes one pair.
func Compare(records []Record, a, b string) Comparison {
	oa, ob := outcomes(records, a), outcomes(records, b)
	c := Comparison{A: a, B: b}
	var dv, ds []float64
	for item, x := range oa {
		y, ok := ob[item]
		if !ok {
			continue
		}
		c.Images++
		switch xs, ys := x.succeeded(), y.succeeded(); {
		case xs && ys:
			c.BothSucceed++
		case xs:
			c.OnlyA++
		case ys:
			c.OnlyB++
		default:
			c.Neither++
		}
		if len(x.deltaV) > 0 && len(y.deltaV) > 0 {
			dv = append(dv, mean(y.deltaV)-mean(x.deltaV))
			ds = append(ds, mean(y.deltaS)-mean(x.deltaS))
		}
	}
	c.McNemarP = score.McNemar(c.OnlyA, c.OnlyB)
	c.DeltaV = paired(dv)
	c.DeltaS = paired(ds)
	return c
}

func paired(diffs []float64) PairedComponent {
	return PairedComponent{Pairs: len(diffs), MedianDiff: score.Summarize(diffs).Median, Test: score.Wilcoxon(diffs)}
}

// WriteComparison writes the comparison as markdown.
func WriteComparison(w io.Writer, c Comparison) error {
	var b strings.Builder
	fmt.Fprintf(&b, "## `%s` vs `%s`\n\n", c.B, c.A)
	fmt.Fprintf(&b, "Images measured by both: %d. An image succeeds when most of its repetitions do.\n\n", c.Images)
	fmt.Fprintf(&b, "| | `%s` succeeds | `%s` fails |\n|---|---:|---:|\n", c.B, c.B)
	fmt.Fprintf(&b, "| **`%s` succeeds** | %d | %d |\n", c.A, c.BothSucceed, c.OnlyA)
	fmt.Fprintf(&b, "| **`%s` fails** | %d | %d |\n\n", c.A, c.OnlyB, c.Neither)
	fmt.Fprintf(&b, "McNemar (exact, two-sided): discordant images %d vs %d, p = %.4g\n\n", c.OnlyB, c.OnlyA, c.McNemarP)
	b.WriteString("| Component (B − A) | Paired images | Median difference | Wilcoxon W+ | p (exact) |\n|---|---:|---:|---:|---:|\n")
	for _, row := range []struct {
		name string
		p    PairedComponent
	}{{"ΔV", c.DeltaV}, {"ΔS", c.DeltaS}} {
		if row.p.Pairs == 0 {
			fmt.Fprintf(&b, "| %s | 0 | – | – | – |\n", row.name)
			continue
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %g | %.4g |\n", row.name, row.p.Pairs, percent(row.p.MedianDiff), row.p.Test.WPlus, row.p.Test.P)
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// VerifyStats are W1's own metrics: how often each check failed before and
// after repair, and how many repair rounds were used.
type VerifyStats struct {
	// Attempts is the number of attempts that went through verify.
	Attempts int `json:"attempts"`
	// NeededRepair counts attempts whose generated candidate failed a check;
	// Repaired those whose kept candidate passes them all.
	NeededRepair int `json:"needed_repair"`
	Repaired     int `json:"repaired"`
	// InitialFailures and FinalFailures count, per check, the attempts whose
	// generated and kept candidate failed it.
	InitialFailures map[string]int `json:"initial_failures"`
	FinalFailures   map[string]int `json:"final_failures"`
	// RepairRounds counts attempts by repair rounds used.
	RepairRounds map[int]int `json:"repair_rounds"`
}

// keptRound is the round whose candidate the verify stage kept: the fewest
// failing checks, the later round on a tie.
func keptRound(rounds []pipeline.VerifyRound) verify.Result {
	best := rounds[0].Result
	for _, r := range rounds[1:] {
		if r.Result.Badness() <= best.Badness() {
			best = r.Result
		}
	}
	return best
}

func verifyStats(recs []Record) *VerifyStats {
	vs := &VerifyStats{InitialFailures: map[string]int{}, FinalFailures: map[string]int{}, RepairRounds: map[int]int{}}
	for _, r := range recs {
		if len(r.Verification) == 0 {
			continue
		}
		vs.Attempts++
		initial, kept := r.Verification[0].Result, keptRound(r.Verification)
		for _, c := range initial.FailedChecks() {
			vs.InitialFailures[c]++
		}
		for _, c := range kept.FailedChecks() {
			vs.FinalFailures[c]++
		}
		if !initial.Passed() {
			vs.NeededRepair++
			if kept.Passed() {
				vs.Repaired++
			}
		}
		vs.RepairRounds[len(r.Verification)-1]++
	}
	if vs.Attempts == 0 {
		return nil
	}
	return vs
}

func writeVerifyStats(b *strings.Builder, vs *VerifyStats) {
	fmt.Fprintf(b, "\n### Verify and repair (W1)\n\n%d attempts verified; the generated candidate failed a check in %d, and repair fixed %d of those.\n\n",
		vs.Attempts, vs.NeededRepair, vs.Repaired)
	b.WriteString("| Check | Generated candidate fails | Kept candidate fails |\n|---|---:|---:|\n")
	for _, c := range verify.AllChecks {
		i, f := vs.InitialFailures[c], vs.FinalFailures[c]
		fmt.Fprintf(b, "| %s | %d (%s) | %d (%s) |\n", c, i, percent(float64(i)/float64(vs.Attempts)), f, percent(float64(f)/float64(vs.Attempts)))
	}
	rounds := make([]int, 0, len(vs.RepairRounds))
	for r := range vs.RepairRounds {
		rounds = append(rounds, r)
	}
	sort.Ints(rounds)
	var parts []string
	for _, r := range rounds {
		parts = append(parts, fmt.Sprintf("%d round(s): %d", r, vs.RepairRounds[r]))
	}
	fmt.Fprintf(b, "\nRepair rounds used: %s.\n", strings.Join(parts, ", "))
}
