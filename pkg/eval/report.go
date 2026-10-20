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
	// Items is the number of images; the unit of the solved rate.
	Items    int `json:"items"`
	Attempts int `json:"attempts"`
	// Solved counts images with at least one successful repetition
	// (pass@k): the primary binary outcome per image, since repetitions are
	// not independent samples. SolvedRate is Solved/Items, with its Wilson
	// interval.
	Solved     int     `json:"solved"`
	SolvedRate float64 `json:"solved_rate"`
	WilsonLow  float64 `json:"wilson_low"`
	WilsonHigh float64 `json:"wilson_high"`
	// AttemptRate is successful attempts over attempts: what one call gives.
	AttemptRate float64 `json:"attempt_rate"`
	// MajorityRate is the share of images where most repetitions succeed, a
	// sensitivity analysis.
	MajorityRate float64 `json:"majority_rate"`
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
	// ByOriginalEOSL splits the attempts by whether O′'s distribution
	// release was past its end of life when scanned ("end of life",
	// "supported"; "unknown" for records without the flag). There the
	// scanner misses findings disclosed since, so a candidate that moves to
	// a supported release can show a higher V.
	ByOriginalEOSL map[string]Group `json:"by_original_eosl"`
	// EOSL counts moves off and onto end-of-life releases.
	EOSL EOSLStats `json:"eosl"`
	// GateFailures counts measured attempts failing each gate.
	GateFailures map[score.GateID]int `json:"gate_failures"`
	// OptimizerFailures counts attempts where the pipeline produced nothing.
	OptimizerFailures int `json:"optimizer_failures"`
	// Unmeasured counts attempts that could not be measured, through the
	// harness's fault or the item's; they are not part of any other number.
	Unmeasured int `json:"unmeasured"`
	// Verify holds W1's metrics, for pipelines with a verify stage.
	Verify *VerifyStats `json:"verify,omitempty"`
	// Bases holds W2's metrics, for pipelines that offered base image tables.
	Bases *BaseStats `json:"bases,omitempty"`
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
		pr := PipelineReport{Pipeline: name, ByDistro: map[string]Group{}, ByOriginalEOSL: map[string]Group{}, GateFailures: map[score.GateID]int{}}

		var counted []Record
		byDistro := map[string][]Record{}
		byEOSL := map[string][]Record{}
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
			byEOSL[originalEOSL(rec)] = append(byEOSL[originalEOSL(rec)], rec)
			pr.EOSL.add(rec)
		}

		pr.All = group(counted)
		pr.Verify = verifyStats(counted)
		pr.Bases = baseStats(counted)
		for distro, rs := range byDistro {
			pr.ByDistro[distro] = group(rs)
		}
		for k, rs := range byEOSL {
			pr.ByOriginalEOSL[k] = group(rs)
		}
		out = append(out, pr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pipeline < out[j].Pipeline })
	return out
}

const (
	eoslYes     = "end of life"
	eoslNo      = "supported"
	eoslUnknown = "unknown"
)

func originalEOSL(rec Record) string {
	switch {
	case rec.Original == nil || rec.Original.EOSL == nil:
		return eoslUnknown
	case *rec.Original.EOSL:
		return eoslYes
	}
	return eoslNo
}

// EOSLStats count, over attempts whose O′ and C were both scanned, how
// often C moved between end-of-life and supported distribution releases. A
// move off an end-of-life release is a security gain V can understate.
type EOSLStats struct {
	// Scanned counts attempts with both flags known; OriginalEOSL those
	// whose O′ is end of life.
	Scanned      int `json:"scanned"`
	OriginalEOSL int `json:"original_eosl"`
	// MovedOff: O′ end of life, C supported. MovedOnto: the reverse.
	MovedOff  int `json:"moved_off"`
	MovedOnto int `json:"moved_onto"`
}

func (s *EOSLStats) add(rec Record) {
	if rec.Original == nil || rec.Original.EOSL == nil || rec.Candidate == nil || rec.Candidate.EOSL == nil {
		return
	}
	o, c := *rec.Original.EOSL, *rec.Candidate.EOSL
	s.Scanned++
	if o {
		s.OriginalEOSL++
	}
	switch {
	case o && !c:
		s.MovedOff++
	case !o && c:
		s.MovedOnto++
	}
}

