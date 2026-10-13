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
	"slices"
)

// z95 is the standard normal quantile for a two-sided 95% interval.
const z95 = 1.959963984540054

// Wilson returns the 95% Wilson score interval for a success proportion.
// successes may be fractional: the schedule averages each image's repeated
// runs, and the repetitions are not independent samples, so n is the number
// of images and successes the sum of their per-image success rates. With
// n = 0 there is no information and the interval is [0, 1].
func Wilson(successes float64, n int) (lo, hi float64) {
	if n <= 0 {
		return 0, 1
	}
	nf := float64(n)
	p := successes / nf
	z2 := z95 * z95
	centre := (p + z2/(2*nf)) / (1 + z2/nf)
	half := z95 / (1 + z2/nf) * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf))
	return math.Max(0, centre-half), math.Min(1, centre+half)
}

// Summary is the median and interquartile range of a sample. N = 0 marks an
// empty sample, where the other fields are meaningless.
type Summary struct {
	N      int     `json:"n"`
	Median float64 `json:"median"`
	Q1     float64 `json:"q1"`
	Q3     float64 `json:"q3"`
}

// IQR is Q3 − Q1.
func (s Summary) IQR() float64 { return s.Q3 - s.Q1 }

// Summarize computes the median and quartiles with linear interpolation
// between order statistics (the default in R and NumPy), so the numbers in
// the report can be reproduced with either.
func Summarize(xs []float64) Summary {
	if len(xs) == 0 {
		return Summary{}
	}
	sorted := slices.Clone(xs)
	slices.Sort(sorted)
	return Summary{
		N:      len(sorted),
		Median: quantile(sorted, 0.5),
		Q1:     quantile(sorted, 0.25),
		Q3:     quantile(sorted, 0.75),
	}
}

func quantile(sorted []float64, q float64) float64 {
	pos := q * float64(len(sorted)-1)
	i := int(math.Floor(pos))
	if i+1 >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := pos - float64(i)
	return sorted[i] + frac*(sorted[i+1]-sorted[i])
}

// FixRateSummary aggregates fix rates for the report: the median and IQR
// over the attempts that had something to fix, and how many had nothing.
type FixRateSummary struct {
	Summary
	// NothingToFix counts the attempts whose O′ had no fixable CVE; they are
	// not part of Summary.
	NothingToFix int `json:"nothing_to_fix"`
}

// SummarizeFixRates summarises fix rates, keeping NothingToFix attempts out
// of the median and counting them separately.
func SummarizeFixRates(rates []FixRate) FixRateSummary {
	var measured []float64
	nothing := 0
	for _, r := range rates {
		if r.NothingToFix() {
			nothing++
			continue
		}
		measured = append(measured, r.Rate())
	}
	return FixRateSummary{Summary: Summarize(measured), NothingToFix: nothing}
}
