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
	"testing"

	"github.com/stretchr/testify/require"
)

// gatesWith returns all six gates passed, with the given overrides.
func gatesWith(overrides ...Gate) []Gate {
	gates := make([]Gate, 0, len(AllGates))
	for _, id := range AllGates {
		g := Gate{ID: id, Status: Pass}
		for _, o := range overrides {
			if o.ID == id {
				g = o
			}
		}
		gates = append(gates, g)
	}
	return gates
}

func TestGatesPassed(t *testing.T) {
	cases := []struct {
		name  string
		gates []Gate
		want  bool
	}{
		{"all passed", gatesWith(), true},
		{"G3 not checked yet is allowed", gatesWith(Gate{ID: G3Versions, Status: NotChecked}), true},
		{"any failure", gatesWith(Gate{ID: G6NoNewSevere, Status: Fail}), false},
		{"a failed G3", gatesWith(Gate{ID: G3Versions, Status: Fail}), false},
		{"another gate not checked means not fully measured", gatesWith(Gate{ID: G5Behaviour, Status: NotChecked}), false},
		{"a missing gate", gatesWith()[1:], false},
		{"no gates", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, GatesPassed(c.gates))
		})
	}
}

func TestSuccess(t *testing.T) {
	const tol = DefaultSizeTolerance
	cases := []struct {
		name   string
		gates  []Gate
		dV, dS float64
		want   bool
	}{
		{"fewer vulnerabilities, same size", gatesWith(), 0.3, 0, true},
		{"fewer vulnerabilities, a bit larger", gatesWith(), 0.3, -0.05, true},
		{"identical O′ and C: no improvement", gatesWith(), 0, 0, false},
		{"V_O′ = 0 and a much smaller image", gatesWith(), 0, 0.4, true},
		{"V_O′ = 0 and a slightly smaller image", gatesWith(), 0, 0.05, false},
		{"more vulnerabilities, even if much smaller", gatesWith(), -0.01, 0.5, false},
		{"size grows exactly by the tolerance", gatesWith(), 0.2, -tol, true},
		{"size grows beyond the tolerance", gatesWith(), 0.2, -tol - 0.001, false},
		{"size shrinks exactly by the tolerance is not enough on its own", gatesWith(), 0, tol, false},
		{"size shrinks just beyond the tolerance", gatesWith(), 0, tol + 0.001, true},
		{"a failed gate overrides everything", gatesWith(Gate{ID: G2NoInvention, Status: Fail}), 0.9, 0.9, false},
		{"G3 not checked still succeeds", gatesWith(Gate{ID: G3Versions, Status: NotChecked}), 0.3, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, Success(c.gates, c.dV, c.dS, tol))
		})
	}
}

// TestSuccess_ToleranceBoundaryFromSizes checks the boundary with ΔS computed
// from byte counts. No epsilon is needed: ΔS is one correctly rounded
// division of integers, so a change of exactly the tolerance rounds to the
// same float64 as the tolerance itself.
func TestSuccess_ToleranceBoundaryFromSizes(t *testing.T) {
	for _, sizes := range [][2]int64{{1000, 1100}, {30, 33}, {123456780, 135802458}} {
		dS, err := DeltaS(sizes[0], sizes[1])
		require.NoError(t, err)
		require.True(t, Success(gatesWith(), 0.1, dS, DefaultSizeTolerance), "sizes %v, ΔS %v", sizes, dS)
	}
}

// TestSuccess_FixtureEndToEnd scores the fixture pair the way the harness
// will: gates, then components, then the rule.
func TestSuccess_FixtureEndToEnd(t *testing.T) {
	o, c := loadTrivy(t, "trivy_o.json"), loadTrivy(t, "trivy_c.json")

	dV := DeltaV(DefaultWeights.V(o), DefaultWeights.V(c))
	dS, err := DeltaS(120_000_000, 128_000_000)
	require.NoError(t, err)

	gates := gatesWith(CheckNoNewSevere(o, c), CheckVersions(false, nil, nil))
	require.True(t, Success(gates, dV, dS, DefaultSizeTolerance))

	require.Equal(t, FixRate{Fixable: 5, Fixed: 4}, ComputeFixRate(o, c))
}
