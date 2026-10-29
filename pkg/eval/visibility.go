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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/czifrushanna/sleeko/pkg/score"
	"github.com/czifrushanna/sleeko/pkg/versions"
)

// G6 counts a critical or high vulnerability ID in C that O′'s scan does
// not report. Such a finding is not always new to the image: when C moves
// to a release the scanner covers better (an end-of-life base moved to a
// supported one) or makes a package visible (an SBOM the newer pip ships),
// the vulnerability may have been in O′ all along, only unreported.
//
// The saved scans list the vulnerable packages of every image, not the full
// inventory, so whether a package was in O′ is only known when O′'s scan
// reports it. The classes below therefore bound the effect instead of
// measuring it exactly: Certain and Probable are what the scans prove or
// strongly suggest, Undetermined is what they cannot tell apart.

// VisibilityClass says why a new critical or high finding of C is not in
// O′'s scan.
type VisibilityClass int

const (
	// Certain: O′ has the same package at the same version, yet the scanner
	// does not report the vulnerability there: only the coverage changed.
	Certain VisibilityClass = iota
	// Probable: O′ has the package at an older version than C's affected
	// one, so it was very likely affected too.
	Probable
	// Undetermined: O′'s scan does not report the package, O′ is on an
	// end-of-life release and C on another release; the package may have
	// been hidden or may be new.
	Undetermined
	// LikelyNew: O′'s scan does not report the package and O′ is not on an
	// end-of-life release that C left: most likely a package C added.
	LikelyNew
	// Absent: O′'s inventory (a rebuild of O′ listing every package) does
	// not contain the package: C added it.
	Absent
	// New: O′ has the package at a newer version than C's: C introduced it.
	New
)

// VisibilityClasses lists the classes in order, for reports.
var VisibilityClasses = []VisibilityClass{Certain, Probable, Undetermined, LikelyNew, Absent, New}

func (c VisibilityClass) String() string {
	return [...]string{"certainly newly visible", "probably newly visible", "undetermined", "probably new", "absent from O′", "new"}[c]
}

// classify decides the class of one finding of C, given the versions of
// each package in O′: from O′'s scan, and from its inventory when there is
// one (inventory true).
func classify(v score.Vulnerability, oVersions map[string][]string, inventory bool, o, c ScanOutput) VisibilityClass {
	inO := oVersions[v.Package]
	if len(inO) == 0 {
		if inventory {
			return Absent
		}
		if o.EOSL != nil && *o.EOSL && o.OS != c.OS {
			return Undetermined
		}
		return LikelyNew
	}
	best := New
	for _, ov := range inO {
		if ov == v.InstalledVersion {
			return Certain
		}
		// The scans do not record each package's ecosystem; Debian's
		// ordering also orders Alpine and dotted versions, and only ever
		// leads to Probable, never Certain.
		if cmp, err := versions.Deb(ov, v.InstalledVersion); err == nil && cmp <= 0 {
			best = Probable
		}
	}
	return best
}

// newSevere classifies every new critical or high ID of C, as G6 counts
// them, by its least visibility-like occurrence: an ID counts as newly
// visible only if all of C's packages that carry it are. inv, when not nil,
// is O′'s inventory: it adds the packages O′'s scan does not report.
func newSevere(o, c ScanOutput, inv *Inventory) map[string]VisibilityClass {
	oIDs := map[string]bool{}
	oVersions := map[string][]string{}
	for _, v := range o.Result.Vulnerabilities {
		oIDs[v.ID] = true
		oVersions[v.Package] = append(oVersions[v.Package], v.InstalledVersion)
	}
	if inv != nil {
		for _, p := range inv.Packages {
			oVersions[p.Name] = append(oVersions[p.Name], p.Version)
		}
	}
	out := map[string]VisibilityClass{}
	for _, v := range c.Result.Vulnerabilities {
		if v.Severity < score.SeverityHigh || oIDs[v.ID] {
			continue
		}
		cl := classify(v, oVersions, inv != nil, o, c)
		if prev, ok := out[v.ID]; !ok || cl > prev {
			out[v.ID] = cl
		}
	}
	return out
}

