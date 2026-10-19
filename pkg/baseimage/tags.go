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

// Package baseimage proposes base images a candidate may switch to: tags of
// the same image, in the same distribution family, that are no older than
// the current one, each measured (compressed size, vulnerabilities) so the
// model chooses from facts instead of inventing a tag.
//
// Official images name their tags in one grammar — a version, variant words,
// a distribution: "3.12-slim-bookworm", "22-alpine3.20", "8.1-cli-alpine",
// "8.0-jammy", "17-jre-focal" — so one parser and one rule cover python,
// node, php, ruby, golang, elixir, the .NET images, eclipse-temurin, and the
// distribution images themselves.
package baseimage

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Distro is a tag's distribution: a family and, when the tag names it, a
// release ("alpine" 3.20, "debian" 12, "ubuntu" 22.04).
type Distro struct {
	Family  string
	Release []int // nil when the tag does not say (the image's default)
}

// Tag is a parsed image tag.
type Tag struct {
	Raw string
	// Version is the tag's version ("3.12" → [3 12]); nil for tags like
	// "alpine" or "lts" that name none.
	Version []int
	// Variant are the words that are neither version nor distribution, in
	// order ("slim", "cli", "apache", "jre").
	Variant []string
	Distro  Distro
	// Excluded marks tags never proposed: pre-releases, Windows and other
	// non-Linux-distribution bases.
	Excluded bool
}

var (
	debianCodenames = map[string]int{"jessie": 8, "stretch": 9, "buster": 10, "bullseye": 11, "bookworm": 12, "trixie": 13}
	// floatingAliases name no version: they follow whatever is current.
	floatingAliases = map[string]bool{"latest": true, "lts": true, "current": true, "stable": true, "mainline": true}
	ubuntuCodenames = map[string][]int{"trusty": {14, 4}, "xenial": {16, 4}, "bionic": {18, 4}, "focal": {20, 4}, "jammy": {22, 4}, "noble": {24, 4}}
	// nodeCodenames are Node.js LTS release names, used as tags.
	nodeCodenames = map[string]int{"argon": 4, "boron": 6, "carbon": 8, "dubnium": 10, "erbium": 12, "fermium": 14, "gallium": 16, "hydrogen": 18, "iron": 20, "jod": 22, "krypton": 24}

	leadingVersion = regexp.MustCompile(`^(\d+(?:\.\d+)*)`)
	alpineRelease  = regexp.MustCompile(`^alpine(\d+(?:\.\d+)*)$`)
	// excludedWord marks tags never proposed: non-Linux and non-standard
	// bases, pre-releases, and Debian's testing and unstable suites.
	excludedWord = regexp.MustCompile(`^(?:(?:windowsservercore|nanoserver|ltsc|azurelinux|cbl|mariner|ubi|distroless|chiseled)\S*|rc\d*|beta\d*|alpha\d*|dev|preview\d*|nightly|onbuild|composer|wheezy|forky|sid|testing|unstable|experimental|rolling|devel)$`)
	preRelease   = regexp.MustCompile(`\d(a|b|rc|alpha|beta)\d*$`)
)

// ParseTag parses a tag of the given repository ("library/python",
// "dotnet/aspnet"). Distribution images ("debian", "ubuntu", "alpine") carry
// their release as the version.
func ParseTag(repo, raw string) Tag {
	t := Tag{Raw: raw}
	name := repo[strings.LastIndex(repo, "/")+1:]
	words := strings.Split(strings.ToLower(raw), "-")

	// The version: leading dotted numbers ("3.12", "14.17.3", "17.0.12_7"),
	// or a codename standing for one.
	first := words[0]
	switch {
	case leadingVersion.MatchString(first):
		v := leadingVersion.FindString(first)
		rest := strings.TrimPrefix(first, v)
		if preRelease.MatchString(first) {
			t.Excluded = true
		}
		if rest != "" && !strings.HasPrefix(rest, "_") {
			t.Excluded = true // e.g. "3.13.0rc1", "1.22rc2"
		}
		t.Version = parseInts(v)
		words = words[1:]
	case name == "node" && nodeCodenames[first] > 0:
		t.Version = []int{nodeCodenames[first]}
		words = words[1:]
	case floatingAliases[first]:
		words = words[1:]
	}

	for _, w := range words {
		switch {
		case w == "":
		case excludedWord.MatchString(w):
			t.Excluded = true
		case w == "alpine":
			t.Distro = Distro{Family: "alpine"}
		case alpineRelease.MatchString(w):
			t.Distro = Distro{Family: "alpine", Release: parseInts(alpineRelease.FindStringSubmatch(w)[1])}
		case debianCodenames[w] > 0:
			t.Distro = Distro{Family: "debian", Release: []int{debianCodenames[w]}}
		case ubuntuCodenames[w] != nil:
			t.Distro = Distro{Family: "ubuntu", Release: ubuntuCodenames[w]}
		default:
			t.Variant = append(t.Variant, w)
		}
	}

	// Ubuntu's interim releases (yy.10, odd-year .04) get nine months of
	// support; only LTS releases are proposed.
	if r := t.Distro.Release; t.Distro.Family == "ubuntu" && len(r) == 2 && (r[1] != 4 || r[0]%2 != 0) {
		t.Excluded = true
	}
	if name == "ubuntu" && len(t.Version) == 2 && (t.Version[1] != 4 || t.Version[0]%2 != 0) {
		t.Excluded = true
	}

	// A distribution image's version is its release.
	switch name {
	case "debian", "ubuntu", "alpine":
		if t.Version != nil {
			t.Distro = Distro{Family: name, Release: t.Version}
		} else if t.Distro.Family == "" {
			t.Distro.Family = name
		}
		if t.Distro.Release != nil && t.Version == nil {
			t.Version = t.Distro.Release
		}
	}
	return t
}

