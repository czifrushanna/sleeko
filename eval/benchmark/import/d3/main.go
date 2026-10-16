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

// d3 turns the D³ dataset (Cadre, https://github.com/dw763j/Cadre) into
// candidate benchmark items.
//
// Every D³ instance is a CI build that failed after a code change. The
// commit before it is the last one that built, so the candidate is the
// parent of head_sha, with the build parameters the CI used. One candidate is
// kept per (repository, Dockerfile, target): the latest failing run's.
//
//	go run ./eval/benchmark/import/d3 \
//	  -in dataset_valid_fixed_params_with_dockerfile_relative.json \
//	  -repos 500+_repos.json -out eval/benchmark/candidates/d3.yaml
//
// The dataset file is in Git LFS in the Cadre repository. Candidates are not
// yet known to build: `sleeko-eval run --pipeline noop` over them, then
// eval/benchmark/import/select, keeps the ones that do.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/czifrushanna/sleeko/pkg/eval"
)

// instance is the part of a D³ instance the import reads.
type instance struct {
	HeadSHA     string      `json:"head_sha"`
	BuildParams buildParams `json:"build_params"`
}

type buildParams struct {
	File     string            `json:"file"`
	Context  string            `json:"context"`
	Platform json.RawMessage   `json:"platform"`
	BuildArg map[string]string `json:"build-arg"`
	Target   string            `json:"target"`
	Secret   json.RawMessage   `json:"secret"`
}

// candidate is one instance chosen for a (repository, Dockerfile, target).
type candidate struct {
	repo  string // owner#name
	runID int64
	inst  instance
}

// notLanguages are what GitHub lists for a repository without being its
// programming language.
var notLanguages = map[string]bool{
	"dockerfile": true, "shell": true, "makefile": true, "html": true, "css": true, "scss": true,
	"less": true, "batchfile": true, "powershell": true, "smarty": true, "jinja": true, "hcl": true,
	"nix": true, "just": true, "procfile": true, "starlark": true, "mustache": true, "roff": true,
	"jsonnet": true, "cmake": true, "gdb": true, "mdx": true, "dockerignore": true,
}

func main() {
	in := flag.String("in", "", "D³ dataset JSON (dataset_valid_fixed_params_with_dockerfile_relative.json)")
	reposFile := flag.String("repos", "", "Cadre's 500+_repos.json, for each repository's language (optional)")
	out := flag.String("out", "", "candidate manifest to write")
	flag.Parse()
	if *in == "" || *out == "" {
		log.Fatal("-in and -out are required")
	}

	data, err := os.ReadFile(*in)
	if err != nil {
		log.Fatal(err)
	}
	var dataset map[string]map[string]instance
	if err := json.Unmarshal(data, &dataset); err != nil {
		log.Fatalf("decode %s: %v", *in, err)
	}
	languages, err := readLanguages(*reposFile)
	if err != nil {
		log.Fatal(err)
	}

	cands, skipped := choose(dataset)
	log.Printf("%d candidates; skipped %v", len(cands), skipped)

	m := eval.Manifest{}
	ids := map[string]int{}
	for _, c := range cands {
		parent, err := parentCommit(c.repo, c.inst.HeadSHA)
		if err != nil {
			log.Printf("%s %s: %v (skipped)", c.repo, c.inst.HeadSHA, err)
			continue
		}
		it := toItem(c, parent, languages[c.repo])
		ids[it.ID]++
		if n := ids[it.ID]; n > 1 {
			it.ID = fmt.Sprintf("%s-%d", it.ID, n)
		}
		m.Items = append(m.Items, it)
	}

	enc, err := yaml.Marshal(m)
	if err != nil {
		log.Fatal(err)
	}
	header := fmt.Sprintf("# Candidates imported from D³ (Cadre) on %s by eval/benchmark/import/d3.\n"+
		"# Not yet filtered: distro_family is filled in by eval/benchmark/import/select.\n",
		time.Now().UTC().Format("2006-01-02"))
	if err := os.WriteFile(*out, append([]byte(header), enc...), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d items to %s", len(m.Items), *out)
}

// choose keeps, per (repository, Dockerfile, target), the latest run's
// instance that can be rebuilt locally: one that built for linux/amd64 and
// needed no build secrets.
func choose(dataset map[string]map[string]instance) ([]candidate, map[string]int) {
	skipped := map[string]int{}
	best := map[string]candidate{}
	for repo, instances := range dataset {
		for key, inst := range instances {
			p := inst.BuildParams
			switch {
			case !hasAMD64(p.Platform):
				skipped["not amd64"]++
				continue
			case len(p.Secret) > 0 && string(p.Secret) != "null":
				skipped["needs secrets"]++
				continue
			}
			runID := runIDOf(key)
			k := repo + "|" + p.File + "|" + p.Context + "|" + p.Target
			if prev, ok := best[k]; !ok || runID > prev.runID {
				best[k] = candidate{repo: repo, runID: runID, inst: inst}
			}
		}
	}

	out := make([]candidate, 0, len(best))
	for _, c := range best {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].repo != out[j].repo {
			return out[i].repo < out[j].repo
		}
		return out[i].runID < out[j].runID
	})
	return out, skipped
}