// LoadScan reads a scan the harness saved (Runner.ScanDir).
func LoadScan(dir, digest string) (ScanOutput, error) {
	var s ScanOutput
	data, err := os.ReadFile(filepath.Join(dir, strings.ReplaceAll(digest, ":", "-")+".json"))
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(data, &s)
}

// VisibilityResult is one pipeline's G6 failures, split by visibility.
type VisibilityResult struct {
	Attempts, G6Failures int
	// IDs counts the new critical/high IDs behind G6 failures by class.
	IDs map[VisibilityClass]int
	// ExplainedLow counts G6 failures whose new IDs are all Certain or
	// Probable; ExplainedHigh those whose IDs are all at most Undetermined.
	ExplainedLow, ExplainedHigh int
	// EOSL splits the G6 failures by whether O′ is on an end-of-life
	// release.
	EOSLFailures, EOSLExplainedLow, EOSLExplainedHigh int
	// Rules is the success under each G6 variant (VisibilityRules).
	Rules map[string]RuleResult
}

// VisibilityRules are the G6 variants compared, as measured first.
var VisibilityRules = []string{
	"as measured (G1–G6)",
	"G6 without certainly or probably newly visible findings (lower bound)",
	"G6 without newly visible or undetermined findings (upper bound)",
	"without G6",
}

// VisibilityAnalysis is the scanner-visibility analysis of G6.
type VisibilityAnalysis struct {
	Pipelines []string
	Results   map[string]*VisibilityResult
	Pairs     map[string][]PairResult
	// Missing counts G6 failures whose scans were not found: they keep
	// G6's verdict in every variant.
	Missing int
	// Inventoried counts G6 failures whose O′ had an inventory; Drift
	// compares each inventoried O′ with the study's O′ scan.
	Inventoried int
	Drift       map[string]Drift
}

// Drift compares a rebuilt O′'s findings with the study's O′ scan: the
// rebuild is only a faithful stand-in when they barely differ.
type Drift struct {
	EOSL                   bool
	Study, Rebuilt, Shared int
	StudyOS, RebuiltOS     string
}

func drift(study ScanOutput, inv Inventory) Drift {
	a, b := study.Result.Vulnerabilities, inv.Scan.Vulnerabilities
	ids := func(vs []score.Vulnerability) map[string]bool {
		m := map[string]bool{}
		for _, v := range vs {
			m[v.ID+" "+v.Package] = true
		}
		return m
	}
	x, y := ids(a), ids(b)
	d := Drift{EOSL: inv.EOSL, Study: len(x), Rebuilt: len(y), StudyOS: study.OS, RebuiltOS: inv.OS}
	for k := range x {
		if y[k] {
			d.Shared++
		}
	}
	return d
}

