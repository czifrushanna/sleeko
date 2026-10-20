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

// Package verify checks a candidate Dockerfile for the ways an LLM's rewrite
// goes wrong, before anything is built: it does not parse, copies files the
// original never used, drops commands, pins base images or package versions
// that do not exist, pins a package below its fixed version, or changes how
// the image runs. Each finding says what is wrong in words the model can act
// on in a repair round.
//
// A check that cannot run — a lookup failed, the base's release is unknown,
// Hadolint is not installed — is reported as skipped, never as a failure: a
// candidate is not blamed for the verifier's blind spots.
package verify

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/czifrushanna/sleeko/pkg/dfparse"
	"github.com/czifrushanna/sleeko/pkg/lookup"
	"github.com/czifrushanna/sleeko/pkg/score"
	"github.com/czifrushanna/sleeko/pkg/versions"
)

// Check names, as they appear in findings, records and reports.
const (
	CheckParse      = "parse"
	CheckContext    = "context_copies"
	CheckDroppedRun = "dropped_run"
	CheckBaseTag    = "base_tag_exists"
	CheckAptVersion = "apt_version_exists"
	CheckPipVersion = "pip_version_exists"
	CheckFixVersion = "fix_version"
	CheckContract   = "runtime_contract"
	CheckLint       = "hadolint"
)

// AllChecks lists the checks in the order they run.
var AllChecks = []string{CheckParse, CheckContext, CheckDroppedRun, CheckBaseTag, CheckAptVersion, CheckPipVersion, CheckFixVersion, CheckContract, CheckLint}

// Severity says whether a finding fails the candidate.
type Severity string

const (
	// Fail findings are sent back to the model to repair.
	Fail Severity = "fail"
	// Warn findings are diagnostics only (a dropped RUN may be deliberate).
	Warn Severity = "warn"
)

// Finding is one problem a check found.
type Finding struct {
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	// Line is the candidate's line the finding is about; 0 when none.
	Line int `json:"line,omitempty"`
}

// Result is the outcome of verifying one candidate.
type Result struct {
	Findings []Finding `json:"findings,omitempty"`
	// Skipped maps checks that could not run to why.
	Skipped map[string]string `json:"skipped,omitempty"`
}

// Failures are the findings that fail the candidate.
func (r Result) Failures() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Severity == Fail {
			out = append(out, f)
		}
	}
	return out
}

// Passed reports whether nothing failed.
func (r Result) Passed() bool { return len(r.Failures()) == 0 }

// Badness ranks candidates for keeping the best of several: the number of
// distinct checks that failed, with a candidate that does not parse counting
// as failing them all, since nothing else about it could be checked.
func (r Result) Badness() int {
	failed := r.FailedChecks()
	for _, c := range failed {
		if c == CheckParse {
			return len(AllChecks)
		}
	}
	return len(failed)
}

// FailedChecks lists the distinct checks with a failure, in check order.
func (r Result) FailedChecks() []string {
	failed := map[string]bool{}
	for _, f := range r.Failures() {
		failed[f.Check] = true
	}
	var out []string
	for _, c := range AllChecks {
		if failed[c] {
			out = append(out, c)
		}
	}
	return out
}

func (r *Result) add(check string, sev Severity, line int, format string, args ...any) {
	r.Findings = append(r.Findings, Finding{Check: check, Severity: sev, Message: fmt.Sprintf(format, args...), Line: line})
}

func (r *Result) skip(check, why string) {
	if r.Skipped == nil {
		r.Skipped = map[string]string{}
	}
	if _, already := r.Skipped[check]; !already {
		r.Skipped[check] = why
	}
}

// Lookups answers whether pinned versions exist; *lookup.Client implements it.
type Lookups interface {
	TagExists(ctx context.Context, ref string) (bool, error)
	AptHas(ctx context.Context, rel lookup.Release, pkg, version string) (bool, error)
	PipHas(ctx context.Context, project, version string) (bool, error)
}

