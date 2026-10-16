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
	"errors"
	"fmt"
	"strings"

	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/score"
	"github.com/czifrushanna/sleeko/pkg/verify"
)

// VerifyStageName is the configuration name of the Verify stage.
const VerifyStageName = "verify"

// DefaultMaxRepairRounds is how many times the model may repair a candidate
// that fails verification.
const DefaultMaxRepairRounds = 2

// Verify checks the candidate and, when a check fails, asks the model to
// repair it: the conversation so far plus the findings, up to maxRounds
// times. It keeps the candidate failing the fewest checks (one that does not
// parse fails them all; the later candidate wins a tie), so a repair that
// makes things worse is not taken. It never fails the
// run: a candidate that still fails after the last round is returned with
// its findings logged as diagnostics.
type Verify struct {
	llm       LLM
	verifier  *verify.Verifier
	maxRounds int
}

func (v *Verify) Name() string { return VerifyStageName }

func (v *Verify) Run(ctx context.Context, st *State) error {
	if st.Candidate == "" {
		return errors.New("no candidate to verify: run generate first")
	}
	if v.verifier == nil {
		return errors.New("verify stage has no verifier configured")
	}
	scan, err := score.ParseHarbor([]byte(st.ScanReport))
	if err != nil {
		// The fix-version check then has nothing to compare; the others run.
		st.Diagnostics = append(st.Diagnostics, Diagnostic{Stage: VerifyStageName, Message: "scan report unreadable: " + err.Error()})
	}
	check := func(candidate string) verify.Result {
		return v.verifier.Verify(ctx, verify.Input{Original: st.Original, Candidate: candidate, Scan: scan})
	}

	best, bestRes := st.Candidate, check(st.Candidate)
	st.Verification = append(st.Verification, VerifyRound{Round: 0, Result: bestRes})
	current := bestRes

	for round := 1; round <= v.maxRounds && !current.Passed(); round++ {
		messages := append(append([]dockerfile.Message{}, st.Messages...),
			dockerfile.Message{Role: "user", Content: RepairMessage(current)})
		reply, err := dockerfile.Chat(ctx, v.llm.APIBaseURL, v.llm.APIKey, v.llm.Model, messages, v.llm.Budget.MaxTokens)
		st.LLMRounds++
		if err != nil {
			// Keep what we have: a failed repair call is not a failed run.
			st.Diagnostics = append(st.Diagnostics, Diagnostic{Stage: VerifyStageName, Message: fmt.Sprintf("repair round %d failed: %v", round, err)})
			break
		}
		st.Messages = append(messages, dockerfile.Message{Role: "assistant", Content: reply})

		candidate := stripCodeFences(reply)
		current = check(candidate)
		st.Verification = append(st.Verification, VerifyRound{Round: round, Result: current})
		if current.Badness() <= bestRes.Badness() {
			best, bestRes = candidate, current
		}
	}

	st.Candidate = best
	for _, f := range bestRes.Findings {
		st.Diagnostics = append(st.Diagnostics, Diagnostic{Stage: VerifyStageName, Message: fmt.Sprintf("%s %s: %s", f.Severity, f.Check, f.Message)})
	}
	return nil
}

// RepairMessage asks the model to fix the findings, in its own terms: what is
// wrong, where, and what to do instead.
func RepairMessage(r verify.Result) string {
	var b strings.Builder
	b.WriteString("The Dockerfile you returned has problems that must be fixed before it can be used:\n")
	for _, f := range r.Failures() {
		b.WriteString("- ")
		if f.Line > 0 {
			fmt.Fprintf(&b, "line %d: ", f.Line)
		}
		b.WriteString(f.Message)
		b.WriteString("\n")
	}
	var warnings []verify.Finding
	for _, f := range r.Findings {
		if f.Severity == verify.Warn {
			warnings = append(warnings, f)
		}
	}
	if len(warnings) > 0 {
		b.WriteString("\nAlso check these; keep them only if they were intended:\n")
		for _, f := range warnings {
			b.WriteString("- " + f.Message + "\n")
		}
	}
	b.WriteString("\nReturn the complete corrected Dockerfile and nothing else. Keep the improvements that were not part of these problems.\n")
	return b.String()
}
