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

// DefaultSizeTolerance is the size tolerance of the success rule. The
// schedule proposes 5% and marks it as still to be agreed; 10% is the value
// agreed for now.
const DefaultSizeTolerance = 0.10

// GatesPassed reports whether an attempt's gates allow it to succeed: every
// gate is present and passed, except G3, which is NotChecked when the
// harness runs without version lookups (records from before G3 was checked).
// Any other NotChecked gate means the attempt was not fully measured, so it
// cannot count as a success.
func GatesPassed(gates []Gate) bool {
	byID := map[GateID]Gate{}
	for _, g := range gates {
		byID[g.ID] = g
	}
	for _, id := range AllGates {
		g, ok := byID[id]
		switch {
		case !ok, g.Status == Fail:
			return false
		case g.Status == NotChecked && id != G3Versions:
			return false
		}
	}
	return true
}

// Success is the schedule's success rule with size tolerance tol:
//
//	all gates passed and ΔV ≥ 0 and ΔS ≥ −tol and (ΔV > 0 or ΔS > tol)
//
// The candidate is valid, worse on neither axis (its image may grow by at
// most tol), and better on at least one: fewer weighted vulnerabilities, or
// an image smaller by more than tol. The schedule uses the one tolerance for
// both size bounds.
func Success(gates []Gate, deltaV, deltaS, tol float64) bool {
	if !GatesPassed(gates) {
		return false
	}
	if deltaV < 0 || deltaS < -tol {
		return false
	}
	return deltaV > 0 || deltaS > tol
}
