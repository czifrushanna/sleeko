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
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/czifrushanna/sleeko/pkg/score"
	"github.com/czifrushanna/sleeko/pkg/verify"
)

// The S9 analyses: how much the conclusions depend on how success is
// defined, and what the gates themselves show. Everything is computed from
// the records; nothing is re-run except Hadolint, for the lint gate (G7)
// that was added after the measurement.

// G7Lint is the lint gate: C adds no error-level Hadolint finding whose rule
// O′ does not have, the same rule as W1's lint check.
const G7Lint score.GateID = "G7_no_new_lint_errors"

// lintGate computes G7 for an attempt. It is not checked when C does not
// parse or Hadolint cannot run.
func lintGate(ctx context.Context, lint verify.Linter, r Record) (score.Gate, error) {
	if r.CandidateDockerfile == "" || gateStatus(r, score.G1Parse) != score.Pass {
		return score.Gate{ID: G7Lint, Status: score.NotChecked, Reason: "the candidate does not parse"}, nil
	}
	before, err := lint.Lint(ctx, r.OriginalDockerfile)
	if err != nil {
		return score.Gate{}, err
	}
	after, err := lint.Lint(ctx, r.CandidateDockerfile)
	if err != nil {
		return score.Gate{}, err
	}
	var errs []string
	for _, f := range verify.NewLintFindings(before, after) {
		if f.Level == "error" {
			errs = append(errs, fmt.Sprintf("%s at line %d", f.Code, f.Line))
		}
	}
	if len(errs) > 0 {
		return score.Gate{ID: G7Lint, Status: score.Fail, Reason: "new lint errors: " + strings.Join(errs, ", ")}, nil
	}
	return score.Gate{ID: G7Lint, Status: score.Pass}, nil
}

func gateStatus(r Record, id score.GateID) score.Status {
	for _, g := range r.Gates {
		if g.ID == id {
			return g.Status
		}
	}
	return ""
}

// CachedLinter remembers Hadolint's findings per Dockerfile: the originals
// recur across attempts and pipelines.
type CachedLinter struct {
	Linter verify.Linter
	// Version identifies the linter in the cache key.
	Version string
	Cache   *Cache
}

// Lint returns the cached findings, running the linter on a miss.
func (c *CachedLinter) Lint(ctx context.Context, dockerfile string) ([]verify.LintFinding, error) {
	key := hashKey("lint", c.Version, dockerfile)
	var out []verify.LintFinding
	if ok, err := c.Cache.get("lint", key, &out); err != nil {
		return nil, err
	} else if ok {
		return out, nil
	}
	out, err := c.Linter.Lint(ctx, dockerfile)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []verify.LintFinding{}
	}
	return out, c.Cache.put("lint", key, out)
}

// PrefetchLint lints every Dockerfile G7 needs, workers at a time, so a
// cached linter answers Analyze without waiting: Hadolint from its container
// image takes seconds per Dockerfile.
func PrefetchLint(ctx context.Context, records []Record, lint verify.Linter, workers int) error {
	seen := map[string]bool{}
	var todo []string
	for _, r := range records {
		if r.Status != StatusMeasured || gateStatus(r, score.G1Parse) != score.Pass {
			continue
		}
		for _, df := range []string{r.OriginalDockerfile, r.CandidateDockerfile} {
			if !seen[df] {
				seen[df] = true
				todo = append(todo, df)
			}
		}
	}
	jobs := make(chan string)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			var first error
			for df := range jobs {
				if _, err := lint.Lint(ctx, df); err != nil && first == nil {
					first = err
				}
			}
			errs <- first
		}()
	}
	for _, df := range todo {
		jobs <- df
	}
	close(jobs)
	var first error
	for i := 0; i < workers; i++ {
		if err := <-errs; err != nil && first == nil {
			first = err
		}
	}
	return first
}

// view is one counted attempt, with G7.
type view struct {
	Record
	g7 score.Gate
}

// SuccessRule is one definition of success, applied to a counted attempt.
type SuccessRule struct {
	Name string
	ok   func(v view, tol float64) bool
}

// successWith recomputes the success rule with the checked gates in override
// re-decided, and with G7 required when lint is set.
func successWith(v view, tol float64, override map[score.GateID]func(Record) score.Status, lint bool) bool {
	if v.Status != StatusMeasured || v.Components == nil {
		return false
	}
	gates := make([]score.Gate, 0, len(v.Gates))
	for _, g := range v.Gates {
		if f, ok := override[g.ID]; ok && g.Status != score.NotChecked {
			g.Status = f(v.Record)
		}
		gates = append(gates, g)
	}
	if lint && v.g7.Status == score.Fail {
		return false
	}
	return score.Success(gates, v.Components.DeltaV, v.Components.DeltaS, tol)
}

