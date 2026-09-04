// Package registry is a minimal Docker Registry v2 HTTP API client, sufficient
// for pulling manifests and blobs given a pre-authorized request. Harbor hands
// sleeko a ready-made Authorization header value (a short-lived robot
// credential) with every optimize request, so there is no auth-negotiation
// logic here — unlike Harbor's own pkg/registry client, which this package
// intentionally does not depend on (sleeko has no Go dependency on
// goharbor/harbor).
package registry

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// manifestAccepts lists the manifest media types this client asks for. OCI
// index / Docker manifest-list come first since BuildKit provenance
// attestations hang off the index.
var manifestAccepts = []string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
	"application/vnd.docker.distribution.manifest.v1+json",
}

// Client pulls manifests and blobs from a Docker Registry v2 API endpoint.
type Client interface {
	// PullManifest returns the raw JSON payload of the manifest/index/manifest-list
	// identified by reference (a tag or digest).
	PullManifest(repository, reference string) ([]byte, error)
	// PullBlob returns a reader for the raw blob identified by digest. The caller
	// must close it.
	PullBlob(repository, digest string) (io.ReadCloser, error)
}

type client struct {
	url           string
	authorization string
	httpClient    *http.Client
}

// NewClient creates a registry client that sends the given pre-built
// Authorization header value (may be empty) on every request.
func NewClient(url, authorization string, insecure bool) Client {
	transport := http.DefaultTransport
	if insecure {
		//nolint:gosec // insecure is an explicit, per-registration opt-in mirroring Harbor's own scanner/optimizer registration setting.
		transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}

	return &client{
		url:           strings.TrimSuffix(url, "/"),
		authorization: authorization,
		httpClient:    &http.Client{Transport: transport},
	}
}

func (c *client) do(req *http.Request) (*http.Response, error) {
	if c.authorization != "" {
		req.Header.Set("Authorization", c.authorization)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registry returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return resp, nil
}

func (c *client) PullManifest(repository, reference string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v2/%s/manifests/%s", c.url, repository, reference), nil)
	if err != nil {
		return nil, err
	}
	for _, mt := range manifestAccepts {
		req.Header.Add("Accept", mt)
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (c *client) PullBlob(repository, digest string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/v2/%s/blobs/%s", c.url, repository, digest), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept-Encoding", "identity")

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}