// Input is what a candidate is verified against.
type Input struct {
	Original  string
	Candidate string
	// Scan is O′'s scan: the vulnerabilities the candidate is meant to fix.
	Scan score.ScanResult
}

// Verifier runs the checks.
type Verifier struct {
	// Lookups answers the version checks; nil skips them.
	Lookups Lookups
	// Lint runs Hadolint; nil skips the lint check.
	Lint Linter
}

// Verify checks the candidate. A candidate that does not parse gets only
// that finding: nothing else can be checked.
func (v *Verifier) Verify(ctx context.Context, in Input) Result {
	var res Result
	cFile, err := dfparse.Parse(in.Candidate)
	if err != nil {
		line := 0
		if pe, ok := err.(*dfparse.ParseError); ok {
			line = pe.Line
		}
		res.add(CheckParse, Fail, line, "the Dockerfile does not parse: %v", err)
		return res
	}
	oFile, err := dfparse.Parse(in.Original)
	if err != nil {
		// Nothing to compare against; the original is the benchmark's.
		for _, c := range []string{CheckContext, CheckDroppedRun, CheckContract} {
			res.skip(c, "the original Dockerfile does not parse")
		}
	} else {
		checkContext(&res, oFile, cFile)
		checkDroppedRuns(&res, oFile, cFile)
		checkContract(&res, oFile, cFile)
	}

	pins := cFile.PackageInstalls()
	v.checkBaseTags(ctx, &res, cFile)
	v.checkPins(ctx, &res, in, oFile, cFile, pins)
	checkFixVersions(&res, in.Scan, pins)
	v.checkLint(ctx, &res, in)
	return res
}

func checkContext(res *Result, o, c *dfparse.File) {
	if g := score.CheckNoInvention(o, c); g.Status == score.Fail {
		res.add(CheckContext, Fail, 0, "%s; COPY/ADD may only use files the original Dockerfile copies (COPY --from another stage is fine)", g.Reason)
	}
}

// checkDroppedRuns warns about original RUN commands missing from the
// candidate. They may have been merged or removed on purpose, so this is a
// warning, not a failure; the model sees it in a repair round all the same.
func checkDroppedRuns(res *Result, o, c *dfparse.File) {
	have := map[string]int{}
	for _, r := range c.RunCommands() {
		have[r.Normalized()]++
	}
	// A RUN merged into another (cmd1 && cmd2) still counts as kept.
	all := strings.Join(keys(have), "\n")
	for _, r := range o.RunCommands() {
		n := r.Normalized()
		if have[n] > 0 {
			have[n]--
			continue
		}
		if strings.Contains(all, n) {
			continue
		}
		res.add(CheckDroppedRun, Warn, 0, "the original's RUN at line %d is gone: %s", r.StartLine, truncate(n, 160))
	}
}

func keys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// checkContract compares the Dockerfile-level runtime contract. The image
// configs are compared again after the build (G5); this catches the change
// before a build is spent on it.
func checkContract(res *Result, o, c *dfparse.File) {
	oc, err1 := o.RuntimeContract()
	cc, err2 := c.RuntimeContract()
	if err1 != nil || err2 != nil {
		res.skip(CheckContract, "the runtime contract could not be read")
		return
	}
	if g := score.CheckBehaviour(nil, score.ContractFromDockerfile(oc), score.ContractFromDockerfile(cc)); g.Status == score.Fail {
		res.add(CheckContract, Fail, 0, "%s; keep ENTRYPOINT, CMD, EXPOSE and WORKDIR as they are, and USER unless moving from root to a non-root user", g.Reason)
	}
}

// Existence runs only the checks that what the candidate pins exists: base
// image tags, apt and pip versions. It is the evaluation's G3; Verify runs
// the same checks among the others.
func (v *Verifier) Existence(ctx context.Context, in Input) Result {
	var res Result
	cFile, err := dfparse.Parse(in.Candidate)
	if err != nil {
		for _, c := range []string{CheckBaseTag, CheckAptVersion, CheckPipVersion} {
			res.skip(c, "the Dockerfile does not parse")
		}
		return res
	}
	oFile, err := dfparse.Parse(in.Original)
	if err != nil {
		oFile = nil
	}
	v.checkBaseTags(ctx, &res, cFile)
	v.checkPins(ctx, &res, in, oFile, cFile, cFile.PackageInstalls())
	return res
}

