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

func TestRunCommands_Heredoc(t *testing.T) {
	runs := parseFixture(t, "heredoc.Dockerfile").RunCommands()
	require.Len(t, runs, 3)

	script := runs[0]
	require.Equal(t, 4, script.StartLine)
	require.Equal(t, 11, script.EndLine)
	require.False(t, script.Exec)
	require.Equal(t, "<<EOF", script.Command)
	require.Equal(t, []Heredoc{{
		Name:    "EOF",
		Content: "set -eux\napt-get update\napt-get install -y \\\n    python3-pip \\\n    git=1:2.43.0-1ubuntu7\nrm -rf /var/lib/apt/lists/*\n",
	}}, script.Heredocs)
	require.Equal(t,
		"<<EOF\nset -eux\napt-get update\napt-get install -y \\\npython3-pip \\\ngit=1:2.43.0-1ubuntu7\nrm -rf /var/lib/apt/lists/*",
		script.Normalized())

	require.Equal(t, 13, runs[1].StartLine)
	require.Equal(t, "python3 -m pip install --break-system-packages flask==3.0.3 > /tmp/pip.log 2>&1", runs[1].Command)

	exec := runs[2]
	require.True(t, exec.Exec)
	require.Equal(t, "/bin/echo exec form", exec.Command)
	require.Empty(t, exec.Heredocs)
}

func TestRunCommands_ContinuationsAndStages(t *testing.T) {
	cases := []struct {
		name       string
		file       func(t *testing.T) *File
		normalized []string
		stages     []int
	}{
		{
			name: "single stage, continuations joined",
			file: func(t *testing.T) *File { return parseFixture(t, "single.Dockerfile") },
			normalized: []string{
				"apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends -o Dpkg::Options::=--force-confold curl=7.88.1-10+deb12u6 ca-certificates && rm -rf /var/lib/apt/lists/*",
				`pip install --no-cache-dir -r requirements.txt && pip install --no-cache-dir "gunicorn==22.0.0" 'requests[socks]>=2.31' uvicorn===0.30.1`,
				"useradd --create-home app",
			},
			stages: []int{0, 0, 0},
		},
		{
			name: "multi-stage: --mount is a flag, not part of the command",
			file: func(t *testing.T) *File { return parseFixture(t, "multistage.Dockerfile") },
			normalized: []string{
				"go mod download",
				"CGO_ENABLED=0 go build -o /out/server ./cmd/server",
				"go test ./...",
				"apt-get update && apt-get install -y tzdata && rm -rf /var/lib/apt/lists/*",
			},
			stages: []int{0, 0, 1, 2},
		},
		{
			name: "the repo's own Dockerfile",
			file: parseRepoDockerfile,
			normalized: []string{
				`CGO_ENABLED=0 GOOS=linux go build -ldflags "-X github.com/czifrushanna/sleeko/pkg/adapter.Version=${version}" -o /sleeko-bin ./cmd/sleeko`,
			},
			stages: []int{0},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runs := c.file(t).RunCommands()
			var normalized []string
			var stages []int
			for _, r := range runs {
				normalized = append(normalized, r.Normalized())
				stages = append(stages, r.Stage)
			}
			require.Equal(t, c.normalized, normalized)
			require.Equal(t, c.stages, stages)
		})
	}
}

func TestRunCommands_OnbuildLeftOut(t *testing.T) {
	f, err := Parse("FROM alpine\nONBUILD RUN make\nRUN true\n")
	require.NoError(t, err)
	runs := f.RunCommands()
	require.Len(t, runs, 1)
	require.Equal(t, "true", runs[0].Command)
}
