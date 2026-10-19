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
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/baseimage"
	"github.com/czifrushanna/sleeko/pkg/lookup"
	"github.com/czifrushanna/sleeko/pkg/score"
)

func TestImageName(t *testing.T) {
	for ref, want := range map[string]string{
		"python:3.12-slim":                    "python",
		"python":                              "python",
		"localhost:5000/app:1":                "localhost:5000/app",
		"localhost:5000/app":                  "localhost:5000/app",
		"python:3.12@sha256:abc":              "python",
		"mcr.microsoft.com/dotnet/aspnet:8.0": "mcr.microsoft.com/dotnet/aspnet",
	} {
		require.Equal(t, want, imageName(ref), ref)
	}
}

func TestDockerRegistry_ImageSize(t *testing.T) {
	const (
		index  = `{"manifests":[{"digest":"sha256:arm","platform":{"os":"linux","architecture":"arm64"}},{"digest":"sha256:amd","platform":{"os":"linux","architecture":"amd64"}}]}`
		leaf   = `{"layers":[{"size":100},{"size":23}]}`
		single = `{"layers":[{"size":7}]}`
	)
	rec := &recordingExec{answer: func(args []string) (execResult, error) {
		switch args[len(args)-1] {
		case "python:3.12-slim":
			return execResult{Stdout: []byte(index)}, nil
		case "python@sha256:amd":
			return execResult{Stdout: []byte(leaf)}, nil
		case "x:single":
			return execResult{Stdout: []byte(single)}, nil
		case "x:windows":
			return execResult{Stdout: []byte(`{"manifests":[{"digest":"sha256:w","platform":{"os":"windows","architecture":"amd64"}}]}`)}, nil
		case "x:nested", "x@sha256:n":
			return execResult{Stdout: []byte(`{"manifests":[{"digest":"sha256:n","platform":{"os":"linux","architecture":"amd64"}}]}`)}, nil
		case "x:garbage":
			return execResult{Stdout: []byte(`{`)}, nil
		}
		return execResult{ExitCode: 1, Stderr: []byte("not found")}, nil
	}}
	d := &DockerRegistry{exec: rec.run}
	ctx := context.Background()
	hash := func(s string) string { sum := sha256.Sum256([]byte(s)); return "sha256:" + hex.EncodeToString(sum[:]) }

	got, err := d.ImageSize(ctx, "python:3.12-slim")
	require.NoError(t, err)
	require.Equal(t, lookup.ImageSize{Digest: hash(leaf), Size: 123}, got, "the amd64 manifest's own digest")
	require.Equal(t, []string{"docker", "buildx", "imagetools", "inspect", "--raw", "python@sha256:amd"}, rec.calls[1])

	_, err = d.ImageSize(ctx, "python:3.12-slim")
	require.NoError(t, err)
	require.Len(t, rec.calls, 2, "remembered within the run")

	got, err = d.ImageSize(ctx, "x:single")
	require.NoError(t, err)
	require.Equal(t, lookup.ImageSize{Digest: hash(single), Size: 7}, got)

	for ref, want := range map[string]string{
		"x:windows": "has no linux/amd64 image",
		"x:nested":  "nested manifest lists",
		"x:garbage": "manifest of x:garbage",
		"x:missing": "inspect x:missing: exit code 1: not found",
	} {
		_, err := d.ImageSize(ctx, ref)
		require.ErrorContains(t, err, want, ref)
	}
}

type stubTags []string

func (s stubTags) ListTags(context.Context, string) ([]string, error) { return s, nil }

func TestDockerRegistry_ListTags(t *testing.T) {
	tags, err := (&DockerRegistry{Tags: stubTags{"a", "b"}}).ListTags(context.Background(), "x")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, tags)
}

// archiveScanner returns a fixed scan and records what it scanned.
type archiveScanner struct {
	out     ScanOutput
	err     error
	scanned []Image
}

func (s *archiveScanner) Scan(_ context.Context, img Image) (ScanOutput, error) {
	s.scanned = append(s.scanned, img)
	_, statErr := os.Stat(img.Archive)
	if statErr != nil {
		return ScanOutput{}, statErr
	}
	return s.out, s.err
}

func (s *archiveScanner) DBVersion(context.Context) (string, error) { return "db1", nil }

