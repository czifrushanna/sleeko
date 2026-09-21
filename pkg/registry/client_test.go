package registry

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPullManifest(t *testing.T) {
	const body = `{"manifests":[{"digest":"sha256:attestation"}]}`

	var gotPath, gotAuth string
	var gotAccept []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Values("Accept")
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	data, err := NewClient(srv.URL, "Basic dXNlcjpwYXNz", false).
		PullManifest("library/photon", "sha256:artifact")
	require.NoError(t, err)
	require.JSONEq(t, body, string(data))

	require.Equal(t, "/v2/library/photon/manifests/sha256:artifact", gotPath)
	require.Equal(t, "Basic dXNlcjpwYXNz", gotAuth)
	// The index / manifest-list types must be offered, and first: BuildKit
	// provenance attestations hang off the index, so a registry that content
	// negotiates would otherwise hand back a plain manifest.
	require.Equal(t, manifestAccepts, gotAccept)
}

// A repository reference is passed through with its namespace separators intact
// rather than escaped, which is what a Docker Registry v2 endpoint expects.
func TestPullManifest_NestedRepositoryPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = io.WriteString(w, "{}")
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "", false).PullManifest("library/oracle/nosql", "5.0")
	require.NoError(t, err)
	require.Equal(t, "/v2/library/oracle/nosql/manifests/5.0", gotPath)
}

// Harbor's registry URL may carry a trailing slash; the client must not emit a
// doubled slash, which some registries reject.
func TestPullManifest_TrimsTrailingSlashInURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, "{}")
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL+"/", "", false).PullManifest("library/photon", "5.0")
	require.NoError(t, err)
	require.Equal(t, "/v2/library/photon/manifests/5.0", gotPath)
}

// An empty authorization (an unauthenticated registry) must not send the header
// at all, rather than sending an empty one.
func TestPullManifest_OmitsEmptyAuthorization(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		_, _ = io.WriteString(w, "{}")
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "", false).PullManifest("library/photon", "5.0")
	require.NoError(t, err)
	require.False(t, hadAuth)
}

func TestPullBlob(t *testing.T) {
	const body = `{"predicateType":"https://slsa.dev/provenance/v1"}`

	var gotPath, gotEncoding string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotEncoding = r.Header.Get("Accept-Encoding")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	rc, err := NewClient(srv.URL, "", false).PullBlob("library/photon", "sha256:statement")
	require.NoError(t, err)
	defer rc.Close()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, body, string(data))

	require.Equal(t, "/v2/library/photon/blobs/sha256:statement", gotPath)
	// Blob digests are verified against the raw bytes, so transport compression
	// must be declined.
	require.Equal(t, "identity", gotEncoding)
}

func TestNonSuccessStatusIsAnError(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"not found", http.StatusNotFound, "{\"errors\":[{\"code\":\"MANIFEST_UNKNOWN\"}]}\n",
			`registry returned 404 Not Found: {"errors":[{"code":"MANIFEST_UNKNOWN"}]}`},
		{"unauthorized", http.StatusUnauthorized, "unauthorized",
			"registry returned 401 Unauthorized: unauthorized"},
		{"server error with empty body", http.StatusInternalServerError, "",
			"registry returned 500 Internal Server Error: "},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, c.body)
			}))
			defer srv.Close()

			cli := NewClient(srv.URL, "", false)

			_, err := cli.PullManifest("library/photon", "5.0")
			require.EqualError(t, err, c.want)

			rc, err := cli.PullBlob("library/photon", "sha256:missing")
			require.EqualError(t, err, c.want)
			require.Nil(t, rc)
		})
	}
}

// A 3xx is treated as a failure rather than silently followed by the client's
// own redirect handling returning a non-blob body.
func TestRedirectToUnreachableLocationIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "http://blob-store.invalid/blob", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "", false).PullBlob("library/photon", "sha256:statement")
	require.Error(t, err)
}

func TestTransportErrorIsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening any more

	cli := NewClient(url, "", false)

	_, err := cli.PullManifest("library/photon", "5.0")
	require.Error(t, err)

	_, err = cli.PullBlob("library/photon", "sha256:statement")
	require.Error(t, err)
}

func TestMalformedURLIsReturnedAsAnError(t *testing.T) {
	// A control character makes http.NewRequest itself fail, before any I/O.
	cli := NewClient("http://registry.invalid\x7f", "", false)

	_, err := cli.PullManifest("library/photon", "5.0")
	require.Error(t, err)

	_, err = cli.PullBlob("library/photon", "sha256:statement")
	require.Error(t, err)
}

// An insecure client must reach a TLS endpoint serving a self-signed
// certificate, and a secure one must refuse it.
func TestInsecureSkipsTLSVerification(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "{}")
	}))
	defer srv.Close()

	data, err := NewClient(srv.URL, "", true).PullManifest("library/photon", "5.0")
	require.NoError(t, err)
	require.JSONEq(t, "{}", string(data))

	_, err = NewClient(srv.URL, "", false).PullManifest("library/photon", "5.0")
	require.Error(t, err)
}
