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

// Package pipeline turns a recovered Dockerfile and its scan report into an
// optimized candidate through a configurable sequence of stages.
//
// The adapter used to hard-code a single LLM call after extraction. Splitting
// that step out into named stages lets later workflows (verification, base-image
// candidates, rule-based rewrites) be switched on by configuration, and lets the
// evaluation harness run the exact same code in-process rather than through
// the HTTP adapter.
package pipeline

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// State is what flows through the stages of one optimization. Stages read the
// inputs, replace Candidate and append to Diagnostics; they never touch
// Original or ScanReport, so every stage sees what the artifact actually holds.
type State struct {
	// Original is the Dockerfile recovered from the artifact: extracted from its
	// provenance attestation or reconstructed from image history.
	Original string
	// Generated reports that Original was reconstructed from image history, so
	// it is a best-effort approximation rather than the file that was built.
	Generated bool
	// ScanReport is the raw Harbor vulnerability scan report JSON.
	ScanReport string
	// Candidate is the current optimized Dockerfile; empty until a stage
	// produces one.
	Candidate string
	// Diagnostics collects non-fatal findings. They go to the logs (and later to
	// the evaluation records), never to the Harbor report, whose contract has no
	// field for them.
	Diagnostics []Diagnostic
	// LLMRounds counts the gateway calls made for this optimization.
	LLMRounds int
}

// Diagnostic is a non-fatal finding recorded by a stage.
type Diagnostic struct {
	Stage   string
	Message string
}

// Stage is one step of the pipeline. Returning an error ends the run: a stage
// that can carry on despite a problem records a Diagnostic instead.
//
// One Stage value serves every optimization the adapter runs concurrently, so
// it must keep per-run data in State, never in its own fields.
type Stage interface {
	Name() string
	Run(ctx context.Context, st *State) error
}

// StageError identifies which stage ended a run. Err is kept unwrapped so
// callers can surface the stage's own message unchanged.
type StageError struct {
	Stage string
	Err   error
}

func (e *StageError) Error() string {
	return fmt.Sprintf("stage %s: %v", e.Stage, e.Err)
}

func (e *StageError) Unwrap() error {
	return e.Err
}

// Pipeline runs its stages in order.
type Pipeline struct {
	stages []Stage
}

// NewPipeline assembles a pipeline from already constructed stages.
func NewPipeline(stages ...Stage) *Pipeline {
	return &Pipeline{stages: stages}
}

// Run executes every stage in order and stops at the first error, which is
// returned as a *StageError.
func (p *Pipeline) Run(ctx context.Context, st *State) error {
	for _, s := range p.stages {
		if err := s.Run(ctx, st); err != nil {
			return &StageError{Stage: s.Name(), Err: err}
		}
	}
	return nil
}

// Names returns the stage names in execution order.
func (p *Pipeline) Names() []string {
	names := make([]string, len(p.stages))
	for i, s := range p.stages {
		names[i] = s.Name()
	}
	return names
}

// Deps carries what stages need to be constructed. Each stage takes only the
// parts it uses.
type Deps struct {
	LLM LLM
}

// constructors maps every stage name accepted in configuration to its
// constructor.
var constructors = map[string]func(Deps) Stage{
	GenerateStageName: func(d Deps) Stage { return NewGenerate(d.LLM) },
}

// DefaultNames returns the pipeline used when none is configured: the single
// LLM call the adapter has always made. It is a function rather than a slice
// variable so no caller can change the default for everyone else.
func DefaultNames() []string {
	return []string{GenerateStageName}
}

// ParseNames splits a comma-separated stage list and checks that every name is
// known, so a typo is caught at startup rather than on the first request.
func ParseNames(spec string) ([]string, error) {
	var names []string
	for _, n := range strings.Split(spec, ",") {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := constructors[n]; !ok {
			return nil, fmt.Errorf("unknown stage %q (known: %s)", n, strings.Join(KnownNames(), ", "))
		}
		names = append(names, n)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no stages configured")
	}
	return names, nil
}

// KnownNames lists the stage names accepted by ParseNames, sorted.
func KnownNames() []string {
	names := make([]string, 0, len(constructors))
	for n := range constructors {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// New builds a pipeline from stage names, as returned by ParseNames.
func New(names []string, deps Deps) (*Pipeline, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("no stages configured")
	}
	stages := make([]Stage, 0, len(names))
	for _, n := range names {
		ctor, ok := constructors[n]
		if !ok {
			return nil, fmt.Errorf("unknown stage %q", n)
		}
		stages = append(stages, ctor(deps))
	}
	return NewPipeline(stages...), nil
}
