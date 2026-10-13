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

func TestWilson(t *testing.T) {
	cases := []struct {
		name      string
		successes float64
		n         int
		lo, hi    float64
	}{
		// Published reference values for the 95% Wilson interval.
		{"5 of 10", 5, 10, 0.2366, 0.7634},
		{"0 of 10", 0, 10, 0, 0.2775},
		{"10 of 10", 10, 10, 0.7225, 1},
		{"1 of 1", 1, 1, 0.2065, 1},
		// Fractional successes: per-image averages over repetitions.
		{"7.5 of 12", 7.5, 12, 0.3544, 0.8350},
		{"no data", 0, 0, 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lo, hi := Wilson(c.successes, c.n)
			require.InDelta(t, c.lo, lo, 5e-5)
			require.InDelta(t, c.hi, hi, 5e-5)
		})
	}
}

func TestSummarize(t *testing.T) {
	cases := []struct {
		name string
		xs   []float64
		want Summary
	}{
		// Expected values follow NumPy's default (linear) quantile method.
		{"even count", []float64{4, 1, 3, 2}, Summary{N: 4, Median: 2.5, Q1: 1.75, Q3: 3.25}},
		{"odd count", []float64{3, 1, 2}, Summary{N: 3, Median: 2, Q1: 1.5, Q3: 2.5}},
		{"quartiles on order statistics", []float64{0.1, 0.9, 0.4, 0.2, 0.7}, Summary{N: 5, Median: 0.4, Q1: 0.2, Q3: 0.7}},
		{"single value", []float64{5}, Summary{N: 1, Median: 5, Q1: 5, Q3: 5}},
		{"negative values", []float64{-0.5, 0, 0.5}, Summary{N: 3, Median: 0, Q1: -0.25, Q3: 0.25}},
		{"empty", nil, Summary{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Summarize(c.xs)
			require.Equal(t, c.want.N, got.N)
			require.InDelta(t, c.want.Median, got.Median, 1e-12)
			require.InDelta(t, c.want.Q1, got.Q1, 1e-12)
			require.InDelta(t, c.want.Q3, got.Q3, 1e-12)
		})
	}

	xs := []float64{3, 1, 2}
	Summarize(xs)
	require.Equal(t, []float64{3, 1, 2}, xs, "the input is not reordered")
	require.InDelta(t, 1.5, Summarize([]float64{4, 1, 3, 2}).IQR(), 1e-12)
}

func TestSummarizeFixRates(t *testing.T) {
	got := SummarizeFixRates([]FixRate{
		{Fixable: 5, Fixed: 4}, // 0.8
		{},                     // nothing to fix
		{Fixable: 2, Fixed: 1}, // 0.5
		{Fixable: 4, Fixed: 0}, // 0
		{},                     // nothing to fix
	})
	require.Equal(t, 2, got.NothingToFix)
	require.Equal(t, 3, got.N, "nothing-to-fix attempts are not in the median")
	require.InDelta(t, 0.5, got.Median, 1e-12)
	require.InDelta(t, 0.25, got.Q1, 1e-12)
	require.InDelta(t, 0.65, got.Q3, 1e-12)

	none := SummarizeFixRates([]FixRate{{}, {}})
	require.Equal(t, FixRateSummary{NothingToFix: 2}, none)
	require.Equal(t, FixRateSummary{}, SummarizeFixRates(nil))
}
