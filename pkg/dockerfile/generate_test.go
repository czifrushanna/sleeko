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
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateDockerfileFromSource_DirectManifest(t *testing.T) {
	src := &fakeBlobSource{
		index: []byte(`{"config":{"digest":"sha256:cfg"},"layers":[{"digest":"sha256:layer1"}]}`),
		blobs: map[string][]byte{
			"sha256:cfg": []byte(`{
				"config": {"Env": ["PATH=/usr/local/bin"], "Cmd": ["/bin/sh"]},
				"history": [
					{"created_by": "/bin/sh -c #(nop)  ENV PATH=/usr/local/bin", "empty_layer": true},
					{"created_by": "/bin/sh -c apk add --no-cache curl", "empty_layer": false},
					{"created_by": "/bin/sh -c #(nop) WORKDIR /app", "empty_layer": true},
					{"created_by": "/bin/sh -c #(nop) COPY file:abc123 in /app/hello.sh ", "empty_layer": false},
					{"created_by": "/bin/sh -c #(nop)  CMD [\"/bin/sh\"]", "empty_layer": true}
				]
			}`),
		},
	}

	result, err := GenerateDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	require.True(t, result.Generated)
	require.Contains(t, result.Dockerfile, "FROM scratch")
	require.Contains(t, result.Dockerfile, "ENV PATH=/usr/local/bin")
	require.Contains(t, result.Dockerfile, "RUN apk add --no-cache curl")
	require.Contains(t, result.Dockerfile, "WORKDIR /app")
	require.Contains(t, result.Dockerfile, `COPY <unresolvable-source sha256:abc123> /app/hello.sh`)
	require.Contains(t, result.Dockerfile, `CMD ["/bin/sh"]`)
	require.Empty(t, result.AttestationManifestDigest)
	require.Empty(t, result.StatementDigest)
}

func TestGenerateDockerfileFromSource_ManifestList(t *testing.T) {
	src := &fakeBlobSource{
		index: []byte(`{"manifests":[
			{"digest":"sha256:attestation","annotations":{"vnd.docker.reference.type":"attestation-manifest"}},
			{"digest":"sha256:platform-manifest"}
		]}`),
		blobs: map[string][]byte{
			"sha256:attestation": []byte(`{"layers":[{"digest":"sha256:statement"}]}`),
			"sha256:platform-manifest": []byte(
				`{"config":{"digest":"sha256:cfg"},"layers":[{"digest":"sha256:layer1"}]}`,
			),
			"sha256:cfg": []byte(`{
				"config": {},
				"history": [
					{"created_by": "/bin/sh -c echo hi", "empty_layer": false}
				]
			}`),
		},
	}

	result, err := GenerateDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	require.True(t, result.Generated)
	require.Contains(t, result.Dockerfile, "RUN echo hi")
}

func TestGenerateDockerfileFromSource_NoManifestFound(t *testing.T) {
	src := &fakeBlobSource{index: []byte(`{"manifests":[]}`)}

	_, err := GenerateDockerfileFromSource(context.Background(), src)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no image manifest found")
}

func TestGenerateDockerfileFromSource_NoHistory(t *testing.T) {
	src := &fakeBlobSource{
		index: []byte(`{"config":{"digest":"sha256:cfg"}}`),
		blobs: map[string][]byte{
			"sha256:cfg": []byte(`{"config": {}, "history": []}`),
		},
	}

	_, err := GenerateDockerfileFromSource(context.Background(), src)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no history metadata")
}

