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

package verify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/lookup"
	"github.com/czifrushanna/sleeko/pkg/score"
)

// fakeLookups knows a few tags and versions; anything mentioning "broken"
// fails as a lookup error.
type fakeLookups struct {
	tags map[string]bool
	apt  map[string]bool // "distro/suite pkg=version"
	pip  map[string]bool // "name==version"
}

func (f *fakeLookups) TagExists(_ context.Context, ref string) (bool, error) {
	if strings.Contains(ref, "broken") {
		return false, errors.New("registry unreachable")
	}
	return f.tags[ref], nil
}

func (f *fakeLookups) AptHas(_ context.Context, rel lookup.Release, pkg, version string) (bool, error) {
	return f.apt[rel.Distro+"/"+rel.Suite+" "+pkg+"="+version], nil
}

func (f *fakeLookups) PipHas(_ context.Context, name, version string) (bool, error) {
	return f.pip[name+"=="+version], nil
}

func lookups() *fakeLookups {
	return &fakeLookups{
		tags: map[string]bool{"python:3.12-slim-bookworm": true, "python:3.11-slim": true, "debian:bookworm-slim": true, "alpine:3.20": true},
		apt: map[string]bool{
			"debian/bookworm curl=7.88.1-10+deb12u12":  true,
			"debian/bookworm libssl3=3.0.15-1~deb12u1": true,
			"debian/bookworm libssl3=3.0.11-1~deb12u2": true,
		},
		pip: map[string]bool{"requests==2.32.3": true, "setuptools==65.5.1": true, "setuptools==70.0.0": true},
	}
}

const original = `FROM python:3.11-slim
WORKDIR /app
RUN apt-get update && apt-get install -y curl
COPY requirements.txt .
RUN pip install -r requirements.txt
COPY src/ ./src/
EXPOSE 8000
CMD ["python", "-m", "app"]
`

// scan is O′'s scan: a fixable libssl3 and setuptools, versions built for
// Debian 12.
var scan = score.NewScanResult([]score.Vulnerability{
	{ID: "CVE-2024-5535", Package: "libssl3", InstalledVersion: "3.0.11-1~deb12u2", FixedVersion: "3.0.13-1~deb12u1", Severity: score.SeverityCritical},
	{ID: "CVE-2024-6345", Package: "setuptools", InstalledVersion: "65.5.1", FixedVersion: "70.0.0", Severity: score.SeverityHigh},
	{ID: "CVE-2024-2398", Package: "curl", InstalledVersion: "7.88.1-10+deb12u5", FixedVersion: "7.88.1-10+deb12u6, 8.0.0-1", Severity: score.SeverityMedium},
})

// candidate is the original with lines replaced and added.
func candidate(replace map[string]string, add ...string) string {
	c := original
	for from, to := range replace {
		c = strings.Replace(c, from, to, 1)
	}
	return c + strings.Join(add, "\n")
}

func verify(t *testing.T, cand string, lk Lookups, lint Linter) Result {
	t.Helper()
	return (&Verifier{Lookups: lk, Lint: lint}).Verify(context.Background(), Input{Original: original, Candidate: cand, Scan: scan})
}

func messages(r Result, check string) []string {
	var out []string
	for _, f := range r.Findings {
		if f.Check == check {
			out = append(out, string(f.Severity)+": "+f.Message)
		}
	}
	return out
}

func TestVerify_CleanCandidate(t *testing.T) {
	r := verify(t, candidate(map[string]string{
		"FROM python:3.11-slim":   "FROM python:3.12-slim-bookworm",
		"apt-get install -y curl": "apt-get install -y curl=7.88.1-10+deb12u12 libssl3=3.0.15-1~deb12u1",
	}, "RUN pip install --no-cache-dir setuptools==70.0.0\n"), lookups(), nil)
	require.Empty(t, r.Findings)
	require.True(t, r.Passed())
	require.Equal(t, map[string]string{CheckLint: "hadolint not available"}, r.Skipped)
}

func TestVerify_Parse(t *testing.T) {
	r := verify(t, "FROM python:3.12\nCOPYY . .\n", lookups(), nil)
	require.Len(t, r.Findings, 1, "nothing else is checked")
	require.Equal(t, CheckParse, r.Findings[0].Check)
	require.Equal(t, 2, r.Findings[0].Line)
	require.Equal(t, []string{CheckParse}, r.FailedChecks())
}

