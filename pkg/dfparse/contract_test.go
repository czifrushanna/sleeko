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

func TestRuntimeContract(t *testing.T) {
	cases := []struct {
		name string
		file func(t *testing.T) *File
		want RuntimeContract
	}{
		{
			name: "single stage: relative WORKDIR resolved, exec-form CMD",
			file: func(t *testing.T) *File { return parseFixture(t, "single.Dockerfile") },
			want: RuntimeContract{
				Cmd:     &Command{Args: []string{"gunicorn", "-b", "0.0.0.0:8000", "app:app"}},
				Expose:  []string{"8000"},
				Workdir: "/app/src",
				User:    "app",
			},
		},
		{
			name: "multi-stage: inherited from the stage chain, ENTRYPOINT clears the inherited CMD, unrelated stages ignored",
			file: func(t *testing.T) *File { return parseFixture(t, "multistage.Dockerfile") },
			want: RuntimeContract{
				Entrypoint: &Command{Args: []string{"/server"}},
				Cmd:        &Command{},
				Expose:     []string{"9090", "8080"},
				User:       "65532:65532",
			},
		},
		{
			name: "shell-form CMD",
			file: func(t *testing.T) *File { return parseFixture(t, "heredoc.Dockerfile") },
			want: RuntimeContract{
				Cmd: &Command{Args: []string{"python3 /srv/app.py"}, Shell: true},
			},
		},
		{
			name: "the repo's own Dockerfile: build stage settings do not leak into scratch",
			file: parseRepoDockerfile,
			want: RuntimeContract{
				Entrypoint: &Command{Args: []string{"/sleeko"}},
				Cmd:        &Command{},
				Expose:     []string{"8080"},
				User:       "10000",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.file(t).RuntimeContract()
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestRuntimeContract_EdgeCases(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want RuntimeContract
	}{
		{
			name: "nothing set: everything inherited",
			src:  "FROM alpine\nRUN true\n",
			want: RuntimeContract{},
		},
		{
			name: "CMD before ENTRYPOINT in the same stage survives",
			src:  "FROM alpine\nCMD [\"--help\"]\nENTRYPOINT [\"/app\"]\n",
			want: RuntimeContract{Entrypoint: &Command{Args: []string{"/app"}}, Cmd: &Command{Args: []string{"--help"}}},
		},
		{
			name: "a later USER and WORKDIR win; absolute WORKDIR resets",
			src:  "FROM alpine\nUSER a\nWORKDIR /x\nWORKDIR y/../z\nUSER root\nWORKDIR /w\nWORKDIR v\n",
			want: RuntimeContract{Workdir: "/w/v", User: "root"},
		},
		{
			name: "relative WORKDIR with nothing before it stays relative: the base image decides",
			src:  "FROM alpine\nWORKDIR app\n",
			want: RuntimeContract{Workdir: "app"},
		},
		{
			name: "duplicate EXPOSE ports reported once",
			src:  "FROM alpine\nEXPOSE 80 443/tcp\nEXPOSE 80\n",
			want: RuntimeContract{Expose: []string{"443/tcp", "80"}},
		},
		{
			name: "the stage chain is followed through several hops",
			src:  "FROM alpine AS a\nUSER a\nEXPOSE 1\nFROM a AS b\nWORKDIR /b\nFROM b\nEXPOSE 2\n",
			want: RuntimeContract{Expose: []string{"1", "2"}, Workdir: "/b", User: "a"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse(c.src)
			require.NoError(t, err)
			got, err := f.RuntimeContract()
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestRuntimeContract_ExpansionError(t *testing.T) {
	f, err := Parse("ARG T\nFROM alpine:${T:?missing}\n")
	require.NoError(t, err)
	_, err = f.RuntimeContract()
	require.Error(t, err)
}