// SuccessRules are the definitions compared: as measured first.
var SuccessRules = []SuccessRule{
	{"as measured (G1–G6)", func(v view, tol float64) bool { return successWith(v, tol, nil, false) }},
	{"with G7 (lint)", func(v view, tol float64) bool { return successWith(v, tol, nil, true) }},
	{"without G6", func(v view, tol float64) bool {
		return successWith(v, tol, map[score.GateID]func(Record) score.Status{score.G6NoNewSevere: func(Record) score.Status { return score.Pass }}, false)
	}},
	{"softer G6 (no more critical + high than O′)", func(v view, tol float64) bool {
		return successWith(v, tol, map[score.GateID]func(Record) score.Status{score.G6NoNewSevere: softG6}, false)
	}},
	{"G5 accepting shell/exec forms of the same command", func(v view, tol float64) bool {
		return successWith(v, tol, map[score.GateID]func(Record) score.Status{score.G5Behaviour: lenientG5}, false)
	}},
}

// softG6 passes when C has no more critical + high findings than O′, new
// IDs or not.
func softG6(r Record) score.Status {
	if r.Original == nil || r.Candidate == nil {
		return score.NotChecked
	}
	ch := func(s map[string]int) int { return s["Critical"] + s["High"] }
	if ch(r.Candidate.Severities) <= ch(r.Original.Severities) {
		return score.Pass
	}
	return score.Fail
}

// lenientG5 keeps G5's verdict unless every contract change it reported is
// the same command in shell and exec form.
func lenientG5(r Record) score.Status {
	s := gateStatus(r, score.G5Behaviour)
	if s != score.Fail {
		return s
	}
	changes, ok := contractChanges(gateReason(r, score.G5Behaviour))
	if !ok {
		return s
	}
	for _, c := range changes {
		if c.Kind != kindShellExec {
			return s
		}
	}
	return score.Pass
}

func gateReason(r Record, id score.GateID) string {
	for _, g := range r.Gates {
		if g.ID == id {
			return g.Reason
		}
	}
	return ""
}

// Contract change kinds found by the G5 audit.
const (
	kindShellExec = "same command, shell vs exec form"
	kindUnset     = "set ↔ unset"
	kindChanged   = "changed"
)

// ContractChange is one setting a G5 failure reported as changed.
type ContractChange struct {
	Setting string // ENTRYPOINT, CMD, EXPOSE, WORKDIR, USER
	From    string
	To      string
	Kind    string
}

var (
	contractPrefix = "runtime contract changed: "
	contractPart   = regexp.MustCompile(`^(ENTRYPOINT|CMD|EXPOSE|WORKDIR|USER) (.*) -> (.*)$`)
	quoted         = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
)

// contractChanges parses a G5 contract reason ("runtime contract changed:
// CMD [...] -> [...]; WORKDIR ..."); ok is false for a smoke-test failure.
func contractChanges(reason string) ([]ContractChange, bool) {
	rest, ok := strings.CutPrefix(reason, contractPrefix)
	if !ok {
		return nil, false
	}
	var out []ContractChange
	for _, part := range strings.Split(rest, "; ") {
		m := contractPart.FindStringSubmatch(part)
		if m == nil {
			out = append(out, ContractChange{Setting: "?", From: part, Kind: kindChanged})
			continue
		}
		c := ContractChange{Setting: m[1], From: m[2], To: m[3], Kind: kindChanged}
		if c.Setting == "CMD" || c.Setting == "ENTRYPOINT" {
			from, to := words(m[2]), words(m[3])
			switch {
			case len(from) == 0 != (len(to) == 0):
				c.Kind = kindUnset
			case sameCommandForms(from, to):
				c.Kind = kindShellExec
			}
		}
		out = append(out, c)
	}
	return out, true
}

func words(list string) []string {
	var out []string
	for _, m := range quoted.FindAllString(list, -1) {
		if s, err := strconv.Unquote(m); err == nil {
			out = append(out, s)
		}
	}
	return out
}

