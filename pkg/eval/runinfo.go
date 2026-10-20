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
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/czifrushanna/sleeko/pkg/score"
)

// RunInfo describes one invocation of a run: everything needed to tell what
// produced its records. A resumed run appends another; the fields that
// determine results must then match the first (see Conflicts).
type RunInfo struct {
	StartedAt time.Time `json:"started_at"`

	// Code is the sleeko commit the harness was built from, with "-dirty"
	// when the working tree had uncommitted changes.
	Code string `json:"code"`

	Pipeline string `json:"pipeline"`
	// Stages is the pipeline's stage list, or "noop".
	Stages string `json:"stages"`
	// Model, the LLM budget and the prompt are empty for noop.
	Model         string `json:"model,omitempty"`
	APIBaseURL    string `json:"api_base_url,omitempty"`
	MaxTokens     int    `json:"max_tokens,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
	PromptHash    string `json:"prompt_hash,omitempty"`
	// Temperature and Seed are the LLM sampling; Seed is the base seed, an
	// attempt using Seed plus its repetition number. Absent: the gateway's
	// defaults.
	Temperature *float64 `json:"temperature,omitempty"`
	Seed        *int64   `json:"seed,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	// MaxRepairRounds and Lint configure the verify stage, when there is
	// one: Lint is the Hadolint used, or empty when lint is skipped.
	MaxRepairRounds int    `json:"max_repair_rounds,omitempty"`
	Lint            string `json:"lint,omitempty"`
	// G3 is what the versions gate checks, or empty when it is not checked.
	G3 string `json:"g3,omitempty"`
	// MaxBaseCandidates configures the baseimage stage, when there is one.
	MaxBaseCandidates int `json:"max_base_candidates,omitempty"`

	Manifest       string   `json:"manifest"`
	ManifestSHA256 string   `json:"manifest_sha256"`
	Items          []string `json:"items"`

	Repetitions    int           `json:"repetitions"`
	Concurrency    int           `json:"concurrency"`
	BuildTimeout   string        `json:"build_timeout"`
	MaxOriginalAge string        `json:"max_original_age"`
	SizeTolerance  float64       `json:"size_tolerance"`
	Weights        score.Weights `json:"weights"`

	TrivyImage string `json:"trivy_image"`
	TrivyDB    string `json:"trivy_db"`
	Docker     string `json:"docker,omitempty"`
}

// conflicts lists the fields a resumed run must share with the first but does not:
// changing any of them changes what the records measure. Concurrency, the
// item selection and the start time may differ.
func conflicts(first, next RunInfo) []string {
	var diffs []string
	check := func(name string, a, b any) {
		if fmt.Sprint(a) != fmt.Sprint(b) {
			diffs = append(diffs, fmt.Sprintf("%s: %v → %v", name, a, b))
		}
	}
	check("code", first.Code, next.Code)
	check("pipeline", first.Pipeline, next.Pipeline)
	check("stages", first.Stages, next.Stages)
	check("model", first.Model, next.Model)
	check("max_tokens", first.MaxTokens, next.MaxTokens)
	check("context_window", first.ContextWindow, next.ContextWindow)
	check("prompt_hash", first.PromptHash, next.PromptHash)
	check("temperature", deref(first.Temperature), deref(next.Temperature))
	check("seed", deref(first.Seed), deref(next.Seed))
	check("max_repair_rounds", first.MaxRepairRounds, next.MaxRepairRounds)
	check("lint", first.Lint, next.Lint)
	check("max_base_candidates", first.MaxBaseCandidates, next.MaxBaseCandidates)
	check("g3", first.G3, next.G3)
	check("manifest_sha256", first.ManifestSHA256, next.ManifestSHA256)
	check("repetitions", first.Repetitions, next.Repetitions)
	check("build_timeout", first.BuildTimeout, next.BuildTimeout)
	check("size_tolerance", first.SizeTolerance, next.SizeTolerance)
	check("weights", first.Weights, next.Weights)
	check("trivy_image", first.TrivyImage, next.TrivyImage)
	check("trivy_db", first.TrivyDB, next.TrivyDB)
	return diffs
}

// deref shows an optional setting, "unset" for nil.
func deref[T any](p *T) any {
	if p == nil {
		return "unset"
	}
	return *p
}

// AppendRunInfo records a run's info in file, one JSON object per line. When
// the file already holds an earlier run's info that differs in a field that
// determines results, it refuses (unless allowMixed), so one results file
// never silently mixes two experiments.
func AppendRunInfo(file string, info RunInfo, allowMixed bool) error {
	previous, err := ReadRunInfo(file)
	if err != nil {
		return err
	}
	if len(previous) > 0 && !allowMixed {
		if diffs := conflicts(previous[0], info); len(diffs) > 0 {
			return fmt.Errorf("this run differs from the one that started %s in: %s (use a new --out, or --allow-mixed-runs)",
				file, strings.Join(diffs, "; "))
		}
	}

	line, err := json.Marshal(info)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// ReadRunInfo reads every run's info from file; a missing file reads as none.
func ReadRunInfo(file string) ([]RunInfo, error) {
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []RunInfo
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for line := 1; sc.Scan(); line++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var info RunInfo
		if err := json.Unmarshal(sc.Bytes(), &info); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", file, line, err)
		}
		out = append(out, info)
	}
	return out, sc.Err()
}
