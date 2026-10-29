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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/score"
)

func vuln(id, pkg, version string, sev score.Severity) score.Vulnerability {
	return score.Vulnerability{ID: id, Package: pkg, InstalledVersion: version, Severity: sev}
}

func scanOf(os string, eosl bool, vs ...score.Vulnerability) ScanOutput {
	return ScanOutput{Result: score.ScanResult{Vulnerabilities: vs}, OS: os, EOSL: &eosl}
}

func TestNewSevere(t *testing.T) {
	o := scanOf("debian 9.13", true,
		vuln("CVE-OLD", "openssl", "1.1.0l-1~deb9u1", score.SeverityHigh),
		vuln("CVE-LOW", "zlib", "1:1.2.8.dfsg-5", score.SeverityLow),
		vuln("CVE-PY", "urllib3", "1.26.5", score.SeverityMedium))
	c := scanOf("debian 12.8", false,
		// Already in O′ (any severity): not new, whatever the package.
		vuln("CVE-OLD", "libssl3", "3.0.15-1~deb12u1", score.SeverityCritical),
		vuln("CVE-LOW", "zlib1g", "1:1.2.13.dfsg-1", score.SeverityHigh),
		// Below high: G6 does not count it.
		vuln("CVE-MED", "curl", "7.88.1-10+deb12u8", score.SeverityMedium),
		// Same package, same version: only the coverage changed.
		vuln("CVE-SAME", "zlib", "1:1.2.8.dfsg-5", score.SeverityHigh),
		// Same package, newer in C than in O′: O′'s older one was affected too.
		vuln("CVE-OLDER", "urllib3", "1.26.18", score.SeverityHigh),
		// Not in O′'s scan, O′ end of life and C on another release.
		vuln("CVE-HIDDEN", "libxml2", "2.9.14+dfsg-1.3~deb12u1", score.SeverityHigh),
		// One ID in two packages: the least visibility-like class wins.
		vuln("CVE-TWO", "zlib", "1:1.2.8.dfsg-5", score.SeverityHigh),
		vuln("CVE-TWO", "urllib3", "1.26.0", score.SeverityHigh))
	require.Equal(t, map[string]VisibilityClass{
		"CVE-SAME":   Certain,
		"CVE-OLDER":  Probable,
		"CVE-HIDDEN": Undetermined,
		"CVE-TWO":    New, // urllib3 1.26.0 is older than O′'s 1.26.5
	}, newSevere(o, c, nil))

	// The same package missing from a supported O′'s scan: probably new.
	supported := scanOf("debian 12.8", false, vuln("CVE-X", "zlib", "1:1.2.13.dfsg-1", score.SeverityLow))
	require.Equal(t, map[string]VisibilityClass{"CVE-HIDDEN": LikelyNew},
		newSevere(supported, scanOf("debian 12.8", false, vuln("CVE-HIDDEN", "libxml2", "2.9.14", score.SeverityHigh)), nil))
	// End of life, but C stayed on the same release: nothing changed in
	// coverage, so a package O′'s scan lacks is probably new.
	require.Equal(t, map[string]VisibilityClass{"CVE-HIDDEN": LikelyNew},
		newSevere(o, scanOf("debian 9.13", true, vuln("CVE-HIDDEN", "libxml2", "2.9.4", score.SeverityHigh)), nil))
}

