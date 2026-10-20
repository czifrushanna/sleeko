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
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"
)

func TestParseImageRef(t *testing.T) {
	cases := map[string]ImageRef{
		"python":                                    {"registry-1.docker.io", "library/python", "latest"},
		"python:3.12-slim":                          {"registry-1.docker.io", "library/python", "3.12-slim"},
		"docker.io/library/node:20":                 {"registry-1.docker.io", "library/node", "20"},
		"bitnami/redis:7.2":                         {"registry-1.docker.io", "bitnami/redis", "7.2"},
		"ghcr.io/astral-sh/uv:latest":               {"ghcr.io", "astral-sh/uv", "latest"},
		"gcr.io/distroless/static-debian12:nonroot": {"gcr.io", "distroless/static-debian12", "nonroot"},
		"mcr.microsoft.com/dotnet/aspnet:8.0":       {"mcr.microsoft.com", "dotnet/aspnet", "8.0"},
		"localhost:5000/app:dev":                    {"localhost:5000", "app", "dev"},
		"alpine@sha256:" + strings.Repeat("a", 64):  {"registry-1.docker.io", "library/alpine", "sha256:" + strings.Repeat("a", 64)},
		// A tag beside a digest is ignored: Docker pulls by the digest.
		"golang:1.27.2@sha256:" + strings.Repeat("b", 64):          {"registry-1.docker.io", "library/golang", "sha256:" + strings.Repeat("b", 64)},
		"localhost:5000/app:dev@sha256:" + strings.Repeat("c", 64): {"localhost:5000", "app", "sha256:" + strings.Repeat("c", 64)},
	}
	for in, want := range cases {
		got, err := ParseImageRef(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "${BASE}", "python:${TAG}", "alpine@sha256:short"} {
		_, err := ParseImageRef(bad)
		require.Error(t, err, bad)
	}
	_, err := ParseImageRef("python:3.11-slim@sha256:your-verified-digest-here")
	require.ErrorIs(t, err, ErrBadDigest)
}

// fakeRegistry demands an anonymous token, like Docker Hub, and knows one tag.
func fakeRegistry(t *testing.T) (*httptest.Server, *atomic.Int32) {
	var tokens atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			require.Equal(t, "repository:library/python:pull", r.URL.Query().Get("scope"))
			tokens.Add(1)
			fmt.Fprint(w, `{"token":"anon"}`)
		case r.Header.Get("Authorization") != "Bearer anon":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="registry.test",scope="repository:library/python:pull"`, srv.URL))
			w.WriteHeader(http.StatusUnauthorized)
		case r.Method == http.MethodHead && r.URL.Path == "/v2/library/python/manifests/3.12-slim":
			require.Contains(t, r.Header.Get("Accept"), "application/vnd.oci.image.index.v1+json")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v2/library/python/tags/list" && r.URL.Query().Get("last") == "":
			w.Header().Set("Link", `</v2/library/python/tags/list?n=1000&last=3.12>; rel="next"`)
			fmt.Fprint(w, `{"name":"library/python","tags":["3.11","3.12"]}`)
		case r.URL.Path == "/v2/library/python/tags/list":
			fmt.Fprint(w, `{"name":"library/python","tags":["3.12-slim"]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/library/python/manifests/3.12-slim":
			w.Header().Set("Docker-Content-Digest", "sha256:index")
			fmt.Fprint(w, `{"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[`+
				`{"digest":"sha256:arm","platform":{"os":"linux","architecture":"arm64"}},`+
				`{"digest":"sha256:amd","platform":{"os":"linux","architecture":"amd64"}}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/library/python/manifests/sha256:amd":
			fmt.Fprint(w, `{"layers":[{"size":100},{"size":23}]}`)
		case r.URL.Path == "/v2/library/python/manifests/windows":
			fmt.Fprint(w, `{"manifests":[{"digest":"sha256:win","platform":{"os":"windows","architecture":"amd64"}}]}`)
		case r.URL.Path == "/v2/library/python/manifests/nested":
			fmt.Fprint(w, `{"manifests":[{"digest":"nested","platform":{"os":"linux","architecture":"amd64"}}]}`)
		case r.URL.Path == "/v2/library/python/manifests/garbage":
			fmt.Fprint(w, `{`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &tokens
}

func TestTagExists(t *testing.T) {
	srv, tokens := fakeRegistry(t)
	host := strings.TrimPrefix(srv.URL, "http://")
	c := &Client{PlainHTTPRegistries: map[string]bool{host: true}}
	ctx := context.Background()

	ok, err := c.TagExists(ctx, host+"/library/python:3.12-slim")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = c.TagExists(ctx, host+"/library/python:3.99")
	require.NoError(t, err)
	require.False(t, ok, "an invented tag")

	_, err = c.TagExists(ctx, host+"/library/python:3.12-slim")
	require.NoError(t, err)
	require.Equal(t, int32(2), tokens.Load(), "answers are cached")

	_, err = c.TagExists(ctx, "python:${TAG}")
	require.Error(t, err)
}

func TestListTags(t *testing.T) {
	srv, tokens := fakeRegistry(t)
	host := strings.TrimPrefix(srv.URL, "http://")
	c := &Client{PlainHTTPRegistries: map[string]bool{host: true}}
	ctx := context.Background()

	tags, err := c.ListTags(ctx, host+"/library/python:3.12")
	require.NoError(t, err)
	require.Equal(t, []string{"3.11", "3.12", "3.12-slim"}, tags, "both pages")
	require.Equal(t, int32(1), tokens.Load(), "the token is reused across pages")

	_, err = c.ListTags(ctx, host+"/library/nothere")
	require.ErrorContains(t, err, "404 Not Found")
	_, err = c.ListTags(ctx, "python:${TAG}")
	require.Error(t, err)
}

func TestImageSize(t *testing.T) {
	srv, _ := fakeRegistry(t)
	host := strings.TrimPrefix(srv.URL, "http://")
	c := &Client{PlainHTTPRegistries: map[string]bool{host: true}}
	ctx := context.Background()

	got, err := c.ImageSize(ctx, host+"/library/python:3.12-slim")
	require.NoError(t, err)
	require.Equal(t, ImageSize{Digest: "sha256:amd", Size: 123}, got, "the index is followed to amd64")

	for ref, want := range map[string]string{
		"windows": "has no linux/amd64 image",
		"nested":  "nested manifest lists",
		"garbage": "unexpected EOF",
		"3.99":    "404",
	} {
		_, err := c.ImageSize(ctx, host+"/library/python:"+ref)
		require.ErrorContains(t, err, want, ref)
	}
	_, err = c.ImageSize(ctx, "python:${TAG}")
	require.Error(t, err)
}

func TestTagExists_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	_, err := (&Client{PlainHTTPRegistries: map[string]bool{host: true}}).TagExists(context.Background(), host+"/x:1")
	require.ErrorContains(t, err, "429")
}

func xzBytes(t *testing.T, s string) []byte {
	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	require.NoError(t, err)
	_, err = w.Write([]byte(s))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func gzBytes(t *testing.T, s string) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write([]byte(s))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// archive serves files by path, counting requests.
func archive(t *testing.T, files map[string][]byte) (*httptest.Server, *atomic.Int32) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		data, ok := files[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

const bookwormMain = `Package: curl
Version: 7.88.1-10+deb12u5
Architecture: amd64

Package: libssl3
Version: 3.0.11-1~deb12u2
Architecture: amd64
`

const bookwormSecurity = `Package: libssl3
Version: 3.0.13-1~deb12u1
Architecture: amd64
`

func TestAptHas_Debian(t *testing.T) {
	srv, hits := archive(t, map[string][]byte{
		"/debian/dists/bookworm/main/binary-amd64/Packages.xz":                   xzBytes(t, bookwormMain),
		"/debian-security/dists/bookworm-security/main/binary-amd64/Packages.xz": xzBytes(t, bookwormSecurity),
		// No bookworm-updates index: optional.
		"/archive/dists/buster/main/binary-amd64/Packages.xz":                  xzBytes(t, "Package: curl\nVersion: 7.64.0-4+deb10u2\n"),
		"/archive-security/dists/buster/updates/main/binary-amd64/Packages.xz": xzBytes(t, "Package: curl\nVersion: 7.64.0-4+deb10u9\n"),
	})
	c := &Client{
		DebianMirror: srv.URL + "/debian", DebianSecurity: srv.URL + "/debian-security",
		DebianArchive: srv.URL + "/archive", DebianArchiveSecurity: srv.URL + "/archive-security",
	}
	ctx := context.Background()
	bookworm := Release{"debian", "bookworm"}

	for _, c2 := range []struct {
		pkg, version string
		want         bool
	}{
		{"curl", "7.88.1-10+deb12u5", true},
		{"libssl3", "3.0.13-1~deb12u1", true}, // only in the security suite
		{"libssl3", "3.0.14-1~deb12u9", false},
		{"nonexistent", "1.0", false},
	} {
		got, err := c.AptHas(ctx, bookworm, c2.pkg, c2.version)
		require.NoError(t, err)
		require.Equal(t, c2.want, got, "%s=%s", c2.pkg, c2.version)
	}
	before := hits.Load()
	vs, err := c.AptVersions(ctx, bookworm, "libssl3")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"3.0.11-1~deb12u2", "3.0.13-1~deb12u1"}, vs)
	require.Equal(t, before, hits.Load(), "the index is fetched once")

	// An archived release falls back to archive.debian.org, with the old
	// "<suite>/updates" security layout.
	got, err := c.AptHas(ctx, Release{"debian", "buster"}, "curl", "7.64.0-4+deb10u9")
	require.NoError(t, err)
	require.True(t, got)

	_, err = c.AptHas(ctx, Release{"debian", "nosuchsuite"}, "curl", "1")
	require.Error(t, err)
}

func TestAptHas_Ubuntu(t *testing.T) {
	srv, _ := archive(t, map[string][]byte{
		"/ubuntu/dists/jammy/main/binary-amd64/Packages.gz":            gzBytes(t, "Package: openssl\nVersion: 3.0.2-0ubuntu1\n"),
		"/ubuntu/dists/jammy/universe/binary-amd64/Packages.gz":        gzBytes(t, "Package: jq\nVersion: 1.6-2.1ubuntu3\n"),
		"/security/dists/jammy-security/main/binary-amd64/Packages.gz": gzBytes(t, "Package: openssl\nVersion: 3.0.2-0ubuntu1.18\n"),
		"/ubuntu/dists/jammy-updates/main/binary-amd64/Packages.gz":    gzBytes(t, "Package: openssl\nVersion: 3.0.2-0ubuntu1.15\n"),
	})
	c := &Client{UbuntuMirror: srv.URL + "/ubuntu", UbuntuSecurity: srv.URL + "/security"}
	jammy := Release{"ubuntu", "jammy"}
	for _, v := range []string{"3.0.2-0ubuntu1", "3.0.2-0ubuntu1.15", "3.0.2-0ubuntu1.18"} {
		got, err := c.AptHas(context.Background(), jammy, "openssl", v)
		require.NoError(t, err)
		require.True(t, got, v)
	}
	got, err := c.AptHas(context.Background(), jammy, "jq", "1.6-2.1ubuntu3")
	require.NoError(t, err)
	require.True(t, got, "universe")
}

func TestReleases(t *testing.T) {
	r, ok := ReleaseOf("debian", "12.8")
	require.True(t, ok)
	require.Equal(t, Release{"debian", "bookworm"}, r)
	r, ok = ReleaseOf("Ubuntu", "22.04")
	require.True(t, ok)
	require.Equal(t, Release{"ubuntu", "jammy"}, r)
	_, ok = ReleaseOf("alpine", "3.20.1")
	require.False(t, ok)

	for ref, want := range map[string]Release{
		"python:3.12-slim-bookworm": {"debian", "bookworm"},
		"node:20-bullseye":          {"debian", "bullseye"},
		"ubuntu:jammy":              {"ubuntu", "jammy"},
		"ubuntu:22.04":              {"ubuntu", "jammy"},
		"debian:trixie-slim":        {"debian", "trixie"},
	} {
		got, ok := ReleaseFromTag(ref)
		require.True(t, ok, ref)
		require.Equal(t, want, got, ref)
	}
	for _, ref := range []string{"python:3.12-slim", "alpine:3.20", "golang:1.22"} {
		_, ok := ReleaseFromTag(ref)
		require.False(t, ok, ref)
	}
}

func TestPipHas(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/pypi/typing-extensions/json":
			fmt.Fprint(w, `{"releases": {
				"4.12.2": [{"yanked": false}],
				"4.12.0": [{"yanked": true}],
				"4.0": [{"yanked": false}],
				"0.0.1": []
			}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := &Client{PyPI: srv.URL + "/pypi"}
	ctx := context.Background()

	for _, c2 := range []struct {
		project, version string
		want             bool
	}{
		{"typing_extensions", "4.12.2", true},  // PEP 503 name normalisation
		{"Typing.Extensions", "4.0.0", true},   // PEP 440 equality: 4.0 == 4.0.0
		{"typing-extensions", "4.12.0", false}, // yanked
		{"typing-extensions", "0.0.1", false},  // no files
		{"typing-extensions", "9.9", false},
		{"no-such-project", "1.0", false},
	} {
		got, err := c.PipHas(ctx, c2.project, c2.version)
		require.NoError(t, err)
		require.Equal(t, c2.want, got, "%s==%s", c2.project, c2.version)
	}
	require.Equal(t, int32(2), hits.Load(), "one request per project")

	_, err := c.PipHas(ctx, "x", "not a version")
	require.Error(t, err)
}
