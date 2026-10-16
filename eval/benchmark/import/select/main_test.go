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
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/dfparse"
	"github.com/czifrushanna/sleeko/pkg/eval"
	"github.com/czifrushanna/sleeko/pkg/score"
)

func gates(override ...score.Gate) []score.Gate {
	var out []score.Gate
	for _, id := range score.AllGates {
		g := score.Gate{ID: id, Status: score.Pass}
		if id == score.G3Versions {
			g.Status = score.NotChecked
		}
		for _, o := range override {
			if o.ID == id {
				g = o
			}
		}
		out = append(out, g)
	}
	return out
}

func measured(item, os, user string, g []score.Gate) eval.Record {
	return eval.Record{Item: item, Pipeline: "noop", Status: eval.StatusMeasured, Gates: g, TrivyDB: "trivy 0.75.0 db X",
		Original: &eval.ImageInfo{OS: os, User: user, Size: 27_200_000, Severities: map[string]int{"Critical": 1, "High": 2, "Low": 3}}}
}

func TestFilter(t *testing.T) {
	items := []eval.Item{
		{ID: "ok-debian", DistroFamily: "unknown", Language: "python"},
		{ID: "ok-scratch", DistroFamily: "unknown", Language: "go"},
		{ID: "smoke-fails", DistroFamily: "unknown"},
		{ID: "build-fails", DistroFamily: "unknown"},
		{ID: "not-run", DistroFamily: "unknown"},
	}
	recs := latest([]eval.Record{
		{Item: "ok-debian", Pipeline: "noop", Status: eval.StatusInfraError, Error: "transient"},
		measured("ok-debian", "debian 12.8", "", gates()),
		measured("ok-scratch", "", "65532", gates()),
		{Item: "smoke-fails", Pipeline: "noop", Status: eval.StatusInfraError, Error: "O′ fails its own smoke test: exited with code 1\nlogs..."},
		measured("build-fails", "alpine 3.20", "", gates(score.Gate{ID: score.G4Build, Status: score.Fail, Reason: "exit code 1"})),
		measured("not-run", "debian 12", "", gates()),
	}, "noop")
	delete(recs, "not-run")

	kept, dropped := filter(items, recs, nil, nil, false)
	require.Len(t, kept, 2)
	require.Equal(t, "debian", kept[0].item.DistroFamily, "set from Trivy; the later record wins")
	require.Equal(t, "scratch", kept[1].item.DistroFamily)
	require.Equal(t, []droppedItem{
		{"smoke-fails", "infra_error: O′ fails its own smoke test: exited with code 1"},
		{"build-fails", "G4_build fail: exit code 1"},
		{"not-run", "no record in the results"},
	}, dropped)

	only, _ := filter(items, recs, map[string]bool{"ok-scratch": true}, nil, false)
	require.Len(t, only, 1)
}

func TestWriteManifestIsLoadable(t *testing.T) {
	it := eval.Item{ID: "x", Source: "demo", Repo: "https://example.com/r", Commit: strings.Repeat("a", 40),
		Context: ".", Dockerfile: "Dockerfile", DistroFamily: "debian", Smoke: eval.Smoke{RunFor: 10e9}}
	file := filepath.Join(t.TempDir(), "v1.yaml")
	require.NoError(t, writeManifest(file, eval.Manifest{Items: []eval.Item{it}}, []keptItem{{item: it, rec: measured("x", "debian 12", "", gates())}}))

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(data), "(trivy 0.75.0 db X)")
	m, err := eval.LoadManifest(file)
	require.NoError(t, err)
	require.Equal(t, it.ID, m.Items[0].ID)
}

