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

package baseimage

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/czifrushanna/sleeko/pkg/lookup"
	"github.com/czifrushanna/sleeko/pkg/score"
)

// Registry lists tags and resolves an image's linux/amd64 digest and
// compressed size.
type Registry interface {
	ListTags(ctx context.Context, ref string) ([]string, error)
	ImageSize(ctx context.Context, ref string) (lookup.ImageSize, error)
}

// ImageScan is a vulnerability scan of one image.
type ImageScan struct {
	Result score.ScanResult
	// EOSL: the scanner reports the image's distribution release as past
	// its end of life. Advisories stop at that date, so the findings
	// disclosed since are missing and V reads low.
	EOSL bool
}

// Scanner scans an image, identified by reference and amd64 digest.
type Scanner interface {
	ScanImage(ctx context.Context, ref, digest string) (ImageScan, error)
}

// Measurement is one measured image.
type Measurement struct {
	Ref    string                 `json:"ref"`
	Digest string                 `json:"digest"`
	Size   int64                  `json:"size"`
	Counts map[score.Severity]int `json:"counts"`
	V      int                    `json:"v"`
	// EOSL is recorded for the analysis; the prompt does not show it, so
	// the model chooses by the same V the metric uses.
	EOSL    bool `json:"eosl"`
	Current bool `json:"current,omitempty"`
}

// Table is the measured choice for one base image.
type Table struct {
	Current    Measurement   `json:"current"`
	Candidates []Measurement `json:"candidates"`
	// Dropped lists candidates that could not be measured, with why.
	Dropped []string `json:"dropped,omitempty"`
}

// Refs lists every image in the table, the current one first.
func (t Table) Refs() []string {
	out := []string{t.Current.Ref}
	for _, c := range t.Candidates {
		out = append(out, c.Ref)
	}
	return out
}

// Best is the candidate the table recommends by the rule the prompt states:
// the lowest V, a smaller image breaking ties. It reports false when no
// candidate is better than the current base, which is then kept.
func (t Table) Best() (Measurement, bool) {
	better := func(a, b Measurement) bool {
		return a.V < b.V || a.V == b.V && a.Size < b.Size
	}
	best, found := t.Current, false
	for _, c := range t.Candidates {
		if better(c, best) {
			best, found = c, true
		}
	}
	return best, found
}

// Proposer builds candidate tables.
type Proposer struct {
	Registry Registry
	Scanner  Scanner
	Weights  score.Weights
	// Max bounds the candidates per table.
	Max int
}

// DefaultMaxCandidates is how many candidates a table offers besides the
// current image: enough for a choice, few enough to scan.
const DefaultMaxCandidates = 4

// Official reports an image whose tags follow the grammar ParseTag reads:
// Docker's official images and Microsoft's .NET images. Other publishers
// name tags their own way ("0.13-python3.11-trixie-slim" pins a tool
// version; "ubi9/ruby-30:9.8" is a build number), so their tags are not
// proposed.
func Official(ref lookup.ImageRef) bool {
	switch ref.Registry {
	case "registry-1.docker.io":
		return strings.HasPrefix(ref.Repository, "library/")
	case "mcr.microsoft.com":
		return strings.HasPrefix(ref.Repository, "dotnet/")
	}
	return false
}