func TestBaseScanner(t *testing.T) {
	result := score.NewScanResult([]score.Vulnerability{{ID: "CVE-1", Package: "p", Severity: score.SeverityHigh}})
	eosl := true
	scanner := &archiveScanner{out: ScanOutput{Result: result, OS: "debian 10.13", EOSL: &eosl}}
	rec := &recordingExec{}
	dir := t.TempDir()
	b := &BaseScanner{Scanner: scanner, DBVersion: "db1", Cache: &Cache{Dir: dir}, Dir: dir + "/bases", exec: rec.run}
	ctx := context.Background()

	got, err := b.ScanImage(ctx, "python:3.12-slim", "sha256:amd")
	require.NoError(t, err)
	require.Equal(t, result.Counts(), got.Result.Counts())
	require.True(t, got.EOSL)
	require.Equal(t, []string{"docker", "pull", "-q", "--platform", "linux/amd64", "python@sha256:amd"}, rec.calls[0])
	require.Equal(t, []string{"docker", "save", "--platform", "linux/amd64", "-o"}, rec.calls[1][:5])
	require.Equal(t, "python@sha256:amd", rec.calls[1][6])
	require.NoFileExists(t, scanner.scanned[0].Archive, "the archive is removed after the scan")

	got, err = b.ScanImage(ctx, "python:3.12-slim-trixie", "sha256:amd")
	require.NoError(t, err)
	require.Len(t, rec.calls, 2, "cached by digest and database")
	require.True(t, got.EOSL, "the flag is cached too")

	// A scan cached before the flag was recorded is scanned again.
	require.NoError(t, b.Cache.put("basescan", scanKey("sha256:old", "db1"), ScanOutput{Result: result, OS: "debian 10.13"}))
	_, err = b.ScanImage(ctx, "x:old", "sha256:old")
	require.NoError(t, err)
	require.Len(t, rec.calls, 4)

	scanner.out.OS = ""
	_, err = b.ScanImage(ctx, "busybox:1", "sha256:busybox")
	require.ErrorContains(t, err, "the scanner detects no distribution in busybox:1")

	scanner.err = errors.New("trivy broke")
	_, err = b.ScanImage(ctx, "x:1", "sha256:x")
	require.ErrorContains(t, err, "trivy broke")

	failing := &recordingExec{answer: func(args []string) (execResult, error) {
		return execResult{ExitCode: 1, Stderr: []byte("toomanyrequests")}, nil
	}}
	b.exec = failing.run
	_, err = b.ScanImage(ctx, "x:2", "sha256:y")
	require.ErrorContains(t, err, "pull x@sha256:y: exit code 1: toomanyrequests")
}

func TestNewBaseChoice(t *testing.T) {
	table := baseimage.Table{
		Current:    baseimage.Measurement{Ref: "python:3.10-slim"},
		Candidates: []baseimage.Measurement{{Ref: "python:3.12-slim-trixie"}},
	}
	cases := []struct {
		name, candidate string
		want            BaseChoice
	}{
		{"kept", "FROM python:3.10-slim\n", BaseChoice{Chosen: "python:3.10-slim"}},
		{"from the table", "FROM golang:1 AS b\nFROM python:3.12-slim-trixie\n", BaseChoice{Chosen: "python:3.12-slim-trixie", Changed: true}},
		{"off the table", "FROM python:3.13-slim\n", BaseChoice{Chosen: "python:3.13-slim", Changed: true, OffTable: true}},
		{"unreadable", "FROM scratch\n", BaseChoice{}},
	}
	// With a target, the built stage's base counts, not the last stage's.
	got := newBaseChoice(table, "FROM python:3.12-slim-trixie AS app\nFROM python:3.13-slim AS dev\n", "app")
	require.Equal(t, "python:3.12-slim-trixie", got.Chosen)
	require.False(t, got.OffTable)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.want.Table = table
			require.Equal(t, &c.want, newBaseChoice(table, c.candidate, ""))
		})
	}
}

func TestBaseStats(t *testing.T) {
	table := baseimage.Table{
		Current: baseimage.Measurement{Ref: "python:3.10-slim", V: 20},
		Candidates: []baseimage.Measurement{
			{Ref: "python:3.12-slim-trixie", V: 3},
			{Ref: "python:3.13-slim-trixie", V: 9},
		},
	}
	choice := func(candidate string) *BaseChoice { return newBaseChoice(table, candidate, "") }
	recs := []Record{
		{BaseImage: choice("FROM python:3.12-slim-trixie\n")}, // lowest V
		{BaseImage: choice("FROM python:3.13-slim-trixie\n")},
		{BaseImage: choice("FROM python:3.10-slim\n")},
		{BaseImage: choice("FROM python:3.99-slim\n")},
		{BaseImage: choice("FROM scratch\n")},
		{}, // no table offered
	}
	bs := baseStats(recs)
	require.Equal(t, &BaseStats{Attempts: 5, Kept: 1, FromTable: 2, OffTable: 1, Unreadable: 1, LowestV: 1}, bs)
	require.Nil(t, baseStats([]Record{{}}))

	var b strings.Builder
	writeBaseStats(&b, bs)
	require.Equal(t, "\n### Base image candidates (W2)\n\n5 attempts were offered a base image table.\n\n"+
		"| Final base | Attempts | Share |\n|---|---:|---:|\n"+
		"| kept the current base | 1 | 20.0% |\n"+
		"| changed to a table candidate | 2 | 40.0% |\n"+
		"| changed to a base not in the table | 1 | 20.0% |\n"+
		"| unreadable | 1 | 20.0% |\n"+
		"\nThe chosen base has the table's lowest V in 1 attempts (20.0%).\n", b.String())

	reports := BuildReport([]Record{{Item: "a", Pipeline: "p", Status: StatusMeasured, BaseImage: choice("FROM python:3.10-slim\n")}})
	require.Equal(t, &BaseStats{Attempts: 1, Kept: 1}, reports[0].Bases)
}
