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
	"strings"

	"github.com/czifrushanna/sleeko/pkg/dockerfile"
)

// GenerateStageName is the configuration name of the Generate stage.
const GenerateStageName = "generate"

// LLM carries what an LLM-backed stage needs to reach the gateway.
type LLM struct {
	APIBaseURL string
	APIKey     string
	Model      string
	// Prompt is the instruction text prepended to the Dockerfile.
	Prompt string
	Budget dockerfile.Budget
	// Sampling is the temperature and base seed; unset fields leave the
	// gateway's defaults.
	Sampling dockerfile.Sampling
}

// sampling is the sampling for one optimization: the base seed plus the
// state's seed offset, so repeated optimizations of the same input (an
// evaluation's repetitions) sample differently yet reproducibly.
func (l LLM) sampling(st *State) dockerfile.Sampling {
	s := l.Sampling
	if s.Seed != nil {
		seed := *s.Seed + st.SeedOffset
		s.Seed = &seed
	}
	return s
}

// Generate asks the LLM gateway for an optimized Dockerfile in a single call:
// the original Dockerfile plus the condensed scan report in, a candidate out.
// It is the whole of the adapter's original behaviour.
type Generate struct {
	llm LLM
}

// NewGenerate returns a Generate stage that calls the given gateway.
func NewGenerate(llm LLM) *Generate {
	return &Generate{llm: llm}
}

func (g *Generate) Name() string { return GenerateStageName }

func (g *Generate) Run(ctx context.Context, st *State) error {
	prompt := dockerfile.BuildPromptWith(g.llm.Prompt, st.Original, st.ScanReport, g.llm.Budget, strings.Join(st.PromptSections, ""))
	messages := []dockerfile.Message{{Role: "user", Content: prompt}}
	optimized, err := dockerfile.Chat(ctx, g.llm.APIBaseURL, g.llm.APIKey, g.llm.Model, messages, g.llm.Budget.MaxTokens, g.llm.sampling(st))
	st.LLMRounds++
	if err != nil {
		return err
	}

	st.Messages = append(messages, dockerfile.Message{Role: "assistant", Content: optimized})
	st.Candidate = stripCodeFences(optimized)
	return nil
}

// stripCodeFences removes a leading ``` / ```dockerfile line and a trailing ```
// line that LLM gateways commonly wrap around the returned Dockerfile.
func stripCodeFences(s string) string {
	out := strings.TrimSpace(s)

	if strings.HasPrefix(out, "```") {
		if idx := strings.IndexByte(out, '\n'); idx >= 0 {
			out = out[idx+1:]
		} else {
			// A lone fence line with no content
			return ""
		}
	}

	if strings.HasSuffix(strings.TrimSpace(out), "```") {
		trimmed := strings.TrimSpace(out)
		trimmed = strings.TrimSuffix(trimmed, "```")
		out = strings.TrimRight(trimmed, " \t\n")
	}

	return strings.TrimSpace(out) + "\n"
}
