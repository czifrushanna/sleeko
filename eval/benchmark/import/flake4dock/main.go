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

// flake4dock turns a sample of FLAKE4DOCK's non-flaky Dockerfiles
// (github.com/flakidock/docker-flakiness-study) into candidate benchmark
// items.
//
// The dataset records a repository and its Dockerfile's text, but no commit
// or path. Both are recovered from git: a blobless clone has every commit
// and tree without file contents, `git log --raw` lists each Dockerfile
// version's blob id, and the version whose content equals the dataset's text
// gives the commit that introduced it and its path. The text is compared
// with trailing whitespace and trailing blank lines normalised, since the
// dataset's copy usually differs from the committed file by a final newline.
//
// The sample is drawn per base-image family with explicit counts, weighted
// towards the ecosystems the benchmark is thin on: the point is variety, not
// volume.
//
//	go run ./eval/benchmark/import/flake4dock -in repo_non_flaky_dataset.json.gz \
//	  -out eval/benchmark/candidates/flake4dock.yaml
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/czifrushanna/sleeko/pkg/eval"
)

// record is one FLAKE4DOCK project.
type record struct {
	RepoName   string `json:"repo_name"` // "owner@name"
	Dockerfile string `json:"dockerfile"`
}

// defaultSample is how many records to draw per base-image family. The
// benchmark already has many Python and Go items, so those get few.
const defaultSample = "ruby=30,php=30,rust=20,java=30,dotnet=20,node=25,elixir=10,ubuntu=10,debian=10,alpine=10,other=15,python=4,golang=4"

func main() {
	in := flag.String("in", "", "FLAKE4DOCK's repo_non_flaky_dataset.json.gz")
	out := flag.String("out", "", "candidate manifest to write")
	sample := flag.String("sample", defaultSample, "records to draw per base-image family")
	seed := flag.Int64("seed", 1, "sampling seed, for a reproducible sample")
	cache := flag.String("cache", filepath.Join(os.TempDir(), "flake4dock-clones"), "where blobless clones are kept")
	workers := flag.Int("workers", 8, "repositories matched at once")
	flag.Parse()
	if *in == "" || *out == "" {
		log.Fatal("-in and -out are required")
	}

	recs, err := readDataset(*in)
	if err != nil {
		log.Fatal(err)
	}
	counts, err := parseSample(*sample)
	if err != nil {
		log.Fatal(err)
	}
	picked := drawSample(recs, counts, *seed)
	log.Printf("%d records, %d sampled", len(recs), len(picked))

	type result struct {
		item eval.Item
		err  error
	}
	results := make([]result, len(picked))
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(*workers, 1))
	for i, r := range picked {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r record) {
			defer func() { <-sem; wg.Done() }()
			it, err := importRecord(r, *cache)
			results[i] = result{it, err}
		}(i, r)
	}
	wg.Wait()

	m := eval.Manifest{}
	failures := map[string]int{}
	for i, res := range results {
		if res.err != nil {
			reason := res.err.Error()
			if idx := strings.Index(reason, ":"); idx > 0 {
				reason = reason[:idx]
			}
			failures[reason]++
			log.Printf("%s: %v", picked[i].RepoName, res.err)
			continue
		}
		m.Items = append(m.Items, res.item)
	}
	sort.Slice(m.Items, func(i, j int) bool { return m.Items[i].ID < m.Items[j].ID })
	log.Printf("matched %d of %d; not matched: %v", len(m.Items), len(picked), failures)

	data, err := yaml.Marshal(m)
	if err != nil {
		log.Fatal(err)
	}
	header := fmt.Sprintf("# Candidates sampled from FLAKE4DOCK's non-flaky Dockerfiles on %s by\n"+
		"# eval/benchmark/import/flake4dock (-sample %s -seed %d).\n"+
		"# Not yet filtered: distro_family is filled in by eval/benchmark/import/select.\n",
		time.Now().UTC().Format("2006-01-02"), *sample, *seed)
	if err := os.WriteFile(*out, append([]byte(header), data...), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d items to %s", len(m.Items), *out)
}

func readDataset(file string) ([]record, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var recs []record
	if err := json.NewDecoder(gz).Decode(&recs); err != nil {
		return nil, fmt.Errorf("decode %s: %w", file, err)
	}
	return recs, nil
}

func parseSample(spec string) (map[string]int, error) {
	out := map[string]int{}
	for _, part := range strings.Split(spec, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		n, err := strconv.Atoi(v)
		if !ok || err != nil || n < 0 {
			return nil, fmt.Errorf("bad sample entry %q, want family=count", part)
		}
		out[k] = n
	}
	return out, nil
}

var fromLine = regexp.MustCompile(`(?im)^\s*FROM\s+(?:--platform=\S+\s+)?(\S+)`)