func writeEOSLStats(b *strings.Builder, s EOSLStats) {
	if s.Scanned == 0 {
		return
	}
	fmt.Fprintf(b, "\nEnd of life (as the scanner reports it; V misses findings disclosed since): "+
		"O′ is past its end of life in %d of %d attempts with C scanned; C moved off it in %d (%s of those), "+
		"and onto it from a supported O′ in %d.\n",
		s.OriginalEOSL, s.Scanned, s.MovedOff, percent(ratio(s.MovedOff, s.OriginalEOSL)), s.MovedOnto)
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

func group(recs []Record) Group {
	perItem := map[string]*itemOutcome{}
	var dV, dS []float64
	var fix []score.FixRate
	successes := 0
	for _, rec := range recs {
		o := perItem[rec.Item]
		if o == nil {
			o = &itemOutcome{}
			perItem[rec.Item] = o
		}
		o.attempts++
		if rec.Success {
			o.successes++
			successes++
		}
		if rec.Success && rec.Components != nil {
			dV = append(dV, rec.Components.DeltaV)
			dS = append(dS, rec.Components.DeltaS)
			fix = append(fix, rec.Components.FixRate)
		}
	}

	g := Group{
		Items:    len(perItem),
		Attempts: len(recs),
		DeltaV:   score.Summarize(dV),
		FixRate:  score.SummarizeFixRates(fix),
		DeltaS:   score.Summarize(dS),
	}
	majority := 0
	for _, o := range perItem {
		if o.solved() {
			g.Solved++
		}
		if o.majority() {
			majority++
		}
	}
	if g.Items > 0 {
		g.SolvedRate = float64(g.Solved) / float64(g.Items)
		g.MajorityRate = float64(majority) / float64(g.Items)
	}
	if g.Attempts > 0 {
		g.AttemptRate = float64(successes) / float64(g.Attempts)
	}
	g.WilsonLow, g.WilsonHigh = score.Wilson(float64(g.Solved), g.Items)
	return g
}

// WriteMarkdown writes the reports as markdown tables, for the paper.
func WriteMarkdown(w io.Writer, reports []PipelineReport) error {
	var b strings.Builder
	for _, pr := range reports {
		fmt.Fprintf(&b, "## Pipeline `%s`\n\n", pr.Pipeline)
		b.WriteString("Solved: images with at least one successful repetition (pass@k), with its 95% Wilson interval. " +
			"Per attempt: successful attempts over attempts, what one call gives. Majority: images where most repetitions succeed (sensitivity). " +
			"ΔV, fix rate and ΔS summarise the successful attempts.\n\n")
		b.WriteString("| Group | Images | Attempts | Solved | 95% CI (Wilson) | Per attempt | Majority | ΔV median (IQR) | Fix rate median (IQR) | Nothing to fix | ΔS median (IQR) |\n")
		b.WriteString("|---|---:|---:|---:|---|---:|---:|---|---|---:|---|\n")
		writeGroupRow(&b, "all", pr.All)
		distros := make([]string, 0, len(pr.ByDistro))
		for d := range pr.ByDistro {
			distros = append(distros, d)
		}
		sort.Strings(distros)
		for _, d := range distros {
			writeGroupRow(&b, d, pr.ByDistro[d])
		}
		for _, k := range []string{eoslYes, eoslNo, eoslUnknown} {
			if g, ok := pr.ByOriginalEOSL[k]; ok {
				writeGroupRow(&b, "O′ "+k, g)
			}
		}

		b.WriteString("\n| Failure | Attempts | Share of attempts |\n|---|---:|---:|\n")
		for _, id := range score.AllGates {
			writeFailureRow(&b, string(id), pr.GateFailures[id], pr.All.Attempts)
		}
		writeFailureRow(&b, "optimizer produced no candidate", pr.OptimizerFailures, pr.All.Attempts)
		fmt.Fprintf(&b, "\nNot measured (harness errors or invalid items, excluded above): %d attempts.\n", pr.Unmeasured)
		writeEOSLStats(&b, pr.EOSL)
		if pr.Verify != nil {
			writeVerifyStats(&b, pr.Verify)
		}
		if pr.Bases != nil {
			writeBaseStats(&b, pr.Bases)
		}
		b.WriteString("\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeGroupRow(b *strings.Builder, name string, g Group) {
	fmt.Fprintf(b, "| %s | %d | %d | %d (%s) | [%s, %s] | %s | %s | %s | %s | %d | %s |\n",
		name, g.Items, g.Attempts, g.Solved, percent(g.SolvedRate), percent(g.WilsonLow), percent(g.WilsonHigh),
		percent(g.AttemptRate), percent(g.MajorityRate),
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