// sameCommandForms reports the same command written once in shell form
// (["/bin/sh", "-c", "python app.py"]) and once in exec form (["python",
// "app.py"]). The exec form needs no shell, so the two run the same program;
// they differ in signal handling (PID 1), which the paper should mention.
func sameCommandForms(a, b []string) bool {
	shell := func(c []string) (string, bool) {
		if len(c) == 3 && (c[0] == "/bin/sh" || c[0] == "/bin/bash" || c[0] == "sh") && c[1] == "-c" {
			return strings.Join(strings.Fields(c[2]), " "), true
		}
		return "", false
	}
	if sa, ok := shell(a); ok {
		if _, ok := shell(b); !ok {
			return sa == strings.Join(b, " ")
		}
	}
	if sb, ok := shell(b); ok {
		if _, ok := shell(a); !ok {
			return sb == strings.Join(a, " ")
		}
	}
	return false
}

// RuleResult is one success definition for one pipeline.
type RuleResult struct {
	Images, Solved, Attempts, Successes int
}

// PairResult is McNemar under one success definition for a pipeline pair.
type PairResult struct {
	A, B         string
	OnlyA, OnlyB int
	P            float64
}

// G3Result is one pipeline's G3 failures.
type G3Result struct {
	Checked, Failed, ImagesFailing int
}

// G3Pair compares G3 failures per image between two pipelines.
type G3Pair struct {
	A, B string
	// OnlyA and OnlyB count images with a G3 failure under one pipeline only.
	OnlyA, OnlyB int
	McNemarP     float64
	// Counts compares each image's number of G3-failing repetitions, B − A.
	Counts score.WilcoxonResult
}

// Analysis is the S9 analyses over several pipelines' records.
type Analysis struct {
	Pipelines []string
	Tolerance float64
	// Rules holds, per success definition, each pipeline's result; Pairs
	// the McNemar tests between consecutive pipelines and first and last.
	Rules map[string]map[string]RuleResult
	Pairs map[string][]PairResult
	// Mismatches counts attempts whose recomputed "as measured" success
	// differs from the recorded one: a check that the recomputation is
	// right; it must be 0.
	Mismatches int
	// LintGates is G7 for every counted attempt, in pipeline order, for
	// eval/plots/figures.py: the records themselves hold G1–G6 only.
	LintGates []LintGate
	// LintNotChecked counts candidates G7 could not be checked for.
	LintNotChecked map[string]int
	LintFailed     map[string]int
	G3             map[string]G3Result
	G3Pairs        []G3Pair
	// Contract counts G5 contract changes by setting and kind, per
	// pipeline; ContractFailures the G5 contract failures per pipeline.
	Contract         map[string]map[string]int
	ContractFailures map[string]int
	SmokeFailures    map[string]int
}

// LintGate is G7 for one counted attempt.
type LintGate struct {
	Pipeline   string       `json:"pipeline"`
	Item       string       `json:"item"`
	Repetition int          `json:"repetition"`
	Status     score.Status `json:"status"`
	Reason     string       `json:"reason,omitempty"`
}

