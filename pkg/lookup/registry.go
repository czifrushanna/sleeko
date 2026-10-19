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

// Package lookup answers "does this version exist?" for the things a
// Dockerfile pins: base image tags in public registries, apt package versions
// in the Debian and Ubuntu archives, and pip package versions on PyPI. Every
// answer is cached for the life of the Client, so a candidate pinning the
// same things as the last costs nothing.
package lookup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Client looks versions up. The zero value uses the public services; the
// endpoint fields exist for tests and mirrors.
type Client struct {
	HTTP *http.Client

	// PlainHTTPRegistries are registry hosts reached over http, not https.
	PlainHTTPRegistries map[string]bool

	DebianMirror          string // default http://deb.debian.org/debian
	DebianSecurity        string // default http://security.debian.org/debian-security
	DebianArchive         string // default http://archive.debian.org/debian (releases moved off the mirrors)
	DebianArchiveSecurity string // default http://archive.debian.org/debian-security
	UbuntuMirror          string // default http://archive.ubuntu.com/ubuntu
	UbuntuSecurity        string // default http://security.ubuntu.com/ubuntu
	PyPI                  string // default https://pypi.org/pypi

	mu       sync.Mutex
	tokens   map[string]string
	tags     map[string]bool
	indexes  map[string]*aptIndex
	inflight map[string]*sync.Mutex
	pypi     map[string][]string
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func or(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// keyLock serialises work on one cache key, so concurrent callers fetch an
// index once.
func (c *Client) keyLock(key string) func() {
	c.mu.Lock()
	if c.inflight == nil {
		c.inflight = map[string]*sync.Mutex{}
	}
	l, ok := c.inflight[key]
	if !ok {
		l = &sync.Mutex{}
		c.inflight[key] = l
	}
	c.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// ImageRef is a parsed image reference.
type ImageRef struct {
	Registry   string // e.g. registry-1.docker.io, ghcr.io
	Repository string // e.g. library/python
	Reference  string // tag or digest
}

var digestRef = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ParseImageRef resolves a FROM reference the way Docker does: no registry
// means Docker Hub, a single-name repository there means library/, no tag
// means latest.
func ParseImageRef(ref string) (ImageRef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.ContainsAny(ref, "$ ") {
		return ImageRef{}, fmt.Errorf("not a concrete image reference: %q", ref)
	}
	name, reference := ref, "latest"
	if i := strings.Index(ref, "@"); i >= 0 {
		name, reference = ref[:i], ref[i+1:]
		if !digestRef.MatchString(reference) {
			return ImageRef{}, fmt.Errorf("bad digest in %q", ref)
		}
	} else if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		name, reference = ref[:i], ref[i+1:]
	}

	registry := "registry-1.docker.io"
	if first, rest, ok := strings.Cut(name, "/"); ok && (strings.ContainsAny(first, ".:") || first == "localhost") {
		registry, name = first, rest
	}
	if registry == "docker.io" || registry == "index.docker.io" {
		registry = "registry-1.docker.io"
	}
	if registry == "registry-1.docker.io" && !strings.Contains(name, "/") {
		name = "library/" + name
	}
	if name == "" || reference == "" {
		return ImageRef{}, fmt.Errorf("not a concrete image reference: %q", ref)
	}
	return ImageRef{Registry: registry, Repository: strings.ToLower(name), Reference: reference}, nil
}

// manifestTypes are the manifest media types a tag may resolve to.
var manifestTypes = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

// TagExists reports whether the image reference resolves in its registry.
// Registries that want a token get an anonymous one (the Bearer flow every
// public registry implements); HEAD requests do not count against Docker
// Hub's pull limit.
func (c *Client) TagExists(ctx context.Context, ref string) (bool, error) {
	r, err := ParseImageRef(ref)
	if err != nil {
		return false, err
	}
	key := r.Registry + "/" + r.Repository + ":" + r.Reference

	c.mu.Lock()
	if exists, ok := c.tags[key]; ok {
		c.mu.Unlock()
		return exists, nil
	}
	c.mu.Unlock()

	scheme := "https"
	if c.PlainHTTPRegistries[r.Registry] {
		scheme = "http"
	}
	u := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", scheme, r.Registry, r.Repository, r.Reference)

	resp, err := c.head(ctx, u, "")
	if err != nil {
		return false, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		token, err := c.anonymousToken(ctx, resp.Header.Get("WWW-Authenticate"))
		if err != nil {
			return false, fmt.Errorf("%s: %w", ref, err)
		}
		if resp, err = c.head(ctx, u, token); err != nil {
			return false, err
		}
	}

	var exists bool
	switch resp.StatusCode {
	case http.StatusOK:
		exists = true
	case http.StatusNotFound:
		exists = false
	case http.StatusUnauthorized, http.StatusForbidden:
		// Anonymous access was refused: a private image, or a repository
		// that does not exist (Docker Hub answers both the same way).
		exists = false
	default:
		return false, fmt.Errorf("%s: registry answered %s", ref, resp.Status)
	}

	c.mu.Lock()
	if c.tags == nil {
		c.tags = map[string]bool{}
	}
	c.tags[key] = exists
	c.mu.Unlock()
	return exists, nil
}

func (c *Client) head(ctx context.Context, u, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", manifestTypes)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	return resp, nil
}

var challengeParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

// anonymousToken fetches a token from the realm a Bearer challenge names.
func (c *Client) anonymousToken(ctx context.Context, challenge string) (string, error) {
	scheme, params, _ := strings.Cut(challenge, " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return "", fmt.Errorf("unsupported auth challenge %q", challenge)
	}
	p := map[string]string{}
	for _, m := range challengeParam.FindAllStringSubmatch(params, -1) {
		p[m[1]] = m[2]
	}
	if p["realm"] == "" {
		return "", fmt.Errorf("auth challenge without realm: %q", challenge)
	}
	q := url.Values{}
	for _, k := range []string{"service", "scope"} {
		if p[k] != "" {
			q.Set(k, p[k])
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p["realm"]+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request answered %s", resp.Status)
	}
	var t struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&t); err != nil {
		return "", fmt.Errorf("token response: %w", err)
	}
	return or(t.Token, t.AccessToken), nil
}

// get fetches a registry URL, getting an anonymous token when challenged.
// Tokens are reused per repository: listing a repository's tags takes many
// paginated requests.
func (c *Client) get(ctx context.Context, r ImageRef, u, accept string) (*http.Response, error) {
	do := func(token string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return c.http().Do(req)
	}
	key := r.Registry + "/" + r.Repository
	c.mu.Lock()
	token := c.tokens[key]
	c.mu.Unlock()

	resp, err := do(token)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	challenge := resp.Header.Get("WWW-Authenticate")
	resp.Body.Close()
	if token, err = c.anonymousToken(ctx, challenge); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.tokens == nil {
		c.tokens = map[string]string{}
	}
	c.tokens[key] = token
	c.mu.Unlock()
	return do(token)
}

func (c *Client) registryURL(r ImageRef, path string) string {
	scheme := "https"
	if c.PlainHTTPRegistries[r.Registry] {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/v2/%s/%s", scheme, r.Registry, r.Repository, path)
}

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// ListTags lists a repository's tags (ref's tag, if any, is ignored),
// following the registry's pagination.
func (c *Client) ListTags(ctx context.Context, ref string) ([]string, error) {
	r, err := ParseImageRef(ref)
	if err != nil {
		return nil, err
	}
	u := c.registryURL(r, "tags/list?n=1000")
	var tags []string
	for pages := 0; u != "" && pages < 100; pages++ {
		resp, err := c.get(ctx, r, u, "")
		if err != nil {
			return nil, err
		}
		var page struct {
			Tags []string `json:"tags"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&page)
		link := resp.Header.Get("Link")
		status := resp.Status
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list tags of %s: %s", r.Repository, status)
		}
		if err != nil {
			return nil, fmt.Errorf("list tags of %s: %w", r.Repository, err)
		}
		tags = append(tags, page.Tags...)

		u = ""
		if m := nextLink.FindStringSubmatch(link); m != nil {
			next, err := url.Parse(m[1])
			if err != nil {
				return nil, err
			}
			base, _ := url.Parse(c.registryURL(r, ""))
			u = base.ResolveReference(next).String()
		}
	}
	return tags, nil
}

// ImageSize is an image's linux/amd64 manifest digest and compressed size.
type ImageSize struct {
	Digest string
	// Size is the sum of the manifest's compressed layer sizes, the size
	// the metric compares.
	Size int64
}

// ImageSize resolves ref to its linux/amd64 image manifest and sums its
// layers. A multi-platform index is followed to the amd64 entry.
func (c *Client) ImageSize(ctx context.Context, ref string) (ImageSize, error) {
	r, err := ParseImageRef(ref)
	if err != nil {
		return ImageSize{}, err
	}
	reference := r.Reference
	for depth := 0; depth < 2; depth++ {
		resp, err := c.get(ctx, r, c.registryURL(r, "manifests/"+reference), manifestTypes)
		if err != nil {
			return ImageSize{}, err
		}
		var m struct {
			MediaType string `json:"mediaType"`
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
		err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&m)
		digest := resp.Header.Get("Docker-Content-Digest")
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return ImageSize{}, fmt.Errorf("manifest of %s: %s", ref, resp.Status)
		}
		if err != nil {
			return ImageSize{}, fmt.Errorf("manifest of %s: %w", ref, err)
		}
		if len(m.Manifests) == 0 {
			var size int64
			for _, l := range m.Layers {
				size += l.Size
			}
			if digest == "" && strings.HasPrefix(reference, "sha256:") {
				digest = reference
			}
			return ImageSize{Digest: digest, Size: size}, nil
		}
		reference = ""
		for _, e := range m.Manifests {
			if e.Platform.OS == "linux" && e.Platform.Architecture == "amd64" {
				reference = e.Digest
				break
			}
		}
		if reference == "" {
			return ImageSize{}, fmt.Errorf("%s has no linux/amd64 image", ref)
		}
	}
	return ImageSize{}, fmt.Errorf("%s: nested manifest lists", ref)
}