func TestWriteDiversity(t *testing.T) {
	kept := []keptItem{
		{item: eval.Item{ID: "a", Source: "demo", Language: "python", DistroFamily: "debian"}, rec: measured("a", "debian 12", "", gates())},
		{item: eval.Item{ID: "b", Source: "d3", Language: "go", DistroFamily: "scratch"}, rec: measured("b", "", "65532:65532", gates())},
		{item: eval.Item{ID: "c", Source: "d3", DistroFamily: "debian"}, rec: measured("c", "debian 12", "root:root", gates())},
	}
	parse := func(it eval.Item) (*dfparse.File, error) {
		if it.ID == "c" {
			return nil, errors.New("no checkout")
		}
		return dfparse.Parse("FROM a AS b\nFROM b\n")
	}
	var buf bytes.Buffer
	require.NoError(t, writeDiversity(&buf, kept, parse))
	out := buf.String()

	require.Regexp(t, `a\s+demo\s+python\s+debian\s+2\s+root\s+23\s+1\s+2\s+27\.2`, out) // V = 10 + 2·5 + 3·1
	require.Regexp(t, `b\s+d3\s+go\s+scratch\s+2\s+non-root`, out)
	require.Regexp(t, `c\s+d3\s+\?\s+debian\s+\?\s+root`, out)
	require.Contains(t, out, "3 items; languages: ? 1, go 1, python 1; distro families: debian 2, scratch 1")
}

func TestDistroFamily(t *testing.T) {
	require.Equal(t, "debian", distroFamily("debian 12.8"))
	require.Equal(t, "alpine", distroFamily("Alpine 3.20.10"))
	require.Equal(t, "scratch", distroFamily(""))
}

func sized(id, lang, distro string, size int64) keptItem {
	r := measured(id, distro+" 1", "", gates())
	r.Original.Size = size
	return keptItem{item: eval.Item{ID: id, Language: lang, DistroFamily: distro}, rec: r}
}

func ids(ks []keptItem) []string {
	var out []string
	for _, k := range ks {
		out = append(out, k.item.ID)
	}
	return out
}

func TestBalance(t *testing.T) {
	passing := []keptItem{
		sized("py-1", "python", "debian", 100), sized("py-2", "python", "alpine", 100),
		sized("py-3", "python", "debian", 100), sized("go-1", "go", "alpine", 100),
		sized("rb-big", "ruby", "debian", 900), sized("rb-small", "ruby", "debian", 50),
		sized("rb-alpine", "ruby", "alpine", 500), sized("php-1", "php", "debian", 100),
		sized("unknown", "", "debian", 1),
	}

	// The current benchmark stays; new items fill the thinnest languages
	// first, distro variety before size within a language.
	got := balance(passing, map[string]bool{"py-1": true, "py-2": true}, 6, 0)
	require.Equal(t, []string{"py-1", "py-2", "go-1", "php-1", "rb-small", "rb-alpine"}, ids(got))

	// A per-language cap leaves out good candidates; the unknown language
	// comes last.
	got = balance(passing, nil, 0, 2)
	require.Equal(t, []string{"go-1", "php-1", "py-1", "rb-alpine", "py-2", "rb-small", "unknown"}, ids(got))

	// No limits keeps everything.
	require.Len(t, balance(passing, nil, 0, 0), len(passing))
}

func TestFilter_ScannerBlindAndExcluded(t *testing.T) {
	items := []eval.Item{{ID: "blind"}, {ID: "scratch-clean"}, {ID: "seen"}, {ID: "excluded"}}
	blind := measured("blind", "ubuntu 14.04", "", gates())
	blind.Original.Severities = map[string]int{}
	clean := measured("scratch-clean", "", "", gates())
	clean.Original.Severities = map[string]int{}
	recs := latest([]eval.Record{blind, clean, measured("seen", "debian 12", "", gates()), measured("excluded", "debian 12", "", gates())}, "noop")

	kept, dropped := filter(items, recs, nil, map[string]bool{"excluded": true}, false)
	require.Equal(t, []string{"scratch-clean", "seen"}, ids(kept), "a scratch image may genuinely have nothing")
	require.Equal(t, []droppedItem{
		{"blind", "ubuntu 14.04 image scans with no findings: likely unreadable by the scanner"},
		{"excluded", "excluded by -exclude"},
	}, dropped)

	kept, _ = filter(items, recs, nil, nil, true)
	require.Len(t, kept, 4, "-allow-zero-v keeps it")
}