// Analyze computes the analyses. pipelines gives the order; lint may be nil,
// which leaves G7 not checked.
func Analyze(ctx context.Context, records []Record, pipelines []string, lint verify.Linter, tol float64) (*Analysis, error) {
	a := &Analysis{
		Pipelines: pipelines, Tolerance: tol,
		Rules: map[string]map[string]RuleResult{}, Pairs: map[string][]PairResult{},
		LintNotChecked: map[string]int{}, LintFailed: map[string]int{},
		G3: map[string]G3Result{}, Contract: map[string]map[string]int{},
		ContractFailures: map[string]int{}, SmokeFailures: map[string]int{},
	}
	views := map[string][]view{}
	for _, p := range pipelines {
		last := map[attemptKey]Record{}
		for _, r := range records {
			if r.Pipeline == p {
				last[r.key()] = r
			}
		}
		keys := make([]attemptKey, 0, len(last))
		for k := range last {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].item != keys[j].item {
				return keys[i].item < keys[j].item
			}
			return keys[i].repetition < keys[j].repetition
		})
		for _, k := range keys {
			r := last[k]
			if r.Status == StatusInfraError || r.Status == StatusItemInvalid {
				continue
			}
			v := view{Record: r, g7: score.Gate{ID: G7Lint, Status: score.NotChecked}}
			if lint != nil && r.Status == StatusMeasured {
				g, err := lintGate(ctx, lint, r)
				if err != nil {
					return nil, fmt.Errorf("%s %s #%d: %w", p, r.Item, r.Repetition, err)
				}
				v.g7 = g
			}
			a.LintGates = append(a.LintGates, LintGate{Pipeline: p, Item: r.Item, Repetition: r.Repetition, Status: v.g7.Status, Reason: v.g7.Reason})
			switch v.g7.Status {
			case score.NotChecked:
				a.LintNotChecked[p]++
			case score.Fail:
				a.LintFailed[p]++
			}
			if successWith(v, tol, nil, false) != r.Success {
				a.Mismatches++
			}
			views[p] = append(views[p], v)
		}
	}

	for _, rule := range SuccessRules {
		a.Rules[rule.Name] = map[string]RuleResult{}
		outcomes := map[string]map[string]*itemOutcome{}
		for _, p := range pipelines {
			per := map[string]*itemOutcome{}
			var res RuleResult
			for _, v := range views[p] {
				o := per[v.Item]
				if o == nil {
					o = &itemOutcome{}
					per[v.Item] = o
				}
				o.attempts++
				res.Attempts++
				if rule.ok(v, tol) {
					o.successes++
					res.Successes++
				}
			}
			res.Images = len(per)
			for _, o := range per {
				if o.solved() {
					res.Solved++
				}
			}
			a.Rules[rule.Name][p] = res
			outcomes[p] = per
		}
		for _, pr := range pairs(pipelines) {
			x, y := outcomes[pr[0]], outcomes[pr[1]]
			r := PairResult{A: pr[0], B: pr[1]}
			for item, ox := range x {
				oy, ok := y[item]
				if !ok {
					continue
				}
				switch {
				case ox.solved() && !oy.solved():
					r.OnlyA++
				case oy.solved() && !ox.solved():
					r.OnlyB++
				}
			}
			r.P = score.McNemar(r.OnlyA, r.OnlyB)
			a.Pairs[rule.Name] = append(a.Pairs[rule.Name], r)
		}
	}

	g3fails := map[string]map[string]int{}
	for _, p := range pipelines {
		g3fails[p] = map[string]int{}
		var res G3Result
		a.Contract[p] = map[string]int{}
		for _, v := range views[p] {
			g3fails[p][v.Item] += 0 // every counted image is in the paired test
			switch gateStatus(v.Record, score.G3Versions) {
			case score.Pass:
				res.Checked++
			case score.Fail:
				res.Checked++
				res.Failed++
				g3fails[p][v.Item]++
			}
			if gateStatus(v.Record, score.G5Behaviour) == score.Fail {
				changes, ok := contractChanges(gateReason(v.Record, score.G5Behaviour))
				if !ok {
					a.SmokeFailures[p]++
					continue
				}
				a.ContractFailures[p]++
				for _, c := range changes {
					a.Contract[p][c.Setting+": "+c.Kind]++
				}
			}
		}
		for _, n := range g3fails[p] {
			if n > 0 {
				res.ImagesFailing++
			}
		}
		a.G3[p] = res
	}
	for _, pr := range pairs(pipelines) {
		x, y := g3fails[pr[0]], g3fails[pr[1]]
		gp := G3Pair{A: pr[0], B: pr[1]}
		var diffs []float64
		for item, nx := range x {
			ny, ok := y[item]
			if !ok {
				continue
			}
			switch {
			case nx > 0 && ny == 0:
				gp.OnlyA++
			case ny > 0 && nx == 0:
				gp.OnlyB++
			}
			diffs = append(diffs, float64(ny-nx))
		}
		gp.McNemarP = score.McNemar(gp.OnlyA, gp.OnlyB)
		gp.Counts = score.Wilcoxon(diffs)
		a.G3Pairs = append(a.G3Pairs, gp)
	}
	return a, nil
}

// pairs are the consecutive pipelines, then the first against the last.
func pairs(ps []string) [][2]string {
	var out [][2]string
	for i := 0; i+1 < len(ps); i++ {
		out = append(out, [2]string{ps[i], ps[i+1]})
	}
	if len(ps) > 2 {
		out = append(out, [2]string{ps[0], ps[len(ps)-1]})
	}
	return out
}