// Visibility classifies the new critical or high findings behind every G6
// failure and recomputes success with G6 ignoring the newly visible ones.
// scanDirs maps each pipeline to the directory its runs saved scans to.
// inventories, which may be nil, maps items to their rebuilt O′.
func Visibility(records []Record, pipelines []string, scanDirs map[string]string, inventories map[string]Inventory, tol float64) (*VisibilityAnalysis, error) {
	va := &VisibilityAnalysis{Pipelines: pipelines, Results: map[string]*VisibilityResult{}, Pairs: map[string][]PairResult{}, Drift: map[string]Drift{}}
	outcomes := map[string]map[string]map[string]*itemOutcome{}
	for _, rule := range VisibilityRules {
		outcomes[rule] = map[string]map[string]*itemOutcome{}
	}
	for _, p := range pipelines {
		res := &VisibilityResult{IDs: map[VisibilityClass]int{}, Rules: map[string]RuleResult{}}
		va.Results[p] = res
		per := map[string]map[string]*itemOutcome{}
		for _, rule := range VisibilityRules {
			per[rule] = map[string]*itemOutcome{}
		}
		for _, r := range Counted(records, p) {
			res.Attempts++
			v := view{Record: r}
			g6 := gateStatus(r, score.G6NoNewSevere)
			low, high := g6, g6
			if g6 == score.Fail {
				res.G6Failures++
				eosl := r.Original != nil && r.Original.EOSL != nil && *r.Original.EOSL
				if eosl {
					res.EOSLFailures++
				}
				o, errO := LoadScan(scanDirs[p], r.Original.Digest)
				c, errC := LoadScan(scanDirs[p], r.Candidate.Digest)
				if errO != nil || errC != nil {
					va.Missing++
				} else {
					var inv *Inventory
					if x, ok := inventories[r.Item]; ok {
						inv = &x
						va.Inventoried++
						if _, seen := va.Drift[r.Item]; !seen {
							va.Drift[r.Item] = drift(o, x)
						}
					}
					worst := Certain
					for _, cl := range newSevere(o, c, inv) {
						res.IDs[cl]++
						if cl > worst {
							worst = cl
						}
					}
					if worst <= Probable {
						low = score.Pass
						res.ExplainedLow++
						if eosl {
							res.EOSLExplainedLow++
						}
					}
					if worst <= Undetermined {
						high = score.Pass
						res.ExplainedHigh++
						if eosl {
							res.EOSLExplainedHigh++
						}
					}
				}
			}
			decide := map[string]bool{
				VisibilityRules[0]: successWith(v, tol, nil, false),
				VisibilityRules[1]: successWith(v, tol, map[score.GateID]func(Record) score.Status{score.G6NoNewSevere: func(Record) score.Status { return low }}, false),
				VisibilityRules[2]: successWith(v, tol, map[score.GateID]func(Record) score.Status{score.G6NoNewSevere: func(Record) score.Status { return high }}, false),
				VisibilityRules[3]: successWith(v, tol, map[score.GateID]func(Record) score.Status{score.G6NoNewSevere: func(Record) score.Status { return score.Pass }}, false),
			}
			for rule, ok := range decide {
				o := per[rule][r.Item]
				if o == nil {
					o = &itemOutcome{}
					per[rule][r.Item] = o
				}
				o.attempts++
				if ok {
					o.successes++
				}
			}
		}
		for _, rule := range VisibilityRules {
			rr := RuleResult{Images: len(per[rule])}
			for _, o := range per[rule] {
				rr.Attempts += o.attempts
				rr.Successes += o.successes
				if o.solved() {
					rr.Solved++
				}
			}
			res.Rules[rule] = rr
			outcomes[rule][p] = per[rule]
		}
	}
	for _, rule := range VisibilityRules {
		for _, pr := range pairs(pipelines) {
			x, y := outcomes[rule][pr[0]], outcomes[rule][pr[1]]
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
			va.Pairs[rule] = append(va.Pairs[rule], r)
		}
	}
	return va, nil
}

// Counted is a pipeline's attempts as the analyses count them: the last
// record of each attempt, without harness errors and invalid items, in
// item and repetition order.
func Counted(records []Record, p string) []Record {
	last := map[attemptKey]Record{}
	for _, r := range records {
		if r.Pipeline == p {
			last[r.key()] = r
		}
	}
	out := make([]Record, 0, len(last))
	for _, r := range last {
		if r.Status == StatusInfraError || r.Status == StatusItemInvalid {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Item != out[j].Item {
			return out[i].Item < out[j].Item
		}
		return out[i].Repetition < out[j].Repetition
	})
	return out
}