func parseInts(s string) []int {
	var out []int
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out
		}
		out = append(out, n)
	}
	return out
}

// compareInts compares versions component by component; a missing component
// counts as 0.
func compareInts(a, b []int) int {
	for i := 0; i < max(len(a), len(b)); i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// lineDepth is how many version components name a release line whose tag
// floats over patches: python 3.12, node 22, php 8.1, dotnet 8.0, temurin 17,
// postgres 17 (from 10 on; 9.6 before). current is the current tag's version,
// whose major the candidates keep.
func lineDepth(repo string, current []int) int {
	switch repo[strings.LastIndex(repo, "/")+1:] {
	case "node", "eclipse-temurin", "debian":
		return 1
	case "postgres":
		if len(current) > 0 && current[0] < 10 {
			return 2
		}
		return 1
	}
	return 2
}

// distroImage reports a distribution image, whose major release may change
// (bullseye → bookworm) since the release is the point of the image.
func distroImage(repo string) bool {
	switch repo[strings.LastIndex(repo, "/")+1:] {
	case "debian", "ubuntu", "alpine":
		return true
	}
	return false
}

// defaultFamily is the distribution an image's tags use when they name none.
func defaultFamily(repo string) string {
	name := repo[strings.LastIndex(repo, "/")+1:]
	switch {
	case name == "eclipse-temurin":
		return "ubuntu"
	case name == "alpine":
		return "alpine"
	case name == "ubuntu":
		return "ubuntu"
	}
	return "debian"
}

func (t Tag) family(repo string) string {
	if t.Distro.Family != "" {
		return t.Distro.Family
	}
	return defaultFamily(repo)
}

// Candidates proposes up to max tags to move to from current: the newest
// release lines of the same image that are no older than the current one,
// same major version (except for distribution images), same variant words,
// same distribution family at the same release or newer. Each line is
// offered once, with the newest distribution release that has a tag; a tag
// naming the release explicitly is preferred to one that leaves it implied.
// Only floating tags (a release line, not a patch) are proposed, so the
// image keeps receiving fixes. The current tag is not among them. The
// current release line on a newer distribution release comes first, when
// there is one: it changes the least; the newest lines follow.
func Candidates(repo, current string, tags []string, max int) []string {
	cur := ParseTag(repo, current)
	depth := lineDepth(repo, cur.Version)
	family := cur.family(repo)

	type choice struct {
		tag Tag
		key string
	}
	best := map[string]choice{} // release line → best tag in it
	for _, raw := range tags {
		if raw == current {
			continue
		}
		t := ParseTag(repo, raw)
		if t.Excluded || len(t.Version) != depth {
			continue
		}
		if !sameWords(t.Variant, cur.Variant) || t.family(repo) != family {
			continue
		}
		if cur.Version != nil {
			if !distroImage(repo) && t.Version[0] != cur.Version[0] {
				continue // a new major version breaks applications
			}
			if compareInts(t.Version, truncate(cur.Version, depth)) < 0 {
				continue
			}
		}
		if cur.Distro.Release != nil && (t.Distro.Release == nil || compareInts(t.Distro.Release, cur.Distro.Release) < 0) {
			continue
		}
		if cur.Version != nil && compareInts(t.Version, truncate(cur.Version, depth)) == 0 &&
			t.Distro.Release != nil && cur.Distro.Release != nil && compareInts(t.Distro.Release, cur.Distro.Release) == 0 {
			continue // the current image under another name (bookworm-slim, 12-slim)
		}
		if repo[strings.LastIndex(repo, "/")+1:] == "node" && t.Version[0]%2 == 1 {
			continue // odd Node.js majors never become LTS
		}
		line := fmtInts(t.Version)
		prev, ok := best[line]
		if !ok || betterDistro(t, prev.tag) {
			best[line] = choice{tag: t, key: line}
		}
	}

	var lines []choice
	for _, c := range best {
		lines = append(lines, c)
	}
	currentLine := ""
	if cur.Version != nil {
		currentLine = fmtInts(truncate(cur.Version, depth))
	}
	sort.Slice(lines, func(i, j int) bool {
		if a, b := lines[i].key == currentLine, lines[j].key == currentLine; a != b {
			return a
		}
		return compareInts(lines[i].tag.Version, lines[j].tag.Version) > 0
	})
	var out []string
	for _, c := range lines {
		if len(out) == max {
			break
		}
		out = append(out, c.tag.Raw)
	}
	return out
}

func fmtInts(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

func truncate(v []int, n int) []int {
	if len(v) > n {
		return v[:n]
	}
	return v
}

// betterDistro prefers the newer distribution release, then an explicit
// release over an implied one (it makes the release known to the checks),
// then the shorter tag.
func betterDistro(a, b Tag) bool {
	switch {
	case a.Distro.Release != nil && b.Distro.Release == nil:
		return true
	case a.Distro.Release == nil && b.Distro.Release != nil:
		return false
	case a.Distro.Release != nil:
		if c := compareInts(a.Distro.Release, b.Distro.Release); c != 0 {
			return c > 0
		}
	}
	if len(a.Raw) != len(b.Raw) {
		return len(a.Raw) < len(b.Raw)
	}
	return a.Raw < b.Raw
}

func sameWords(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
