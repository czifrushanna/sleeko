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

// select keeps the candidate items that build and pass their smoke test
// today, and prints the benchmark's diversity table.
//
// The evidence is a no-op run over the candidates: an item is kept when O′
// built, passed its smoke test, and the unchanged candidate passed every gate
// (so the measurement works for it). Its distro_family is set from what
// Trivy detected in O′ ("scratch" when Trivy found no distribution).
//
//	sleeko-eval run --pipeline noop --reps 1 --manifest candidates/demo.yaml --out noop.jsonl ...
//	go run ./eval/benchmark/import/select -results noop.jsonl -out eval/benchmark/v1.yaml \
//	  eval/benchmark/candidates/demo.yaml eval/benchmark/candidates/d3.yaml
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/czifrushanna/sleeko/pkg/dfparse"
	"github.com/czifrushanna/sleeko/pkg/eval"
	"github.com/czifrushanna/sleeko/pkg/score"
)

func main() {
	results := flag.String("results", "", "JSONL of a --pipeline noop run over the candidates")
	pipeline := flag.String("pipeline", "noop", "pipeline name of the no-op records")
	out := flag.String("out", "", "manifest to write the kept items to")
	src := flag.String("src", defaultSrc(), "sleeko-eval's checkout directory (<work>/src), to read Dockerfiles for the diversity table")
	ids := flag.String("items", "", "comma-separated ids to keep (default: every item that passes)")
	flag.Parse()
	if *results == "" || *out == "" || flag.NArg() == 0 {
		log.Fatal("usage: select -results noop.jsonl -out v1.yaml CANDIDATES.yaml...")
	}

	var items []eval.Item
	for _, f := range flag.Args() {
		m, err := eval.LoadManifest(f)
		if err != nil {
			log.Fatalf("%s: %v", f, err)
		}
		items = append(items, m.Items...)
	}
	recs, err := eval.ReadRecords(*results)
	if err != nil {
		log.Fatal(err)
	}

	var want map[string]bool
	if *ids != "" {
		want = map[string]bool{}
		for _, id := range strings.Split(*ids, ",") {
			want[id] = true
		}
	}

	kept, dropped := filter(items, latest(recs, *pipeline), want)
	for _, d := range dropped {
		log.Printf("dropped %s: %s", d.id, d.reason)
	}

	m := eval.Manifest{Items: make([]eval.Item, 0, len(kept))}
	for _, k := range kept {
		m.Items = append(m.Items, k.item)
	}
	if err := writeManifest(*out, m, kept); err != nil {
		log.Fatal(err)
	}
	log.Printf("kept %d of %d candidates in %s", len(kept), len(items), *out)

	git := &eval.GitSource{Dir: *src}
	if err := writeDiversity(os.Stdout, kept, func(it eval.Item) (*dfparse.File, error) {
		return readDockerfile(git, it)
	}); err != nil {
		log.Fatal(err)
	}
}

func defaultSrc() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "sleeko-eval", "src")
	}
	return ""
}

// latest returns each item's last record of the pipeline.
func latest(recs []eval.Record, pipeline string) map[string]eval.Record {
	out := map[string]eval.Record{}
	for _, r := range recs {
		if r.Pipeline == pipeline {
			out[r.Item] = r
		}
	}
	return out
}

type keptItem struct {
	item eval.Item
	rec  eval.Record
}

type droppedItem struct {
	id, reason string
}

// filter keeps the items whose no-op attempt measured cleanly.
func filter(items []eval.Item, recs map[string]eval.Record, want map[string]bool) ([]keptItem, []droppedItem) {
	var kept []keptItem
	var dropped []droppedItem
	for _, it := range items {
		if want != nil && !want[it.ID] {
			continue
		}
		rec, ok := recs[it.ID]
		if !ok {
			dropped = append(dropped, droppedItem{it.ID, "no record in the results"})
			continue
		}
		if reason := failure(rec); reason != "" {
			dropped = append(dropped, droppedItem{it.ID, reason})
			continue
		}
		it.DistroFamily = distroFamily(rec.Original.OS)
		kept = append(kept, keptItem{item: it, rec: rec})
	}
	return kept, dropped
}

// failure says why a no-op record disqualifies its item; "" when it does not.
func failure(rec eval.Record) string {
	if rec.Status != eval.StatusMeasured {
		return rec.Status + ": " + firstLine(rec.Error)
	}
	if rec.Original == nil {
		return "no O′ measurement"
	}
	for _, g := range rec.Gates {
		if g.ID == score.G3Versions {
			continue
		}
		if g.Status != score.Pass {
			return fmt.Sprintf("%s %s: %s", g.ID, g.Status, firstLine(g.Reason))
		}
	}
	return ""
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	if len(line) > 160 {
		line = line[:160] + "…"
	}
	return line
}

// distroFamily turns Trivy's "debian 12.8" into "debian"; an image without a
// detected distribution was built FROM scratch.
func distroFamily(os string) string {
	if family, _, _ := strings.Cut(strings.TrimSpace(os), " "); family != "" {
		return strings.ToLower(family)
	}
	return "scratch"
}

func writeManifest(file string, m eval.Manifest, kept []keptItem) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	db := ""
	if len(kept) > 0 {
		db = kept[0].rec.TrivyDB
	}
	header := fmt.Sprintf("# Benchmark items that built, passed their smoke test and measured cleanly\n"+
		"# with a no-op pipeline on %s (%s). Selected by eval/benchmark/import/select.\n",
		time.Now().UTC().Format("2006-01-02"), db)
	return os.WriteFile(file, append([]byte(header), data...), 0o644)
}

// readDockerfile parses an item's Dockerfile from the checkout the run left.
func readDockerfile(git *eval.GitSource, it eval.Item) (*dfparse.File, error) {
	dir, err := git.Checkout(context.Background(), it.Repo, it.Commit)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, it.Dockerfile))
	if err != nil {
		return nil, err
	}
	return dfparse.Parse(string(data))
}

// writeDiversity prints the diversity table and the totals per language and
// distro family, so gaps in the benchmark are visible.
func writeDiversity(w io.Writer, kept []keptItem, parse func(eval.Item) (*dfparse.File, error)) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSOURCE\tLANGUAGE\tDISTRO\tSTAGES\tUSER\tV\tCRIT\tHIGH\tSIZE (MB)")
	languages, distros := map[string]int{}, map[string]int{}
	for _, k := range kept {
		stages := "?"
		if f, err := parse(k.item); err == nil {
			stages = fmt.Sprint(len(f.Stages))
		}
		user := "root"
		if u := k.rec.Original.User; u != "" && u != "root" && u != "0" && !strings.HasPrefix(u, "0:") && !strings.HasPrefix(u, "root:") {
			user = "non-root"
		}
		sev := k.rec.Original.Severities
		v := score.DefaultWeights.Critical*sev["Critical"] + score.DefaultWeights.High*sev["High"] +
			score.DefaultWeights.Medium*sev["Medium"] + score.DefaultWeights.Low*sev["Low"]
		lang := k.item.Language
		if lang == "" {
			lang = "?"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%d\t%.1f\n", k.item.ID, k.item.Source, lang,
			k.item.DistroFamily, stages, user, v, sev["Critical"], sev["High"], float64(k.rec.Original.Size)/1e6)
		languages[lang]++
		distros[k.item.DistroFamily]++
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\n%d items; languages: %s; distro families: %s\n", len(kept), counts(languages), counts(distros))
	return err
}

func counts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s %d", k, m[k])
	}
	return strings.Join(parts, ", ")
}
