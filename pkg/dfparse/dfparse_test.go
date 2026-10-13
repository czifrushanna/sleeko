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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func parseFixture(t *testing.T, name string) *File {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	f, err := Parse(string(data))
	require.NoError(t, err)
	return f
}

// parseRepoDockerfile parses the Dockerfile sleeko itself is built from: a
// real multi-stage file with an ARG in FROM and a scratch final stage.
func parseRepoDockerfile(t *testing.T) *File {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	require.NoError(t, err)
	f, err := Parse(string(data))
	require.NoError(t, err)
	return f
}

func TestParse_Stages(t *testing.T) {
	cases := []struct {
		name       string
		file       func(t *testing.T) *File
		metaArgs   int
		stageNames []string
		stageLines [][2]int
	}{
		{
			name:       "single stage",
			file:       func(t *testing.T) *File { return parseFixture(t, "single.Dockerfile") },
			stageNames: []string{""},
			stageLines: [][2]int{{1, 26}},
		},
		{
			name:       "multi-stage, stage names lower-cased",
			file:       func(t *testing.T) *File { return parseFixture(t, "multistage.Dockerfile") },
			metaArgs:   3,
			stageNames: []string{"build", "test", "runtime-base", "unused", ""},
			stageLines: [][2]int{{6, 11}, {13, 14}, {16, 20}, {22, 22}, {24, 29}},
		},
		{
			name:       "heredocs count towards their instruction",
			file:       func(t *testing.T) *File { return parseFixture(t, "heredoc.Dockerfile") },
			stageNames: []string{""},
			stageLines: [][2]int{{2, 21}},
		},
		{
			name:       "the repo's own Dockerfile",
			file:       parseRepoDockerfile,
			metaArgs:   1,
			stageNames: []string{"build", ""},
			stageLines: [][2]int{{7, 15}, {18, 24}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := c.file(t)
			require.Len(t, f.MetaArgs, c.metaArgs)
			require.Len(t, f.Stages, len(c.stageNames))
			for i, st := range f.Stages {
				require.Equal(t, i, st.Index)
				require.Equal(t, c.stageNames[i], st.Name)
				require.Equal(t, c.stageLines[i], [2]int{st.StartLine(), st.EndLine()}, "stage %d lines", i)
			}
			require.Same(t, &f.Stages[len(f.Stages)-1], f.FinalStage())
		})
	}
}

func TestParse_InstructionLocations(t *testing.T) {
	f := parseFixture(t, "single.Dockerfile")
	st := f.Stages[0]

	require.Equal(t, "from", st.From.Name)
	require.Equal(t, "FROM python:3.11-slim", st.From.Original)

	// The apt RUN spans six lines through its continuations.
	run := st.Instructions[2]
	require.Equal(t, "run", run.Name)
	require.Equal(t, 7, run.StartLine)
	require.Equal(t, 12, run.EndLine)
	require.Contains(t, run.Original, "apt-get update")
	require.Contains(t, run.Original, "rm -rf /var/lib/apt/lists/*")
}

func TestParse_Errors(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantLine int
		wantMsg  string
	}{
		{"empty", "", 0, "file with no instructions"},
		{"comments only", "# nothing here\n", 1, "file with no instructions"},
		{"meta ARG only", "ARG A=1\n", 0, "no FROM instruction"},
		{"instruction before FROM", "RUN true\nFROM alpine\n", 1, "RUN before the first FROM"},
		{"unknown instruction", "FROM alpine\nRUN true\nCOPYY a /b\n", 3, "unknown instruction: COPYY"},
		{"unknown flag", "FROM alpine\nCOPY --frm=build /a /b\n", 2, "unknown flag: --frm"},
		{"FROM without an image", "FROM\n", 1, "FROM requires"},
		{"COPY without a destination", "FROM alpine\nCOPY onlyone\n", 2, "COPY requires at least two arguments"},
		{"bad stage name", "FROM alpine AS 1bad\n", 1, "invalid name for build stage"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse(c.src)
			require.Nil(t, f)

			var pe *ParseError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, c.wantLine, pe.Line)
			require.Contains(t, err.Error(), c.wantMsg)
			require.NotNil(t, errors.Unwrap(err), "the BuildKit error stays reachable")
		})
	}
}
