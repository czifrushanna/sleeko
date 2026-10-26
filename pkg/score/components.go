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
	"fmt"
)

// Weights are the per-severity weights of the vulnerability score V.
type Weights struct {
	Critical int
	High     int
	Medium   int
	Low      int
}

// DefaultWeights are the schedule's weights, V = 10·critical + 5·high +
// 2·medium + 1·low. The schedule marks them as still to be agreed: change
// them here.
var DefaultWeights = Weights{Critical: 10, High: 5, Medium: 2, Low: 1}

// V is the weighted vulnerability score of a scan. Findings of unknown
// severity weigh nothing.
func (w Weights) V(r ScanResult) int {
	c := r.Counts()
	return w.Critical*c[SeverityCritical] + w.High*c[SeverityHigh] + w.Medium*c[SeverityMedium] + w.Low*c[SeverityLow]
}

// DeltaV is the relative vulnerability reduction from O′ to C,
// (V_O′ − V_C) / V_O′. Negative when C is worse.
//
// When O′ has no findings the ratio is undefined. It is 0 when C has none
// either, and -1 when C has any: a candidate that adds vulnerabilities to a
// clean image must count as worse, so the success rule (ΔV ≥ 0) rejects it,
// however much smaller the image becomes. Vulnerabilities come before size.
func DeltaV(vO, vC int) float64 {
	if vO == 0 {
		if vC > 0 {
			return -1
		}
		return 0
	}
	return float64(vO-vC) / float64(vO)
}

// FixRate is the outcome of comparing O′'s fixable CVE IDs — those with a
// fixed version for at least one package — with C. It is counted per ID, not
// per (ID, package) pair, because a base image swap can rename the package
// that carries the same CVE.
type FixRate struct {
	// Fixable is the number of fixable CVE IDs in O′.
	Fixable int `json:"fixable"`
	// Fixed is how many of them no longer appear in C under any package.
	Fixed int `json:"fixed"`
}

// NothingToFix reports that O′ had no fixable CVE. Such an attempt has no fix
// rate: it is neither 0 (nothing was missed) nor 1 (nothing was done), so it
// is kept out of the fix-rate median and counted on its own.
func (f FixRate) NothingToFix() bool {
	return f.Fixable == 0
}

// Rate is Fixed / Fixable, the share of fixable CVE IDs C got rid of. It is 0
// when NothingToFix; check that first.
func (f FixRate) Rate() float64 {
	if f.NothingToFix() {
		return 0
	}
	return float64(f.Fixed) / float64(f.Fixable)
}

// MarshalJSON records the outcome explicitly: "nothing_to_fix" is always
// present, and "rate" only when there was something to fix, so a record never
// shows a made-up 0 or 1.
func (f FixRate) MarshalJSON() ([]byte, error) {
	type plain FixRate
	out := struct {
		plain
		NothingToFix bool     `json:"nothing_to_fix"`
		Rate         *float64 `json:"rate,omitempty"`
	}{plain: plain(f), NothingToFix: f.NothingToFix()}
	if !f.NothingToFix() {
		r := f.Rate()
		out.Rate = &r
	}
	return json.Marshal(out)
}

// ComputeFixRate compares O′'s fixable CVE IDs with C.
func ComputeFixRate(o, c ScanResult) FixRate {
	fixable := map[string]bool{}
	for _, v := range o.Vulnerabilities {
		if v.Fixable() {
			fixable[v.ID] = true
		}
	}

	inC := c.ids(SeverityUnknown)
	fixed := 0
	for id := range fixable {
		if !inC[id] {
			fixed++
		}
	}
	return FixRate{Fixable: len(fixable), Fixed: fixed}
}

// DeltaS is the relative size reduction from O′ to C, (S_O′ − S_C) / S_O′,
// on compressed image sizes in bytes. Negative when C is larger.
func DeltaS(sO, sC int64) (float64, error) {
	if sO <= 0 || sC <= 0 {
		return 0, fmt.Errorf("image sizes must be positive, got O′=%d C=%d", sO, sC)
	}
	return float64(sO-sC) / float64(sO), nil
}
