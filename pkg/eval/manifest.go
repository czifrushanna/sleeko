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

// Package eval runs an optimization pipeline over the benchmark and measures
// every attempt with pkg/score: build O′ from the original Dockerfile, scan
// it, run the pipeline in-process, build, scan and smoke-test each candidate
// C the same way, and write one JSONL record per attempt. A second step turns
// the records into the report.
//
// The orchestration talks to git, Docker and Trivy only through the runner
// interfaces (Source, Builder, Scanner, Smoker, Optimizer), so it is tested
// with fakes; the real runners shell out to the tools.
package eval

import (
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Sources an item can come from.
var validSources = map[string]bool{"d3": true, "flake4dock": true, "demo": true}

// Manifest is a benchmark definition: the items an evaluation runs over.
type Manifest struct {
	Items []Item `yaml:"items"`
}

// Item is one benchmark image: a Dockerfile in a repository at a fixed
// commit, with the parameters it is built with.
type Item struct {
	// ID names the item in records and reports; lower-case letters, digits,
	// '.', '_' and '-'.
	ID string `yaml:"id"`
	// Source is the dataset the item comes from: d3, flake4dock or demo.
	Source string `yaml:"source"`
	// Repo is the git URL to clone.
	Repo   string `yaml:"repo"`
	Commit string `yaml:"commit"`
	// Context and Dockerfile are paths relative to the repository root.
	Context    string            `yaml:"context"`
	Dockerfile string            `yaml:"dockerfile"`
	BuildArgs  map[string]string `yaml:"build_args,omitempty"`
	// Platform defaults to linux/amd64.
	Platform string `yaml:"platform,omitempty"`
	// Target is the stage to build; empty builds the final stage.
	Target string `yaml:"target,omitempty"`
	Smoke  Smoke  `yaml:"smoke"`
	// DistroFamily is the final image's distribution family (debian, alpine,
	// ...), the report's split.
	DistroFamily string `yaml:"distro_family"`
	// Language is the project's main programming language, for the
	// benchmark's diversity table; optional.
	Language string `yaml:"language,omitempty"`
}

// Smoke is the smoke test: run the image and compare the exit code, or, with
// RunFor, check that it starts and keeps running.
type Smoke struct {
	// Command replaces the image's CMD; empty runs the image as it is.
	Command []string `yaml:"command,omitempty"`
	// Entrypoint, when set, replaces the image's ENTRYPOINT.
	Entrypoint string `yaml:"entrypoint,omitempty"`
	// ExitCode is the expected exit code.
	ExitCode int `yaml:"exit_code,omitempty"`
	// Timeout bounds the run; defaults to DefaultSmokeTimeout.
	Timeout time.Duration `yaml:"timeout,omitempty"`
	// RunFor switches to the "container starts" test for images that run a
	// service and never exit: the test passes when the container is still
	// running after RunFor, or when it has already exited with ExitCode.
	// Timeout does not apply.
	RunFor time.Duration `yaml:"run_for,omitempty"`
}

const (
	// DefaultPlatform is the only platform the first benchmark uses.
	DefaultPlatform = "linux/amd64"
	// DefaultSmokeTimeout bounds a smoke test without its own timeout.
	DefaultSmokeTimeout = 2 * time.Minute
)

var (
	validID     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	validCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// LoadManifest reads and validates a manifest file.
func LoadManifest(file string) (*Manifest, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return ParseManifest(data)
}

// ParseManifest parses and validates a manifest, filling in defaults.
// Unknown fields are rejected so a typo cannot silently drop a setting.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if len(m.Items) == 0 {
		return nil, errors.New("manifest has no items")
	}

	seen := map[string]bool{}
	for i := range m.Items {
		it := &m.Items[i]
		if err := it.validate(); err != nil {
			return nil, fmt.Errorf("item %d (%q): %w", i, it.ID, err)
		}
		if seen[it.ID] {
			return nil, fmt.Errorf("item %d: duplicate id %q", i, it.ID)
		}
		seen[it.ID] = true

		if it.Platform == "" {
			it.Platform = DefaultPlatform
		}
		if it.Smoke.Timeout == 0 {
			it.Smoke.Timeout = DefaultSmokeTimeout
		}
	}
	return &m, nil
}

func (it *Item) validate() error {
	switch {
	case !validID.MatchString(it.ID):
		return fmt.Errorf("id must match %s", validID)
	case !validSources[it.Source]:
		return fmt.Errorf("source %q must be one of d3, flake4dock, demo", it.Source)
	case it.Repo == "":
		return errors.New("repo is required")
	case !validCommit.MatchString(it.Commit):
		// A full SHA, so the item can never drift to another commit.
		return errors.New("commit must be a full 40-character SHA")
	case it.Context == "":
		return errors.New("context is required (use . for the repository root)")
	case it.Dockerfile == "":
		return errors.New("dockerfile is required")
	case it.DistroFamily == "":
		return errors.New("distro_family is required")
	case it.Smoke.Timeout < 0 || it.Smoke.RunFor < 0:
		return errors.New("smoke.timeout and smoke.run_for must not be negative")
	}
	for _, p := range []string{it.Context, it.Dockerfile} {
		if path.IsAbs(p) || strings.HasPrefix(path.Clean(p), "..") {
			return fmt.Errorf("path %q must stay inside the repository", p)
		}
	}
	return nil
}

// Select returns the items whose IDs are listed, in manifest order; all
// items when ids is empty. An unknown ID is an error.
func (m *Manifest) Select(ids []string) ([]Item, error) {
	if len(ids) == 0 {
		return m.Items, nil
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []Item
	for _, it := range m.Items {
		if want[it.ID] {
			out = append(out, it)
			delete(want, it.ID)
		}
	}
	if len(want) > 0 {
		var missing []string
		for id := range want {
			missing = append(missing, id)
		}
		return nil, fmt.Errorf("unknown item ids: %s", strings.Join(missing, ", "))
	}
	return out, nil
}