// runIDOf reads the run ID from an instance key,
// "<owner>#<repo>#<workflow id>#<run id>#<index>".
func runIDOf(key string) int64 {
	parts := strings.Split(key, "#")
	if len(parts) < 4 {
		return 0
	}
	id, _ := strconv.ParseInt(parts[3], 10, 64)
	return id
}

// hasAMD64 reads the platform parameter, a list or a comma-separated
// string; no platform means the runner's own, linux/amd64.
func hasAMD64(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return false
		}
		list = strings.Split(s, ",")
	}
	for _, p := range list {
		if strings.TrimSpace(p) == "linux/amd64" {
			return true
		}
	}
	return false
}

// gitContext matches a remote build context, "https://.../repo.git#ref[:subdir]".
var gitContext = regexp.MustCompile(`^(?:https?://|git@)[^#]*(?:#[^:]*(?::(.*))?)?$`)

// toItem turns a candidate into a manifest item. A remote git context is the
// same repository at the commit, possibly in a subdirectory; the Dockerfile
// is then relative to that context, otherwise to the repository root.
func toItem(c candidate, commit, language string) eval.Item {
	owner, name, _ := strings.Cut(c.repo, "#")
	p := c.inst.BuildParams

	context := strings.TrimPrefix(path.Clean(p.Context), "./")
	file := strings.TrimPrefix(path.Clean(p.File), "./")
	if m := gitContext.FindStringSubmatch(p.Context); m != nil {
		context = path.Clean(m[1])
		if p.File != "" {
			file = path.Join(context, file)
		}
	}
	if context == "" {
		context = "."
	}
	if p.File == "" {
		file = path.Join(context, "Dockerfile")
	}

	id := "d3-" + slug(owner+"-"+name)
	if base := path.Base(file); base != "Dockerfile" {
		id += "-" + slug(strings.TrimSuffix(strings.TrimPrefix(base, "Dockerfile."), ".Dockerfile"))
	}
	if p.Target != "" {
		id += "-" + slug(p.Target)
	}

	return eval.Item{
		ID:           id,
		Source:       "d3",
		Repo:         "https://github.com/" + owner + "/" + name,
		Commit:       commit,
		Context:      context,
		Dockerfile:   file,
		BuildArgs:    p.BuildArg,
		Target:       p.Target,
		DistroFamily: "unknown",
		Language:     language,
		Smoke:        eval.Smoke{RunFor: 10 * time.Second},
	}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	return s
}

// readLanguages maps "owner#name" to the repository's main programming
// language, the first GitHub lists that is one.
func readLanguages(file string) (map[string]string, error) {
	out := map[string]string{}
	if file == "" {
		return out, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Repos []struct {
			URL       string   `json:"url"`
			Languages []string `json:"languages"`
		} `json:"repos"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode %s: %w", file, err)
	}
	for _, r := range doc.Repos {
		key := strings.Replace(strings.TrimPrefix(r.URL, "https://github.com/"), "/", "#", 1)
		for _, l := range r.Languages {
			if !notLanguages[strings.ToLower(l)] {
				out[key] = strings.ToLower(l)
				break
			}
		}
	}
	return out, nil
}

// parentCommit finds head_sha's first parent with a fetch that transfers
// two commits and no files.
func parentCommit(repo, sha string) (string, error) {
	owner, name, _ := strings.Cut(repo, "#")
	dir, err := os.MkdirTemp("", "d3-parent-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	}
	if _, err := git("init", "--quiet", "--bare"); err != nil {
		return "", err
	}
	url := "https://github.com/" + owner + "/" + name
	if _, err := git("fetch", "--quiet", "--depth", "2", "--filter=tree:0", url, sha); err != nil {
		return "", err
	}
	return git("rev-parse", sha+"^")
}
