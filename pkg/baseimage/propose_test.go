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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/lookup"
	"github.com/czifrushanna/sleeko/pkg/score"
)

// fakeRegistry serves tags and sizes; a ref mapped to size 0 fails.
type fakeRegistry struct {
	tags  []string
	sizes map[string]lookup.ImageSize
}

func (f *fakeRegistry) ListTags(context.Context, string) ([]string, error) { return f.tags, nil }

func (f *fakeRegistry) ImageSize(_ context.Context, ref string) (lookup.ImageSize, error) {
	s, ok := f.sizes[ref]
	if !ok || s.Size == 0 {
		return lookup.ImageSize{}, errors.New("manifest unknown")
	}
	return s, nil
}

// fakeScanner gives each digest some findings.
type fakeScanner map[string][]score.Severity

func (f fakeScanner) ScanImage(_ context.Context, _, digest string) (ImageScan, error) {
	var vulns []score.Vulnerability
	for i, sev := range f[digest] {
		vulns = append(vulns, score.Vulnerability{ID: "CVE-" + string(rune('A'+i)), Package: "p", Severity: sev})
	}
	return ImageScan{Result: score.NewScanResult(vulns), EOSL: digest == "sha256:cur"}, nil
}

const (
	crit = score.SeverityCritical
	high = score.SeverityHigh
	med  = score.SeverityMedium
	low  = score.SeverityLow
)

func proposer() *Proposer {
	return &Proposer{
		Registry: &fakeRegistry{
			tags: []string{"3.10-slim", "3.11-slim-bookworm", "3.12-slim-bookworm", "3.13-slim-trixie", "3.14-slim-trixie", "3.12-alpine"},
			sizes: map[string]lookup.ImageSize{
				"python:3.10-slim":          {Digest: "sha256:cur", Size: 45_000_000},
				"python:3.14-slim-trixie":   {Digest: "sha256:314", Size: 0}, // cannot be measured
				"python:3.13-slim-trixie":   {Digest: "sha256:313", Size: 44_000_000},
				"python:3.12-slim-bookworm": {Digest: "sha256:312", Size: 43_000_000},
				"python:3.11-slim-bookworm": {Digest: "sha256:cur", Size: 45_000_000}, // same image as current
			},
		},
		Scanner: fakeScanner{
			"sha256:cur": {crit, high, high, med, low},
			"sha256:313": {med, low},
			"sha256:312": {med, low},
		},
		Weights: score.DefaultWeights,
	}
}

func TestPropose(t *testing.T) {
	table, err := proposer().Propose(context.Background(), "python:3.10-slim")
	require.NoError(t, err)

	require.Equal(t, Measurement{Ref: "python:3.10-slim", Digest: "sha256:cur", Size: 45_000_000,
		Counts: map[score.Severity]int{crit: 1, high: 2, med: 1, low: 1}, V: 23, EOSL: true, Current: true}, table.Current)
	require.False(t, table.Candidates[0].EOSL)
	// Equal V: the smaller image first. The 3.11 tag is the current image.
	require.Equal(t, []string{"python:3.12-slim-bookworm", "python:3.13-slim-trixie"}, []string{table.Candidates[0].Ref, table.Candidates[1].Ref})
	require.Len(t, table.Candidates, 2)
	require.Equal(t, []string{"python:3.14-slim-trixie: manifest unknown"}, table.Dropped)
	require.Equal(t, []string{"python:3.10-slim", "python:3.12-slim-bookworm", "python:3.13-slim-trixie"}, table.Refs())

	require.Equal(t, "\n\nBase image candidates for the final stage, measured with the same vulnerability scanner "+
		"(same image, same distribution family, no older than the current base; V = 10×critical + 5×high + 2×medium + 1×low):\n"+
		"| Base image | Compressed size | Critical | High | Medium | Low | V |\n|---|---:|---:|---:|---:|---:|---:|\n"+
		"| python:3.10-slim (current) | 45.0 MB | 1 | 2 | 1 | 1 | 23 |\n"+
		"| python:3.12-slim-bookworm | 43.0 MB | 0 | 0 | 1 | 1 | 3 |\n"+
		"| python:3.13-slim-trixie | 44.0 MB | 0 | 0 | 1 | 1 | 3 |\n"+
		"To change the final stage's base image, choose one from this table and write it exactly as shown. "+
		"Prefer the lowest V; a smaller image breaks ties. Keep the current base if no candidate is better. "+
		"Build stages do not ship and need no change.\n", table.Render())
}

func TestPropose_Errors(t *testing.T) {
	p := proposer()
	_, err := p.Propose(context.Background(), "python@sha256:"+string(make([]byte, 0))+"0000000000000000000000000000000000000000000000000000000000000000")
	require.ErrorContains(t, err, "pinned by digest")

	_, err = p.Propose(context.Background(), "python:3.99")
	require.ErrorContains(t, err, "measure python:3.99")

	_, err = p.Propose(context.Background(), "ghcr.io/astral-sh/uv:python3.11-bookworm-slim")
	require.EqualError(t, err, "ghcr.io/astral-sh/uv:python3.11-bookworm-slim is not an official image, whose tags the candidate rule knows")

	_, err = p.Propose(context.Background(), "${BASE}")
	require.Error(t, err)
}

func TestPropose_UntaggedBase(t *testing.T) {
	p := proposer()
	reg := p.Registry.(*fakeRegistry)
	reg.sizes["python"] = lookup.ImageSize{Digest: "sha256:latest", Size: 1}
	p.Scanner.(fakeScanner)["sha256:latest"] = nil
	table, err := p.Propose(context.Background(), "python")
	require.NoError(t, err)
	for _, c := range table.Candidates {
		require.Regexp(t, `^python:3\.1[0-9]`, c.Ref)
	}
}

// TestPropose_SkippedTagsTakeNoPlace: a tag that fails, or is the current
// image under another name, does not use up one of the Max places.
func TestPropose_SkippedTagsTakeNoPlace(t *testing.T) {
	p := proposer()
	p.Max = 2
	reg := p.Registry.(*fakeRegistry)
	reg.tags = append(reg.tags, "3.10-slim-trixie") // offered first, as the current line
	reg.sizes["python:3.10-slim-trixie"] = lookup.ImageSize{Digest: "sha256:cur", Size: 45_000_000}

	table, err := p.Propose(context.Background(), "python:3.10-slim")
	require.NoError(t, err)
	require.Equal(t, []string{"python:3.10-slim", "python:3.12-slim-bookworm", "python:3.13-slim-trixie"}, table.Refs())
	require.Equal(t, []string{"python:3.14-slim-trixie: manifest unknown"}, table.Dropped)
}

func TestOfficial(t *testing.T) {
	for ref, want := range map[string]bool{
		"python:3.12":                             true,
		"docker.io/library/alpine:3.21":           true,
		"mcr.microsoft.com/dotnet/aspnet:8.0":     true,
		"mcr.microsoft.com/dotnet/core/sdk:3.1":   true,
		"tiangolo/uvicorn-gunicorn-fastapi:py":    false,
		"registry.access.redhat.com/ubi9/ruby-30": false,
		"gcr.io/distroless/static:nonroot":        false,
		"mcr.microsoft.com/mssql/server:2022":     false,
	} {
		r, err := lookup.ParseImageRef(ref)
		require.NoError(t, err)
		require.Equal(t, want, Official(r), ref)
	}
}
