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

package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// LintFinding is one Hadolint finding.
type LintFinding struct {
	Code    string `json:"code"`
	Level   string `json:"level"` // error, warning, info, style
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// Linter lints a Dockerfile.
type Linter interface {
	Lint(ctx context.Context, dockerfile string) ([]LintFinding, error)
}

// DefaultHadolintImage pins Hadolint when it runs from its container image.
const DefaultHadolintImage = "hadolint/hadolint:v2.12.0"

// Hadolint runs Hadolint on stdin: Command is the binary (["hadolint"]) or
// a container (["docker", "run", "--rm", "-i", DefaultHadolintImage, "hadolint"]).
type Hadolint struct {
	Command []string
}

// FindHadolint returns a Hadolint on PATH, else nil: the adapter's image
// has no binaries, so the lint check is optional there (D2).
func FindHadolint() *Hadolint {
	if path, err := exec.LookPath("hadolint"); err == nil {
		return &Hadolint{Command: []string{path}}
	}
	return nil
}

// Lint returns Hadolint's findings. Hadolint exits non-zero when it finds
// something, so the exit code is ignored when its JSON output parses.
func (h *Hadolint) Lint(ctx context.Context, dockerfile string) ([]LintFinding, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	args := append(append([]string{}, h.Command[1:]...), "--no-fail", "-f", "json", "-")
	cmd := exec.CommandContext(ctx, h.Command[0], args...)
	cmd.Stdin = strings.NewReader(dockerfile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()

	var findings []LintFinding
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &findings); err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("hadolint: %v: %s", runErr, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("hadolint output: %w", err)
	}
	return findings, nil
}

// checkLint reports lint problems the candidate introduces: codes the
// original does not have. New errors fail; new warnings are only reported.
func (v *Verifier) checkLint(ctx context.Context, res *Result, in Input) {
	if v.Lint == nil {
		res.skip(CheckLint, "hadolint not available")
		return
	}
	before, err := v.Lint.Lint(ctx, in.Original)
	if err != nil {
		res.skip(CheckLint, err.Error())
		return
	}
	after, err := v.Lint.Lint(ctx, in.Candidate)
	if err != nil {
		res.skip(CheckLint, err.Error())
		return
	}

	had := map[string]bool{}
	for _, f := range before {
		had[f.Code] = true
	}
	sort.SliceStable(after, func(i, j int) bool { return after[i].Line < after[j].Line })
	reported := map[string]bool{}
	for _, f := range after {
		if had[f.Code] || reported[f.Code] {
			continue
		}
		switch f.Level {
		case "error":
			res.add(CheckLint, Fail, f.Line, "new lint error %s at line %d: %s", f.Code, f.Line, f.Message)
		case "warning":
			res.add(CheckLint, Warn, f.Line, "new lint warning %s at line %d: %s", f.Code, f.Line, f.Message)
		default:
			continue
		}
		reported[f.Code] = true
	}
}
