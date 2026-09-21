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

package dockerfile

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeTarEntry(t *testing.T, tw *tar.Writer, name string, content []byte) {
	t.Helper()

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0600,
		Size: int64(len(content)),
	}))
	_, err := tw.Write(content)
	require.NoError(t, err)
}

func createOCIArchive(t *testing.T, dockerfile string) string {
	t.Helper()

	encodedDockerfile := base64.StdEncoding.EncodeToString([]byte(dockerfile))
	indexJSON := map[string]any{
		"manifests": []map[string]any{
			{
				"digest": "sha256:attestation",
				"annotations": map[string]string{
					"vnd.docker.reference.type": attestationManifestType,
				},
			},
		},
	}
	attestationJSON := map[string]any{
		"layers": []map[string]string{{"digest": "sha256:statement"}},
	}
	statementJSON := map[string]any{
		"predicate": map[string]any{
			"runDetails": map[string]any{
				"metadata": map[string]any{
					"buildkit_metadata": map[string]any{
						"source": map[string]any{
							"infos": []map[string]string{{"data": encodedDockerfile}},
						},
					},
				},
			},
		},
	}

	indexContent, err := json.Marshal(indexJSON)
	require.NoError(t, err)
	attestationContent, err := json.Marshal(attestationJSON)
	require.NoError(t, err)
	statementContent, err := json.Marshal(statementJSON)
	require.NoError(t, err)

	file, err := os.CreateTemp(t.TempDir(), "image-*.oci")
	require.NoError(t, err)
	defer file.Close()

	tw := tar.NewWriter(file)
	writeTarEntry(t, tw, "index.json", indexContent)
	writeTarEntry(t, tw, filepath.Join("blobs", "sha256", "attestation"), attestationContent)
	writeTarEntry(t, tw, filepath.Join("blobs", "sha256", "statement"), statementContent)
	require.NoError(t, tw.Close())
	require.NoError(t, file.Close())

	return file.Name()
}

func TestExtractDockerfile(t *testing.T) {
	archivePath := createOCIArchive(t, "FROM alpine:3.20\nRUN echo hello\n")

	result, err := NewWorkflow().ExtractDockerfile(context.Background(), archivePath)
	require.NoError(t, err)
	require.Equal(t, "FROM alpine:3.20\nRUN echo hello\n", result.Dockerfile)
	require.Equal(t, "sha256:attestation", result.AttestationManifestDigest)
	require.Equal(t, "sha256:statement", result.StatementDigest)
}

// fakeBlobSource implements BlobSource from an in-memory map for testing.
type fakeBlobSource struct {
	index []byte
	blobs map[string][]byte
}

func (f *fakeBlobSource) Index(_ context.Context) ([]byte, error) { return f.index, nil }
func (f *fakeBlobSource) Blob(_ context.Context, digest string) ([]byte, error) {
	b, ok := f.blobs[digest]
	if !ok {
		return nil, fmt.Errorf("blob %s not found", digest)
	}
	return b, nil
}

func TestExtractDockerfileFromSource(t *testing.T) {
	const expected = "FROM alpine:3.20\nRUN echo hello\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(expected))

	indexJSON, _ := json.Marshal(map[string]any{
		"manifests": []map[string]any{
			{
				"digest": "sha256:attestation",
				"annotations": map[string]string{
					"vnd.docker.reference.type": attestationManifestType,
				},
			},
		},
	})
	attestationJSON, _ := json.Marshal(map[string]any{
		"layers": []map[string]string{{"digest": "sha256:statement"}},
	})
	statementJSON, _ := json.Marshal(map[string]any{
		"predicate": map[string]any{
			"runDetails": map[string]any{
				"metadata": map[string]any{
					"buildkit_metadata": map[string]any{
						"source": map[string]any{
							"infos": []map[string]string{{"data": encoded}},
						},
					},
				},
			},
		},
	})

	src := &fakeBlobSource{
		index: indexJSON,
		blobs: map[string][]byte{
			"sha256:attestation": attestationJSON,
			"sha256:statement":   statementJSON,
		},
	}

	result, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, expected, result.Dockerfile)
	require.Equal(t, "sha256:attestation", result.AttestationManifestDigest)
	require.Equal(t, "sha256:statement", result.StatementDigest)
}

