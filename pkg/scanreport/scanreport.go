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

// Package scanreport condenses a Harbor vulnerability scan report into the
// smallest text that still supports rewriting a Dockerfile.
//
// Harbor hands the adapter the full normalized report: one record per CVE, each
// carrying a prose description, an upstream link list and per-vendor CVSS
// scores. On a real base image that is megabytes of JSON — more than an LLM
// context window holds, and none of those bytes can change a line of the
// Dockerfile. Three reductions are applied in order:
//
//  1. Project the fields. Only id, package, version, fix_version and severity
//     are decoded; everything else is dropped at unmarshal time.
//  2. Separate fixable from unfixable. A record with no fix_version supports no
//     Dockerfile edit, so those collapse into a single counted line.
//  3. Group by package. Every edit the model can make — bump the base image,
//     pin or upgrade a package, drop it — is per package, not per CVE, so the
//     per-package aggregate carries the same signal at a fraction of the size.
//
// Render then fits the result into a character budget, dropping the
// lowest-severity packages last and always saying how many it dropped.
package scanreport

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// rawReport and rawItem decode only the fields that survive reduction 1. The
// remaining fields of Harbor's vuln.VulnerabilityItem (description, links,
// preferred_cvss, vendor_attributes, cwe_ids, artifact_digests) are ignored by
// encoding/json and never allocated.
type rawReport struct {
	// A pointer distinguishes "vulnerabilities": [] (a real report with no
	// findings) from a payload that has no vulnerabilities field at all, which
	// is some other shape we should not claim to have summarized.
	Vulnerabilities *[]rawItem `json:"vulnerabilities"`
}

type rawItem struct {
	ID         string `json:"id"`
	Package    string `json:"package"`
	Version    string `json:"version"`
	FixVersion string `json:"fix_version"`
	Severity   string `json:"severity"`
}

// Severity buckets, most severe first. Anything Harbor reports outside this set
// (or with an empty severity) lands in the last bucket.
const (
	sevCritical = iota
	sevHigh
	sevMedium
	sevLow
	sevNegligible
	sevUnknown
	numSeverities
)

var severityNames = [numSeverities]string{"Critical", "High", "Medium", "Low", "Negligible", "Unknown"}

var severityIndex = map[string]int{
	"critical":   sevCritical,
	"high":       sevHigh,
	"medium":     sevMedium,
	"low":        sevLow,
	"negligible": sevNegligible,
	"none":       sevNegligible,
	"unknown":    sevUnknown,
}

// counts is a per-severity finding tally.
type counts [numSeverities]int

func (c *counts) add(severity string) {
	i, ok := severityIndex[strings.ToLower(strings.TrimSpace(severity))]
	if !ok {
		i = sevUnknown
	}
	c[i]++
}

func (c *counts) addAll(other counts) {
	for i, n := range other {
		c[i] += n
	}
}

func (c counts) total() int {
	sum := 0
	for _, n := range c {
		sum += n
	}
	return sum
}

// String renders the non-zero buckets, e.g. "3 Critical, 7 High".
func (c counts) String() string {
	parts := make([]string, 0, numSeverities)
	for i, n := range c {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, severityNames[i]))
		}
	}
	return strings.Join(parts, ", ")
}

// moreSevereThan compares two tallies severity bucket by severity bucket, so no
// number of Medium findings can outrank a single Critical one.
func (c counts) moreSevereThan(other counts) bool {
	for i := range c {
		if c[i] != other[i] {
			return c[i] > other[i]
		}
	}
	return false
}

// Package is one installed package with everything known about its findings.
type Package struct {
	Name string
	// Version is the version installed in the image.
	Version string
	// FixVersion is the highest fix version across this package's findings:
	// upgrading to it clears all of them, whereas any lower one would not.
	FixVersion string
	Counts     counts
}

func (p Package) line() string {
	return fmt.Sprintf("%s %s -> %s (%s)", p.Name, p.Version, p.FixVersion, p.Counts.String())
}

// Summary is the condensed report: a per-package list of what can be fixed, and
// a tally of what cannot.
type Summary struct {
	// Findings is the number of distinct findings in the report.
	Findings int
	// Packages is the number of distinct affected packages.
	Packages int
	// All is the severity tally across every finding.
	All counts

	// Fixable holds the packages with a fix available, most severe first.
	Fixable         []Package
	FixableFindings int

	UnfixablePackages int
	UnfixableFindings int
	UnfixableCounts   counts
}