// families maps a base image name fragment to its family, in priority order:
// the first match wins, so "maven:3-openjdk" is java, not something else.
var families = []struct{ fragment, family string }{
	{"ruby", "ruby"}, {"php", "php"}, {"rust", "rust"}, {"elixir", "elixir"}, {"erlang", "elixir"},
	{"openjdk", "java"}, {"maven", "java"}, {"gradle", "java"}, {"temurin", "java"}, {"java", "java"},
	{"dotnet", "dotnet"}, {"mono", "dotnet"}, {"node", "node"}, {"python", "python"}, {"golang", "golang"},
	{"ubuntu", "ubuntu"}, {"debian", "debian"}, {"alpine", "alpine"},
}

// family classifies a Dockerfile by its first base image.
func family(dockerfile string) string {
	m := fromLine.FindStringSubmatch(dockerfile)
	if m == nil {
		return "other"
	}
	base := strings.ToLower(m[1])
	for _, f := range families {
		if strings.Contains(base, f.fragment) {
			return f.family
		}
	}
	return "other"
}

// drawSample draws, per family, up to the requested number of records,
// deterministically for a seed. Only Dockerfiles that copy from the build
// context are drawn: the benchmark is about Dockerfiles that build a project.
func drawSample(recs []record, counts map[string]int, seed int64) []record {
	byFamily := map[string][]record{}
	for _, r := range recs {
		if !strings.Contains(strings.ToUpper(r.Dockerfile), "COPY") && !strings.Contains(strings.ToUpper(r.Dockerfile), "ADD ") {
			continue
		}
		f := family(r.Dockerfile)
		byFamily[f] = append(byFamily[f], r)
	}
	rng := rand.New(rand.NewSource(seed))
	var out []record
	fams := make([]string, 0, len(counts))
	for f := range counts {
		fams = append(fams, f)
	}
	sort.Strings(fams)
	for _, f := range fams {
		pool := byFamily[f]
		sort.Slice(pool, func(i, j int) bool { return pool[i].RepoName < pool[j].RepoName })
		rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		out = append(out, pool[:min(counts[f], len(pool))]...)
	}
	return out
}

// normalize makes two copies of a Dockerfile comparable: line endings, trailing
// whitespace on each line and trailing blank lines do not count.
func normalize(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// blobID is git's object id for content, as `git hash-object` computes it.
func blobID(content string) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00%s", len(content), content)
	return hex.EncodeToString(h.Sum(nil))
}

// exactVariants are the texts a committed Dockerfile is most likely to have,
// given the dataset's copy: matching their blob ids needs no download.
func exactVariants(text string) []string {
	n := normalize(text)
	return []string{text, n + "\n", n, n + "\n\n", strings.ReplaceAll(n, "\n", "\r\n") + "\r\n"}
}

// version is one Dockerfile version in a repository's history.
type version struct {
	commit string
	time   int64
	path   string
	blob   string
}

var dockerfileName = regexp.MustCompile(`(?i)(^|[./_-])dockerfile([._-][^/]*)?$`)

// parseRawLog reads `git log --raw --no-abbrev --no-renames --format=@%H %ct`
// output into the Dockerfile versions it introduces. --no-abbrev matters: by
// default --raw shortens blob ids, which then never equal a computed one.
func parseRawLog(out []byte) []version {
	var vs []version
	var commit string
	var ts int64
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if rest, ok := strings.CutPrefix(line, "@"); ok {
			c, t, _ := strings.Cut(rest, " ")
			commit = c
			ts, _ = strconv.ParseInt(t, 10, 64)
			continue
		}
		// ":100644 100644 <old> <new> M\t<path>"
		meta, p, ok := strings.Cut(line, "\t")
		if !ok || !strings.HasPrefix(meta, ":") {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) < 5 || strings.HasPrefix(fields[4], "D") {
			continue
		}
		if dockerfileName.MatchString(path.Base(p)) {
			vs = append(vs, version{commit: commit, time: ts, path: p, blob: fields[3]})
		}
	}
	return vs
}

// git runs git without ever prompting for credentials and without fetching
// missing objects implicitly; each call is time-limited.
func git(dir string, timeout time.Duration, stdin string, allowFetch bool, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "credential.helper="}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/usr/bin/true")
	if !allowFetch {
		cmd.Env = append(cmd.Env, "GIT_NO_LAZY_FETCH=1")
	}
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git %s timed out", args[0])
		}
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		return nil, fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout.Bytes(), nil
}