func TestVisibility(t *testing.T) {
	dir := t.TempDir()
	save := func(digest string, s ScanOutput) {
		data, err := json.Marshal(s)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "sha256-"+digest+".json"), data, 0o644))
	}
	eosl := true
	o := scanOf("debian 9.13", true, vuln("CVE-A", "zlib", "1.2.8", score.SeverityLow))
	save("o", o)
	save("visible", scanOf("debian 12.8", false, vuln("CVE-B", "zlib", "1.2.8", score.SeverityHigh)))
	save("new", scanOf("debian 9.13", true, vuln("CVE-C", "curl", "7.52", score.SeverityHigh)))

	rec := func(item, digest string, success bool, g6 score.Status) Record {
		return Record{
			Item: item, Pipeline: "W0", Repetition: 1, Status: StatusMeasured, Success: success,
			Original:   &ImageInfo{Digest: "sha256:o", EOSL: &eosl},
			Candidate:  &ImageInfo{Digest: "sha256:" + digest},
			Components: &Components{DeltaV: 0.5, DeltaS: 0},
			Gates: []score.Gate{
				{ID: score.G1Parse, Status: score.Pass}, {ID: score.G2NoInvention, Status: score.Pass},
				{ID: score.G3Versions, Status: score.Pass}, {ID: score.G4Build, Status: score.Pass},
				{ID: score.G5Behaviour, Status: score.Pass}, {ID: score.G6NoNewSevere, Status: g6},
			},
		}
	}
	records := []Record{
		rec("a", "visible", false, score.Fail),
		rec("b", "new", false, score.Fail),
		rec("c", "visible", true, score.Pass),
	}
	va, err := Visibility(records, []string{"W0"}, map[string]string{"W0": dir}, nil, 0.1)
	require.NoError(t, err)
	r := va.Results["W0"]
	require.Equal(t, 3, r.Attempts)
	require.Equal(t, 2, r.G6Failures)
	require.Equal(t, map[VisibilityClass]int{Certain: 1, LikelyNew: 1}, r.IDs)
	require.Equal(t, 1, r.ExplainedLow)
	require.Equal(t, 1, r.ExplainedHigh)
	require.Equal(t, 0, va.Missing)
	require.Equal(t, 1, r.Rules[VisibilityRules[0]].Solved)
	require.Equal(t, 2, r.Rules[VisibilityRules[1]].Solved) // a passes once its visible-only ID is ignored
	require.Equal(t, 3, r.Rules[VisibilityRules[3]].Solved)
}

func TestNewSevereWithInventory(t *testing.T) {
	// An end-of-life O′ whose scan reports almost nothing.
	o := scanOf("debian 9.13", true, vuln("CVE-OLD", "zlib1g", "1:1.2.8.dfsg-5", score.SeverityLow))
	c := scanOf("debian 12.8", false,
		vuln("CVE-SAME", "libbz2-1.0", "1.0.8-5+b1", score.SeverityHigh),
		vuln("CVE-OLDER", "libc6", "2.36-9+deb12u9", score.SeverityHigh),
		vuln("CVE-ADDED", "curl", "7.88.1-10+deb12u8", score.SeverityHigh),
		vuln("CVE-DOWN", "perl-base", "5.24.1-3", score.SeverityHigh))
	inv := &Inventory{Packages: []InstalledPackage{
		{Name: "libbz2-1.0", Version: "1.0.8-5+b1"},
		{Name: "libc6", Version: "2.24-11+deb9u4"},
		{Name: "perl-base", Version: "5.36.0-7"},
	}}
	// Without the inventory, the packages O′'s scan lacks are undetermined.
	require.Equal(t, map[string]VisibilityClass{
		"CVE-SAME": Undetermined, "CVE-OLDER": Undetermined, "CVE-ADDED": Undetermined, "CVE-DOWN": Undetermined,
	}, newSevere(o, c, nil))
	// With it, they are present (same or older version), absent, or newer.
	require.Equal(t, map[string]VisibilityClass{
		"CVE-SAME": Certain, "CVE-OLDER": Probable, "CVE-ADDED": Absent, "CVE-DOWN": New,
	}, newSevere(o, c, inv))
}

func TestTrivyPackages(t *testing.T) {
	report := []byte(`{"SchemaVersion": 2, "Results": [
	  {"Target": "x (debian 12.8)", "Class": "os-pkgs", "Packages": [
	    {"Name": "zlib1g", "Version": "1.2.13.dfsg", "Release": "1", "Epoch": 1},
	    {"Name": "libc6", "Version": "2.36", "Release": "9+deb12u9"}]},
	  {"Target": "Python", "Class": "lang-pkgs", "Packages": [
	    {"Name": "urllib3", "Version": "2.2.3"}]}]}`)
	pkgs, err := trivyPackages(report)
	require.NoError(t, err)
	require.Equal(t, []InstalledPackage{
		{Name: "zlib1g", Version: "1:1.2.13.dfsg-1"},
		{Name: "libc6", Version: "2.36-9+deb12u9"},
		{Name: "urllib3", Version: "2.2.3"},
	}, pkgs)
}