// Summarize parses a Harbor vulnerability scan report and applies reductions
// 1-3. It fails only on input that is not a vulnerability report: callers are
// expected to fall back to the raw text in that case rather than dropping it.
func Summarize(raw string) (*Summary, error) {
	var r rawReport
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, fmt.Errorf("parse scan report: %w", err)
	}
	if r.Vulnerabilities == nil {
		return nil, fmt.Errorf("parse scan report: no vulnerabilities field")
	}

	type key struct{ name, version string }

	grouped := map[key]*Package{}
	order := []key{}
	seen := map[string]bool{}
	sum := &Summary{}

	for _, item := range *r.Vulnerabilities {
		// Harbor already de-duplicates on id+package+version when merging
		// reports; repeat the check so a duplicated record can't inflate a
		// count the model reads as severity.
		id := item.ID + "\x00" + item.Package + "\x00" + item.Version
		if seen[id] {
			continue
		}
		seen[id] = true

		name := item.Package
		if name == "" {
			name = "(unnamed package)"
		}

		k := key{name, item.Version}
		pkg, ok := grouped[k]
		if !ok {
			pkg = &Package{Name: name, Version: item.Version}
			grouped[k] = pkg
			order = append(order, k)
		}

		pkg.Counts.add(item.Severity)
		if item.FixVersion != "" && compareVersions(item.FixVersion, pkg.FixVersion) > 0 {
			pkg.FixVersion = item.FixVersion
		}

		sum.All.add(item.Severity)
		sum.Findings++
	}

	sum.Packages = len(order)

	for _, k := range order {
		pkg := grouped[k]
		if pkg.FixVersion == "" {
			sum.UnfixablePackages++
			sum.UnfixableFindings += pkg.Counts.total()
			sum.UnfixableCounts.addAll(pkg.Counts)
			continue
		}

		sum.Fixable = append(sum.Fixable, *pkg)
		sum.FixableFindings += pkg.Counts.total()
	}

	// Deterministic, severity-first order: the same image always produces the
	// same prompt, and truncation always drops the least important lines.
	sort.SliceStable(sum.Fixable, func(i, j int) bool {
		a, b := sum.Fixable[i], sum.Fixable[j]
		if a.Counts != b.Counts {
			return a.Counts.moreSevereThan(b.Counts)
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version < b.Version
	})

	return sum, nil
}

const fixableHeading = "Packages with a fix available (installed version -> upgrade to at least):"

// Render writes the summary as prompt text no longer than budget characters.
// A budget of zero or less means unlimited.
func (s *Summary) Render(budget int) string {
	head := s.head()
	if s.Findings == 0 {
		return truncate(head, budget)
	}

	tail := s.unfixableLine()
	lines := make([]string, 0, len(s.Fixable))
	for _, p := range s.Fixable {
		lines = append(lines, p.line())
	}

	if out := assemble(head, lines, "", tail); budget <= 0 || len(out) <= budget {
		return out
	}

	// Over budget: keep the most severe packages and account for the notice
	// that tells the model the list is partial. Sizes are measured from prefix
	// sums so this stays linear in the number of packages.
	sizes := make([]int, len(lines)+1)
	for i, l := range lines {
		sizes[i+1] = sizes[i] + len(l) + 1
	}
	fixed := len(head) + len("\n"+fixableHeading+"\n")
	if tail != "" {
		fixed += len("\n" + tail + "\n")
	}

	for shown := len(lines); shown > 0; shown-- {
		notice := s.omissionNotice(shown)
		if fixed+sizes[shown]+len(notice)+1 <= budget {
			return assemble(head, lines[:shown], notice, tail)
		}
	}

	// Not even one package line fits. Report the shape of what was dropped for
	// as long as that fits, then fall back to the header alone: an oversized
	// prompt prefix is not something this package can cut its way out of.
	notice := s.omissionNotice(0)
	if out := assemble(head, nil, notice, tail); len(out) <= budget {
		return out
	}
	if out := assemble(head, nil, notice, ""); len(out) <= budget {
		return out
	}
	return truncate(head, budget)
}

