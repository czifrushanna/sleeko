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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/baseimage"
	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/lookup"
	"github.com/czifrushanna/sleeko/pkg/score"
)

// stubRegistry serves python tags; every ref has a size, digest d:<ref>.
type stubRegistry struct{ tags []string }

func (s stubRegistry) ListTags(context.Context, string) ([]string, error) { return s.tags, nil }

func (s stubRegistry) ImageSize(_ context.Context, ref string) (lookup.ImageSize, error) {
	if ref == "python:broken" {
		return lookup.ImageSize{}, errors.New("manifest unknown")
	}
	return lookup.ImageSize{Digest: "d:" + ref, Size: int64(40e6 + len(ref))}, nil
}

// stubScanner finds one high per image, none in trixie.
type stubScanner struct{}

func (stubScanner) ScanImage(_ context.Context, ref, _ string) (baseimage.ImageScan, error) {
	if ref == "python:3.12-slim-trixie" {
		return baseimage.ImageScan{Result: score.NewScanResult(nil)}, nil
	}
	return baseimage.ImageScan{Result: score.NewScanResult([]score.Vulnerability{{ID: "CVE-1", Package: "p", Severity: score.SeverityHigh}})}, nil
}

func stubProposer(tags ...string) *baseimage.Proposer {
	return &baseimage.Proposer{Registry: stubRegistry{tags}, Scanner: stubScanner{}, Weights: score.DefaultWeights}
}

func TestFinalBase(t *testing.T) {
	cases := []struct {
		name, dockerfile, want, err string
	}{
		{"single stage", "FROM python:3.12-slim\nRUN true\n", "python:3.12-slim", ""},
		{"multi-stage takes the last", "FROM golang:1.22 AS build\nFROM debian:bookworm-slim\n", "debian:bookworm-slim", ""},
		{"follows stage chains", "FROM node:20-slim AS base\nFROM node:20 AS build\nFROM base AS final\n", "node:20-slim", ""},
		{"meta ARG", "ARG V=3.12\nFROM python:${V}-slim\n", "python:3.12-slim", ""},
		{"scratch", "FROM golang:1.22 AS b\nFROM scratch\n", "", "the final stage is built FROM scratch"},
		{"unresolved", "ARG V\nFROM python:${V}\n", "", "the final stage's base python:${V} uses unresolved variables"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := FinalBase(c.dockerfile)
			if c.err != "" {
				require.EqualError(t, err, c.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestStageBase(t *testing.T) {
	const df = "FROM python:3.10-alpine AS builder\nFROM builder AS dev\nFROM node:20 AS other\n"
	got, err := StageBase(df, "Builder")
	require.NoError(t, err)
	require.Equal(t, "python:3.10-alpine", got)
	got, err = StageBase(df, "")
	require.NoError(t, err)
	require.Equal(t, "node:20", got)
	_, err = StageBase(df, "nope")
	require.EqualError(t, err, "no stage named nope")
}

func TestBaseImage_Skips(t *testing.T) {
	cases := []struct {
		name       string
		proposer   TableProposer
		dockerfile string
		diagnostic string
	}{
		{"no scanner", nil, "FROM python:3.12-slim\n", "no base image table: no scanner configured"},
		{"scratch", stubProposer(), "FROM scratch\n", "no base image table: the final stage is built FROM scratch"},
		{"pinned", stubProposer(), "FROM python@sha256:" + sha + "\n", "no base image table: python@sha256:" + sha + " is pinned by digest"},
		{"nothing newer", stubProposer("3.11-slim", "3.12-slim"), "FROM python:3.12-slim\n", "no base image table: python:3.12-slim has no newer image in its family"},
		{"current unmeasurable", stubProposer(), "FROM python:broken\n", "no base image table: measure python:broken: manifest unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := &State{Original: c.dockerfile}
			require.NoError(t, (&BaseImage{proposer: c.proposer}).Run(context.Background(), st))
			require.Nil(t, st.BaseTable)
			require.Empty(t, st.PromptSections)
			require.Equal(t, []Diagnostic{{Stage: BaseImageStageName, Message: c.diagnostic}}, st.Diagnostics)
		})
	}
}

const sha = "0000000000000000000000000000000000000000000000000000000000000000"

func TestBaseImage_AddsTheTable(t *testing.T) {
	st := &State{Original: "FROM python:3.12-slim-bookworm\n"}
	stage, err := New([]string{BaseImageStageName}, Deps{Proposer: stubProposer("3.12-slim-bookworm", "3.12-slim-trixie", "3.13-slim-trixie")})
	require.NoError(t, err)
	require.NoError(t, stage.Run(context.Background(), st))

	require.NotNil(t, st.BaseTable)
	require.Equal(t, []string{"python:3.12-slim-bookworm", "python:3.12-slim-trixie", "python:3.13-slim-trixie"}, st.BaseTable.Refs())
	require.Equal(t, []string{st.BaseTable.Render()}, st.PromptSections)
	require.Empty(t, st.Diagnostics)
}

// TestGenerate_PromptWithTableGolden pins the prompt with a base image table:
// the baseline prompt with the table after the scan report.
func TestGenerate_PromptWithTableGolden(t *testing.T) {
	var sent sentRequest
	gw := fakeGateway(t, "FROM python:3.12-slim\n", &sent)
	st := &State{
		Original:   readTestdata(t, "generate_input.Dockerfile"),
		ScanReport: readTestdata(t, "generate_scanreport.json"),
	}
	p, err := New([]string{BaseImageStageName, GenerateStageName}, Deps{
		LLM: LLM{
			APIBaseURL: gw.URL, APIKey: "key", Model: "test-model",
			Prompt: dockerfile.DefaultPrompt,
			Budget: dockerfile.Budget{MaxTokens: 4096, ContextWindow: 32768},
		},
		Proposer: stubProposer("3.12-slim", "3.12-slim-trixie", "3.13-slim-trixie", "3.12-alpine"),
	})
	require.NoError(t, err)
	require.NoError(t, p.Run(context.Background(), st))
	require.NotNil(t, st.BaseTable, "%v", st.Diagnostics)

	golden := filepath.Join("testdata", "generate_prompt_table.golden")
	if *update {
		require.NoError(t, os.WriteFile(golden, []byte(sent.Messages[0].Content), 0o644))
	}
	require.Equal(t, readTestdata(t, "generate_prompt_table.golden"), sent.Messages[0].Content)
	require.Equal(t, readTestdata(t, "generate_prompt.golden")+st.BaseTable.Render(), sent.Messages[0].Content,
		"the table follows the baseline prompt unchanged (the report fits either way)")
}
