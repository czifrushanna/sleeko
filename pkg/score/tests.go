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
	"math"
	"sort"
)

// McNemar is the exact two-sided McNemar test for paired success/failure:
// b counts pairs where only the first pipeline succeeded, c where only the
// second did. With few discordant pairs the exact binomial test is the
// right one; it returns 1 when there are none.
func McNemar(b, c int) float64 {
	n := b + c
	if n == 0 {
		return 1
	}
	k := min(b, c)
	// P(X <= k) for X ~ Binomial(n, 1/2), summed in log space.
	tail := 0.0
	for i := 0; i <= k; i++ {
		tail += math.Exp(logChoose(n, i) - float64(n)*math.Ln2)
	}
	return math.Min(1, 2*tail)
}

func logChoose(n, k int) float64 {
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}

// sameMagnitude treats absolute differences equal up to floating-point
// rounding as tied: ΔV and ΔS differences that are equal in exact arithmetic
// (0.25 − 0.20 and 0.50 − 0.45) need not be equal as computed.
func sameMagnitude(a, b float64) bool {
	x, y := math.Abs(a), math.Abs(b)
	return math.Abs(x-y) <= 1e-12*math.Max(1, math.Max(x, y))
}

// WilcoxonResult is a Wilcoxon signed-rank test on paired differences.
type WilcoxonResult struct {
	// N is the number of non-zero differences the test used.
	N int `json:"n"`
	// WPlus is the sum of the ranks of the positive differences.
	WPlus float64 `json:"w_plus"`
	// P is the exact two-sided p-value; 1 when N is 0.
	P float64 `json:"p"`
}

// Wilcoxon runs the two-sided Wilcoxon signed-rank test on paired
// differences (second minus first, or any consistent order). Zero
// differences are dropped (Wilcoxon's method) and tied absolute differences
// share their average rank. The p-value is exact: the distribution of W+
// over all 2^N sign assignments to the observed ranks, which stays correct
// with ties, unlike the tabulated or normal approximations.
func Wilcoxon(diffs []float64) WilcoxonResult {
	var d []float64
	for _, x := range diffs {
		if x != 0 && !math.IsNaN(x) {
			d = append(d, x)
		}
	}
	n := len(d)
	if n == 0 {
		return WilcoxonResult{P: 1}
	}

	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return math.Abs(d[idx[a]]) < math.Abs(d[idx[b]]) })
	ranks := make([]float64, n)
	for i := 0; i < n; {
		j := i
		for j+1 < n && sameMagnitude(d[idx[j+1]], d[idx[i]]) {
			j++
		}
		avg := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			ranks[idx[k]] = avg
		}
		i = j + 1
	}

	var wPlus float64
	for i, x := range d {
		if x > 0 {
			wPlus += ranks[i]
		}
	}

	// Ranks are whole or half numbers: count sign assignments by twice the
	// rank sum, in a table of probabilities.
	total := 0
	doubled := make([]int, n)
	for i, r := range ranks {
		doubled[i] = int(math.Round(2 * r))
		total += doubled[i]
	}
	dist := make([]float64, total+1)
	dist[0] = 1
	for _, r := range doubled {
		next := make([]float64, total+1)
		for s, p := range dist {
			if p == 0 {
				continue
			}
			next[s] += p / 2
			next[s+r] += p / 2
		}
		dist = next
	}
	w := int(math.Round(2 * wPlus))
	var lower, upper float64
	for s, p := range dist {
		if s <= w {
			lower += p
		}
		if s >= w {
			upper += p
		}
	}
	return WilcoxonResult{N: n, WPlus: wPlus, P: math.Min(1, 2*math.Min(lower, upper))}
}
