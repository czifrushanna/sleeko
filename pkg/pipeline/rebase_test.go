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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/baseimage"
)

func TestReplaceFinalBase(t *testing.T) {
	cases := []struct {
		name, dockerfile, want, err string
	}{
		{"single stage", "FROM python:3.12-slim\nRUN pip install x\n", "FROM python:3.13-slim\nRUN pip install x\n", ""},
		{"keeps flags, name and case",
			"from --platform=$BUILDPLATFORM python:3.12-slim AS app\nCMD [\"x\"]\n",
			"from --platform=$BUILDPLATFORM python:3.13-slim AS app\nCMD [\"x\"]\n", ""},
		{"only the final stage's base",
			"FROM python:3.12-slim AS build\nRUN make\nFROM python:3.12-slim\nCOPY --from=build /a /a\n",
			"FROM python:3.12-slim AS build\nRUN make\nFROM python:3.13-slim\nCOPY --from=build /a /a\n", ""},
		{"follows the final stage back to its image",
			"FROM python:3.12-slim AS base\nFROM golang:1 AS build\nFROM base\nCOPY --from=build /a /a\n",
			"FROM python:3.13-slim AS base\nFROM golang:1 AS build\nFROM base\nCOPY --from=build /a /a\n", ""},
		{"a base from an ARG", "ARG V=3.12\nFROM python:${V}-slim\n", "ARG V=3.12\nFROM python:3.13-slim\n", ""},
		{"no trailing newline", "FROM python:3.12-slim", "FROM python:3.13-slim", ""},
		{"scratch", "FROM scratch\n", "", "the final stage is built FROM scratch"},
		{"continued FROM", "FROM \\\n  python:3.12-slim\n", "", "the FROM on line 1 spans several lines"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ReplaceFinalBase(c.dockerfile, "python:3.13-slim")
			if c.err != "" {
				require.EqualError(t, err, c.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestRebase(t *testing.T) {
	m := func(ref string, v int, size int64) baseimage.Measurement {
		return baseimage.Measurement{Ref: ref, V: v, Size: size}
	}
	const original = "FROM python:3.12-slim\nCMD [\"app\"]\n"
	cases := []struct {
		name       string
		table      *baseimage.Table
		want       string
		diagnostic string
	}{
		{"no table", nil, original, "original kept: no base image table"},
		{"lowest V wins over size",
			&baseimage.Table{Current: m("python:3.12-slim", 50, 40), Candidates: []baseimage.Measurement{m("python:3.12-alpine", 30, 20), m("python:3.13-slim", 10, 60)}},
			"FROM python:3.13-slim\nCMD [\"app\"]\n", ""},
		{"a smaller image breaks ties",
			&baseimage.Table{Current: m("python:3.12-slim", 10, 40), Candidates: []baseimage.Measurement{m("python:3.13-slim", 10, 50), m("python:3.12-alpine", 10, 20)}},
			"FROM python:3.12-alpine\nCMD [\"app\"]\n", ""},
		{"no better candidate",
			&baseimage.Table{Current: m("python:3.12-slim", 10, 40), Candidates: []baseimage.Measurement{m("python:3.13-slim", 10, 40), m("python:3.13-alpine", 20, 10)}},
			original, "original kept: no candidate is better than python:3.12-slim"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := &State{Original: original, BaseTable: c.table}
			require.NoError(t, Rebase{}.Run(context.Background(), st))
			require.Equal(t, c.want, st.Candidate)
			require.Zero(t, st.LLMRounds)
			if c.diagnostic == "" {
				require.Empty(t, st.Diagnostics)
			} else {
				require.Equal(t, []Diagnostic{{Stage: RebaseStageName, Message: c.diagnostic}}, st.Diagnostics)
			}
		})
	}
}

// TestRebase_AfterBaseImage runs the baseline as configured: the table the
// baseimage stage measures, then the rebase onto its best candidate.
func TestRebase_AfterBaseImage(t *testing.T) {
	p, err := New([]string{BaseImageStageName, RebaseStageName}, Deps{Proposer: stubProposer("3.12-slim-bookworm", "3.12-slim-trixie", "3.13-slim-trixie")})
	require.NoError(t, err)
	st := &State{Original: "FROM python:3.12-slim-bookworm\nCMD [\"app\"]\n"}
	require.NoError(t, p.Run(context.Background(), st))
	// The stub scanner finds nothing in trixie; 3.12 is the smaller image.
	require.Equal(t, "FROM python:3.12-slim-trixie\nCMD [\"app\"]\n", st.Candidate)
	require.Empty(t, st.Diagnostics)
}