func TestExtractDockerfileMissingAttestation(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "image-*.oci")
	require.NoError(t, err)
	defer file.Close()

	tw := tar.NewWriter(file)
	writeTarEntry(t, tw, "index.json", []byte(`{"manifests":[]}`))
	require.NoError(t, tw.Close())
	require.NoError(t, file.Close())

	_, err = NewWorkflow().ExtractDockerfile(context.Background(), file.Name())
	require.Error(t, err)
	require.Contains(t, err.Error(), "attestation manifest not found")
}

func TestExtractDockerfileFromReaderDirect(t *testing.T) {
	archivePath := createOCIArchive(t, "FROM golang:1.23\nRUN go build .\n")

	f, err := os.Open(archivePath)
	require.NoError(t, err)
	defer f.Close()

	result, err := NewWorkflow().ExtractDockerfileFromReader(context.Background(), f)
	require.NoError(t, err)
	require.Equal(t, "FROM golang:1.23\nRUN go build .\n", result.Dockerfile)
}

func TestExtractDockerfileContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	archivePath := createOCIArchive(t, "FROM scratch\n")

	f, err := os.Open(archivePath)
	require.NoError(t, err)
	defer f.Close()

	_, err = NewWorkflow().ExtractDockerfileFromReader(ctx, f)
	require.ErrorIs(t, err, context.Canceled)
}

func TestAttestationManifestNoLayers(t *testing.T) {
	indexJSON, _ := json.Marshal(map[string]any{
		"manifests": []map[string]any{
			{
				"digest": "sha256:nolay",
				"annotations": map[string]string{
					"vnd.docker.reference.type": attestationManifestType,
				},
			},
		},
	})
	// Attestation manifest with an empty layers array.
	attestationJSON, _ := json.Marshal(map[string]any{"layers": []any{}})

	src := &fakeBlobSource{
		index: indexJSON,
		blobs: map[string][]byte{
			"sha256:nolay": attestationJSON,
		},
	}

	_, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), src)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no layers")
}

func TestNestedAttestationManifest(t *testing.T) {
	const expected = "FROM ubuntu:24.04\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(expected))

	// Top-level index points to a nested manifest list (not directly annotated).
	indexJSON, _ := json.Marshal(map[string]any{
		"manifests": []map[string]any{
			{"digest": "sha256:outer"},
		},
	})
	// The nested manifest list contains the real attestation manifest.
	nestedIndexJSON, _ := json.Marshal(map[string]any{
		"manifests": []map[string]any{
			{
				"digest": "sha256:attest",
				"annotations": map[string]string{
					"vnd.docker.reference.type": attestationManifestType,
				},
			},
		},
	})
	attestationJSON, _ := json.Marshal(map[string]any{
		"layers": []map[string]string{{"digest": "sha256:stmt"}},
	})
	statementJSON, _ := json.Marshal(map[string]any{
		"predicate": map[string]any{
			"runDetails": map[string]any{
				"metadata": map[string]any{
					"buildkit_metadata": map[string]any{
						"source": map[string]any{
							"infos": []map[string]string{{"data": encoded}},
						},
					},
				},
			},
		},
	})

	src := &fakeBlobSource{
		index: indexJSON,
		blobs: map[string][]byte{
			"sha256:outer":  nestedIndexJSON,
			"sha256:attest": attestationJSON,
			"sha256:stmt":   statementJSON,
		},
	}

	result, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, expected, result.Dockerfile)
	require.Equal(t, "sha256:attest", result.AttestationManifestDigest)
}

