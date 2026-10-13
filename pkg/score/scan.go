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

// Package score implements the evaluation metric: the scan results it is
// computed from, the six gates, the components ΔV, fix rate and ΔS, the
// success rule, and the aggregates the report needs. It does no I/O; builds,
// scans and smoke tests happen elsewhere and are passed in as results.
//
// O is the original Dockerfile, O′ is O rebuilt unchanged, and C is the
// optimized candidate. O′ and C are built from the same source with the same
// build parameters and scanned with the same Trivy version and database, so
// differences between them come from the Dockerfile alone.
package score

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Severity is a vulnerability severity, ordered so that a larger value is
// more severe.
type Severity int

const (
	// SeverityUnknown covers anything that is not one of the four weighted
	// severities: Trivy's UNKNOWN, Harbor's Negligible, Unknown and None.
	SeverityUnknown Severity = iota
	SeverityLow
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

var severityNames = map[Severity]string{
	SeverityUnknown:  "Unknown",
	SeverityLow:      "Low",
	SeverityMedium:   "Medium",
	SeverityHigh:     "High",
	SeverityCritical: "Critical",
}

// String returns the severity as Harbor spells it.
func (s Severity) String() string {
	return severityNames[s]
}

// ParseSeverity reads a severity in either scanner's spelling, ignoring case.
func ParseSeverity(s string) Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "medium":
		return SeverityMedium
	case "low":
		return SeverityLow
	default:
		return SeverityUnknown
	}
}

// Vulnerability is one finding: a CVE (or other advisory ID) in one package.
type Vulnerability struct {
	ID               string
	Package          string
	InstalledVersion string
	// FixedVersion is empty when the scanner knows no fix.
	FixedVersion string
	Severity     Severity
}

// Fixable reports whether the scanner knows a fixed version.
func (v Vulnerability) Fixable() bool {
	return v.FixedVersion != ""
}

// ScanResult is a normalized scan: one entry per unique (ID, package) pair,
// sorted by ID then package. Build it with NewScanResult or a decoder, so
// duplicates are merged.
type ScanResult struct {
	Vulnerabilities []Vulnerability
}

type pairKey struct{ id, pkg string }

// NewScanResult normalizes findings into a ScanResult. A pair reported more
// than once — the same package found in several layers or scan targets — is
// counted once, keeping its highest severity and any fixed version reported.
func NewScanResult(vulns []Vulnerability) ScanResult {
	byPair := map[pairKey]Vulnerability{}
	for _, v := range vulns {
		k := pairKey{v.ID, v.Package}
		prev, seen := byPair[k]
		if !seen {
			byPair[k] = v
			continue
		}
		if v.Severity > prev.Severity {
			prev.Severity = v.Severity
		}
		if prev.FixedVersion == "" {
			prev.FixedVersion = v.FixedVersion
		}
		byPair[k] = prev
	}

	out := make([]Vulnerability, 0, len(byPair))
	for _, v := range byPair {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Package < out[j].Package
	})
	return ScanResult{Vulnerabilities: out}
}

// trivyReport is the part of `trivy image --format json` the metric reads.
type trivyReport struct {
	Results *[]struct {
		Vulnerabilities []struct {
			VulnerabilityID  string `json:"VulnerabilityID"`
			PkgName          string `json:"PkgName"`
			InstalledVersion string `json:"InstalledVersion"`
			FixedVersion     string `json:"FixedVersion"`
			Severity         string `json:"Severity"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
	SchemaVersion int `json:"SchemaVersion"`
}

// ParseTrivy decodes a Trivy JSON report. An image with no findings is a
// valid, empty result; a document that is not a Trivy report is an error.
func ParseTrivy(data []byte) (ScanResult, error) {
	var r trivyReport
	if err := json.Unmarshal(data, &r); err != nil {
		return ScanResult{}, fmt.Errorf("decode trivy report: %w", err)
	}
	if r.SchemaVersion == 0 {
		return ScanResult{}, errors.New("decode trivy report: no SchemaVersion, not a trivy JSON report")
	}

	var vulns []Vulnerability
	if r.Results != nil {
		for _, res := range *r.Results {
			for _, v := range res.Vulnerabilities {
				vulns = append(vulns, Vulnerability{
					ID:               v.VulnerabilityID,
					Package:          v.PkgName,
					InstalledVersion: v.InstalledVersion,
					FixedVersion:     v.FixedVersion,
					Severity:         ParseSeverity(v.Severity),
				})
			}
		}
	}
	return NewScanResult(vulns), nil
}

// harborItem is one finding in the Harbor report shape: the fields
// pkg/scanreport reads from what Harbor sends the adapter.
type harborItem struct {
	ID         string `json:"id"`
	Package    string `json:"package"`
	Version    string `json:"version"`
	FixVersion string `json:"fix_version,omitempty"`
	Severity   string `json:"severity"`
}

type harborReport struct {
	Vulnerabilities *[]harborItem `json:"vulnerabilities"`
}

// ParseHarbor decodes a Harbor vulnerability report, the scan report the
// adapter receives with an optimize request.
func ParseHarbor(data []byte) (ScanResult, error) {
	var r harborReport
	if err := json.Unmarshal(data, &r); err != nil {
		return ScanResult{}, fmt.Errorf("decode harbor report: %w", err)
	}
	if r.Vulnerabilities == nil {
		return ScanResult{}, errors.New("decode harbor report: no vulnerabilities field, not a harbor report")
	}

	vulns := make([]Vulnerability, 0, len(*r.Vulnerabilities))
	for _, v := range *r.Vulnerabilities {
		vulns = append(vulns, Vulnerability{
			ID:               v.ID,
			Package:          v.Package,
			InstalledVersion: v.Version,
			FixedVersion:     v.FixVersion,
			Severity:         ParseSeverity(v.Severity),
		})
	}
	return NewScanResult(vulns), nil
}

// HarborReport renders the result in the Harbor report shape, so the
// evaluation harness can hand the pipeline what Harbor would send for the
// same image. Only the fields the adapter reads are included.
func (r ScanResult) HarborReport() ([]byte, error) {
	items := make([]harborItem, 0, len(r.Vulnerabilities))
	for _, v := range r.Vulnerabilities {
		items = append(items, harborItem{
			ID:         v.ID,
			Package:    v.Package,
			Version:    v.InstalledVersion,
			FixVersion: v.FixedVersion,
			Severity:   v.Severity.String(),
		})
	}
	return json.Marshal(harborReport{Vulnerabilities: &items})
}

// Counts tallies the unique pairs per severity.
func (r ScanResult) Counts() map[Severity]int {
	c := map[Severity]int{}
	for _, v := range r.Vulnerabilities {
		c[v.Severity]++
	}
	return c
}

// ids returns the set of IDs, optionally only those with a severity at or
// above min.
func (r ScanResult) ids(min Severity) map[string]bool {
	out := map[string]bool{}
	for _, v := range r.Vulnerabilities {
		if v.Severity >= min {
			out[v.ID] = true
		}
	}
	return out
}