func (s *Summary) head() string {
	var b strings.Builder
	b.WriteString("Vulnerability summary (condensed from the artifact's scan report):\n")

	if s.Findings == 0 {
		b.WriteString("No vulnerabilities reported.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "%s in %s; %d in %d %s have a fix available.\n",
		plural(s.Findings, "finding"), plural(s.Packages, "package"),
		s.FixableFindings, len(s.Fixable), pluralWord(len(s.Fixable), "package"))
	fmt.Fprintf(&b, "Totals by severity: %s.\n", s.All.String())
	return b.String()
}

func (s *Summary) unfixableLine() string {
	if s.UnfixablePackages == 0 {
		return ""
	}

	return fmt.Sprintf("No fix available for %s / %s (%s) - no Dockerfile change resolves these.",
		plural(s.UnfixablePackages, "package"), plural(s.UnfixableFindings, "finding"),
		s.UnfixableCounts.String())
}

// omissionNotice describes the fixable packages past the first shown of them.
func (s *Summary) omissionNotice(shown int) string {
	omitted := s.Fixable[shown:]
	if len(omitted) == 0 {
		return ""
	}

	var c counts
	for _, p := range omitted {
		c.addAll(p.Counts)
	}

	return fmt.Sprintf("(+%s with a fix omitted for length: %s - %s)",
		plural(len(omitted), "package"), plural(c.total(), "finding"), c.String())
}

func assemble(head string, lines []string, notice, tail string) string {
	var b strings.Builder
	b.WriteString(head)

	if len(lines) > 0 {
		b.WriteString("\n")
		b.WriteString(fixableHeading)
		b.WriteString("\n")
		for _, l := range lines {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}

	if notice != "" {
		if len(lines) == 0 {
			b.WriteString("\n")
		}
		b.WriteString(notice)
		b.WriteString("\n")
	}

	if tail != "" {
		b.WriteString("\n")
		b.WriteString(tail)
		b.WriteString("\n")
	}

	return b.String()
}

// Condense reduces a raw Harbor scan report to prompt text no longer than
// budget characters (zero or less means unlimited). Input that does not parse
// as a vulnerability report is passed through truncated, so an unexpected
// payload shape degrades to less of the same rather than being dropped.
func Condense(raw string, budget int) string {
	sum, err := Summarize(raw)
	if err != nil {
		return truncate(raw, budget)
	}

	return sum.Render(budget)
}

const truncationMarker = "\n... (truncated for length)\n"

// truncate cuts s to budget bytes at a line boundary where there is one, never
// mid-rune, and says that it did.
func truncate(s string, budget int) string {
	if budget <= 0 || len(s) <= budget {
		return s
	}
	if budget <= len(truncationMarker) {
		return cutRunes(s, budget)
	}

	cut := cutRunes(s, budget-len(truncationMarker))
	if i := strings.LastIndexByte(cut, '\n'); i > len(cut)/2 {
		cut = cut[:i]
	}

	return cut + truncationMarker
}

func cutRunes(s string, n int) string {
	if n >= len(s) {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}

	return s[:n]
}

func plural(n int, noun string) string {
	return fmt.Sprintf("%d %s", n, pluralWord(n, noun))
}

func pluralWord(n int, noun string) string {
	if n == 1 {
		return noun
	}

	return noun + "s"
}

// compareVersions orders two package version strings, returning -1, 0 or 1. It
// follows dpkg's algorithm: runs of digits compare numerically, everything else
// compares character by character under dpkg's ordering, where '~' sorts before
// the end of a string and the end of a string sorts before a letter. That gets
// the layouts a Debian or Ubuntu base image produces right, including
// 1.0~rc1 < 1.0 < 1.0rc1 and epochs like 1:1.2.13.
//
// Alpine's apk and language-ecosystem semver differ in their own edge cases, so
// an unusual layout costs precision here, not correctness: the result only
// decides which fix version a package line recommends, and the line says
// "upgrade to at least".
func compareVersions(a, b string) int {
	for a != "" || b != "" {
		// Compare the leading non-digit run character by character.
		for (a != "" && !isDigit(a[0])) || (b != "" && !isDigit(b[0])) {
			ac, bc := 0, 0
			if a != "" {
				ac = orderChar(a[0])
				a = a[1:]
			}
			if b != "" {
				bc = orderChar(b[0])
				b = b[1:]
			}
			if ac != bc {
				return sign(ac - bc)
			}
		}

		// Compare the leading digit run numerically. Stripping leading zeros
		// first means the longer run is the larger number, and the first
		// differing digit decides equal-length runs.
		a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")

		firstDiff := 0
		for a != "" && b != "" && isDigit(a[0]) && isDigit(b[0]) {
			if firstDiff == 0 {
				firstDiff = int(a[0]) - int(b[0])
			}
			a, b = a[1:], b[1:]
		}

		if a != "" && isDigit(a[0]) {
			return 1
		}
		if b != "" && isDigit(b[0]) {
			return -1
		}
		if firstDiff != 0 {
			return sign(firstDiff)
		}
	}

	return 0
}

// orderChar is dpkg's character ordering for the non-digit parts of a version:
// '~' first, then the end of the string, then letters, then everything else.
func orderChar(c byte) int {
	switch {
	case c == '~':
		return -1
	case isAlpha(c):
		return int(c)
	default:
		return int(c) + 256
	}
}

func sign(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}

	return 0
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isAlpha(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