func TestDecodeDockerfileRawBase64(t *testing.T) {
	// Raw base64 omits padding; StdEncoding rejects it, RawStdEncoding accepts it.
	rawEncoded := base64.RawStdEncoding.EncodeToString([]byte("FROM scratch\n"))

	indexJSON, _ := json.Marshal(map[string]any{
		"manifests": []map[string]any{
			{
				"digest": "sha256:a1",
				"annotations": map[string]string{
					"vnd.docker.reference.type": attestationManifestType,
				},
			},
		},
	})
	attestationJSON, _ := json.Marshal(map[string]any{
		"layers": []map[string]string{{"digest": "sha256:s1"}},
	})
	statementJSON, _ := json.Marshal(map[string]any{
		"predicate": map[string]any{
			"runDetails": map[string]any{
				"metadata": map[string]any{
					"buildkit_metadata": map[string]any{
						"source": map[string]any{
							"infos": []map[string]string{{"data": rawEncoded}},
						},
					},
				},
			},
		},
	})

	src := &fakeBlobSource{
		index: indexJSON,
		blobs: map[string][]byte{
			"sha256:a1": attestationJSON,
			"sha256:s1": statementJSON,
		},
	}

	result, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, "FROM scratch\n", result.Dockerfile)
}

func TestExtractDockerfileBlobNotInArchive(t *testing.T) {
	indexJSON, _ := json.Marshal(map[string]any{
		"manifests": []map[string]any{
			{
				"digest": "sha256:missing",
				"annotations": map[string]string{
					"vnd.docker.reference.type": attestationManifestType,
				},
			},
		},
	})

	src := &fakeBlobSource{
		index: indexJSON,
		blobs: map[string][]byte{},
	}

	_, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), src)
	require.Error(t, err)
}

// attestationIndexJSON builds a top-level OCI index pointing at a single
// attestation manifest under the given digest.
func attestationIndexJSON(t *testing.T, digest string) []byte {
	t.Helper()

	content, err := json.Marshal(map[string]any{
		"manifests": []map[string]any{
			{
				"digest": digest,
				"annotations": map[string]string{
					"vnd.docker.reference.type": attestationManifestType,
				},
			},
		},
	})
	require.NoError(t, err)

	return content
}