// WriteVisibility writes the analysis as markdown.
func WriteVisibility(w io.Writer, va *VisibilityAnalysis) error {
	var b strings.Builder
	b.WriteString("# Scanner visibility and G6\n\n")
	b.WriteString("G6 fails when C has a critical or high vulnerability ID that O′'s scan does not report. Each such ID is classified by comparing C's affected packages with O′'s scan: ")
	b.WriteString("*certainly newly visible* when O′ has the same package at the same version, *probably newly visible* when O′ has it at an older version, ")
	b.WriteString("*undetermined* when O′'s scan does not report the package while O′ is on an end-of-life release that C left, *probably new* when it does not report the package otherwise, ")
	b.WriteString("and *new* when O′ has the package at a newer version. An ID takes the class of its least visibility-like package. ")
	b.WriteString("The scans list vulnerable packages only, so the classes bound the effect: the lower bound ignores certainly and probably newly visible IDs, the upper bound also undetermined ones.\n\n")
	if va.Missing > 0 {
		fmt.Fprintf(&b, "Scans not found for %d G6 failures; they keep G6's verdict.\n\n", va.Missing)
	}
	if len(va.Drift) > 0 {
		fmt.Fprintf(&b, "%d G6 failures had an inventory of O′ (a rebuild of O′ listing every package, with the same scanner and database); for them, a package O′'s scan does not report is either found in the inventory or *absent from O′*. ", va.Inventoried)
		b.WriteString("The rebuild stands in for the study's O′ only if their findings match; the drift table at the end compares them.\n\n")
	}
	b.WriteString("## New critical/high IDs behind G6 failures\n\n| Pipeline | Attempts | G6 failures |")
	for _, c := range VisibilityClasses {
		fmt.Fprintf(&b, " %s |", c)
	}
	b.WriteString(" Failures explained (lower–upper) | of which O′ end of life |\n|---|---:|---:|")
	for range VisibilityClasses {
		b.WriteString("---:|")
	}
	b.WriteString("---:|---:|\n")
	for _, p := range va.Pipelines {
		r := va.Results[p]
		fmt.Fprintf(&b, "| `%s` | %d | %d |", p, r.Attempts, r.G6Failures)
		for _, c := range VisibilityClasses {
			fmt.Fprintf(&b, " %d |", r.IDs[c])
		}
		fmt.Fprintf(&b, " %d–%d | %d–%d of %d |\n", r.ExplainedLow, r.ExplainedHigh, r.EOSLExplainedLow, r.EOSLExplainedHigh, r.EOSLFailures)
	}
	b.WriteString("\n## Success with G6 counting only findings that are not newly visible\n\n| Definition |")
	for _, p := range va.Pipelines {
		fmt.Fprintf(&b, " `%s` solved | per attempt |", p)
	}
	b.WriteString("\n|---|")
	for range va.Pipelines {
		b.WriteString("---:|---:|")
	}
	b.WriteString("\n")
	for _, rule := range VisibilityRules {
		fmt.Fprintf(&b, "| %s |", rule)
		for _, p := range va.Pipelines {
			rr := va.Results[p].Rules[rule]
			fmt.Fprintf(&b, " %d/%d (%s) | %s |", rr.Solved, rr.Images, percent(ratio(rr.Solved, rr.Images)), percent(ratio(rr.Successes, rr.Attempts)))
		}
		b.WriteString("\n")
	}
	b.WriteString("\nMcNemar on solved images, \"only the later pipeline vs only the earlier\":\n\n| Definition |")
	for _, pr := range pairs(va.Pipelines) {
		fmt.Fprintf(&b, " %s → %s |", pr[0], pr[1])
	}
	b.WriteString("\n|---|")
	for range pairs(va.Pipelines) {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, rule := range VisibilityRules {
		fmt.Fprintf(&b, "| %s |", rule)
		for _, pr := range va.Pairs[rule] {
			fmt.Fprintf(&b, " %d vs %d, p = %.3g |", pr.OnlyB, pr.OnlyA, pr.P)
		}
		b.WriteString("\n")
	}
	if len(va.Drift) > 0 {
		b.WriteString("\n## Drift between the study's O′ and its rebuild\n\nFindings are pairs of vulnerability and package.\n\n| Item | O′ end of life | Study O′ | Rebuilt O′ | Study findings | Rebuilt findings | Shared |\n|---|---|---|---|---:|---:|---:|\n")
		items := make([]string, 0, len(va.Drift))
		for it := range va.Drift {
			items = append(items, it)
		}
		sort.Strings(items)
		for _, it := range items {
			d := va.Drift[it]
			fmt.Fprintf(&b, "| `%s` | %v | %s | %s | %d | %d | %d |\n", it, d.EOSL, d.StudyOS, d.RebuiltOS, d.Study, d.Rebuilt, d.Shared)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
