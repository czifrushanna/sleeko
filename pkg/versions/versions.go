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

// Package versions compares package versions the way the package managers
// do: Debian's dpkg and Python's PEP 440. Deciding whether a pinned version
// is at least the fixed one needs the real ordering — "1.0~rc1" is older than
// "1.0", "1:1.0" newer than "2.0", "1.0.post1" newer than "1.0" — which a
// generic dotted-number comparison gets wrong.
package versions

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Deb compares two Debian package versions as `dpkg --compare-versions`
// does (Debian Policy §5.6.12): negative if a < b, 0 if equal, positive if
// a > b.
func Deb(a, b string) (int, error) {
	ea, ua, ra, err := splitDeb(a)
	if err != nil {
		return 0, err
	}
	eb, ub, rb, err := splitDeb(b)
	if err != nil {
		return 0, err
	}
	if ea != eb {
		return sign(ea - eb), nil
	}
	if c := verrevcmp(ua, ub); c != 0 {
		return sign(c), nil
	}
	return sign(verrevcmp(ra, rb)), nil
}

// splitDeb splits [epoch:]upstream[-revision].
func splitDeb(v string) (epoch int, upstream, revision string, err error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, "", "", fmt.Errorf("empty Debian version")
	}
	if e, rest, ok := strings.Cut(v, ":"); ok {
		epoch, err = strconv.Atoi(e)
		if err != nil || epoch < 0 {
			return 0, "", "", fmt.Errorf("bad epoch in Debian version %q", v)
		}
		v = rest
	}
	upstream = v
	if i := strings.LastIndex(v, "-"); i >= 0 {
		upstream, revision = v[:i], v[i+1:]
	}
	if upstream == "" || !isDigit(upstream[0]) {
		return 0, "", "", fmt.Errorf("Debian version %q must start with a digit", v)
	}
	return epoch, upstream, revision, nil
}

// order is dpkg's character weight: '~' before everything (even the end of
// the string), then digits and the end, then letters, then other characters.
func order(s string, i int) int {
	if i >= len(s) {
		return 0
	}
	c := s[i]
	switch {
	case isDigit(c):
		return 0
	case isLetter(c):
		return int(c)
	case c == '~':
		return -1
	default:
		return int(c) + 256
	}
}

// verrevcmp is dpkg's comparison of an upstream version or a revision:
// alternating non-digit runs (compared by order) and digit runs (compared
// numerically).
func verrevcmp(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		for (i < len(a) && !isDigit(a[i])) || (j < len(b) && !isDigit(b[j])) {
			if ac, bc := order(a, i), order(b, j); ac != bc {
				return ac - bc
			}
			i++
			j++
		}
		for i < len(a) && a[i] == '0' {
			i++
		}
		for j < len(b) && b[j] == '0' {
			j++
		}
		firstDiff := 0
		for i < len(a) && isDigit(a[i]) && j < len(b) && isDigit(b[j]) {
			if firstDiff == 0 {
				firstDiff = int(a[i]) - int(b[j])
			}
			i++
			j++
		}
		if i < len(a) && isDigit(a[i]) {
			return 1
		}
		if j < len(b) && isDigit(b[j]) {
			return -1
		}
		if firstDiff != 0 {
			return firstDiff
		}
	}
	return 0
}

func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// pep440 is the PEP 440 version grammar, as in Python's `packaging` library,
// including its accepted alternative spellings.
var pep440 = regexp.MustCompile(`^\s*v?` +
	`(?:(?P<epoch>[0-9]+)!)?` +
	`(?P<release>[0-9]+(?:\.[0-9]+)*)` +
	`(?:[-_.]?(?P<pre_l>alpha|a|beta|b|preview|pre|c|rc)[-_.]?(?P<pre_n>[0-9]+)?)?` +
	`(?:(?:-(?P<post_n1>[0-9]+))|(?:[-_.]?(?P<post_l>post|rev|r)[-_.]?(?P<post_n2>[0-9]+)?))?` +
	`(?:[-_.]?(?P<dev_l>dev)[-_.]?(?P<dev_n>[0-9]+)?)?` +
	`(?:\+(?P<local>[a-z0-9]+(?:[-_.][a-z0-9]+)*))?\s*$`)

// PEP440Version is a parsed PEP 440 version.
type PEP440Version struct {
	Epoch   int
	Release []int
	// Pre is the pre-release: phase 0 (a), 1 (b) or 2 (rc), and number.
	Pre     *[2]int
	Post    *int
	Dev     *int
	Local   []string
	literal string
}