// Propose measures the current base image and its candidates. It returns a
// table with no candidates when there is nothing to propose; an error only
// when the current image itself cannot be measured.
func (p *Proposer) Propose(ctx context.Context, current string) (Table, error) {
	ref, err := lookup.ParseImageRef(current)
	if err != nil {
		return Table{}, err
	}
	if strings.HasPrefix(ref.Reference, "sha256:") {
		return Table{}, fmt.Errorf("%s is pinned by digest", current)
	}
	if !Official(ref) {
		return Table{}, fmt.Errorf("%s is not an official image, whose tags the candidate rule knows", current)
	}
	cur, err := p.measure(ctx, current)
	if err != nil {
		return Table{}, fmt.Errorf("measure %s: %w", current, err)
	}
	cur.Current = true
	t := Table{Current: cur}

	tags, err := p.Registry.ListTags(ctx, current)
	if err != nil {
		return Table{}, fmt.Errorf("list tags of %s: %w", current, err)
	}
	max := p.Max
	if max <= 0 {
		max = DefaultMaxCandidates
	}
	// Candidates keep the image name as written ("python", "docker.io/library/python").
	name := current
	if i := strings.LastIndex(current, ":"); i > strings.LastIndex(current, "/") {
		name = current[:i]
	}
	prefix := name + ":"
	// Tags are walked in preference order until max distinct images are
	// measured: a tag that is the current image under another name, or that
	// fails, does not take a place. The digest is compared before scanning,
	// which is the expensive part.
	seen := map[string]bool{cur.Digest: true}
	for _, tag := range Candidates(ref.Repository, ref.Reference, tags, len(tags)) {
		if len(t.Candidates) == max {
			break
		}
		size, err := p.Registry.ImageSize(ctx, prefix+tag)
		if err == nil && seen[size.Digest] {
			continue // an image already in the table, under another tag
		}
		var c Measurement
		if err == nil {
			c, err = p.scan(ctx, prefix+tag, size)
		}
		if err != nil {
			t.Dropped = append(t.Dropped, fmt.Sprintf("%s: %v", prefix+tag, err))
			continue
		}
		seen[c.Digest] = true
		t.Candidates = append(t.Candidates, c)
	}
	sort.SliceStable(t.Candidates, func(i, j int) bool {
		a, b := t.Candidates[i], t.Candidates[j]
		if a.V != b.V {
			return a.V < b.V
		}
		return a.Size < b.Size
	})
	return t, nil
}

func (p *Proposer) measure(ctx context.Context, ref string) (Measurement, error) {
	size, err := p.Registry.ImageSize(ctx, ref)
	if err != nil {
		return Measurement{}, err
	}
	return p.scan(ctx, ref, size)
}

func (p *Proposer) scan(ctx context.Context, ref string, size lookup.ImageSize) (Measurement, error) {
	scan, err := p.Scanner.ScanImage(ctx, ref, size.Digest)
	if err != nil {
		return Measurement{}, err
	}
	return Measurement{Ref: ref, Digest: size.Digest, Size: size.Size, Counts: scan.Result.Counts(), V: p.Weights.V(scan.Result), EOSL: scan.EOSL}, nil
}

// Render is the table as a prompt section: what was measured, how V is
// computed, and how to choose. The model is asked to copy a tag exactly, so
// the base it writes is one that exists and was measured.
func (t Table) Render() string {
	var b strings.Builder
	b.WriteString("\n\nBase image candidates for the final stage, measured with the same vulnerability scanner " +
		"(same image, same distribution family, no older than the current base; " +
		"V = 10×critical + 5×high + 2×medium + 1×low):\n")
	b.WriteString("| Base image | Compressed size | Critical | High | Medium | Low | V |\n|---|---:|---:|---:|---:|---:|---:|\n")
	row := func(m Measurement, note string) {
		fmt.Fprintf(&b, "| %s%s | %.1f MB | %d | %d | %d | %d | %d |\n", m.Ref, note, float64(m.Size)/1e6,
			m.Counts[score.SeverityCritical], m.Counts[score.SeverityHigh], m.Counts[score.SeverityMedium], m.Counts[score.SeverityLow], m.V)
	}
	row(t.Current, " (current)")
	for _, c := range t.Candidates {
		row(c, "")
	}
	b.WriteString("To change the final stage's base image, choose one from this table and write it exactly as shown. " +
		"Prefer the lowest V; a smaller image breaks ties. Keep the current base if no candidate is better. " +
		"Build stages do not ship and need no change.\n")
	return b.String()
}
