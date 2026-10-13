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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func vuln(id, pkg string, sev Severity, fixed string) Vulnerability {
	return Vulnerability{ID: id, Package: pkg, Severity: sev, FixedVersion: fixed}
}

func TestV(t *testing.T) {
	cases := []struct {
		name string
		w    Weights
		r    ScanResult
		want int
	}{
		{"fixture O′: 3 critical, 3 high, 1 medium, 1 low, 1 unknown", DefaultWeights, loadTrivy(t, "trivy_o.json"), 48},
		{"fixture C", DefaultWeights, loadTrivy(t, "trivy_c.json"), 13},
		{"empty", DefaultWeights, NewScanResult(nil), 0},
		{"unknown severity weighs nothing", DefaultWeights, NewScanResult([]Vulnerability{vuln("X", "p", SeverityUnknown, "")}), 0},
		{"duplicate pair counted once", DefaultWeights, NewScanResult([]Vulnerability{vuln("X", "p", SeverityHigh, ""), vuln("X", "p", SeverityHigh, "")}), 5},
		{"other weights", Weights{Critical: 4, High: 3, Medium: 2, Low: 1}, loadTrivy(t, "trivy_o.json"), 3*4 + 3*3 + 2 + 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, c.w.V(c.r))
		})
	}
}

func TestDeltaV(t *testing.T) {
	cases := []struct {
		name   string
		vO, vC int
		want   float64
	}{
		{"V_O′ = 0: nothing to reduce", 0, 0, 0},
		{"V_O′ = 0 even if C is worse", 0, 7, 0},
		{"identical", 48, 48, 0},
		{"fixtures", 48, 13, 35.0 / 48},
		{"everything fixed", 48, 0, 1},
		{"worse", 10, 15, -0.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.InDelta(t, c.want, DeltaV(c.vO, c.vC), 1e-12)
		})
	}
}

func TestComputeFixRate(t *testing.T) {
	cases := []struct {
		name string
		o, c ScanResult
		want FixRate
		rate float64
	}{
		{
			name: "fixtures: 4 of 5 fixable IDs gone",
			o:    loadTrivy(t, "trivy_o.json"), c: loadTrivy(t, "trivy_c.json"),
			want: FixRate{Fixable: 5, Fixed: 4}, rate: 0.8,
		},
		{
			name: "identical: nothing fixed",
			o:    loadTrivy(t, "trivy_o.json"), c: loadTrivy(t, "trivy_o.json"),
			want: FixRate{Fixable: 5, Fixed: 0}, rate: 0,
		},
		{
			name: "base swap: the CVE moved to another package name, so it is not fixed",
			o:    NewScanResult([]Vulnerability{vuln("CVE-1", "libssl3", SeverityHigh, "3.0.13")}),
			c:    NewScanResult([]Vulnerability{vuln("CVE-1", "libssl3t64", SeverityHigh, "3.0.13")}),
			want: FixRate{Fixable: 1, Fixed: 0}, rate: 0,
		},
		{
			name: "an ID fixed in one package of two still counts once",
			o: NewScanResult([]Vulnerability{
				vuln("CVE-1", "libssl3", SeverityCritical, "3.0.13"),
				vuln("CVE-1", "openssl", SeverityCritical, "3.0.13"),
				vuln("CVE-2", "zlib1g", SeverityHigh, "1.3"),
			}),
			c:    NewScanResult([]Vulnerability{vuln("CVE-1", "openssl", SeverityCritical, "3.0.13")}),
			want: FixRate{Fixable: 2, Fixed: 1}, rate: 0.5,
		},
		{
			name: "unfixable findings do not count",
			o: NewScanResult([]Vulnerability{
				vuln("CVE-1", "a", SeverityHigh, "2"),
				vuln("CVE-2", "b", SeverityCritical, ""),
			}),
			c:    NewScanResult(nil),
			want: FixRate{Fixable: 1, Fixed: 1}, rate: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeFixRate(c.o, c.c)
			require.Equal(t, c.want, got)
			require.False(t, got.NothingToFix())
			require.InDelta(t, c.rate, got.Rate(), 1e-12)
		})
	}
}

func TestComputeFixRate_NothingToFix(t *testing.T) {
	cases := map[string]ScanResult{
		"only unfixable findings": NewScanResult([]Vulnerability{vuln("CVE-2", "b", SeverityCritical, "")}),
		"empty O′":                loadTrivy(t, "trivy_empty.json"),
	}
	for name, o := range cases {
		t.Run(name, func(t *testing.T) {
			got := ComputeFixRate(o, NewScanResult(nil))
			require.True(t, got.NothingToFix())
			require.Equal(t, FixRate{}, got)
			require.Zero(t, got.Rate())
		})
	}
}

func TestFixRate_JSON(t *testing.T) {
	measured, err := json.Marshal(FixRate{Fixable: 5, Fixed: 4})
	require.NoError(t, err)
	require.JSONEq(t, `{"fixable": 5, "fixed": 4, "nothing_to_fix": false, "rate": 0.8}`, string(measured))

	nothing, err := json.Marshal(FixRate{})
	require.NoError(t, err)
	require.JSONEq(t, `{"fixable": 0, "fixed": 0, "nothing_to_fix": true}`, string(nothing))

	// Records read back into a FixRate unchanged.
	var back FixRate
	require.NoError(t, json.Unmarshal(measured, &back))
	require.Equal(t, FixRate{Fixable: 5, Fixed: 4}, back)
}

func TestDeltaS(t *testing.T) {
	cases := []struct {
		name   string
		sO, sC int64
		want   float64
	}{
		{"identical", 1000, 1000, 0},
		{"smaller", 1000, 600, 0.4},
		{"larger", 1000, 1100, -0.1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DeltaS(c.sO, c.sC)
			require.NoError(t, err)
			require.InDelta(t, c.want, got, 1e-12)
		})
	}

	for _, sizes := range [][2]int64{{0, 100}, {100, 0}, {-1, 100}} {
		_, err := DeltaS(sizes[0], sizes[1])
		require.Error(t, err, "%v", sizes)
	}
}
