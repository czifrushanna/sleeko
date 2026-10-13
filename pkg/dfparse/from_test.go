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

package dfparse

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromRefs(t *testing.T) {
	cases := []struct {
		name      string
		file      func(t *testing.T) *File
		buildArgs map[string]string
		want      []FromRef
	}{
		{
			name: "single stage",
			file: func(t *testing.T) *File { return parseFixture(t, "single.Dockerfile") },
			want: []FromRef{
				{Stage: 0, Line: 1, Raw: "python:3.11-slim", Image: "python:3.11-slim", BaseStage: -1},
			},
		},
		{
			name: "multi-stage: ARG defaults chain, stage references, undeclared and valueless ARGs",
			file: func(t *testing.T) *File { return parseFixture(t, "multistage.Dockerfile") },
			want: []FromRef{
				{Stage: 0, Line: 6, Raw: "${BUILDER}", Image: "golang:1.22-bookworm", BaseStage: -1},
				{Stage: 1, Line: 13, Raw: "build", Image: "build", BaseStage: 0},
				{Stage: 2, Line: 16, Raw: "debian:bookworm-slim", Image: "debian:bookworm-slim", BaseStage: -1},
				{Stage: 3, Line: 22, Raw: "gcr.io/distroless/static-debian12:${RUNTIME_TAG}", Image: "gcr.io/distroless/static-debian12:", BaseStage: -1, Unresolved: []string{"RUNTIME_TAG"}},
				{Stage: 4, Line: 24, Raw: "runtime-base", Image: "runtime-base", BaseStage: 2},
			},
		},
		{
			name: "build args override defaults, including ones other defaults use",
			file: func(t *testing.T) *File { return parseFixture(t, "multistage.Dockerfile") },
			buildArgs: map[string]string{
				"GO_VERSION":  "1.23",
				"RUNTIME_TAG": "nonroot",
				"UNDECLARED":  "ignored",
			},
			want: []FromRef{
				{Stage: 0, Line: 6, Raw: "${BUILDER}", Image: "golang:1.23-bookworm", BaseStage: -1},
				{Stage: 1, Line: 13, Raw: "build", Image: "build", BaseStage: 0},
				{Stage: 2, Line: 16, Raw: "debian:bookworm-slim", Image: "debian:bookworm-slim", BaseStage: -1},
				{Stage: 3, Line: 22, Raw: "gcr.io/distroless/static-debian12:${RUNTIME_TAG}", Image: "gcr.io/distroless/static-debian12:nonroot", BaseStage: -1},
				{Stage: 4, Line: 24, Raw: "runtime-base", Image: "runtime-base", BaseStage: 2},
			},
		},
		{
			name: "the repo's own Dockerfile: ARG image, scratch final stage",
			file: parseRepoDockerfile,
			want: []FromRef{
				{Stage: 0, Line: 7, Raw: "${golang_image}", Image: "golang:1.26.4", BaseStage: -1},
				{Stage: 1, Line: 18, Raw: "scratch", Image: "scratch", BaseStage: -1, Scratch: true},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.file(t).FromRefs(c.buildArgs)
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestFromRefs_EdgeCases(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []FromRef
	}{
		{
			name: "a stage can only refer to an earlier stage",
			src:  "FROM later\nFROM alpine AS later\n",
			want: []FromRef{
				{Stage: 0, Line: 1, Raw: "later", Image: "later", BaseStage: -1},
				{Stage: 1, Line: 2, Raw: "alpine", Image: "alpine", BaseStage: -1},
			},
		},
		{
			name: "stage references are case-insensitive",
			src:  "FROM alpine AS Base\nFROM BASE\n",
			want: []FromRef{
				{Stage: 0, Line: 1, Raw: "alpine", Image: "alpine", BaseStage: -1},
				{Stage: 1, Line: 2, Raw: "BASE", Image: "BASE", BaseStage: 0},
			},
		},
		{
			name: "an ARG inside a stage does not reach a later FROM",
			src:  "FROM alpine\nARG TAG=3.20\nFROM alpine:${TAG}\n",
			want: []FromRef{
				{Stage: 0, Line: 1, Raw: "alpine", Image: "alpine", BaseStage: -1},
				{Stage: 1, Line: 3, Raw: "alpine:${TAG}", Image: "alpine:", BaseStage: -1, Unresolved: []string{"TAG"}},
			},
		},
		{
			name: "default form ${VAR:-default}",
			src:  "ARG TAG\nFROM alpine:${TAG:-3.19}\n",
			want: []FromRef{
				{Stage: 0, Line: 2, Raw: "alpine:${TAG:-3.19}", Image: "alpine:3.19", BaseStage: -1, Unresolved: []string{"TAG"}},
			},
		},
		{
			name: "scratch is case-insensitive",
			src:  "FROM SCRATCH\n",
			want: []FromRef{
				{Stage: 0, Line: 1, Raw: "SCRATCH", Image: "SCRATCH", BaseStage: -1, Scratch: true},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse(c.src)
			require.NoError(t, err)
			got, err := f.FromRefs(nil)
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestFromRefs_ExpansionError(t *testing.T) {
	f, err := Parse("ARG TAG\nFROM alpine:${TAG:?missing}\n")
	require.NoError(t, err)

	_, err = f.FromRefs(nil)
	var pe *ParseError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, 2, pe.Line)
	require.Contains(t, err.Error(), "missing")
}

func TestFromRef_IsImage(t *testing.T) {
	require.True(t, FromRef{BaseStage: -1}.IsImage())
	require.False(t, FromRef{BaseStage: 0}.IsImage())
	require.False(t, FromRef{BaseStage: -1, Scratch: true}.IsImage())
}