// WriteAnalysis writes the analyses as markdown.
func WriteAnalysis(w io.Writer, a *Analysis) error {
	var b strings.Builder
	b.WriteString("# S9 analyses\n\n")
	fmt.Fprintf(&b, "Recomputed from the records (size tolerance %s). Recomputing the success rule as measured reproduces the recorded outcome of every attempt: %d mismatches.\n\n",
		percent(a.Tolerance), a.Mismatches)

	b.WriteString("## Success under other definitions\n\n")
	b.WriteString("Solved: images with at least one successful repetition (pass@3); per attempt: successful attempts over attempts. McNemar compares images solved by one pipeline only.\n\n")
	b.WriteString("| Definition |")
	for _, p := range a.Pipelines {
		fmt.Fprintf(&b, " `%s` solved | per attempt |", p)
	}
	for _, pr := range pairs(a.Pipelines) {
		fmt.Fprintf(&b, " %s → %s |", pr[0], pr[1])
	}
	b.WriteString("\n|---|")
	b.WriteString(strings.Repeat("---:|---:|", len(a.Pipelines)))
	b.WriteString(strings.Repeat("---|", len(pairs(a.Pipelines))))
	b.WriteString("\n")
	for _, rule := range SuccessRules {
		fmt.Fprintf(&b, "| %s |", rule.Name)
		for _, p := range a.Pipelines {
			r := a.Rules[rule.Name][p]
			fmt.Fprintf(&b, " %d/%d (%s) | %s |", r.Solved, r.Images, percent(ratio(r.Solved, r.Images)), percent(ratio(r.Successes, r.Attempts)))
		}
		for _, pr := range a.Pairs[rule.Name] {
			fmt.Fprintf(&b, " %d vs %d, p = %.3g |", pr.OnlyB, pr.OnlyA, pr.P)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nPairs read \"images solved only by the later pipeline vs only by the earlier\".\n\n")

	b.WriteString("## G7: no new lint errors\n\n")
	b.WriteString("Hadolint error-level findings in C whose rule O′ does not have (the rule W1's lint check uses), computed after the runs for every measured attempt.\n\n")
	b.WriteString("| Pipeline | G7 fails | Not checked |\n|---|---:|---:|\n")
	for _, p := range a.Pipelines {
		fmt.Fprintf(&b, "| `%s` | %d | %d |\n", p, a.LintFailed[p], a.LintNotChecked[p])
	}

	b.WriteString("\n## G3: invented tags and versions\n\n")
	b.WriteString("| Pipeline | Attempts checked | G3 fails | Images with a G3 failure |\n|---|---:|---:|---:|\n")
	for _, p := range a.Pipelines {
		g := a.G3[p]
		fmt.Fprintf(&b, "| `%s` | %d | %d (%s) | %d |\n", p, g.Checked, g.Failed, percent(ratio(g.Failed, g.Checked)), g.ImagesFailing)
	}
	b.WriteString("\n| Pair | Images failing only in A / only in B | McNemar p | Wilcoxon on failing repetitions per image (B − A): n, W+, p |\n|---|---:|---:|---|\n")
	for _, gp := range a.G3Pairs {
		fmt.Fprintf(&b, "| %s → %s | %d / %d | %.3g | n = %d, W+ = %g, p = %.3g |\n",
			gp.A, gp.B, gp.OnlyA, gp.OnlyB, gp.McNemarP, gp.Counts.N, gp.Counts.WPlus, gp.Counts.P)
	}

	b.WriteString("\n## G5 audit: runtime contract changes\n\n")
	b.WriteString("Each setting a G5 contract failure reported, by kind. \"Same command, shell vs exec form\" runs the same program and is a candidate false positive (it differs in signal handling as PID 1); the definition \"G5 accepting shell/exec forms\" above counts it as a pass.\n\n")
	var kinds []string
	seen := map[string]bool{}
	for _, p := range a.Pipelines {
		for k := range a.Contract[p] {
			if !seen[k] {
				seen[k] = true
				kinds = append(kinds, k)
			}
		}
	}
	sort.Strings(kinds)
	b.WriteString("| Change |")
	for _, p := range a.Pipelines {
		fmt.Fprintf(&b, " `%s` |", p)
	}
	b.WriteString("\n|---|" + strings.Repeat("---:|", len(a.Pipelines)) + "\n")
	for _, k := range kinds {
		fmt.Fprintf(&b, "| %s |", k)
		for _, p := range a.Pipelines {
			fmt.Fprintf(&b, " %d |", a.Contract[p][k])
		}
		b.WriteString("\n")
	}
	b.WriteString("| **attempts failing G5 on the contract** |")
	for _, p := range a.Pipelines {
		fmt.Fprintf(&b, " %d |", a.ContractFailures[p])
	}
	b.WriteString("\n| **attempts failing G5 on the smoke test** |")
	for _, p := range a.Pipelines {
		fmt.Fprintf(&b, " %d |", a.SmokeFailures[p])
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}
