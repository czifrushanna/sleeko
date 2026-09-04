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

package scanreport

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// item builds one record in the shape Harbor's vuln.VulnerabilityItem
// marshals to, including the fields reduction 1 is expected to drop.
func item(id, pkg, version, fixVersion, severity string) map[string]any {
	return map[string]any{
		"id":          id,
		"package":     pkg,
		"version":     version,
		"fix_version": fixVersion,
		"severity":    severity,
		"status":      "fixed",
		"description": strings.Repeat("prose that cannot change a Dockerfile. ", 20),
		"links": []string{
			"https://nvd.nist.gov/vuln/detail/" + id,
			"https://access.redhat.com/security/cve/" + id,
		},
		"artifact_digests": []string{"sha256:" + strings.Repeat("a", 64)},
		"preferred_cvss": map[string]any{
			"score_v3":  9.8,
			"vector_v3": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
		},
		"cwe_ids": []string{"CWE-190"},
		"vendor_attributes": map[string]any{
			"CVSS": map[string]any{
				"nvd":    map[string]any{"V3Score": 9.8, "V3Vector": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
				"redhat": map[string]any{"V3Score": 7.5, "V3Vector": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H"},
			},
		},
	}
}

func report(t *testing.T, items ...map[string]any) string {
	t.Helper()

	raw, err := json.Marshal(map[string]any{
		"generated_at":    "2026-08-24T10:00:00Z",
		"scanner":         map[string]string{"name": "Trivy", "version": "v0.69.3"},
		"severity":        "Critical",
		"vulnerabilities": items,
	})
	require.NoError(t, err)

	return string(raw)
}

func TestSummarize_GroupsPerPackageAndSplitsFixable(t *testing.T) {
	raw := report(t,
		item("CVE-1", "zlib1g", "1:1.2.13", "1:1.2.13+deb12u1", "Critical"),
		item("CVE-2", "zlib1g", "1:1.2.13", "1:1.2.13+deb12u2", "High"),
		item("CVE-3", "zlib1g", "1:1.2.13", "", "Low"),
		item("CVE-4", "libtasn1", "4.19.0", "", "High"),
		item("CVE-5", "libtasn1", "4.19.0", "", "Medium"),
		item("CVE-6", "openssl", "3.0.11", "3.0.14", "Medium"),
	)

	sum, err := Summarize(raw)
	require.NoError(t, err)

	require.Equal(t, 6, sum.Findings)
	require.Equal(t, 3, sum.Packages)
	require.Equal(t, "1 Critical, 2 High, 2 Medium, 1 Low", sum.All.String())

	require.Len(t, sum.Fixable, 2)
	require.Equal(t, "zlib1g", sum.Fixable[0].Name)
	// The highest fix version wins: anything lower leaves CVE-2 unfixed.
	require.Equal(t, "1:1.2.13+deb12u2", sum.Fixable[0].FixVersion)
	// A package keeps its unfixable findings in its own tally rather than
	// splitting across sections.
	require.Equal(t, "1 Critical, 1 High, 1 Low", sum.Fixable[0].Counts.String())
	require.Equal(t, "openssl", sum.Fixable[1].Name)
	require.Equal(t, 4, sum.FixableFindings)

	require.Equal(t, 1, sum.UnfixablePackages)
	require.Equal(t, 2, sum.UnfixableFindings)
	require.Equal(t, "1 High, 1 Medium", sum.UnfixableCounts.String())
}

func TestSummarize_OrdersBySeverityThenName(t *testing.T) {
	raw := report(t,
		item("CVE-1", "many-medium", "1.0", "1.1", "Medium"),
		item("CVE-2", "many-medium", "1.0", "1.1", "Medium"),
		item("CVE-3", "many-medium", "1.0", "1.1", "Medium"),
		item("CVE-4", "one-high", "1.0", "1.1", "High"),
		item("CVE-5", "zzz-one-critical", "1.0", "1.1", "Critical"),
		item("CVE-6", "aaa-one-critical", "1.0", "1.1", "Critical"),
	)

	sum, err := Summarize(raw)
	require.NoError(t, err)

	names := []string{}
	for _, p := range sum.Fixable {
		names = append(names, p.Name)
	}
	// Severity dominates count (three Mediums do not outrank one High), and
	// equal tallies fall back to the package name for a stable order.
	require.Equal(t, []string{"aaa-one-critical", "zzz-one-critical", "one-high", "many-medium"}, names)
}

func TestSummarize_DeduplicatesAndBucketsUnknownSeverity(t *testing.T) {
	raw := report(t,
		item("CVE-1", "curl", "8.0", "8.1", "Critical"),
		item("CVE-1", "curl", "8.0", "8.1", "Critical"),
		item("CVE-2", "curl", "8.0", "8.1", "Wat"),
		item("CVE-3", "curl", "8.0", "8.1", ""),
	)

	sum, err := Summarize(raw)
	require.NoError(t, err)
	require.Equal(t, 3, sum.Findings)
	require.Equal(t, "1 Critical, 2 Unknown", sum.All.String())
}

func TestSummarize_SamePackageDifferentInstalledVersionsStaySeparate(t *testing.T) {
	raw := report(t,
		item("CVE-1", "libssl3", "3.0.11", "3.0.14", "High"),
		item("CVE-2", "libssl3", "3.0.9", "3.0.14", "High"),
	)

	sum, err := Summarize(raw)
	require.NoError(t, err)
	require.Equal(t, 2, sum.Packages)
	require.Len(t, sum.Fixable, 2)
}

func TestSummarize_RejectsNonReportPayloads(t *testing.T) {
	for name, raw := range map[string]string{
		"not json":              "test-report",
		"no vulnerabilities":    `{"generated_at":"2026-08-24T10:00:00Z"}`,
		"wrong item shape":      `{"vulnerabilities":["CVE-2024-1234"]}`,
		"vulnerabilities isn't": `{"vulnerabilities":{"a":1}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Summarize(raw)
			require.Error(t, err)
		})
	}
}

func TestSummarize_EmptyVulnerabilityListIsAValidReport(t *testing.T) {
	sum, err := Summarize(`{"vulnerabilities":[]}`)
	require.NoError(t, err)
	require.Equal(t, 0, sum.Findings)
	require.Equal(t, "Vulnerability summary (condensed from the artifact's scan report):\nNo vulnerabilities reported.\n", sum.Render(0))
}

func TestRender_FullSummary(t *testing.T) {
	raw := report(t,
		item("CVE-1", "zlib1g", "1:1.2.13", "1:1.2.13+deb12u1", "Critical"),
		item("CVE-2", "openssl", "3.0.11", "3.0.14", "High"),
		item("CVE-3", "libtasn1", "4.19.0", "", "Medium"),
	)

	sum, err := Summarize(raw)
	require.NoError(t, err)

	out := sum.Render(0)
	require.Equal(t, strings.Join([]string{
		"Vulnerability summary (condensed from the artifact's scan report):",
		"3 findings in 3 packages; 2 in 2 packages have a fix available.",
		"Totals by severity: 1 Critical, 1 High, 1 Medium.",
		"",
		"Packages with a fix available (installed version -> upgrade to at least):",
		"zlib1g 1:1.2.13 -> 1:1.2.13+deb12u1 (1 Critical)",
		"openssl 3.0.11 -> 3.0.14 (1 High)",
		"",
		"No fix available for 1 package / 1 finding (1 Medium) - no Dockerfile change resolves these.",
		"",
	}, "\n"), out)

	// The dropped fields are what made the report unsendable in the first place.
	require.NotContains(t, out, "prose that cannot change a Dockerfile")
	require.NotContains(t, out, "nvd.nist.gov")
	require.NotContains(t, out, "CVSS")
	require.NotContains(t, out, "CWE-190")
	require.NotContains(t, out, "sha256:")
}

func TestRender_ReductionIsWorthTheEffort(t *testing.T) {
	items := []map[string]any{}
	for i := 0; i < 400; i++ {
		// Twenty packages, twenty findings each: the shape that made a real
		// image's report larger than the context window.
		pkg := fmt.Sprintf("pkg-%02d", i%20)
		items = append(items, item(fmt.Sprintf("CVE-2026-%04d", i), pkg, "1.0", "1.1", "High"))
	}
	raw := report(t, items...)

	sum, err := Summarize(raw)
	require.NoError(t, err)

	out := sum.Render(0)
	require.Equal(t, 400, sum.Findings)
	require.Len(t, sum.Fixable, 20)
	require.Less(t, len(out), len(raw)/50, "expected at least a 50x reduction, got %d -> %d bytes", len(raw), len(out))
}

func TestRender_TruncatesLeastSevereFirstAndSaysSo(t *testing.T) {
	items := []map[string]any{
		item("CVE-C", "critical-pkg", "1.0", "1.1", "Critical"),
	}
	for i := 0; i < 30; i++ {
		items = append(items, item(fmt.Sprintf("CVE-L-%d", i), fmt.Sprintf("low-pkg-%02d", i), "1.0", "1.1", "Low"))
	}

	sum, err := Summarize(report(t, items...))
	require.NoError(t, err)

	full := sum.Render(0)
	budget := len(full) / 2
	out := sum.Render(budget)

	require.LessOrEqual(t, len(out), budget)
	require.Contains(t, out, "critical-pkg 1.0 -> 1.1 (1 Critical)")
	require.Contains(t, out, "with a fix omitted for length")
	require.Contains(t, out, "Totals by severity: 1 Critical, 30 Low.")
	// The severity totals still describe the whole report, so the model is not
	// left thinking the truncated list is everything.
	require.Contains(t, out, "31 findings in 31 packages")
}

func TestRender_BudgetTooSmallForAnyPackageLine(t *testing.T) {
	items := []map[string]any{}
	for i := 0; i < 10; i++ {
		items = append(items, item(fmt.Sprintf("CVE-%d", i), fmt.Sprintf("package-with-a-long-name-%02d", i), "1.0.0", "1.0.1", "High"))
	}
	items = append(items, item("CVE-U", "unfixable-pkg", "2.0", "", "Low"))

	sum, err := Summarize(report(t, items...))
	require.NoError(t, err)

	head := len("Vulnerability summary (condensed from the artifact's scan report):\n")
	out := sum.Render(head + 40)

	require.LessOrEqual(t, len(out), head+40)
	require.Contains(t, out, "Vulnerability summary")
}

func TestCondense_UnparseablePayloadIsTruncatedNotDropped(t *testing.T) {
	raw := strings.Repeat("not a scan report. ", 100)

	out := Condense(raw, 200)
	require.LessOrEqual(t, len(out), 200)
	require.Contains(t, out, "not a scan report")
	require.Contains(t, out, "truncated for length")

	// Unlimited budget leaves an unparseable payload exactly as it arrived.
	require.Equal(t, raw, Condense(raw, 0))
}

func TestCondense_AppliesTheSummaryWhenParseable(t *testing.T) {
	raw := report(t, item("CVE-1", "zlib1g", "1:1.2.13", "1:1.2.13+deb12u1", "Critical"))

	out := Condense(raw, 0)
	require.Contains(t, out, "zlib1g 1:1.2.13 -> 1:1.2.13+deb12u1 (1 Critical)")
	require.NotContains(t, out, "nvd.nist.gov")
}

func TestTruncate_NeverSplitsARune(t *testing.T) {
	s := strings.Repeat("é", 50)

	for budget := 1; budget < len(s); budget++ {
		out := truncate(s, budget)
		require.LessOrEqual(t, len(out), budget)
		require.True(t, utf8.ValidString(out), "budget %d split a rune: %q", budget, out)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "", 1},
		{"1.0", "1.0", 0},
		{"1.10", "1.9", 1},
		{"1.9", "1.10", -1},
		{"1.2.3", "1.2", 1},
		{"3.0.14-1~deb12u2", "3.0.11-1~deb12u2", 1},
		{"1:1.2.13.dfsg-1+deb12u1", "1:1.2.13.dfsg-1", 1},
		{"1.010", "1.9", 1},
		// dpkg ordering: a tilde sorts before the end of a string, a letter
		// after it — which is exactly why Debian spells pre-releases 1.0~rc1.
		{"3.0.14-1~deb12u2", "3.0.14-1", -1},
		{"2.0~rc1", "2.0", -1},
		{"2.0rc1", "2.0", 1},
	}

	for _, c := range cases {
		require.Equal(t, c.want, compareVersions(c.a, c.b), "compareVersions(%q, %q)", c.a, c.b)
		require.Equal(t, -c.want, compareVersions(c.b, c.a), "compareVersions(%q, %q)", c.b, c.a)
	}
}
