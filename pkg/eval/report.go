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

	"github.com/czifrushanna/sleeko/pkg/score"
)

// Group is the report for one set of attempts: a whole pipeline, or one
// distro family within it.
type Group struct {
	// Items is the number of images; the unit of the success rate.
	Items    int `json:"items"`
	Attempts int `json:"attempts"`
	// SuccessRate is the mean over images of each image's success rate
	// across its repetitions, which are not independent samples.
	SuccessRate float64 `json:"success_rate"`
	WilsonLow   float64 `json:"wilson_low"`
	WilsonHigh  float64 `json:"wilson_high"`
	// DeltaV, FixRate and DeltaS summarise the successful attempts only.
	DeltaV  score.Summary        `json:"delta_v"`
	FixRate score.FixRateSummary `json:"fix_rate"`
	DeltaS  score.Summary        `json:"delta_s"`
}

// PipelineReport is the report for one pipeline configuration.
type PipelineReport struct {
	Pipeline string           `json:"pipeline"`
	All      Group            `json:"all"`
	ByDistro map[string]Group `json:"by_distro"`
	// GateFailures counts measured attempts failing each gate.
	GateFailures map[score.GateID]int `json:"gate_failures"`
	// OptimizerFailures counts attempts where the pipeline produced nothing.
	OptimizerFailures int `json:"optimizer_failures"`
	// Unmeasured counts attempts that could not be measured, through the
	// harness's fault or the item's; they are not part of any other number.
	Unmeasured int `json:"unmeasured"`
	// Verify holds W1's metrics, for pipelines with a verify stage.
	Verify *VerifyStats `json:"verify,omitempty"`
}

// BuildReport aggregates records into per-pipeline reports, sorted by
// pipeline name. When an attempt was recorded more than once (a resumed run
// retried it), the last record counts.
func BuildReport(records []Record) []PipelineReport {
	last := map[attemptKey]Record{}
	var order []attemptKey
	for _, rec := range records {
		k := rec.key()
		if _, seen := last[k]; !seen {
			order = append(order, k)
		}
		last[k] = rec
	}

	byPipeline := map[string][]Record{}
	for _, k := range order {
		rec := last[k]
		byPipeline[rec.Pipeline] = append(byPipeline[rec.Pipeline], rec)
	}

	var out []PipelineReport
	for name, recs := range byPipeline {
		pr := PipelineReport{Pipeline: name, ByDistro: map[string]Group{}, GateFailures: map[score.GateID]int{}}

		var counted []Record
		byDistro := map[string][]Record{}
		for _, rec := range recs {
			switch rec.Status {
			case StatusInfraError, StatusItemInvalid:
				pr.Unmeasured++
				continue
			case StatusOptimizerFailed:
				pr.OptimizerFailures++
			}
			for _, g := range rec.Gates {
				if g.Status == score.Fail {
					pr.GateFailures[g.ID]++
				}
			}
			counted = append(counted, rec)
			byDistro[rec.DistroFamily] = append(byDistro[rec.DistroFamily], rec)
		}

		pr.All = group(counted)
		pr.Verify = verifyStats(counted)
		for distro, rs := range byDistro {
			pr.ByDistro[distro] = group(rs)
		}
		out = append(out, pr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pipeline < out[j].Pipeline })
	return out
}

func group(recs []Record) Group {
	perItem := map[string][]bool{}
	var dV, dS []float64
	var fix []score.FixRate
	for _, rec := range recs {
		perItem[rec.Item] = append(perItem[rec.Item], rec.Success)
		if rec.Success && rec.Components != nil {
			dV = append(dV, rec.Components.DeltaV)
			dS = append(dS, rec.Components.DeltaS)
			fix = append(fix, rec.Components.FixRate)
		}
	}

	var successes float64
	for _, outcomes := range perItem {
		n := 0
		for _, ok := range outcomes {
			if ok {
				n++
			}
		}
		successes += float64(n) / float64(len(outcomes))
	}

	g := Group{
		Items:    len(perItem),
		Attempts: len(recs),
		DeltaV:   score.Summarize(dV),
		FixRate:  score.SummarizeFixRates(fix),
		DeltaS:   score.Summarize(dS),
	}
	if g.Items > 0 {
		g.SuccessRate = successes / float64(g.Items)
	}
	g.WilsonLow, g.WilsonHigh = score.Wilson(successes, g.Items)
	return g
}

// WriteMarkdown writes the reports as markdown tables, for the paper.
func WriteMarkdown(w io.Writer, reports []PipelineReport) error {
	var b strings.Builder
	for _, pr := range reports {
		fmt.Fprintf(&b, "## Pipeline `%s`\n\n", pr.Pipeline)
		b.WriteString("| Group | Images | Attempts | Success rate | 95% CI (Wilson) | ΔV median (IQR) | Fix rate median (IQR) | Nothing to fix | ΔS median (IQR) |\n")
		b.WriteString("|---|---:|---:|---:|---|---|---|---:|---|\n")
		writeGroupRow(&b, "all", pr.All)
		distros := make([]string, 0, len(pr.ByDistro))
		for d := range pr.ByDistro {
			distros = append(distros, d)
		}
		sort.Strings(distros)
		for _, d := range distros {
			writeGroupRow(&b, d, pr.ByDistro[d])
		}

		b.WriteString("\n| Failure | Attempts | Share of attempts |\n|---|---:|---:|\n")
		for _, id := range score.AllGates {
			writeFailureRow(&b, string(id), pr.GateFailures[id], pr.All.Attempts)
		}
		writeFailureRow(&b, "optimizer produced no candidate", pr.OptimizerFailures, pr.All.Attempts)
		fmt.Fprintf(&b, "\nNot measured (harness errors or invalid items, excluded above): %d attempts.\n", pr.Unmeasured)
		if pr.Verify != nil {
			writeVerifyStats(&b, pr.Verify)
		}
		b.WriteString("\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeGroupRow(b *strings.Builder, name string, g Group) {
	fmt.Fprintf(b, "| %s | %d | %d | %s | [%s, %s] | %s | %s | %d | %s |\n",
		name, g.Items, g.Attempts, percent(g.SuccessRate), percent(g.WilsonLow), percent(g.WilsonHigh),
		summary(g.DeltaV), summary(g.FixRate.Summary), g.FixRate.NothingToFix, summary(g.DeltaS))
}

func writeFailureRow(b *strings.Builder, name string, n, attempts int) {
	share := 0.0
	if attempts > 0 {
		share = float64(n) / float64(attempts)
	}
	fmt.Fprintf(b, "| %s | %d | %s |\n", name, n, percent(share))
}

func percent(x float64) string {
	return fmt.Sprintf("%.1f%%", 100*x)
}

func summary(s score.Summary) string {
	if s.N == 0 {
		return "–"
	}
	return fmt.Sprintf("%s (%s–%s)", percent(s.Median), percent(s.Q1), percent(s.Q3))
}
