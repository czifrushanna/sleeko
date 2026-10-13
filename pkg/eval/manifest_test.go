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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const validManifest = `
items:
  - id: flask-app
    source: demo
    repo: https://github.com/docker/awesome-compose
    commit: 18f59bdb09ecf520dd5758fbf90dec314baec545
    context: flask/app
    dockerfile: flask/app/Dockerfile
    build_args:
      PYTHON_VERSION: "3.11"
    distro_family: debian
    smoke:
      command: ["python", "-c", "import flask"]
      exit_code: 0
      timeout: 90s
  - id: go-server
    source: d3
    repo: https://github.com/example/server
    commit: 0123456789abcdef0123456789abcdef01234567
    context: .
    dockerfile: build/Dockerfile
    platform: linux/arm64
    target: runtime
    distro_family: alpine
`

func TestParseManifest(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	require.NoError(t, err)
	require.Len(t, m.Items, 2)

	flask := m.Items[0]
	require.Equal(t, "flask-app", flask.ID)
	require.Equal(t, map[string]string{"PYTHON_VERSION": "3.11"}, flask.BuildArgs)
	require.Equal(t, DefaultPlatform, flask.Platform, "platform defaults to linux/amd64")
	require.Equal(t, Smoke{Command: []string{"python", "-c", "import flask"}, Timeout: 90 * time.Second}, flask.Smoke)

	server := m.Items[1]
	require.Equal(t, "linux/arm64", server.Platform)
	require.Equal(t, "runtime", server.Target)
	require.Equal(t, DefaultSmokeTimeout, server.Smoke.Timeout)
	require.Empty(t, server.Smoke.Command, "no command runs the image as it is")
}

func TestParseManifest_Rejects(t *testing.T) {
	item := func(change func(string) string) string {
		return change(strings.SplitN(validManifest, "  - id: go-server", 2)[0])
	}
	cases := []struct {
		name     string
		manifest string
		wantErr  string
	}{
		{"empty", "items: []\n", "no items"},
		{"unknown field", item(func(s string) string { return s + "    distro: debian\n" }), "field distro not found"},
		{"bad id", item(func(s string) string { return strings.Replace(s, "id: flask-app", "id: Flask App", 1) }), "id must match"},
		{"bad source", item(func(s string) string { return strings.Replace(s, "source: demo", "source: github", 1) }), "source"},
		{"short commit", item(func(s string) string {
			return strings.Replace(s, "18f59bdb09ecf520dd5758fbf90dec314baec545", "18f59bd", 1)
		}), "full 40-character SHA"},
		{"no repo", item(func(s string) string {
			return strings.Replace(s, "    repo: https://github.com/docker/awesome-compose\n", "", 1)
		}), "repo is required"},
		{"no distro family", item(func(s string) string { return strings.Replace(s, "distro_family: debian", "", 1) }), "distro_family"},
		{"path escapes the repository", item(func(s string) string {
			return strings.Replace(s, "dockerfile: flask/app/Dockerfile", "dockerfile: ../Dockerfile", 1)
		}), "inside the repository"},
		{"absolute path", item(func(s string) string { return strings.Replace(s, "context: flask/app", "context: /flask", 1) }), "inside the repository"},
		{"duplicate id", strings.Replace(validManifest, "id: go-server", "id: flask-app", 1), "duplicate id"},
		{"not YAML", "items: [", "parse manifest"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(c.manifest))
			require.ErrorContains(t, err, c.wantErr)
		})
	}
}

func TestLoadManifestAndSelect(t *testing.T) {
	file := filepath.Join(t.TempDir(), "m.yaml")
	require.NoError(t, os.WriteFile(file, []byte(validManifest), 0o644))
	m, err := LoadManifest(file)
	require.NoError(t, err)

	all, err := m.Select(nil)
	require.NoError(t, err)
	require.Len(t, all, 2)

	one, err := m.Select([]string{"go-server"})
	require.NoError(t, err)
	require.Equal(t, "go-server", one[0].ID)

	_, err = m.Select([]string{"go-server", "nope"})
	require.ErrorContains(t, err, "unknown item ids: nope")

	_, err = LoadManifest(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
}

// The benchmark manifests in the repository must stay valid.
func TestRepositoryManifestsAreValid(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "eval", "benchmark", "*.yaml"))
	require.NoError(t, err)
	for _, f := range files {
		_, err := LoadManifest(f)
		require.NoError(t, err, f)
	}
}