func TestVerify_Context(t *testing.T) {
	r := verify(t, candidate(nil, "COPY pyproject.toml .\n"), lookups(), nil)
	require.Equal(t, []string{"fail: context sources not in the original: pyproject.toml (line 9); COPY/ADD may only use files the original Dockerfile copies (COPY --from another stage is fine)"}, messages(r, CheckContext))
}

func TestVerify_DroppedRun(t *testing.T) {
	r := verify(t, candidate(map[string]string{"RUN pip install -r requirements.txt\n": ""}), lookups(), nil)
	require.Equal(t, []string{"warn: the original's RUN at line 5 is gone: pip install -r requirements.txt"}, messages(r, CheckDroppedRun))
	require.True(t, r.Passed(), "a warning does not fail the candidate")

	// Merged into another RUN is not dropped.
	merged := candidate(map[string]string{
		"RUN pip install -r requirements.txt": "RUN pip install -r requirements.txt && rm -rf /root/.cache",
	})
	require.Empty(t, messages(verify(t, merged, lookups(), nil), CheckDroppedRun))
}

func TestVerify_BaseTags(t *testing.T) {
	r := verify(t, candidate(map[string]string{"FROM python:3.11-slim": "FROM python:3.99-slim"}), lookups(), nil)
	require.Equal(t, []string{"fail: the base image python:3.99-slim does not exist in its registry; use a tag that exists"}, messages(r, CheckBaseTag))
	require.Equal(t, 1, r.Findings[0].Line)

	r = verify(t, candidate(map[string]string{"FROM python:3.11-slim": "FROM broken.example/python:3"}), lookups(), nil)
	require.Empty(t, messages(r, CheckBaseTag), "a lookup error is not the candidate's fault")
	require.Contains(t, r.Skipped[CheckBaseTag], "registry unreachable")

	r = verify(t, candidate(map[string]string{"FROM python:3.11-slim": "ARG TAG\nFROM python:${TAG}"}), lookups(), nil)
	require.Contains(t, r.Skipped[CheckBaseTag], "unresolved")
}

func TestVerify_AptVersions(t *testing.T) {
	// The release comes from the tag's codename.
	r := verify(t, candidate(map[string]string{
		"FROM python:3.11-slim":   "FROM python:3.12-slim-bookworm",
		"apt-get install -y curl": "apt-get install -y curl=7.88.1-10+deb12u99",
	}), lookups(), nil)
	require.Equal(t, []string{"fail: apt cannot install curl=7.88.1-10+deb12u99: that version is not in debian bookworm (or its -updates/-security); pin a version the archive has, or leave the version out"}, messages(r, CheckAptVersion))

	// Same base as the original: the release comes from O′'s package versions.
	r = verify(t, candidate(map[string]string{"apt-get install -y curl": "apt-get install -y curl=7.88.1-10+deb12u12"}), lookups(), nil)
	require.Empty(t, messages(r, CheckAptVersion))
	require.NotContains(t, r.Skipped, CheckAptVersion)

	// A new base without a codename in its tag: the release is unknown.
	r = verify(t, candidate(map[string]string{
		"FROM python:3.11-slim":   "FROM python:3.12-slim",
		"apt-get install -y curl": "apt-get install -y curl=7.88.1-10+deb12u12",
	}), lookups(), nil)
	require.Contains(t, r.Skipped[CheckAptVersion], "release of stage 0's base image is not known")
}

func TestVerify_PipVersions(t *testing.T) {
	r := verify(t, candidate(nil, "RUN pip install requests==2.99.0 setuptools==70.0.0\n"), lookups(), nil)
	require.Equal(t, []string{"fail: pip cannot install requests==2.99.0: PyPI has no such release; pin a version that exists"}, messages(r, CheckPipVersion))
}