// provenanceStatementJSON builds an SLSA provenance in-toto statement carrying
// the given Dockerfile as BuildKit source info.
func provenanceStatementJSON(t *testing.T, dockerfile string) []byte {
	t.Helper()

	content, err := json.Marshal(map[string]any{
		"predicate": map[string]any{
			"runDetails": map[string]any{
				"metadata": map[string]any{
					"buildkit_metadata": map[string]any{
						"source": map[string]any{
							"infos": []map[string]string{
								{"data": base64.StdEncoding.EncodeToString([]byte(dockerfile))},
							},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	return content
}

// TestExtractDockerfileSBOMLayerFirst covers an image built with both --sbom and
// --provenance: BuildKit writes the SPDX document as layer 0 and the SLSA
// provenance as layer 1, so reading layer 0 finds a statement that parses as
// JSON but carries no source info.
func TestExtractDockerfileSBOMLayerFirst(t *testing.T) {
	const expected = "FROM python:3.11-slim\nRUN pip install -r requirements.txt\n"

	attestationJSON, _ := json.Marshal(map[string]any{
		"layers": []map[string]any{
			{
				"digest":      "sha256:sbom",
				"annotations": map[string]string{predicateTypeAnnotation: "https://spdx.dev/Document"},
			},
			{
				"digest":      "sha256:provenance",
				"annotations": map[string]string{predicateTypeAnnotation: "https://slsa.dev/provenance/v1"},
			},
		},
	})

	src := &fakeBlobSource{
		index: attestationIndexJSON(t, "sha256:attestation"),
		blobs: map[string][]byte{
			"sha256:attestation": attestationJSON,
			// An SPDX document has no predicate.runDetails at all, so decoding it
			// as a provenance statement yields no source info.
			"sha256:sbom":       []byte(`{"predicateType":"https://spdx.dev/Document","predicate":{"name":"pyapi"}}`),
			"sha256:provenance": provenanceStatementJSON(t, expected),
		},
	}

	result, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, expected, result.Dockerfile)
	require.Equal(t, "sha256:provenance", result.StatementDigest)
}

// TestExtractDockerfileProvenanceLayerOnly is the same selection path when
// provenance is the only statement but is still annotated.
func TestExtractDockerfileProvenanceLayerOnly(t *testing.T) {
	const expected = "FROM alpine:3.20\n"

	attestationJSON, _ := json.Marshal(map[string]any{
		"layers": []map[string]any{
			{
				"digest":      "sha256:provenance",
				"annotations": map[string]string{predicateTypeAnnotation: "https://slsa.dev/provenance/v0.2"},
			},
		},
	})

	src := &fakeBlobSource{
		index: attestationIndexJSON(t, "sha256:attestation"),
		blobs: map[string][]byte{
			"sha256:attestation": attestationJSON,
			"sha256:provenance":  provenanceStatementJSON(t, expected),
		},
	}

	result, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, expected, result.Dockerfile)
	require.Equal(t, "sha256:provenance", result.StatementDigest)
}

// TestExtractDockerfileUnannotatedLayersFallBackToFirst keeps manifests written
// before BuildKit stamped predicate-type annotations working.
func TestExtractDockerfileUnannotatedLayersFallBackToFirst(t *testing.T) {
	const expected = "FROM busybox\n"

	attestationJSON, _ := json.Marshal(map[string]any{
		"layers": []map[string]string{{"digest": "sha256:statement"}},
	})

	src := &fakeBlobSource{
		index: attestationIndexJSON(t, "sha256:attestation"),
		blobs: map[string][]byte{
			"sha256:attestation": attestationJSON,
			"sha256:statement":   provenanceStatementJSON(t, expected),
		},
	}

	result, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, expected, result.Dockerfile)
	require.Equal(t, "sha256:statement", result.StatementDigest)
}

// TestExtractDockerfileSBOMOnly covers --sbom without --provenance: there is an
// attestation manifest, but nothing in it can yield a Dockerfile, so callers
// should see ErrNoProvenance and fall back to generating one.
func TestExtractDockerfileSBOMOnly(t *testing.T) {
	attestationJSON, _ := json.Marshal(map[string]any{
		"layers": []map[string]any{
			{
				"digest":      "sha256:sbom",
				"annotations": map[string]string{predicateTypeAnnotation: "https://spdx.dev/Document"},
			},
		},
	})

	src := &fakeBlobSource{
		index: attestationIndexJSON(t, "sha256:attestation"),
		blobs: map[string][]byte{
			"sha256:attestation": attestationJSON,
			"sha256:sbom":        []byte(`{"predicateType":"https://spdx.dev/Document"}`),
		},
	}

	_, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), src)
	require.ErrorIs(t, err, ErrNoProvenance)
	require.Contains(t, err.Error(), "no slsa provenance statement")
}

// TestExtractDockerfileMissingAttestationIsErrNoProvenance pins the sentinel on
// the other "nothing to extract" path, which the adapter treats identically.
func TestExtractDockerfileMissingAttestationIsErrNoProvenance(t *testing.T) {
	src := &fakeBlobSource{index: []byte(`{"manifests":[]}`), blobs: map[string][]byte{}}

	_, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), src)
	require.ErrorIs(t, err, ErrNoProvenance)
}

