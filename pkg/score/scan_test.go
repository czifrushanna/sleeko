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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/scanreport"
)

func loadTrivy(t *testing.T, name string) ScanResult {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	r, err := ParseTrivy(data)
	require.NoError(t, err)
	return r
}

func TestParseTrivy(t *testing.T) {
	r := loadTrivy(t, "trivy_o.json")

	// setuptools appears in two site-packages targets but is one pair;
	// CVE-2024-5535 in libssl3 and in openssl is two.
	require.Len(t, r.Vulnerabilities, 9)
	require.Equal(t, map[Severity]int{
		SeverityCritical: 3, SeverityHigh: 3, SeverityMedium: 1, SeverityLow: 1, SeverityUnknown: 1,
	}, r.Counts())
	require.Equal(t, Vulnerability{
		ID: "CVE-2024-6345", Package: "setuptools", InstalledVersion: "65.5.1", FixedVersion: "70.0.0", Severity: SeverityHigh,
	}, r.Vulnerabilities[7])

	// Sorted by ID, then package.
	require.Equal(t, "CVE-2011-3374", r.Vulnerabilities[0].ID)
	require.Equal(t, "libssl3", r.Vulnerabilities[5].Package)
	require.Equal(t, "openssl", r.Vulnerabilities[6].Package)
}

func TestParseTrivy_EmptyResultIsValid(t *testing.T) {
	r := loadTrivy(t, "trivy_empty.json")
	require.Empty(t, r.Vulnerabilities)
	require.Equal(t, 0, DefaultWeights.V(r))

	r, err := ParseTrivy([]byte(`{"SchemaVersion": 2, "ArtifactName": "scratch-image"}`))
	require.NoError(t, err)
	require.Empty(t, r.Vulnerabilities)
}

func TestParseTrivy_Rejects(t *testing.T) {
	for name, doc := range map[string]string{
		"not JSON":            `nope`,
		"not a trivy report":  `{"vulnerabilities": []}`,
		"wrong shape of data": `{"SchemaVersion": 2, "Results": {"x": 1}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseTrivy([]byte(doc))
			require.Error(t, err)
		})
	}
}

func TestNewScanResult_Duplicates(t *testing.T) {
	r := NewScanResult([]Vulnerability{
		{ID: "CVE-1", Package: "p", Severity: SeverityMedium},
		{ID: "CVE-1", Package: "p", Severity: SeverityHigh, FixedVersion: "2.0"},
		{ID: "CVE-1", Package: "p", Severity: SeverityLow, FixedVersion: "3.0"},
	})
	require.Equal(t, []Vulnerability{{ID: "CVE-1", Package: "p", Severity: SeverityHigh, FixedVersion: "2.0"}}, r.Vulnerabilities)
}

func TestParseSeverity(t *testing.T) {
	cases := map[string]Severity{
		"CRITICAL": SeverityCritical, "Critical": SeverityCritical, "high": SeverityHigh,
		" Medium ": SeverityMedium, "LOW": SeverityLow,
		"UNKNOWN": SeverityUnknown, "Negligible": SeverityUnknown, "None": SeverityUnknown, "": SeverityUnknown,
	}
	for in, want := range cases {
		require.Equal(t, want, ParseSeverity(in), in)
	}
}

func TestParseHarbor(t *testing.T) {
	r, err := ParseHarbor([]byte(`{"vulnerabilities": [
		{"id": "CVE-2024-5535", "package": "libssl3", "version": "3.0.11", "fix_version": "3.0.13", "severity": "Critical", "description": "ignored"},
		{"id": "CVE-2024-5535", "package": "libssl3", "version": "3.0.11", "fix_version": "3.0.13", "severity": "Critical"},
		{"id": "CVE-2011-3374", "package": "apt", "version": "2.6.1", "severity": "Negligible"}
	]}`))
	require.NoError(t, err)
	require.Equal(t, []Vulnerability{
		{ID: "CVE-2011-3374", Package: "apt", InstalledVersion: "2.6.1", Severity: SeverityUnknown},
		{ID: "CVE-2024-5535", Package: "libssl3", InstalledVersion: "3.0.11", FixedVersion: "3.0.13", Severity: SeverityCritical},
	}, r.Vulnerabilities)

	_, err = ParseHarbor([]byte(`{"Results": []}`))
	require.Error(t, err)
	_, err = ParseHarbor([]byte(`[`))
	require.Error(t, err)

	r, err = ParseHarbor([]byte(`{"vulnerabilities": []}`))
	require.NoError(t, err)
	require.Empty(t, r.Vulnerabilities)
}

// TestHarborReport_RoundTripsAndFeedsTheAdapter checks the Trivy → Harbor
// conversion: decoding the converted report gives the same result, and the
// adapter's own condenser reads it the way it reads a real Harbor report.
func TestHarborReport_RoundTripsAndFeedsTheAdapter(t *testing.T) {
	r := loadTrivy(t, "trivy_o.json")

	report, err := r.HarborReport()
	require.NoError(t, err)

	back, err := ParseHarbor(report)
	require.NoError(t, err)
	require.Equal(t, r, back)

	summary, err := scanreport.Summarize(string(report))
	require.NoError(t, err)
	require.Equal(t, 9, summary.Findings)

	condensed := scanreport.Condense(string(report), 0)
	require.Contains(t, condensed, "9 findings in 9 packages; 6 in 6 packages have a fix available")
	require.Contains(t, condensed, "libssl3 3.0.11-1~deb12u2 -> 3.0.13-1~deb12u1 (1 Critical)")

	empty, err := NewScanResult(nil).HarborReport()
	require.NoError(t, err)
	require.JSONEq(t, `{"vulnerabilities": []}`, string(empty))
}