func TestGenerateDockerfileFromSource_TrailingConfigFallback(t *testing.T) {
	// History carries no metadata instructions at all (e.g. a re-committed
	// image); the final config state must still surface as Dockerfile lines.
	src := &fakeBlobSource{
		index: []byte(`{"config":{"digest":"sha256:cfg"}}`),
		blobs: map[string][]byte{
			"sha256:cfg": []byte(`{
				"config": {
					"Env": ["FOO=bar"],
					"WorkingDir": "/srv",
					"User": "nobody",
					"Labels": {"team": "sre"},
					"ExposedPorts": {"8080/tcp": {}},
					"Cmd": ["/entrypoint.sh"]
				},
				"history": [
					{"created_by": "/bin/sh -c make build", "empty_layer": false}
				]
			}`),
		},
	}

	result, err := GenerateDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	require.Contains(t, result.Dockerfile, "RUN make build")
	require.Contains(t, result.Dockerfile, "ENV FOO=bar")
	require.Contains(t, result.Dockerfile, "WORKDIR /srv")
	require.Contains(t, result.Dockerfile, "USER nobody")
	require.Contains(t, result.Dockerfile, `LABEL team="sre"`)
	require.Contains(t, result.Dockerfile, "EXPOSE 8080")
	require.Contains(t, result.Dockerfile, `CMD ["/entrypoint.sh"]`)
}

func TestRenderHistoryLine(t *testing.T) {
	cases := []struct {
		name      string
		createdBy string
		want      string
	}{
		{"plain RUN", "/bin/sh -c echo hi", "RUN echo hi"},
		{"nop metadata passthrough", "/bin/sh -c #(nop) WORKDIR /app", "WORKDIR /app"},
		{"nop with double space", "/bin/sh -c #(nop)  CMD [\"/bin/sh\"]", `CMD ["/bin/sh"]`},
		{"empty created_by", "", ""},
		{"whitespace only created_by", "   \t  ", ""},
		// A nop marker with no instruction after it contributes no line rather
		// than an empty one.
		{"nop with nothing after it", "/bin/sh -c #(nop) ", ""},
		{"nop with nothing after it, no shell prefix", "#(nop)", ""},
		{
			"ADD file placeholder",
			"/bin/sh -c #(nop) ADD file:9d48c3bd43c520 in /app/hello.sh ",
			"ADD <unresolvable-source sha256:9d48c3bd43c520> /app/hello.sh",
		},
		{
			"no shell prefix at all",
			"echo raw",
			"RUN echo raw",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, renderHistoryLine(tc.createdBy, &ociContainerConfig{}))
		})
	}
}

func TestFirstToken(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"instruction", "WORKDIR /app", "WORKDIR"},
		{"lowercased instruction is normalised", "workdir /app", "WORKDIR"},
		{"leading whitespace", "   RUN make build", "RUN"},
		{"empty", "", ""},
		{"whitespace only", " \t ", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, firstToken(tc.line))
		})
	}
}

// The trailing-config safety net must emit every instruction kind it knows
// about, not just the subset the happy-path fixtures happen to carry.
func TestGenerateDockerfileFromSource_TrailingConfigCoversAllInstructionKinds(t *testing.T) {
	src := &fakeBlobSource{
		index: []byte(`{"config":{"digest":"sha256:cfg"}}`),
		blobs: map[string][]byte{
			"sha256:cfg": []byte(`{
				"config": {
					"Volumes": {"/var/lib/data": {}, "/var/log": {}},
					"StopSignal": "SIGQUIT",
					"Entrypoint": ["/usr/bin/tini", "--"],
					"Cmd": ["/entrypoint.sh"]
				},
				"history": [
					{"created_by": "/bin/sh -c make build", "empty_layer": false}
				]
			}`),
		},
	}

	result, err := GenerateDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	// Volumes come out in sorted order so the generated Dockerfile is stable
	// across runs despite the map.
	require.Contains(t, result.Dockerfile, "VOLUME [\"/var/lib/data\"]\nVOLUME [\"/var/log\"]")
	require.Contains(t, result.Dockerfile, "STOPSIGNAL SIGQUIT")
	require.Contains(t, result.Dockerfile, `ENTRYPOINT ["/usr/bin/tini","--"]`)
	require.Contains(t, result.Dockerfile, `CMD ["/entrypoint.sh"]`)
}

