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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/czifrushanna/sleeko/pkg/pipeline"
)

// PipelineOptimizer runs a pkg/pipeline pipeline in-process: the same code
// the adapter runs for Harbor.
type PipelineOptimizer struct {
	Pipeline *pipeline.Pipeline
}

// Optimize runs the pipeline. A pipeline error, or a run that produced no
// candidate, is an optimizer failure: the attempt is unsuccessful.
func (p *PipelineOptimizer) Optimize(ctx context.Context, in OptimizeInput) (OptimizeOutput, error) {
	st := &pipeline.State{Original: in.Dockerfile, ScanReport: in.ScanReport}
	err := p.Pipeline.Run(ctx, st)

	out := OptimizeOutput{Candidate: st.Candidate, LLMRounds: st.LLMRounds, Verification: st.Verification, BaseTable: st.BaseTable}
	for _, d := range st.Diagnostics {
		out.Diagnostics = append(out.Diagnostics, d.Stage+": "+d.Message)
	}
	if err != nil {
		return out, err
	}
	if st.Candidate == "" {
		return out, errors.New("pipeline produced no candidate")
	}
	return out, nil
}

// NoopOptimizer returns the original Dockerfile unchanged, so C = O. Run
// over real items it checks the measurement itself: every gate must pass
// with ΔV = 0 and ΔS ≈ 0, or the harness is biased. It is a harness
// configuration, not a pipeline stage, so the adapter cannot be configured
// to use it.
type NoopOptimizer struct{}

// Optimize returns the input Dockerfile.
func (NoopOptimizer) Optimize(_ context.Context, in OptimizeInput) (OptimizeOutput, error) {
	return OptimizeOutput{Candidate: in.Dockerfile}, nil
}

// PromptHash identifies the prompt text in records, so results from
// different prompts are never mixed up.
func PromptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])[:16]
}