// importRecord finds the record's commit and Dockerfile path and builds the
// candidate item.
func importRecord(r record, cache string) (eval.Item, error) {
	owner, name, ok := strings.Cut(r.RepoName, "@")
	if !ok {
		return eval.Item{}, fmt.Errorf("bad repo name %q", r.RepoName)
	}
	url := "https://github.com/" + owner + "/" + name
	dir := filepath.Join(cache, owner+"@"+name+".git")

	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err != nil {
		os.RemoveAll(dir)
		if err := os.MkdirAll(cache, 0o755); err != nil {
			return eval.Item{}, err
		}
		if _, err := git(cache, 3*time.Minute, "", true, "clone", "--quiet", "--bare", "--filter=blob:none", url, dir); err != nil {
			os.RemoveAll(dir)
			return eval.Item{}, fmt.Errorf("clone failed: %w", err)
		}
	}

	out, err := git(dir, 2*time.Minute, "", false, "log", "--all", "--no-renames", "--raw", "--no-abbrev", "--format=@%H %ct")
	if err != nil {
		return eval.Item{}, fmt.Errorf("log failed: %w", err)
	}
	versions := parseRawLog(out)
	if len(versions) == 0 {
		return eval.Item{}, errors.New("no Dockerfile in history")
	}

	match, err := findVersion(dir, versions, r.Dockerfile)
	if err != nil {
		return eval.Item{}, err
	}

	files, err := git(dir, time.Minute, "", false, "ls-tree", "-r", "--name-only", match.commit)
	if err != nil {
		return eval.Item{}, fmt.Errorf("ls-tree failed: %w", err)
	}

	context := path.Dir(match.path)
	return eval.Item{
		ID:           "f4d-" + slug(owner+"-"+name),
		Source:       "flake4dock",
		Repo:         url,
		Commit:       match.commit,
		Context:      context,
		Dockerfile:   match.path,
		DistroFamily: "unknown",
		Language:     language(strings.Split(strings.TrimSpace(string(files)), "\n")),
		Smoke:        eval.Smoke{RunFor: 10 * time.Second},
	}, nil
}

// findVersion returns the Dockerfile version whose content is the dataset's
// text: first by blob id (no download), then by normalised content, fetching
// only the Dockerfile blobs. The latest version wins if several match.
func findVersion(dir string, versions []version, text string) (version, error) {
	sort.SliceStable(versions, func(i, j int) bool { return versions[i].time > versions[j].time })

	want := map[string]bool{}
	for _, v := range exactVariants(text) {
		want[blobID(v)] = true
	}
	for _, v := range versions {
		if want[v.blob] {
			return v, nil
		}
	}

	var blobs []string
	seen := map[string]bool{}
	for _, v := range versions {
		if !seen[v.blob] && len(blobs) < 40 {
			seen[v.blob] = true
			blobs = append(blobs, v.blob)
		}
	}
	out, err := git(dir, 3*time.Minute, strings.Join(blobs, "\n")+"\n", true, "cat-file", "--batch")
	if err != nil {
		return version{}, fmt.Errorf("fetch Dockerfile versions failed: %w", err)
	}
	contents := parseBatch(out)
	norm := normalize(text)
	for _, v := range versions {
		if c, ok := contents[v.blob]; ok && normalize(c) == norm {
			return v, nil
		}
	}
	return version{}, fmt.Errorf("no matching Dockerfile version among %d", len(seen))
}

// parseBatch reads `git cat-file --batch` output: "<oid> <type> <size>\n<content>\n".
func parseBatch(out []byte) map[string]string {
	res := map[string]string{}
	for len(out) > 0 {
		nl := bytes.IndexByte(out, '\n')
		if nl < 0 {
			break
		}
		header := strings.Fields(string(out[:nl]))
		out = out[nl+1:]
		if len(header) != 3 {
			continue // "<oid> missing"
		}
		size, err := strconv.Atoi(header[2])
		if err != nil || size > len(out) {
			break
		}
		res[header[0]] = string(out[:size])
		out = out[min(size+1, len(out)):]
	}
	return res
}

// extensions maps source file extensions to languages.
var extensions = map[string]string{
	".py": "python", ".go": "go", ".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".vue": "javascript",
	".ts": "typescript", ".tsx": "typescript", ".rb": "ruby", ".php": "php", ".rs": "rust", ".java": "java",
	".kt": "kotlin", ".scala": "scala", ".cs": "c#", ".fs": "f#", ".c": "c", ".cpp": "c++", ".cc": "c++",
	".ex": "elixir", ".exs": "elixir", ".erl": "erlang", ".hs": "haskell", ".pl": "perl", ".lua": "lua",
	".swift": "swift", ".dart": "dart", ".clj": "clojure", ".cr": "crystal", ".jl": "julia", ".r": "r",
	".groovy": "groovy", ".nim": "nim", ".zig": "zig", ".ml": "ocaml",
}

// vendored marks paths whose files are not the project's own code.
var vendored = regexp.MustCompile(`(^|/)(vendor|node_modules|third_party|thirdparty|dist|build|public|static|assets)/|\.min\.js$`)

// language is the most common source language among the repository's files.
func language(files []string) string {
	counts := map[string]int{}
	for _, f := range files {
		if vendored.MatchString(f) {
			continue
		}
		if l, ok := extensions[strings.ToLower(path.Ext(f))]; ok {
			counts[l]++
		}
	}
	best, bestN := "", 0
	for l, n := range counts {
		if n > bestN || (n == bestN && l < best) {
			best, bestN = l, n
		}
	}
	return best
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 50 {
		s = strings.Trim(s[:50], "-")
	}
	return s
}