// An instruction kind the history already contributed must not be emitted a
// second time from the final config state.
func TestGenerateDockerfileFromSource_TrailingConfigSkipsWhatHistoryAlreadyGave(t *testing.T) {
	src := &fakeBlobSource{
		index: []byte(`{"config":{"digest":"sha256:cfg"}}`),
		blobs: map[string][]byte{
			"sha256:cfg": []byte(`{
				"config": {
					"Entrypoint": ["/usr/bin/tini"],
					"Cmd": ["/bin/sh"],
					"WorkingDir": "/app",
					"Volumes": {"/data": {}},
					"StopSignal": "SIGTERM"
				},
				"history": [
					{"created_by": "/bin/sh -c #(nop)  ENTRYPOINT [\"/usr/bin/tini\"]", "empty_layer": true},
					{"created_by": "/bin/sh -c #(nop)  CMD [\"/bin/sh\"]", "empty_layer": true},
					{"created_by": "/bin/sh -c #(nop) WORKDIR /app", "empty_layer": true},
					{"created_by": "/bin/sh -c #(nop)  VOLUME [/data]", "empty_layer": true},
					{"created_by": "/bin/sh -c #(nop)  STOPSIGNAL SIGTERM", "empty_layer": true}
				]
			}`),
		},
	}

	result, err := GenerateDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	for _, instr := range []string{"ENTRYPOINT", "CMD", "WORKDIR", "VOLUME", "STOPSIGNAL"} {
		require.Equal(t, 1, strings.Count(result.Dockerfile, instr+" "),
			"%s should appear once, from history only:\n%s", instr, result.Dockerfile)
	}
}

func TestGenerateDockerfileFromSource_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GenerateDockerfileFromSource(ctx, &fakeBlobSource{
		index: []byte(`{"config":{"digest":"sha256:cfg"}}`),
		blobs: map[string][]byte{"sha256:cfg": []byte(`{"history":[{"created_by":"x"}]}`)},
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestGenerateDockerfileFromSource_MalformedJSON(t *testing.T) {
	t.Run("manifest", func(t *testing.T) {
		_, err := GenerateDockerfileFromSource(context.Background(), &fakeBlobSource{
			index: []byte(`{"config":`),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal manifest")
	})

	t.Run("image config", func(t *testing.T) {
		_, err := GenerateDockerfileFromSource(context.Background(), &fakeBlobSource{
			index: []byte(`{"config":{"digest":"sha256:cfg"}}`),
			blobs: map[string][]byte{"sha256:cfg": []byte(`{"history": "not an array"}`)},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal image config")
	})
}

func TestGenerateDockerfileFromSource_ConfigBlobUnavailable(t *testing.T) {
	// The manifest names a config blob the registry cannot serve.
	_, err := GenerateDockerfileFromSource(context.Background(), &fakeBlobSource{
		index: []byte(`{"config":{"digest":"sha256:cfg"}}`),
		blobs: map[string][]byte{},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pull image config sha256:cfg")
}

// A nested manifest whose blob is unreadable, or which is not JSON, must be
// skipped in favour of the next candidate rather than failing the walk.
func TestGenerateDockerfileFromSource_SkipsUnusableNestedManifests(t *testing.T) {
	src := &fakeBlobSource{
		index: []byte(`{"manifests":[
			{"digest":"sha256:missing"},
			{"digest":"sha256:notjson"},
			{"digest":"sha256:noconfig"},
			{"digest":"sha256:good"}
		]}`),
		blobs: map[string][]byte{
			"sha256:notjson":  []byte(`{"config":`),
			"sha256:noconfig": []byte(`{"layers":[]}`),
			"sha256:good":     []byte(`{"config":{"digest":"sha256:cfg"}}`),
			"sha256:cfg":      []byte(`{"config":{},"history":[{"created_by":"/bin/sh -c make build"}]}`),
		},
	}

	result, err := GenerateDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)
	require.Contains(t, result.Dockerfile, "RUN make build")
}