func (v *Verifier) checkBaseTags(ctx context.Context, res *Result, c *dfparse.File) {
	if v.Lookups == nil {
		res.skip(CheckBaseTag, "no lookups configured")
		return
	}
	refs, err := c.FromRefs(nil)
	if err != nil {
		res.skip(CheckBaseTag, err.Error())
		return
	}
	for _, ref := range refs {
		if !ref.IsImage() {
			continue
		}
		if len(ref.Unresolved) > 0 {
			res.skip(CheckBaseTag, fmt.Sprintf("FROM %s uses unresolved variables", ref.Raw))
			continue
		}
		ok, err := v.Lookups.TagExists(ctx, ref.Image)
		switch {
		case errors.Is(err, lookup.ErrBadDigest):
			res.add(CheckBaseTag, Fail, ref.Line, "the base image %s has a malformed digest and cannot be pulled; use a tag, or a digest that exists", ref.Image)
		case err != nil:
			res.skip(CheckBaseTag, fmt.Sprintf("%s: %v", ref.Image, err))
		case !ok:
			res.add(CheckBaseTag, Fail, ref.Line, "the base image %s does not exist in its registry; use a tag that exists", ref.Image)
		}
	}
}

// debMarker finds the Debian release a package version was built for
// ("+deb12u1", "~deb11u3").
var debMarker = regexp.MustCompile(`deb(\d+)u\d+`)

// releaseFor finds the release whose archive an apt pin in a stage installs
// from: the codename in the stage's base image tag, or else — when the stage
// keeps the original's base — the release O′'s package versions were built
// for.
func releaseFor(stage int, refs []dfparse.FromRef, oRefs []dfparse.FromRef, scan score.ScanResult) (lookup.Release, bool) {
	for stage >= 0 && refs[stage].BaseStage >= 0 {
		stage = refs[stage].BaseStage
	}
	if stage < 0 || len(refs[stage].Unresolved) > 0 {
		return lookup.Release{}, false
	}
	image := refs[stage].Image
	if rel, ok := lookup.ReleaseFromTag(image); ok {
		return rel, true
	}
	same := false
	for _, o := range oRefs {
		if o.IsImage() && o.Image == image {
			same = true
		}
	}
	if !same {
		return lookup.Release{}, false
	}
	counts := map[string]int{}
	for _, v := range scan.Vulnerabilities {
		if m := debMarker.FindStringSubmatch(v.InstalledVersion); m != nil {
			counts[m[1]]++
		}
	}
	best, bestN := "", 0
	for major, n := range counts {
		if n > bestN {
			best, bestN = major, n
		}
	}
	if best == "" {
		return lookup.Release{}, false
	}
	return lookup.ReleaseOf("debian", best)
}

// Package sources besides the distribution's archive and PyPI: a version
// missing from those can still install from these, so pins are not judged
// against the official indexes when a Dockerfile adds any (anywhere in the
// file: a RUN, an ENV, a copied config). Backports count too: AptHas does
// not search them.
var (
	extraAptSource = regexp.MustCompile(`sources\.list|/etc/apt/|add-apt-repository|apt-key|signed-by=|(?m)(^|["'\s])deb(-src)?\s+(\[[^\]]*\]\s+)?https?://|backports|--target-release|apt-get\s+(\S+\s+)*-t\s`)
	extraPipSource = regexp.MustCompile(`--index-url|--extra-index-url|PIP_INDEX_URL|PIP_EXTRA_INDEX_URL|PIP_FIND_LINKS|pip\.conf|--find-links|--no-index|\s-i\s+https?://|\s-f\s+\S`)
)

