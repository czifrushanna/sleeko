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

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/czifrushanna/sleeko/pkg/eval"
)

func inst(t *testing.T, params string) instance {
	t.Helper()
	var p buildParams
	require.NoError(t, json.Unmarshal([]byte(params), &p))
	return instance{HeadSHA: "head", BuildParams: p}
}

func TestChoose(t *testing.T) {
	dataset := map[string]map[string]instance{
		"cilium#tetragon": {
			// Same Dockerfile, two runs: the later one wins.
			"cilium#tetragon#1#100#0": inst(t, `{"file": "./Dockerfile", "context": ".", "platform": ["linux/amd64", "linux/arm64"]}`),
			"cilium#tetragon#1#200#0": inst(t, `{"file": "./Dockerfile", "context": ".", "platform": "linux/amd64,linux/arm64"}`),
			// Another Dockerfile in the same repository is its own candidate.
			"cilium#tetragon#1#150#0": inst(t, `{"file": "./Dockerfile.rthooks", "context": "."}`),
		},
		"x#y": {
			"x#y#1#1#0": inst(t, `{"file": "Dockerfile", "context": ".", "platform": ["linux/arm64"]}`),
			"x#y#1#2#0": inst(t, `{"file": "Dockerfile", "context": ".", "secret": ["id=npm"]}`),
		},
	}
	got, skipped := choose(dataset)
	require.Equal(t, map[string]int{"not amd64": 1, "needs secrets": 1}, skipped)
	require.Len(t, got, 2)
	require.Equal(t, int64(150), got[0].runID)
	require.Equal(t, int64(200), got[1].runID)
}

func TestHasAMD64(t *testing.T) {
	cases := map[string]bool{
		``:                                true,
		`null`:                            true,
		`["linux/amd64"]`:                 true,
		`"linux/arm64, linux/amd64"`:      true,
		`["linux/arm64", "linux/arm/v7"]`: false,
		`{"unexpected": "shape"}`:         false,
	}
	for raw, want := range cases {
		require.Equal(t, want, hasAMD64(json.RawMessage(raw)), raw)
	}
}

func TestToItem(t *testing.T) {
	cases := []struct {
		name                 string
		repo, params         string
		wantID, wantContext  string
		wantDockerfile       string
		wantTarget, wantLang string
	}{
		{
			name: "local context", repo: "cilium#tetragon",
			params: `{"file": "./Dockerfile.rthooks", "context": ".", "build-arg": {"V": "1"}}`,
			wantID: "d3-cilium-tetragon-rthooks", wantContext: ".", wantDockerfile: "Dockerfile.rthooks",
		},
		{
			name: "subdirectory context and target", repo: "Org#App",
			params: `{"file": "./docker/api/Dockerfile", "context": "./api", "target": "Runtime"}`,
			wantID: "d3-org-app-runtime", wantContext: "api", wantDockerfile: "docker/api/Dockerfile", wantTarget: "Runtime",
		},
		{
			name: "remote git context at the commit", repo: "akuity#kargo",
			params: `{"context": "https://github.com/akuity/kargo.git#a71d63a2d15cdf7287d07f7f10ad53958e60cffc"}`,
			wantID: "d3-akuity-kargo", wantContext: ".", wantDockerfile: "Dockerfile",
		},
		{
			name: "remote git context with a subdirectory", repo: "o#r",
			params: `{"context": "https://github.com/o/r.git#main:services/web", "file": "Dockerfile.prod"}`,
			wantID: "d3-o-r-prod", wantContext: "services/web", wantDockerfile: "services/web/Dockerfile.prod",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := toItem(candidate{repo: c.repo, inst: inst(t, c.params)}, "0123456789abcdef0123456789abcdef01234567", "go")
			require.Equal(t, c.wantID, it.ID)
			require.Equal(t, c.wantContext, it.Context)
			require.Equal(t, c.wantDockerfile, it.Dockerfile)
			require.Equal(t, c.wantTarget, it.Target)
			require.Equal(t, "d3", it.Source)
			require.Equal(t, "unknown", it.DistroFamily)
			require.Equal(t, "go", it.Language)
			require.Equal(t, eval.Smoke{RunFor: 10 * time.Second}, it.Smoke)
		})
	}
}

// An imported item, written the way main writes it, must pass manifest
// validation once its distro family is known.
func TestToItemIsAValidManifestItem(t *testing.T) {
	it := toItem(candidate{repo: "akuity#kargo", inst: inst(t, `{"context": "https://github.com/akuity/kargo.git#abc", "build-arg": {"A": "1"}}`)},
		"0123456789abcdef0123456789abcdef01234567", "go")
	it.DistroFamily = "debian"
	data, err := yaml.Marshal(eval.Manifest{Items: []eval.Item{it}})
	require.NoError(t, err)

	m, err := eval.ParseManifest(data)
	require.NoError(t, err, string(data))
	require.Equal(t, 10*time.Second, m.Items[0].Smoke.RunFor)
	require.Equal(t, map[string]string{"A": "1"}, m.Items[0].BuildArgs)
}

func TestRunIDOfAndSlug(t *testing.T) {
	require.Equal(t, int64(14400796222), runIDOf("cilium#tetragon#25964050#14400796222#0"))
	require.Zero(t, runIDOf("malformed"))
	require.Equal(t, "my-org-my-repo", slug("My_Org/My.Repo"))
	require.Len(t, slug("a-very-long-repository-name-that-goes-on-and-on-forever"), 40)
}

func TestReadLanguages(t *testing.T) {
	file := filepath.Join(t.TempDir(), "repos.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"repos": [
		{"url": "https://github.com/cilium/tetragon", "languages": ["C", "Go"]},
		{"url": "https://github.com/o/r", "languages": ["Dockerfile", "Shell", "TypeScript"]},
		{"url": "https://github.com/o/only-docker", "languages": ["Dockerfile"]}
	]}`), 0o644))
	got, err := readLanguages(file)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"cilium#tetragon": "c", "o#r": "typescript"}, got)

	empty, err := readLanguages("")
	require.NoError(t, err)
	require.Empty(t, empty)
}
