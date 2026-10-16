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

package lookup

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ulikunitz/xz"
)

// Release identifies a Debian or Ubuntu release, e.g. {debian, bookworm}.
type Release struct {
	Distro string // "debian" or "ubuntu"
	Suite  string // codename
}

// releases maps an os-release version, as Trivy reports it ("12.8",
// "22.04"), to the release's codename.
var releases = map[string]map[string]string{
	"debian": {"8": "jessie", "9": "stretch", "10": "buster", "11": "bullseye", "12": "bookworm", "13": "trixie"},
	"ubuntu": {"14.04": "trusty", "16.04": "xenial", "18.04": "bionic", "20.04": "focal", "22.04": "jammy", "24.04": "noble"},
}

// ReleaseOf maps a distribution and version as Trivy reports them ("debian",
// "12.8") to a release; ok is false for anything else.
func ReleaseOf(distro, version string) (Release, bool) {
	distro = strings.ToLower(distro)
	codes, ok := releases[distro]
	if !ok {
		return Release{}, false
	}
	key := version
	if distro == "debian" {
		key, _, _ = strings.Cut(version, ".")
	}
	suite, ok := codes[key]
	return Release{Distro: distro, Suite: suite}, ok
}

// ReleaseFromTag guesses the release an image tag names, from the codename
// official images put in their tags ("python:3.12-slim-bookworm",
// "ubuntu:jammy", "node:20-bullseye"); ok is false when the tag names none.
func ReleaseFromTag(ref string) (Release, bool) {
	tag := strings.ToLower(ref)
	if i := strings.LastIndex(tag, ":"); i > strings.LastIndex(tag, "/") {
		tag = tag[i+1:]
	}
	if strings.HasPrefix(ref, "ubuntu:") || strings.HasPrefix(ref, "docker.io/library/ubuntu:") {
		for version, suite := range releases["ubuntu"] {
			if tag == version {
				return Release{"ubuntu", suite}, true
			}
		}
	}
	for distro, codes := range releases {
		for _, suite := range codes {
			for _, part := range strings.FieldsFunc(tag, func(r rune) bool { return r == '-' || r == '.' || r == '_' }) {
				if part == suite {
					return Release{distro, suite}, true
				}
			}
		}
	}
	return Release{}, false
}

// aptIndex is every package version available for one release on amd64.
type aptIndex struct {
	versions map[string]map[string]bool
}

// AptHas reports whether the release's archive offers the package at the
// version, in the release itself or its -updates or -security suite. These
// are what `apt-get install pkg=version` can install today: superseded
// versions leave the archive.
func (c *Client) AptHas(ctx context.Context, rel Release, pkg, version string) (bool, error) {
	idx, err := c.aptIndex(ctx, rel)
	if err != nil {
		return false, err
	}
	return idx.versions[pkg][version], nil
}

// AptVersions lists the package's available versions, for diagnostics.
func (c *Client) AptVersions(ctx context.Context, rel Release, pkg string) ([]string, error) {
	idx, err := c.aptIndex(ctx, rel)
	if err != nil {
		return nil, err
	}
	var out []string
	for v := range idx.versions[pkg] {
		out = append(out, v)
	}
	return out, nil
}

// indexURL is one Packages index of a release.
type indexURL struct {
	url      string
	optional bool // absent for some releases (no -updates on an archived one)
}

