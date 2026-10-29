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
	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/pipeline"
)

// The records keep how many model calls an attempt made, not what they
// sent, so the token cost of a pipeline is reconstructed: the first request
// is rebuilt exactly, with the code and settings of the run, from O′'s saved
// scan; a repair request resends the conversation with the repair message
// the verify stage wrote. Only the final answer of an attempt is recorded,
// so it stands in for the answers before each repair.

// Call is one reconstructed model call: what was sent, and the answer.
type Call struct {
	Input  string `json:"input"`
	Output string `json:"output"`
	// Approximate marks text that is not exactly what was sent or received:
	// the final answer standing in for an earlier one.
	Approximate bool `json:"approximate,omitempty"`
}

// AttemptCalls is one attempt's reconstructed calls.
type AttemptCalls struct {
	Pipeline   string `json:"pipeline"`
	Item       string `json:"item"`
	Repetition int    `json:"repetition"`
	LatencyMS  int64  `json:"latency_ms"`
	Calls      []Call `json:"calls"`
}

// ReconstructCalls rebuilds the model calls of one attempt. prompt and
// budget are the run's (RunInfo), o is O′'s saved scan.
func ReconstructCalls(r Record, o ScanOutput, prompt string, budget dockerfile.Budget) (AttemptCalls, error) {
	ac := AttemptCalls{Pipeline: r.Pipeline, Item: r.Item, Repetition: r.Repetition, LatencyMS: r.LatencyMS}
	if r.LLMRounds == 0 {
		return ac, nil
	}
	report, err := o.Result.HarborReport()
	if err != nil {
		return ac, err
	}
	extra := ""
	if r.BaseImage != nil {
		extra = r.BaseImage.Table.Render()
	}
	first := dockerfile.BuildPromptWith(prompt, r.OriginalDockerfile, string(report), budget, extra)
	answer := r.CandidateDockerfile
	conversation := first
	for k := 0; k < r.LLMRounds; k++ {
		// Only the first input and the last answer are exact.
		ac.Calls = append(ac.Calls, Call{Input: conversation, Output: answer, Approximate: k > 0 || k < r.LLMRounds-1})
		if k < len(r.Verification) {
			conversation += answer + pipeline.RepairMessage(r.Verification[k].Result)
		}
	}
	return ac, nil
}
