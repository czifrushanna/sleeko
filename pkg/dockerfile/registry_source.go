package dockerfile

import (
	"context"
	"fmt"
	"io"

	"github.com/czifrushanna/sleeko/pkg/registry"
)

// RegistryBlobSource implements BlobSource by pulling data directly from a
// registry. Index() fetches the top-level manifest for the given reference;
// Blob() tries PullManifest first (for nested manifests) and falls back to
// PullBlob (for statement layers and other data blobs).
type RegistryBlobSource struct {
	Client     registry.Client
	Repository string
	Reference  string
}

func (r *RegistryBlobSource) Index(_ context.Context) ([]byte, error) {
	payload, err := r.Client.PullManifest(r.Repository, r.Reference)
	if err != nil {
		return nil, fmt.Errorf("pull index manifest %s@%s: %w", r.Repository, r.Reference, err)
	}
	return payload, nil
}

func (r *RegistryBlobSource) Blob(_ context.Context, digest string) ([]byte, error) {
	if payload, err := r.Client.PullManifest(r.Repository, digest); err == nil {
		return payload, nil
	}

	blob, err := r.Client.PullBlob(r.Repository, digest)
	if err != nil {
		return nil, fmt.Errorf("pull blob %s from %s: %w", digest, r.Repository, err)
	}
	defer blob.Close()

	data, err := io.ReadAll(blob)
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", digest, err)
	}
	return data, nil
}
