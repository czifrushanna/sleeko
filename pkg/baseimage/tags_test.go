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

package baseimage

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// realTags reads a repository's tag list as the registry returned it in
// October 2026 (testdata/*.tags.gz).
func realTags(t *testing.T, file string) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", file+".tags.gz"))
	require.NoError(t, err)
	defer f.Close()
	r, err := gzip.NewReader(f)
	require.NoError(t, err)
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return strings.Fields(string(data))
}

func TestParseTag(t *testing.T) {
	cases := []struct {
		repo, raw string
		want      Tag
	}{
		{"library/python", "3.12-slim-bookworm", Tag{Version: []int{3, 12}, Variant: []string{"slim"}, Distro: Distro{"debian", []int{12}}}},
		{"library/python", "3.10.18-alpine3.22", Tag{Version: []int{3, 10, 18}, Distro: Distro{"alpine", []int{3, 22}}}},
		{"library/python", "3.12-alpine", Tag{Version: []int{3, 12}, Distro: Distro{Family: "alpine"}}},
		{"library/python", "3.13.0rc1-slim", Tag{Version: []int{3, 13, 0}, Variant: []string{"slim"}, Excluded: true}},
		{"library/python", "3.12-windowsservercore-ltsc2022", Tag{Version: []int{3, 12}, Excluded: true}},
		{"library/node", "carbon-alpine", Tag{Version: []int{8}, Distro: Distro{Family: "alpine"}}},
		{"library/node", "lts-buster-slim", Tag{Variant: []string{"slim"}, Distro: Distro{"debian", []int{10}}}},
		{"library/php", "8.1-cli-alpine3.20", Tag{Version: []int{8, 1}, Variant: []string{"cli"}, Distro: Distro{"alpine", []int{3, 20}}}},
		{"library/eclipse-temurin", "17.0.12_7-jre-focal", Tag{Version: []int{17, 0, 12}, Variant: []string{"jre"}, Distro: Distro{"ubuntu", []int{20, 4}}}},
		{"dotnet/aspnet", "8.0-azurelinux3.0", Tag{Version: []int{8, 0}, Excluded: true}},
		{"library/debian", "bookworm-slim", Tag{Version: []int{12}, Variant: []string{"slim"}, Distro: Distro{"debian", []int{12}}}},
		{"library/debian", "forky-slim", Tag{Variant: []string{"slim"}, Distro: Distro{Family: "debian"}, Excluded: true}},
		{"library/ubuntu", "22.04", Tag{Version: []int{22, 4}, Distro: Distro{"ubuntu", []int{22, 4}}}},
		{"library/ubuntu", "25.10", Tag{Version: []int{25, 10}, Distro: Distro{"ubuntu", []int{25, 10}}, Excluded: true}},
		{"library/alpine", "3.20.3", Tag{Version: []int{3, 20, 3}, Distro: Distro{"alpine", []int{3, 20, 3}}}},
	}
	for _, c := range cases {
		got := ParseTag(c.repo, c.raw)
		c.want.Raw = c.raw
		require.Equal(t, c.want, got, c.raw)
	}
}

// TestCandidates covers the benchmark's actual final-stage base images,
// against real tag lists.
func TestCandidates(t *testing.T) {
	cases := []struct {
		repo, file, current string
		want                []string
	}{
		// Same major, newer minors, same variant and family, newest release named.
		{"library/python", "python", "3.10-slim", []string{"3.10-slim-trixie", "3.15-slim-trixie", "3.14-slim-trixie", "3.13-slim-trixie"}},
		{"library/python", "python", "3.13.2-alpine3.21", []string{"3.13-alpine3.24", "3.15-alpine3.24", "3.14-alpine3.24"}},
		{"library/php", "php", "8.0.9-apache", []string{"8.0-apache-bullseye", "8.5-apache-trixie", "8.4-apache-trixie", "8.3-apache-trixie"}},
		{"library/php", "php", "7.1-cli-alpine", []string{"7.1-cli-alpine3.10", "7.4-cli-alpine3.16", "7.3-cli-alpine3.15", "7.2-cli-alpine3.12"}},
		{"library/ruby", "ruby", "2.7-alpine", []string{"2.7-alpine3.16"}},
		// Node: major lines, LTS (even) only; codenames are versions.
		{"library/node", "node", "22-alpine", []string{"22-alpine3.24"}},
		{"library/postgres", "postgres", "17-bookworm", []string{"17-trixie"}},
		{"library/postgres", "postgres", "15.4-alpine3.18", []string{"15-alpine3.24"}},
		{"library/postgres", "postgres", "9.6-alpine", []string{"9.6-alpine3.15"}},
		{"library/node", "node", "carbon-alpine", []string{"8-alpine3.11"}},
		{"library/node", "node", "lts-buster-slim", []string{"26-trixie-slim", "24-trixie-slim", "22-trixie-slim", "20-trixie-slim"}},
		// Distribution images may move to a newer release, LTS/stable only.
		{"library/debian", "debian", "bookworm-slim", []string{"13-slim"}},
		{"library/ubuntu", "ubuntu", "jammy", []string{"26.04", "24.04"}},
		{"library/alpine", "alpine", "3.18.2", []string{"3.24", "3.23", "3.22", "3.21"}},
		// Same distribution family, newer release only.
		{"library/eclipse-temurin", "eclipse-temurin", "17-jre-focal", []string{"17-jre-noble"}},
		{"dotnet/sdk", "mcr_microsoft_com_dotnet_sdk", "6.0-bullseye-slim", []string{"6.0-bookworm-slim"}},
		// Nothing to offer: a new major would be needed.
		{"dotnet/aspnet", "mcr_microsoft_com_dotnet_aspnet", "5.0", nil},
	}
	for _, c := range cases {
		t.Run(c.repo+":"+c.current, func(t *testing.T) {
			require.Equal(t, c.want, Candidates(c.repo, c.current, realTags(t, c.file), 4))
		})
	}
}

func TestCandidates_Limits(t *testing.T) {
	tags := realTags(t, "python")
	require.Len(t, Candidates("library/python", "3.10-slim", tags, 2), 2)
	require.Empty(t, Candidates("library/python", "3.10-slim", nil, 4))
	for _, c := range Candidates("library/python", "3.7-alpine", tags, 10) {
		require.True(t, strings.HasPrefix(c, "3."), "never a new major: %s", c)
		require.Contains(t, c, "alpine", "same family: %s", c)
	}
}
