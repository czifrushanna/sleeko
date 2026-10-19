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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/czifrushanna/sleeko/pkg/baseimage"
	"github.com/czifrushanna/sleeko/pkg/lookup"
)

// basePlatform is the platform base images are measured for: the
// benchmark's builds are linux/amd64.
const basePlatform = "linux/amd64"

// TagLister lists a repository's tags (lookup.Client).
type TagLister interface {
	ListTags(ctx context.Context, ref string) ([]string, error)
}

// DockerRegistry is the base image table's registry in the harness. Tags
// come from the registry API, which Docker Hub does not rate-limit;
// manifests, which it does count as pulls when fetched anonymously, are read
// with `docker buildx imagetools inspect`, authenticated by the Docker login.
type DockerRegistry struct {
	Tags TagLister

	exec  execFunc
	mu    sync.Mutex
	sizes map[string]lookup.ImageSize // within a run a tag keeps one digest
}

func (d *DockerRegistry) ListTags(ctx context.Context, ref string) ([]string, error) {
	return d.Tags.ListTags(ctx, ref)
}

// ImageSize resolves ref to its linux/amd64 manifest and sums its layers.
func (d *DockerRegistry) ImageSize(ctx context.Context, ref string) (lookup.ImageSize, error) {
	d.mu.Lock()
	if s, ok := d.sizes[ref]; ok {
		d.mu.Unlock()
		return s, nil
	}
	d.mu.Unlock()

	raw, err := d.inspect(ctx, ref)
	if err != nil {
		return lookup.ImageSize{}, err
	}
	var m struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
		Layers []struct {
			Size int64 `json:"size"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return lookup.ImageSize{}, fmt.Errorf("manifest of %s: %w", ref, err)
	}
	if len(m.Manifests) > 0 {
		digest := ""
		for _, e := range m.Manifests {
			if e.Platform.OS+"/"+e.Platform.Architecture == basePlatform {
				digest = e.Digest
				break
			}
		}
		if digest == "" {
			return lookup.ImageSize{}, fmt.Errorf("%s has no %s image", ref, basePlatform)
		}
		if raw, err = d.inspect(ctx, imageName(ref)+"@"+digest); err != nil {
			return lookup.ImageSize{}, err
		}
		m.Manifests, m.Layers = nil, nil
		if err := json.Unmarshal(raw, &m); err != nil {
			return lookup.ImageSize{}, fmt.Errorf("manifest of %s: %w", ref, err)
		}
		if len(m.Manifests) > 0 {
			return lookup.ImageSize{}, fmt.Errorf("%s: nested manifest lists", ref)
		}
	}
	// The digest of a manifest is the hash of its bytes.
	sum := sha256.Sum256(raw)
	s := lookup.ImageSize{Digest: "sha256:" + hex.EncodeToString(sum[:])}
	for _, l := range m.Layers {
		s.Size += l.Size
	}

	d.mu.Lock()
	if d.sizes == nil {
		d.sizes = map[string]lookup.ImageSize{}
	}
	d.sizes[ref] = s
	d.mu.Unlock()
	return s, nil
}

func (d *DockerRegistry) inspect(ctx context.Context, ref string) ([]byte, error) {
	run := d.exec
	if run == nil {
		run = runCommand
	}
	res, err := run(ctx, "", "docker", "buildx", "imagetools", "inspect", "--raw", ref)
	res, err = mustSucceed(res, err, "inspect "+ref)
	return res.Stdout, err
}

// imageName is ref without its tag or digest.
func imageName(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	return ref
}

// BaseScanner scans base images the way O′ and C are scanned: pulled by
// digest, saved as an archive, and scanned by the same Trivy and database.
// Scans are cached by (digest, database). Pulled images stay in Docker,
// where a candidate built on them finds them.
type BaseScanner struct {
	Scanner   Scanner
	DBVersion string
	Cache     *Cache
	// Dir holds the archives while they are scanned.
	Dir string

	exec  execFunc
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// ScanImage scans ref at its linux/amd64 digest. An image whose
// distribution the scanner does not detect is an error: its packages would
// go unseen and it would look clean.
func (b *BaseScanner) ScanImage(ctx context.Context, ref, digest string) (baseimage.ImageScan, error) {
	key := scanKey(digest, b.DBVersion)
	defer b.lock(key)()
	var cached ScanOutput
	if ok, err := b.Cache.get("basescan", key, &cached); err != nil {
		return baseimage.ImageScan{}, err
	} else if ok && cached.EOSL != nil {
		return baseimage.ImageScan{Result: cached.Result, EOSL: *cached.EOSL}, nil
	}

	run := b.exec
	if run == nil {
		run = runCommand
	}
	pinned := imageName(ref) + "@" + digest
	res, err := run(ctx, "", "docker", "pull", "-q", "--platform", basePlatform, pinned)
	if _, err := mustSucceed(res, err, "pull "+pinned); err != nil {
		return baseimage.ImageScan{}, err
	}
	if err := os.MkdirAll(b.Dir, 0o755); err != nil {
		return baseimage.ImageScan{}, err
	}
	f, err := os.CreateTemp(b.Dir, "base-*.tar")
	if err != nil {
		return baseimage.ImageScan{}, err
	}
	f.Close()
	defer os.Remove(f.Name())
	res, err = run(ctx, "", "docker", "save", "--platform", basePlatform, "-o", f.Name(), pinned)
	if _, err := mustSucceed(res, err, "save "+pinned); err != nil {
		return baseimage.ImageScan{}, err
	}
	out, err := b.Scanner.Scan(ctx, Image{Tag: pinned, Digest: digest, Archive: f.Name()})
	if err != nil {
		return baseimage.ImageScan{}, err
	}
	if out.OS == "" {
		return baseimage.ImageScan{}, fmt.Errorf("the scanner detects no distribution in %s", ref)
	}
	return baseimage.ImageScan{Result: out.Result, EOSL: out.EOSL != nil && *out.EOSL}, b.Cache.put("basescan", key, out)
}

func (b *BaseScanner) lock(key string) func() {
	b.mu.Lock()
	if b.locks == nil {
		b.locks = map[string]*sync.Mutex{}
	}
	l, ok := b.locks[key]
	if !ok {
		l = &sync.Mutex{}
		b.locks[key] = l
	}
	b.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// BaseStats are W2's metrics over the attempts offered a base image table.
type BaseStats struct {
	// Attempts is the number of attempts offered a table.
	Attempts int `json:"attempts"`
	// Kept, FromTable and OffTable split them by the candidate's final
	// base: the current one, another from the table, or one the table did
	// not offer (an invented or unmeasured tag; the done-when wants none).
	// Unreadable counts candidates whose final base cannot be read.
	Kept       int `json:"kept"`
	FromTable  int `json:"from_table"`
	OffTable   int `json:"off_table"`
	Unreadable int `json:"unreadable"`
	// LowestV counts choices (kept or changed) of a base with the table's
	// lowest V.
	LowestV int `json:"lowest_v"`
}

func baseStats(recs []Record) *BaseStats {
	bs := &BaseStats{}
	for _, r := range recs {
		c := r.BaseImage
		if c == nil {
			continue
		}
		bs.Attempts++
		switch {
		case c.Chosen == "":
			bs.Unreadable++
			continue
		case c.OffTable:
			bs.OffTable++
			continue
		case c.Changed:
			bs.FromTable++
		default:
			bs.Kept++
		}
		lowest, chosen := c.Table.Current.V, c.Table.Current.V
		for _, m := range c.Table.Candidates {
			lowest = min(lowest, m.V)
			if m.Ref == c.Chosen {
				chosen = m.V
			}
		}
		if chosen == lowest {
			bs.LowestV++
		}
	}
	if bs.Attempts == 0 {
		return nil
	}
	return bs
}

func writeBaseStats(b *strings.Builder, bs *BaseStats) {
	share := func(n int) string { return percent(float64(n) / float64(bs.Attempts)) }
	fmt.Fprintf(b, "\n### Base image candidates (W2)\n\n%d attempts were offered a base image table.\n\n", bs.Attempts)
	b.WriteString("| Final base | Attempts | Share |\n|---|---:|---:|\n")
	fmt.Fprintf(b, "| kept the current base | %d | %s |\n", bs.Kept, share(bs.Kept))
	fmt.Fprintf(b, "| changed to a table candidate | %d | %s |\n", bs.FromTable, share(bs.FromTable))
	fmt.Fprintf(b, "| changed to a base not in the table | %d | %s |\n", bs.OffTable, share(bs.OffTable))
	fmt.Fprintf(b, "| unreadable | %d | %s |\n", bs.Unreadable, share(bs.Unreadable))
	fmt.Fprintf(b, "\nThe chosen base has the table's lowest V in %d attempts (%s).\n", bs.LowestV, share(bs.LowestV))
}