func (c *Client) indexURLs(rel Release) ([][]indexURL, error) {
	const path = "binary-amd64/Packages"
	switch rel.Distro {
	case "debian":
		mirror := or(c.DebianMirror, "http://deb.debian.org/debian")
		security := or(c.DebianSecurity, "http://security.debian.org/debian-security")
		archive := or(c.DebianArchive, "http://archive.debian.org/debian")
		archiveSec := or(c.DebianArchiveSecurity, "http://archive.debian.org/debian-security")
		// Security suites were named "<suite>/updates" before bullseye.
		secSuite := rel.Suite + "-security"
		if rel.Suite == "jessie" || rel.Suite == "stretch" || rel.Suite == "buster" {
			secSuite = rel.Suite + "/updates"
		}
		set := func(m, s string) []indexURL {
			return []indexURL{
				{url: fmt.Sprintf("%s/dists/%s/main/%s.xz", m, rel.Suite, path)},
				{url: fmt.Sprintf("%s/dists/%s-updates/main/%s.xz", m, rel.Suite, path), optional: true},
				{url: fmt.Sprintf("%s/dists/%s/main/%s.xz", s, secSuite, path), optional: true},
			}
		}
		// The live mirrors first; releases past their life move to the archive.
		return [][]indexURL{set(mirror, security), set(archive, archiveSec)}, nil
	case "ubuntu":
		mirror := or(c.UbuntuMirror, "http://archive.ubuntu.com/ubuntu")
		security := or(c.UbuntuSecurity, "http://security.ubuntu.com/ubuntu")
		var set []indexURL
		for _, comp := range []string{"main", "universe"} {
			set = append(set,
				indexURL{url: fmt.Sprintf("%s/dists/%s/%s/%s.gz", mirror, rel.Suite, comp, path)},
				indexURL{url: fmt.Sprintf("%s/dists/%s-updates/%s/%s.gz", mirror, rel.Suite, comp, path), optional: true},
				indexURL{url: fmt.Sprintf("%s/dists/%s-security/%s/%s.gz", security, rel.Suite, comp, path), optional: true},
			)
		}
		return [][]indexURL{set}, nil
	}
	return nil, fmt.Errorf("no package archive known for %s", rel.Distro)
}

var errNotFound = errors.New("not found")

func (c *Client) aptIndex(ctx context.Context, rel Release) (*aptIndex, error) {
	key := rel.Distro + "/" + rel.Suite
	defer c.keyLock("apt:" + key)()

	c.mu.Lock()
	if idx, ok := c.indexes[key]; ok {
		c.mu.Unlock()
		return idx, nil
	}
	c.mu.Unlock()

	sets, err := c.indexURLs(rel)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, set := range sets {
		idx := &aptIndex{versions: map[string]map[string]bool{}}
		ok := true
		for _, u := range set {
			err := c.readPackages(ctx, u.url, idx)
			if errors.Is(err, errNotFound) && u.optional {
				continue
			}
			if err != nil {
				lastErr, ok = err, false
				break
			}
		}
		if ok {
			c.mu.Lock()
			if c.indexes == nil {
				c.indexes = map[string]*aptIndex{}
			}
			c.indexes[key] = idx
			c.mu.Unlock()
			return idx, nil
		}
	}
	return nil, fmt.Errorf("package index for %s: %w", key, lastErr)
}

// readPackages adds a Packages index's package versions to idx.
func (c *Client) readPackages(ctx context.Context, u string, idx *aptIndex) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s: %w", u, errNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", u, resp.Status)
	}

	var r io.Reader
	switch {
	case strings.HasSuffix(u, ".xz"):
		if r, err = xz.NewReader(resp.Body); err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
	case strings.HasSuffix(u, ".gz"):
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
		defer gz.Close()
		r = gz
	default:
		r = resp.Body
	}
	if err := parsePackages(r, idx); err != nil {
		return fmt.Errorf("%s: %w", u, err)
	}
	return nil
}

// parsePackages reads the Package and Version fields of each stanza.
func parsePackages(r io.Reader, idx *aptIndex) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	var pkg string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			pkg = ""
		case strings.HasPrefix(line, "Package: "):
			pkg = strings.TrimSpace(line[len("Package: "):])
		case strings.HasPrefix(line, "Version: ") && pkg != "":
			if idx.versions[pkg] == nil {
				idx.versions[pkg] = map[string]bool{}
			}
			idx.versions[pkg][strings.TrimSpace(line[len("Version: "):])] = true
		}
	}
	return sc.Err()
}
