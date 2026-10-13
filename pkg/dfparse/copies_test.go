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

func TestContextCopies(t *testing.T) {
	cases := []struct {
		name string
		file func(t *testing.T) *File
		want []ContextCopy
	}{
		{
			name: "single stage: remote ADD left out",
			file: func(t *testing.T) *File { return parseFixture(t, "single.Dockerfile") },
			want: []ContextCopy{
				{Stage: 0, Line: 14, Instruction: "copy", Sources: []string{"requirements.txt"}, Dest: "."},
				{Stage: 0, Line: 18, Instruction: "copy", Sources: []string{"src/"}, Dest: "./src/"},
			},
		},
		{
			name: "multi-stage: COPY --from a stage or an image left out",
			file: func(t *testing.T) *File { return parseFixture(t, "multistage.Dockerfile") },
			want: []ContextCopy{
				{Stage: 0, Line: 8, Instruction: "copy", Sources: []string{"go.mod", "go.sum"}, Dest: "./"},
				{Stage: 0, Line: 10, Instruction: "copy", Sources: []string{"."}, Dest: "."},
			},
		},
		{
			name: "heredoc sources are inline content, not context",
			file: func(t *testing.T) *File { return parseFixture(t, "heredoc.Dockerfile") },
			want: []ContextCopy{
				{Stage: 0, Line: 19, Instruction: "copy", Sources: []string{"app.py"}, Dest: "/srv/"},
			},
		},
		{
			name: "the repo's own Dockerfile",
			file: parseRepoDockerfile,
			want: []ContextCopy{
				{Stage: 0, Line: 12, Instruction: "copy", Sources: []string{"."}, Dest: "."},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, c.file(t).ContextCopies())
		})
	}
}

func TestContextCopies_EdgeCases(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []ContextCopy
	}{
		{
			name: "JSON form with a space in a path",
			src:  "FROM alpine\nCOPY [\"my file\", \"b\", \"/dst/\"]\n",
			want: []ContextCopy{{Stage: 0, Line: 2, Instruction: "copy", Sources: []string{"my file", "b"}, Dest: "/dst/"}},
		},
		{
			name: "ADD keeps local sources next to remote ones, drops git remotes",
			src:  "FROM alpine\nADD https://example.com/a.tgz local.tgz /opt/\nADD git@github.com:org/repo.git /src\n",
			want: []ContextCopy{{Stage: 0, Line: 2, Instruction: "add", Sources: []string{"local.tgz"}, Dest: "/opt/"}},
		},
		{
			name: "flags other than --from keep the copy in the context",
			src:  "FROM alpine\nCOPY --chown=1:1 --chmod=644 --link a /a\n",
			want: []ContextCopy{{Stage: 0, Line: 2, Instruction: "copy", Sources: []string{"a"}, Dest: "/a"}},
		},
		{
			name: "ONBUILD COPY does not run in this build",
			src:  "FROM alpine\nONBUILD COPY . /app\n",
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse(c.src)
			require.NoError(t, err)
			require.Equal(t, c.want, f.ContextCopies())
		})
	}
}
