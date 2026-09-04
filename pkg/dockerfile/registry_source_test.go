package dockerfile

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeRegistryClient is a hand-written stub of registry.Client for testing
// RegistryBlobSource without a real HTTP registry.
type fakeRegistryClient struct {
	manifests   map[string][]byte
	manifestErr map[string]error
	blobs       map[string][]byte
	blobErr     map[string]error
}

func newFakeRegistryClient() *fakeRegistryClient {
	return &fakeRegistryClient{
		manifests:   map[string][]byte{},
		manifestErr: map[string]error{},
		blobs:       map[string][]byte{},
		blobErr:     map[string]error{},
	}
}

func (f *fakeRegistryClient) PullManifest(_, reference string) ([]byte, error) {
	if err, ok := f.manifestErr[reference]; ok {
		return nil, err
	}
	if data, ok := f.manifests[reference]; ok {
		return data, nil
	}
	return nil, errors.New("manifest not found")
}

func (f *fakeRegistryClient) PullBlob(_, digest string) (io.ReadCloser, error) {
	if err, ok := f.blobErr[digest]; ok {
		return nil, err
	}
	if data, ok := f.blobs[digest]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return nil, errors.New("blob not found")
}

func TestRegistryBlobSource_IndexSuccess(t *testing.T) {
	payload := []byte(`{"manifests":[]}`)
	cli := newFakeRegistryClient()
	cli.manifests["latest"] = payload

	src := &RegistryBlobSource{Client: cli, Repository: "library/repo", Reference: "latest"}
	got, err := src.Index(context.Background())
	require.NoError(t, err)
	require.Equal(t, payload, got)
}

func TestRegistryBlobSource_IndexError(t *testing.T) {
	cli := newFakeRegistryClient()
	cli.manifestErr["latest"] = errors.New("registry unavailable")

	src := &RegistryBlobSource{Client: cli, Repository: "library/repo", Reference: "latest"}
	_, err := src.Index(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "pull index manifest")
}

func TestRegistryBlobSource_BlobViaManifest(t *testing.T) {
	blobData := []byte(`{"layers":[]}`)
	cli := newFakeRegistryClient()
	cli.manifests["sha256:abc"] = blobData

	src := &RegistryBlobSource{Client: cli, Repository: "library/repo", Reference: "latest"}
	got, err := src.Blob(context.Background(), "sha256:abc")
	require.NoError(t, err)
	require.Equal(t, blobData, got)
}

func TestRegistryBlobSource_BlobFallbackToBlob(t *testing.T) {
	blobData := []byte(`some raw blob content`)
	cli := newFakeRegistryClient()
	// PullManifest fails, so Blob falls back to PullBlob.
	cli.manifestErr["sha256:def"] = errors.New("not a manifest")
	cli.blobs["sha256:def"] = blobData

	src := &RegistryBlobSource{Client: cli, Repository: "library/repo", Reference: "latest"}
	got, err := src.Blob(context.Background(), "sha256:def")
	require.NoError(t, err)
	require.Equal(t, blobData, got)
}

func TestRegistryBlobSource_BlobBothFail(t *testing.T) {
	cli := newFakeRegistryClient()
	cli.manifestErr["sha256:jkl"] = errors.New("not found")
	cli.blobErr["sha256:jkl"] = errors.New("blob not found")

	src := &RegistryBlobSource{Client: cli, Repository: "library/repo", Reference: "latest"}
	_, err := src.Blob(context.Background(), "sha256:jkl")
	require.Error(t, err)
	require.Contains(t, err.Error(), "pull blob")
}
