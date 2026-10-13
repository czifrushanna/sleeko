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

package score

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/czifrushanna/sleeko/pkg/dfparse"
)

// GateID names one of the six gates.
type GateID string

const (
	G1Parse       GateID = "G1_parse"
	G2NoInvention GateID = "G2_no_invention"
	G3Versions    GateID = "G3_versions_exist"
	G4Build       GateID = "G4_build"
	G5Behaviour   GateID = "G5_behaviour"
	G6NoNewSevere GateID = "G6_no_new_severe"
)

// AllGates lists the gates in order.
var AllGates = []GateID{G1Parse, G2NoInvention, G3Versions, G4Build, G5Behaviour, G6NoNewSevere}

// Status is a gate's outcome.
type Status string

const (
	Pass Status = "pass"
	Fail Status = "fail"
	// NotChecked records a gate that could not be evaluated: G3 until the
	// version checks exist (S7), or a later gate after an earlier one made it
	// impossible (no build, so no smoke test).
	NotChecked Status = "not_checked"
)

// Gate is one gate's outcome, with the reason when it did not pass.
type Gate struct {
	ID     GateID `json:"id"`
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func pass(id GateID) Gate { return Gate{ID: id, Status: Pass} }

func fail(id GateID, format string, args ...any) Gate {
	return Gate{ID: id, Status: Fail, Reason: fmt.Sprintf(format, args...)}
}

// CheckParse is G1: C parses with BuildKit's Dockerfile parser. The parsed
// file is returned for the gates that need it; nil when the gate fails.
func CheckParse(candidate string) (Gate, *dfparse.File) {
	f, err := dfparse.Parse(candidate)
	if err != nil {
		return fail(G1Parse, "%v", err), nil
	}
	return pass(G1Parse), f
}

// CheckNoInvention is G2: every build-context source C copies is covered by
// what O copies, so C does not reference files it cannot know exist. COPY
// --from is not a context copy and is always allowed. A C source is covered
// when O copies the same path, a directory containing it ("." covers
// everything, "src/" covers "src/app.py"), or a glob matching it — so copying
// requirements.txt ahead of `COPY . .` for layer caching is not an invention.
// Paths are compared after cleaning ("./src/" equals "src").
func CheckNoInvention(o, c *dfparse.File) Gate {
	var oSources []string
	for _, cc := range o.ContextCopies() {
		for _, s := range cc.Sources {
			oSources = append(oSources, cleanSource(s))
		}
	}

	var invented []string
	for _, cc := range c.ContextCopies() {
		for _, s := range cc.Sources {
			if !covered(cleanSource(s), oSources) {
				invented = append(invented, fmt.Sprintf("%s (line %d)", s, cc.Line))
			}
		}
	}
	if len(invented) > 0 {
		return fail(G2NoInvention, "context sources not in the original: %s", strings.Join(invented, ", "))
	}
	return pass(G2NoInvention)
}

func cleanSource(s string) string {
	return path.Clean(strings.TrimPrefix(s, "/"))
}

func covered(src string, oSources []string) bool {
	for _, o := range oSources {
		if src == o || o == "." || strings.HasPrefix(src, o+"/") {
			return true
		}
		if ok, err := path.Match(o, src); err == nil && ok {
			return true
		}
	}
	return false
}

// CheckVersions is G3: every pinned base image tag and apt/pip version
// exists. The lookups are made elsewhere (S7); failures lists what was not
// found. Pass checked=false until those lookups exist.
func CheckVersions(checked bool, failures []string) Gate {
	if !checked {
		return Gate{ID: G3Versions, Status: NotChecked, Reason: "version lookups not available"}
	}
	if len(failures) > 0 {
		return fail(G3Versions, "not found: %s", strings.Join(failures, ", "))
	}
	return pass(G3Versions)
}

// CheckBuild is G4: C builds the same way O′ did. buildErr is the build's
// error, nil on success.
func CheckBuild(buildErr error) Gate {
	if buildErr != nil {
		return fail(G4Build, "%v", buildErr)
	}
	return pass(G4Build)
}

// Contract is how an image is meant to run: the settings G5 requires C to
// keep. For G5, fill it from the built O′ and C image configs: G5 only runs
// after both images built, and only the image config shows what C inherits
// from a new base image (a base swap can change USER, ENTRYPOINT or CMD
// without any Dockerfile line changing). ContractFromDockerfile gives the
// Dockerfile-level view, for checks made before anything is built.
type Contract struct {
	// Entrypoint and Cmd are nil when not set; a set-but-empty list differs
	// from an unset one.
	Entrypoint []string
	Cmd        []string
	// Expose holds ports; "8080" and "8080/tcp" are the same port.
	Expose  []string
	Workdir string
	// User is empty when not set, which means root.
	User string
}

// ContractFromDockerfile converts the Dockerfile-level runtime contract, for
// checks made before a build (S7's verify stage); G5 should use the image
// configs instead (see Contract). A shell-form command becomes the ["/bin/sh", "-c", ...] form Docker stores, so
// a candidate that switches between the forms without changing what runs is
// not penalised.
func ContractFromDockerfile(rc dfparse.RuntimeContract) Contract {
	return Contract{
		Entrypoint: commandArgs(rc.Entrypoint),
		Cmd:        commandArgs(rc.Cmd),
		Expose:     rc.Expose,
		Workdir:    rc.Workdir,
		User:       rc.User,
	}
}

func commandArgs(c *dfparse.Command) []string {
	switch {
	case c == nil:
		return nil
	case c.Shell:
		return []string{"/bin/sh", "-c", strings.Join(c.Args, " ")}
	default:
		return append([]string{}, c.Args...)
	}
}

// CheckBehaviour is G5: the smoke test passes and the runtime contract is
// unchanged — ENTRYPOINT, CMD, EXPOSE and WORKDIR equal, and USER either
// equal or changed from root to a non-root user. smokeErr is the smoke
// test's error, nil on success; o and c come from the built O′ and C image
// configs.
func CheckBehaviour(smokeErr error, o, c Contract) Gate {
	if smokeErr != nil {
		return fail(G5Behaviour, "smoke test: %v", smokeErr)
	}

	var diffs []string
	if !sameCommand(o.Entrypoint, c.Entrypoint) {
		diffs = append(diffs, fmt.Sprintf("ENTRYPOINT %q -> %q", o.Entrypoint, c.Entrypoint))
	}
	if !sameCommand(o.Cmd, c.Cmd) {
		diffs = append(diffs, fmt.Sprintf("CMD %q -> %q", o.Cmd, c.Cmd))
	}
	if oe, ce := portSet(o.Expose), portSet(c.Expose); !slices.Equal(oe, ce) {
		diffs = append(diffs, fmt.Sprintf("EXPOSE %v -> %v", oe, ce))
	}
	if cleanDir(o.Workdir) != cleanDir(c.Workdir) {
		diffs = append(diffs, fmt.Sprintf("WORKDIR %q -> %q", o.Workdir, c.Workdir))
	}
	if o.User != c.User && !(isRoot(o.User) && !isRoot(c.User)) {
		diffs = append(diffs, fmt.Sprintf("USER %q -> %q", o.User, c.User))
	}

	if len(diffs) > 0 {
		return fail(G5Behaviour, "runtime contract changed: %s", strings.Join(diffs, "; "))
	}
	return pass(G5Behaviour)
}

func sameCommand(a, b []string) bool {
	return (a == nil) == (b == nil) && slices.Equal(a, b)
}

func portSet(ports []string) []string {
	set := map[string]bool{}
	for _, p := range ports {
		if !strings.Contains(p, "/") {
			p += "/tcp"
		}
		set[strings.ToLower(p)] = true
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func cleanDir(d string) string {
	if d == "" {
		return ""
	}
	return path.Clean(d)
}

// isRoot reports whether a USER value runs as root: unset, or user root / UID
// 0, with any group.
func isRoot(user string) bool {
	u, _, _ := strings.Cut(user, ":")
	return u == "" || u == "root" || u == "0"
}

// CheckNoNewSevere is G6: C has no critical or high CVE that O′ did not have.
// IDs are compared regardless of package and of the severity O′ had them at,
// since a base swap can rename the package or re-rate the same CVE.
func CheckNoNewSevere(o, c ScanResult) Gate {
	inO := o.ids(SeverityUnknown)
	var added []string
	for id := range c.ids(SeverityHigh) {
		if !inO[id] {
			added = append(added, id)
		}
	}
	if len(added) > 0 {
		sort.Strings(added)
		return fail(G6NoNewSevere, "new critical/high: %s", strings.Join(added, ", "))
	}
	return pass(G6NoNewSevere)
}
