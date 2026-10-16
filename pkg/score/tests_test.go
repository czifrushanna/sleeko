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

// Reference p-values: SciPy 1.x binomtest for McNemar.
func TestMcNemar(t *testing.T) {
	cases := []struct {
		b, c int
		p    float64
	}{
		{0, 0, 1}, {1, 0, 1}, {5, 1, 0.21875}, {3, 3, 1}, {10, 2, 0.03857421875},
		{0, 7, 0.015625}, {12, 25, 0.04703102743951604}, {40, 60, 0.05688793364098089},
	}
	for _, c := range cases {
		require.InDelta(t, c.p, McNemar(c.b, c.c), 1e-12, "b=%d c=%d", c.b, c.c)
		require.InDelta(t, c.p, McNemar(c.c, c.b), 1e-12, "symmetric")
	}
}

// Reference p-values: SciPy's exact Wilcoxon where there are no ties, and a
// brute-force enumeration of all 2^n sign assignments where there are (SciPy's
// "exact" method ignores ties, so it is not the reference there).
func TestWilcoxon(t *testing.T) {
	cases := []struct {
		name  string
		diffs []float64
		n     int
		wPlus float64
		p     float64
	}{
		{"no ties", []float64{0.1, 0.25, -0.05, 0.3, 0.4, 0.12, -0.2, 0.5, 0.33, 0.07}, 10, 49, 0.02734375},
		{"all positive", []float64{1, 2, 3, 4, 5, 6, 7, 8}, 8, 36, 0.0078125},
		{"ties and zeros", []float64{1, -1, 2, -2, 3, 3, -3, 4, 0, 0, 5}, 9, 34, 0.20703125},
		{"many ties", []float64{0.5, 0.5, 0.5, -0.5, 0.2, 0.2, -0.1}, 7, 21.5, 0.25},
		{"all negative", []float64{-3, -2, -1}, 3, 0, 0.25},
		{"n = 30, no ties", []float64{2, -1, 3, -4, 5, 6, -7, 8, 9, 10, 11, -12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, -29, 30}, 30, 412, 7.910653948783875e-05},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Wilcoxon(c.diffs)
			require.Equal(t, c.n, got.N)
			require.InDelta(t, c.wPlus, got.WPlus, 1e-9)
			require.InDelta(t, c.p, got.P, 1e-9)
		})
	}
	require.Equal(t, WilcoxonResult{P: 1}, Wilcoxon([]float64{0, 0}), "no non-zero differences")
	require.Equal(t, WilcoxonResult{P: 1}, Wilcoxon(nil))
}

func TestWilcoxon_TiesUpToRounding(t *testing.T) {
	// 0.25-0.20 and 0.45-0.50 differ in the last bits as computed; they are
	// the same magnitude and must share a rank.
	got := Wilcoxon([]float64{0.25 - 0.20, 0.45 - 0.50, 0.2})
	require.InDelta(t, 1.5+3, got.WPlus, 1e-12)
}