func (v *Verifier) checkPins(ctx context.Context, res *Result, in Input, o, c *dfparse.File, pins []dfparse.PackageInstall) {
	if v.Lookups == nil {
		res.skip(CheckAptVersion, "no lookups configured")
		res.skip(CheckPipVersion, "no lookups configured")
		return
	}
	aptExtra := extraAptSource.MatchString(in.Candidate)
	pipExtra := extraPipSource.MatchString(in.Candidate)
	refs, err := c.FromRefs(nil)
	if err != nil {
		res.skip(CheckAptVersion, err.Error())
		return
	}
	var oRefs []dfparse.FromRef
	if o != nil {
		oRefs, _ = o.FromRefs(nil)
	}

	for _, p := range pins {
		if p.Version == "" {
			continue
		}
		switch p.Manager {
		case dfparse.ManagerApt:
			if aptExtra {
				res.skip(CheckAptVersion, "the Dockerfile adds apt sources besides the distribution's archive")
				continue
			}
			rel, ok := releaseFor(p.Stage, refs, oRefs, in.Scan)
			if !ok {
				res.skip(CheckAptVersion, fmt.Sprintf("the Debian/Ubuntu release of stage %d's base image is not known", p.Stage))
				continue
			}
			has, err := v.Lookups.AptHas(ctx, rel, p.Name, p.Version)
			switch {
			case err != nil:
				res.skip(CheckAptVersion, err.Error())
			case !has:
				res.add(CheckAptVersion, Fail, p.Line, "apt cannot install %s=%s: that version is not in %s %s (or its -updates/-security); pin a version the archive has, or leave the version out", p.Name, p.Version, rel.Distro, rel.Suite)
			}
		case dfparse.ManagerPip:
			if pipExtra {
				res.skip(CheckPipVersion, "the Dockerfile installs from package indexes besides PyPI")
				continue
			}
			has, err := v.Lookups.PipHas(ctx, p.Name, p.Version)
			switch {
			case err != nil:
				res.skip(CheckPipVersion, err.Error())
			case !has:
				res.add(CheckPipVersion, Fail, p.Line, "pip cannot install %s==%s: PyPI has no such release; pin a version that exists", p.Name, p.Version)
			}
		}
	}
}

// checkFixVersions fails pins below the version that fixes a vulnerability
// O′ has in that package: the rewrite would keep the vulnerability while
// looking like it fixed it.
func checkFixVersions(res *Result, scan score.ScanResult, pins []dfparse.PackageInstall) {
	for _, p := range pins {
		if p.Version == "" {
			continue
		}
		cmp := versions.Deb
		match := func(pkg string) bool { return pkg == p.Name }
		if p.Manager == dfparse.ManagerPip {
			cmp = versions.PEP440
			want := normalizePip(p.Name)
			match = func(pkg string) bool { return normalizePip(pkg) == want }
		}
		for _, v := range scan.Vulnerabilities {
			if !v.Fixable() || !match(v.Package) {
				continue
			}
			if fixed, ok := meetsAnyFix(cmp, p.Version, v.FixedVersion); !ok {
				res.add(CheckFixVersion, Fail, p.Line, "%s is pinned to %s, below %s, the version that fixes %s; pin %s or later", p.Name, p.Version, fixed, v.ID, fixed)
			}
		}
	}
}

// meetsAnyFix reports whether version is at least one of the fixed versions
// (Trivy lists several, comma-separated, when the fix was backported to more
// than one release line). It also returns the lowest fixed version, for the
// message. Unparseable versions count as met: the check never guesses.
func meetsAnyFix(cmp func(a, b string) (int, error), version, fixedList string) (string, bool) {
	lowest := ""
	for _, f := range strings.Split(fixedList, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		c, err := cmp(version, f)
		if err != nil || c >= 0 {
			return f, true
		}
		if lowest == "" {
			lowest = f
		} else if lc, err := cmp(f, lowest); err == nil && lc < 0 {
			lowest = f
		}
	}
	return lowest, lowest == ""
}

var pipSep = regexp.MustCompile(`[-_.]+`)

func normalizePip(name string) string {
	return strings.ToLower(pipSep.ReplaceAllString(name, "-"))
}
