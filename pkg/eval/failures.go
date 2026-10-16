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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/czifrushanna/sleeko/pkg/score"
)

// Failure is one measured attempt that failed at least one gate.
type Failure struct {
	Record Record
	// Failed are the gates that failed, in gate order.
	Failed []score.Gate
}

// Failures returns the measured attempts that failed a gate, keeping only
// those failing gate (any gate when empty), the last record per attempt,
// sorted by item and repetition.
func Failures(records []Record, gate score.GateID) []Failure {
	last := map[attemptKey]Record{}
	for _, r := range records {
		last[r.key()] = r
	}
	var out []Failure
	for _, r := range last {
		if r.Status != StatusMeasured {
			continue
		}
		var failed []score.Gate
		for _, g := range r.Gates {
			if g.Status == score.Fail && (gate == "" || g.ID == gate) {
				failed = append(failed, g)
			}
		}
		if len(failed) > 0 {
			out = append(out, Failure{Record: r, Failed: failed})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Record, out[j].Record
		if a.Item != b.Item {
			return a.Item < b.Item
		}
		if a.Pipeline != b.Pipeline {
			return a.Pipeline < b.Pipeline
		}
		return a.Repetition < b.Repetition
	})
	return out
}

// WriteFailures prints each failure: the failing gates with their reasons and
// a line diff from the original Dockerfile to the candidate.
func WriteFailures(w io.Writer, failures []Failure) error {
	var b strings.Builder
	for _, f := range failures {
		r := f.Record
		fmt.Fprintf(&b, "=== %s  %s #%d\n", r.Item, r.Pipeline, r.Repetition)
		for _, g := range f.Failed {
			fmt.Fprintf(&b, "%s: %s\n", g.ID, g.Reason)
		}
		b.WriteString(LineDiff(r.OriginalDockerfile, r.CandidateDockerfile))
		b.WriteString("\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// SaveFailures writes each failure as a fixture directory,
// <dir>/<item>-<pipeline>-<repetition>/ with original.Dockerfile,
// candidate.Dockerfile and gates.json, for tests of the checks that should
// catch it.
func SaveFailures(dir string, failures []Failure) error {
	for _, f := range failures {
		r := f.Record
		d := filepath.Join(dir, fmt.Sprintf("%s-%s-%d", r.Item, sanitizeTag(r.Pipeline), r.Repetition))
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
		gates, err := json.MarshalIndent(f.Failed, "", "  ")
		if err != nil {
			return err
		}
		for name, data := range map[string][]byte{
			"original.Dockerfile":  []byte(r.OriginalDockerfile),
			"candidate.Dockerfile": []byte(r.CandidateDockerfile),
			"gates.json":           append(gates, '\n'),
		} {
			if err := os.WriteFile(filepath.Join(d, name), data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// LineDiff is a minimal line diff from a to b: unchanged lines prefixed with
// two spaces, removed with "- ", added with "+ ". Dockerfiles are short, so
// the quadratic longest-common-subsequence table is fine.
func LineDiff(a, b string) string {
	x := strings.Split(strings.TrimRight(a, "\n"), "\n")
	y := strings.Split(strings.TrimRight(b, "\n"), "\n")
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out strings.Builder
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			out.WriteString("  " + x[i] + "\n")
			i++
			j++
		case i < len(x) && (j == len(y) || lcs[i+1][j] >= lcs[i][j+1]):
			// Removals before additions, so a changed line reads - then +.
			out.WriteString("- " + x[i] + "\n")
			i++
		default:
			out.WriteString("+ " + y[j] + "\n")
			j++
		}
	}
	return out.String()
}