func TestDigestToBlobPath(t *testing.T) {
	cases := []struct {
		name    string
		digest  string
		want    string
		wantErr bool
	}{
		{name: "sha256", digest: "sha256:abc123", want: "blobs/sha256/abc123"},
		{name: "no algorithm", digest: "abc123", wantErr: true},
		{name: "unsupported algorithm", digest: "sha512:abc123", wantErr: true},
		{name: "empty hex", digest: "sha256:", wantErr: true},
		{name: "too many separators", digest: "sha256:abc:123", wantErr: true},
		{name: "empty", digest: "", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := digestToBlobPath(c.digest)
			if c.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "unsupported digest format")
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

// A digest the archive cannot express as a blob path must surface as an error
// rather than a lookup miss.
func TestExtractDockerfileFromReader_MalformedDigestInIndex(t *testing.T) {
	indexContent, err := json.Marshal(map[string]any{
		"manifests": []map[string]any{{
			"digest":      "sha512:abc123",
			"annotations": map[string]string{"vnd.docker.reference.type": attestationManifestType},
		}},
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeTarEntry(t, tw, "index.json", indexContent)
	require.NoError(t, tw.Close())

	_, err = NewWorkflow().ExtractDockerfileFromReader(context.Background(), &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported digest format")
}

func TestExtractDockerfileFromReader_MissingIndexJSON(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeTarEntry(t, tw, filepath.Join("blobs", "sha256", "orphan"), []byte("{}"))
	require.NoError(t, tw.Close())

	_, err := NewWorkflow().ExtractDockerfileFromReader(context.Background(), &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "index.json not found in OCI archive")
}

func TestExtractDockerfileFromReader_NotATarArchive(t *testing.T) {
	_, err := NewWorkflow().ExtractDockerfileFromReader(
		context.Background(), strings.NewReader("this is not a tar archive"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "read OCI archive")
}

// Directory members carry no content and must be skipped rather than stored as
// empty entries that could shadow a real blob.
func TestExtractDockerfileFromReader_SkipsDirectoryEntries(t *testing.T) {
	archivePath := createOCIArchive(t, "FROM alpine:3.20\n")
	original, err := os.ReadFile(archivePath)
	require.NoError(t, err)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "blobs/sha256/",
		Mode:     0700,
		Typeflag: tar.TypeDir,
	}))
	require.NoError(t, tw.Flush())
	// Append the well-formed archive's members after the directory header.
	entries, err := readArchiveEntries(bytes.NewReader(original))
	require.NoError(t, err)
	for _, name := range []string{"index.json", "blobs/sha256/attestation", "blobs/sha256/statement"} {
		writeTarEntry(t, tw, name, entries[name])
	}
	require.NoError(t, tw.Close())

	result, err := NewWorkflow().ExtractDockerfileFromReader(context.Background(), &buf)
	require.NoError(t, err)
	require.Equal(t, "FROM alpine:3.20\n", result.Dockerfile)
}

func TestExtractDockerfile_UnreadableArchivePath(t *testing.T) {
	_, err := NewWorkflow().ExtractDockerfile(
		context.Background(), filepath.Join(t.TempDir(), "no-such-archive.oci"))
	require.Error(t, err)
}

func TestExtractDockerfileFromReader_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewWorkflow().ExtractDockerfileFromReader(ctx, bytes.NewReader(nil))
	require.ErrorIs(t, err, context.Canceled)
}

func TestExtractDockerfileFromSource_MalformedIndex(t *testing.T) {
	_, err := NewWorkflow().ExtractDockerfileFromSource(
		context.Background(), &fakeBlobSource{index: []byte(`{"manifests":`)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal index")
}

func TestExtractDockerfileFromSource_IndexUnavailable(t *testing.T) {
	_, err := NewWorkflow().ExtractDockerfileFromSource(
		context.Background(), &erroringBlobSource{err: errors.New("registry unavailable")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "registry unavailable")
}

// erroringBlobSource fails on Index, covering the path where the artifact
// cannot be reached at all.
type erroringBlobSource struct{ err error }

func (e *erroringBlobSource) Index(context.Context) ([]byte, error) { return nil, e.err }
func (e *erroringBlobSource) Blob(context.Context, string) ([]byte, error) {
	return nil, e.err
}

func TestExtractDockerfileFromSource_MalformedAttestationManifest(t *testing.T) {
	_, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), &fakeBlobSource{
		index: attestationIndexJSON(t, "sha256:attestation"),
		blobs: map[string][]byte{"sha256:attestation": []byte(`{"layers":`)},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal attestation manifest")
}

func TestExtractDockerfileFromSource_StatementBlobUnavailable(t *testing.T) {
	attestation, err := json.Marshal(map[string]any{
		"layers": []map[string]string{{"digest": "sha256:statement"}},
	})
	require.NoError(t, err)

	_, err = NewWorkflow().ExtractDockerfileFromSource(context.Background(), &fakeBlobSource{
		index: attestationIndexJSON(t, "sha256:attestation"),
		blobs: map[string][]byte{"sha256:attestation": attestation},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "sha256:statement")
}

// The nested-index walk is bounded, so an index that references itself
// terminates instead of recursing forever. The depth-exhausted error is
// internal -- the walk keeps trying sibling candidates -- so the caller sees
// ErrNoProvenance and falls back to Dockerfile generation.
func TestFindAttestationManifest_SelfReferentialIndexTerminates(t *testing.T) {
	selfReferential, err := json.Marshal(map[string]any{
		"manifests": []map[string]any{{"digest": "sha256:loop"}},
	})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), &fakeBlobSource{
			index: selfReferential,
			blobs: map[string][]byte{"sha256:loop": selfReferential},
		})
		done <- err
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrNoProvenance)
	case <-time.After(10 * time.Second):
		t.Fatal("the nested-index walk did not terminate")
	}
}

// The recursion guard itself: once the depth budget is spent the walk reports
// it rather than descending further.
func TestFindAttestationManifest_DepthExhausted(t *testing.T) {
	idx := &index{}
	idx.Manifests = append(idx.Manifests, struct {
		Digest      string            `json:"digest"`
		Annotations map[string]string `json:"annotations"`
	}{Digest: "sha256:attestation", Annotations: map[string]string{
		"vnd.docker.reference.type": attestationManifestType,
	}})

	// Even an index holding the attestation is not inspected at depth 0.
	_, err := findAttestationManifestDigestFromSource(
		context.Background(), idx, &fakeBlobSource{}, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "max recursion reached")
}

// Nested candidates that cannot be fetched, are not JSON, or hold no manifests
// are skipped so a later sibling can still yield the attestation.
func TestFindAttestationManifest_SkipsUnusableNestedIndexes(t *testing.T) {
	const expected = "FROM alpine:3.20\nRUN echo nested\n"

	nested, err := json.Marshal(map[string]any{
		"manifests": []map[string]any{{
			"digest":      "sha256:attestation",
			"annotations": map[string]string{"vnd.docker.reference.type": attestationManifestType},
		}},
	})
	require.NoError(t, err)
	attestation, err := json.Marshal(map[string]any{
		"layers": []map[string]string{{"digest": "sha256:statement"}},
	})
	require.NoError(t, err)

	top, err := json.Marshal(map[string]any{
		"manifests": []map[string]any{
			{"digest": "sha256:missing"},
			{"digest": "sha256:notjson"},
			{"digest": "sha256:nomanifests"},
			{"digest": "sha256:nested"},
		},
	})
	require.NoError(t, err)

	result, err := NewWorkflow().ExtractDockerfileFromSource(context.Background(), &fakeBlobSource{
		index: top,
		blobs: map[string][]byte{
			"sha256:notjson":     []byte(`{"manifests":`),
			"sha256:nomanifests": []byte(`{"layers":[]}`),
			"sha256:nested":      nested,
			"sha256:attestation": attestation,
			"sha256:statement":   provenanceStatementJSON(t, expected),
		},
	})
	require.NoError(t, err)
	require.Equal(t, expected, result.Dockerfile)
}

func TestDecodeDockerfile_Errors(t *testing.T) {
	t.Run("malformed statement JSON", func(t *testing.T) {
		_, err := decodeDockerfile([]byte(`{"predicate":`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal in-toto statement")
	})

	t.Run("statement carries no buildkit source info", func(t *testing.T) {
		_, err := decodeDockerfile([]byte(`{"predicate":{"runDetails":{"metadata":{}}}}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "buildkit source info not found")
	})

	t.Run("source info is not valid base64 under either encoding", func(t *testing.T) {
		statement, err := json.Marshal(map[string]any{
			"predicate": map[string]any{
				"runDetails": map[string]any{
					"metadata": map[string]any{
						"buildkit_metadata": map[string]any{
							"source": map[string]any{
								"infos": []map[string]string{{"data": "not!valid!base64!"}},
							},
						},
					},
				},
			},
		})
		require.NoError(t, err)

		_, err = decodeDockerfile(statement)
		require.Error(t, err)
		require.Contains(t, err.Error(), "decode embedded Dockerfile")
	})
}