// ParsePEP440 parses a version, normalising the alternative spellings PEP
// 440 allows ("1.0-alpha.1", "1.0.post-1", "v1.0").
func ParsePEP440(s string) (PEP440Version, error) {
	m := pep440.FindStringSubmatch(strings.ToLower(s))
	if m == nil {
		return PEP440Version{}, fmt.Errorf("not a PEP 440 version: %q", s)
	}
	g := func(name string) string { return m[pep440.SubexpIndex(name)] }
	num := func(s string) int {
		n, _ := strconv.Atoi(s)
		return n
	}

	v := PEP440Version{Epoch: num(g("epoch")), literal: s}
	for _, part := range strings.Split(g("release"), ".") {
		v.Release = append(v.Release, num(part))
	}
	if l := g("pre_l"); l != "" {
		phase := map[string]int{"a": 0, "alpha": 0, "b": 1, "beta": 1, "c": 2, "rc": 2, "pre": 2, "preview": 2}[l]
		v.Pre = &[2]int{phase, num(g("pre_n"))}
	}
	if n := g("post_n1"); n != "" {
		p := num(n)
		v.Post = &p
	} else if g("post_l") != "" {
		p := num(g("post_n2"))
		v.Post = &p
	}
	if g("dev_l") != "" {
		d := num(g("dev_n"))
		v.Dev = &d
	}
	if l := g("local"); l != "" {
		v.Local = strings.FieldsFunc(l, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	}
	return v, nil
}

// PEP440 compares two versions by PEP 440's ordering: negative if a < b, 0
// if equal, positive if a > b.
func PEP440(a, b string) (int, error) {
	va, err := ParsePEP440(a)
	if err != nil {
		return 0, err
	}
	vb, err := ParsePEP440(b)
	if err != nil {
		return 0, err
	}
	return va.Compare(vb), nil
}

// Compare orders versions as packaging's sort key does: epoch, release
// (trailing zeros ignored), then a dev release before any pre-release, a
// pre-release before the release, a post-release after it, and a local
// version after the same public version.
func (v PEP440Version) Compare(o PEP440Version) int {
	if v.Epoch != o.Epoch {
		return sign(v.Epoch - o.Epoch)
	}
	for i := 0; i < max(len(v.Release), len(o.Release)); i++ {
		if c := sign(at(v.Release, i) - at(o.Release, i)); c != 0 {
			return c
		}
	}
	if c := compareKeys(v.preKey(), o.preKey()); c != 0 {
		return c
	}
	if c := compareKeys(v.postKey(), o.postKey()); c != 0 {
		return c
	}
	if c := compareKeys(v.devKey(), o.devKey()); c != 0 {
		return c
	}
	return compareLocal(v.Local, o.Local)
}

func at(xs []int, i int) int {
	if i < len(xs) {
		return xs[i]
	}
	return 0
}

// Sort keys use a leading rank for packaging's ±infinity sentinels:
// rank -1 sorts first, +1 last, 0 compares the values that follow.

func (v PEP440Version) preKey() []int {
	switch {
	case v.Pre == nil && v.Post == nil && v.Dev != nil:
		return []int{-1} // 1.0.dev0 comes before 1.0a0
	case v.Pre == nil:
		return []int{1}
	}
	return []int{0, v.Pre[0], v.Pre[1]}
}

func (v PEP440Version) postKey() []int {
	if v.Post == nil {
		return []int{-1}
	}
	return []int{0, *v.Post}
}

func (v PEP440Version) devKey() []int {
	if v.Dev == nil {
		return []int{1}
	}
	return []int{0, *v.Dev}
}

func compareKeys(a, b []int) int {
	for i := 0; i < min(len(a), len(b)); i++ {
		if a[i] != b[i] {
			return sign(a[i] - b[i])
		}
	}
	return sign(len(a) - len(b))
}

// compareLocal orders local version labels: none before any; segment by
// segment, numbers above strings, numbers numerically, strings lexically,
// and a longer label above its prefix.
func compareLocal(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return -1
	case len(b) == 0:
		return 1
	}
	for i := 0; i < min(len(a), len(b)); i++ {
		na, errA := strconv.Atoi(a[i])
		nb, errB := strconv.Atoi(b[i])
		switch {
		case errA == nil && errB == nil:
			if na != nb {
				return sign(na - nb)
			}
		case errA == nil:
			return 1
		case errB == nil:
			return -1
		default:
			if c := strings.Compare(a[i], b[i]); c != 0 {
				return c
			}
		}
	}
	return sign(len(a) - len(b))
}

// String is the version as given.
func (v PEP440Version) String() string { return v.literal }
