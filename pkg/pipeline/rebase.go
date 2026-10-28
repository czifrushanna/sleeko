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

package pipeline

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// RebaseStageName is the configuration name of the Rebase stage.
const RebaseStageName = "rebase"

// Rebase is the rule-based baseline for W2: it moves the final stage to the
// base image the table recommends, by the rule the prompt gives the model
// (the lowest V, a smaller image breaking ties, the current base unless a
// candidate is better), and changes nothing else. It makes no LLM call, so
// comparing it with W2 shows what the model adds to choosing from the
// table. It must run after baseimage. Whenever it cannot change the base, it
// says why in a diagnostic and returns the original unchanged.
type Rebase struct{}

func (Rebase) Name() string { return RebaseStageName }

func (Rebase) Run(_ context.Context, st *State) error {
	st.Candidate = st.Original
	keep := func(format string, args ...any) error {
		st.Diagnostics = append(st.Diagnostics, Diagnostic{Stage: RebaseStageName, Message: "original kept: " + fmt.Sprintf(format, args...)})
		return nil
	}
	if st.BaseTable == nil {
		return keep("no base image table")
	}
	best, ok := st.BaseTable.Best()
	if !ok {
		return keep("no candidate is better than %s", st.BaseTable.Current.Ref)
	}
	candidate, err := ReplaceFinalBase(st.Original, best.Ref)
	if err != nil {
		return keep("%v", err)
	}
	st.Candidate = candidate
	return nil
}

// ReplaceFinalBase writes image in place of the base image of the final
// stage, as FinalBase finds it, leaving every other line as it is.
func ReplaceFinalBase(dockerfile, image string) (string, error) {
	f, ref, err := baseRef(dockerfile, "")
	if err != nil {
		return "", err
	}
	from := f.Stages[ref.Stage].From
	if from.StartLine != from.EndLine {
		return "", fmt.Errorf("the FROM on line %d spans several lines", from.StartLine)
	}
	lines := strings.SplitAfter(dockerfile, "\n")
	// The keyword, then any flags (--platform=...), then the base as written.
	re := regexp.MustCompile(`^(\s*\S+\s+(?:--\S+\s+)*)` + regexp.QuoteMeta(ref.Raw) + `(\s|$)`)
	line := lines[from.StartLine-1]
	m := re.FindStringSubmatchIndex(line)
	if m == nil {
		return "", fmt.Errorf("cannot find %s in the FROM on line %d", ref.Raw, from.StartLine)
	}
	// m[3] is where the first group ends: where the base starts.
	lines[from.StartLine-1] = line[:m[3]] + image + line[m[3]+len(ref.Raw):]
	return strings.Join(lines, ""), nil
}