func TestVerify_FixVersions(t *testing.T) {
	cases := []struct {
		name, line string
		want       []string
	}{
		{"pinned below the fix", "RUN pip install setuptools==65.5.1",
			[]string{"fail: setuptools is pinned to 65.5.1, below 70.0.0, the version that fixes CVE-2024-6345; pin 70.0.0 or later"}},
		{"pinned at the fix", "RUN pip install setuptools==70.0.0", nil},
		{"name normalisation", "RUN pip install Setuptools==65.5.1",
			[]string{"fail: Setuptools is pinned to 65.5.1, below 70.0.0, the version that fixes CVE-2024-6345; pin 70.0.0 or later"}},
		{"Debian ordering (~ and epochs)", "RUN apt-get install -y libssl3=3.0.11-1~deb12u2",
			[]string{"fail: libssl3 is pinned to 3.0.11-1~deb12u2, below 3.0.13-1~deb12u1, the version that fixes CVE-2024-5535; pin 3.0.13-1~deb12u1 or later"}},
		{"one of several backported fixes is enough", "RUN apt-get install -y curl=7.88.1-10+deb12u6", nil},
		{"unpinned is not checked", "RUN pip install setuptools", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := verify(t, candidate(nil, c.line+"\n"), lookups(), nil)
			require.Equal(t, c.want, messages(r, CheckFixVersion))
		})
	}
}

func TestVerify_Contract(t *testing.T) {
	r := verify(t, candidate(map[string]string{"EXPOSE 8000": "EXPOSE 9000"}), lookups(), nil)
	require.Len(t, messages(r, CheckContract), 1)
	require.Contains(t, messages(r, CheckContract)[0], "EXPOSE [8000/tcp] -> [9000/tcp]")

	r = verify(t, candidate(nil, "USER app\n"), lookups(), nil)
	require.Empty(t, messages(r, CheckContract), "root to non-root is allowed")
}

type fakeLinter map[string][]LintFinding

func (f fakeLinter) Lint(_ context.Context, df string) ([]LintFinding, error) {
	if strings.Contains(df, "LINTFAIL") {
		return nil, errors.New("hadolint crashed")
	}
	if strings.Contains(df, "candidate") {
		return f["candidate"], nil
	}
	return f["original"], nil
}

func TestVerify_Lint(t *testing.T) {
	lint := fakeLinter{
		"original":  {{Code: "DL3008", Level: "warning", Line: 3}},
		"candidate": {{Code: "DL3008", Level: "warning", Line: 3}, {Code: "DL3020", Level: "error", Line: 9, Message: "Use COPY instead of ADD"}, {Code: "DL3020", Level: "error", Line: 10}, {Code: "DL3042", Level: "warning", Line: 5, Message: "Avoid cache"}, {Code: "DL3059", Level: "info", Line: 2}},
	}
	r := verify(t, candidate(nil, "# candidate\n"), lookups(), lint)
	require.Equal(t, []string{
		"warn: new lint warning DL3042 at line 5: Avoid cache",
		"fail: new lint error DL3020 at line 9: Use COPY instead of ADD",
	}, messages(r, CheckLint))

	r = verify(t, candidate(nil, "# LINTFAIL\n"), lookups(), lint)
	require.Contains(t, r.Skipped[CheckLint], "hadolint crashed")
}

func TestVerify_NoLookups(t *testing.T) {
	r := verify(t, candidate(map[string]string{"FROM python:3.11-slim": "FROM python:3.99"}), nil, nil)
	require.True(t, r.Passed(), "nothing could be looked up, nothing fails")
	for _, c := range []string{CheckBaseTag, CheckAptVersion, CheckPipVersion} {
		require.Contains(t, r.Skipped, c)
	}
}

func TestMeetsAnyFix(t *testing.T) {
	cmp := func(a, b string) (int, error) { return strings.Compare(a, b), nil }
	fixed, ok := meetsAnyFix(cmp, "2", "3, 4")
	require.False(t, ok)
	require.Equal(t, "3", fixed, "the lowest fix is suggested")
	_, ok = meetsAnyFix(cmp, "3", "4, 3")
	require.True(t, ok)
	_, ok = meetsAnyFix(cmp, "1", "")
	require.True(t, ok, "no fixed version, nothing to meet")
}

func TestBadness(t *testing.T) {
	r := Result{Findings: []Finding{
		{Check: CheckContext, Severity: Fail}, {Check: CheckContext, Severity: Fail},
		{Check: CheckBaseTag, Severity: Fail}, {Check: CheckDroppedRun, Severity: Warn},
	}}
	require.Equal(t, 2, r.Badness(), "distinct failing checks; warnings do not count")
	require.Equal(t, len(AllChecks), Result{Findings: []Finding{{Check: CheckParse, Severity: Fail}}}.Badness())
	require.Zero(t, Result{}.Badness())
}
