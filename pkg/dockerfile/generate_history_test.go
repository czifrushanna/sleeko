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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/dfparse"
)

// The created_by strings below follow the formats BuildKit's Dockerfile
// frontend writes (dockerfile2llb: commitToHistory, runCommandString) and the
// classic builder's "#(nop)" convention.

func TestRenderHistoryLine_BuildKit(t *testing.T) {
	cases := []struct {
		name      string
		createdBy string
		want      string
	}{
		{"RUN with the shell wrapper and layer marker", "RUN /bin/sh -c apt-get update # buildkit", "RUN apt-get update"},
		{"RUN with build args in scope", "RUN |2 VERSION=1.2 TARGETARCH=amd64 /bin/sh -c make build # buildkit", "RUN make build"},
		{"exec-form RUN has no shell wrapper", "RUN echo hi # buildkit", "RUN echo hi"},
		{"heredoc RUN script spanning lines", "RUN /bin/sh -c set -e\napt-get update\n # buildkit", "RUN <<SLEEKO_EOF\nset -e\napt-get update\nSLEEKO_EOF"},
		{"heredoc delimiter clash avoided", "RUN /bin/sh -c echo a\nSLEEKO_EOF\n # buildkit", "RUN <<SLEEKO_EOF_\necho a\nSLEEKO_EOF\nSLEEKO_EOF_"},
		{"WORKDIR", "WORKDIR /app", "WORKDIR /app"},
		{"COPY keeps its real sources and flags", "COPY --chown=app:app src/ requirements.txt ./ # buildkit", "COPY --chown=app:app src/ requirements.txt ./"},
		{"COPY heredoc source lost its body", "COPY <<EOT /etc/motd # buildkit", "COPY <unresolvable-source heredoc:EOT> /etc/motd"},
		{"ENV", "ENV PYTHON_VERSION=3.12.8", "ENV PYTHON_VERSION=3.12.8"},
		{"ARG", "ARG APP_VERSION=1.0", "ARG APP_VERSION=1.0"},
		{"CMD with several arguments", `CMD ["gunicorn" "-b" "0.0.0.0:8000"]`, `CMD ["gunicorn","-b","0.0.0.0:8000"]`},
		{"ENTRYPOINT with an escaped quote", `ENTRYPOINT ["sh" "-c" "echo \"hi\""]`, `ENTRYPOINT ["sh","-c","echo \"hi\""]`},
		{"EXPOSE list", "EXPOSE [8000/tcp 9090/udp]", "EXPOSE 8000/tcp 9090/udp"},
		{"VOLUME list", "VOLUME [/data /logs]", `VOLUME ["/data","/logs"]`},
		{"SHELL list", "SHELL [/bin/bash -o pipefail -c]", `SHELL ["/bin/bash","-o","pipefail","-c"]`},
		{"USER", "USER app", "USER app"},
		{"STOPSIGNAL", "STOPSIGNAL SIGQUIT", "STOPSIGNAL SIGQUIT"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, renderHistoryLine(c.createdBy, &ociContainerConfig{}))
		})
	}
}

func TestRenderHistoryLine_Classic(t *testing.T) {
	cases := []struct {
		name      string
		createdBy string
		want      string
	}{
		{"RUN with build args in scope", "|1 version=dev /bin/sh -c go build ./...", "RUN go build ./..."},
		{"CMD with several arguments", `/bin/sh -c #(nop)  CMD ["nginx" "-g" "daemon off;"]`, `CMD ["nginx","-g","daemon off;"]`},
		{"LABEL value with spaces", "/bin/sh -c #(nop)  LABEL maintainer=NGINX Docker Maintainers <docker-maint@nginx.com>", `LABEL maintainer="NGINX Docker Maintainers <docker-maint@nginx.com>"`},
		{"EXPOSE single port", "/bin/sh -c #(nop)  EXPOSE 80", "EXPOSE 80"},
		{"EXPOSE in old map form", "/bin/sh -c #(nop)  EXPOSE map[80/tcp:{}]", "EXPOSE 80/tcp"},
		{"VOLUME list", "/bin/sh -c #(nop)  VOLUME [/var/lib/mysql]", `VOLUME ["/var/lib/mysql"]`},
		{"ADD file placeholder unchanged", "/bin/sh -c #(nop) ADD file:9d48c3bd43c520 in / ", "ADD <unresolvable-source sha256:9d48c3bd43c520> /"},
		{"COPY placeholder keeps its --chown", "/bin/sh -c #(nop) COPY --chown=1000:1000 dir:9d48c3bd43c520 in /app ", "COPY --chown=1000:1000 <unresolvable-source sha256:9d48c3bd43c520> /app"},
		{"bare comment entry kept as a comment", "# debian.sh --arch 'amd64' out/ 'bookworm' '@1736726400'", "# debian.sh --arch 'amd64' out/ 'bookworm' '@1736726400'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, renderHistoryLine(c.createdBy, &ociContainerConfig{}))
		})
	}
}

func TestRenderKeyValues(t *testing.T) {
	cases := []struct {
		name string
		kw   string
		args string
		want string
	}{
		{"plain pairs untouched", "ENV", "A=1 B=two", "ENV A=1 B=two"},
		{"spaces end up in the right value", "LABEL", "title=My App maintainer=Jane Doe <jane@example.com>", `LABEL title="My App" maintainer="Jane Doe <jane@example.com>"`},
		{"empty value", "ENV", "EMPTY=", `ENV EMPTY=""`},
		{"= inside a value", "ENV", "JAVA_OPTS=-Dx=y -Xmx1g", `ENV JAVA_OPTS="-Dx=y -Xmx1g"`},
		{"dollar, quote and backslash stay literal", "ENV", `MSG=costs $5 "now" C:\x`, `ENV MSG="costs \$5 \"now\" C:\\x"`},
		{"old KEY value form left alone", "ENV", "PATH /usr/bin", "ENV PATH /usr/bin"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, renderKeyValues(c.kw, c.args))
		})
	}
}

func TestRenderHealthcheck(t *testing.T) {
	const s = int64(1e9)
	cases := []struct {
		name string
		hc   *ociHealthcheck
		want string
	}{
		{"none in the config", nil, ""},
		{"disabled", &ociHealthcheck{Test: []string{"NONE"}}, "HEALTHCHECK NONE"},
		{
			"shell form with every option",
			&ociHealthcheck{Test: []string{"CMD-SHELL", "curl -f http://localhost/ || exit 1"}, Interval: 30 * s, Timeout: 5 * s, StartPeriod: 90 * s, StartInterval: 2 * s, Retries: 3},
			"HEALTHCHECK --interval=30s --timeout=5s --start-period=1m30s --start-interval=2s --retries=3 CMD curl -f http://localhost/ || exit 1",
		},
		{"exec form, defaults left out", &ociHealthcheck{Test: []string{"CMD", "/bin/check", "--quick"}}, `HEALTHCHECK CMD ["/bin/check","--quick"]`},
		{"unknown test type", &ociHealthcheck{Test: []string{"WEIRD"}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, renderHealthcheck(c.hc))
		})
	}
}

func TestJSONArray_EmptyIsAnArray(t *testing.T) {
	require.Equal(t, "[]", jsonArray(nil))
}

func TestParseRecordedList(t *testing.T) {
	cases := []struct {
		in     string
		want   []string
		wantOK bool
	}{
		{`["/bin/sh"]`, []string{"/bin/sh"}, true},
		{`["a","b"]`, []string{"a", "b"}, true},
		{`["a" "b c"]`, []string{"a", "b c"}, true},
		{`[/data /logs]`, []string{"/data", "/logs"}, true},
		{`[]`, []string{}, true},
		{`["unterminated]`, nil, false},
		{`/bin/sh -c`, nil, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := parseRecordedList(c.in)
			require.Equal(t, c.wantOK, ok)
			require.Equal(t, c.want, got)
		})
	}
}

// historyImage serves an image whose config carries the given history and
// container config.
func historyImage(t *testing.T, config map[string]any, history []string) *fakeBlobSource {
	t.Helper()
	entries := make([]map[string]any, 0, len(history))
	for _, h := range history {
		entries = append(entries, map[string]any{"created_by": h})
	}
	cfg, err := json.Marshal(map[string]any{"config": config, "history": entries})
	require.NoError(t, err)
	return &fakeBlobSource{
		index: []byte(`{"config":{"digest":"sha256:cfg"}}`),
		blobs: map[string][]byte{"sha256:cfg": cfg},
	}
}

// TestGenerateDockerfileFromSource_BuildKitImage reconstructs a Python app
// built with BuildKit on top of an official Debian-based image, and checks
// the result is a Dockerfile BuildKit itself would accept, with the runtime
// settings the image actually has.
func TestGenerateDockerfileFromSource_BuildKitImage(t *testing.T) {
	src := historyImage(t, map[string]any{
		"Env":          []string{"PATH=/usr/local/bin:/usr/bin:/bin", "PYTHON_VERSION=3.12.8"},
		"Entrypoint":   []string{"gunicorn", "-b", "0.0.0.0:8000"},
		"Cmd":          []string{"app:app"},
		"WorkingDir":   "/app",
		"User":         "app",
		"ExposedPorts": map[string]any{"8000/tcp": map[string]any{}},
		"Healthcheck": map[string]any{
			"Test":     []string{"CMD-SHELL", "curl -f http://localhost:8000/ || exit 1"},
			"Interval": 30e9, "Timeout": 5e9, "Retries": 3,
		},
	}, []string{
		"# debian.sh --arch 'amd64' out/ 'bookworm' '@1736726400'",
		"ENV PATH=/usr/local/bin:/usr/bin:/bin",
		"RUN /bin/sh -c set -eux; \tapt-get update; \tapt-get install -y --no-install-recommends ca-certificates netbase tzdata; \trm -rf /var/lib/apt/lists/* # buildkit",
		"ENV PYTHON_VERSION=3.12.8",
		`CMD ["python3"]`,
		"WORKDIR /app",
		"ARG APP_VERSION=1.0",
		"COPY requirements.txt . # buildkit",
		"RUN |1 APP_VERSION=1.0 /bin/sh -c pip install --no-cache-dir -r requirements.txt # buildkit",
		"COPY --chown=app:app src/ ./src/ # buildkit",
		"COPY <<EOT /etc/motd # buildkit",
		"RUN |1 APP_VERSION=1.0 /bin/sh -c set -e\nuseradd app\n # buildkit",
		"LABEL org.opencontainers.image.title=My App",
		"EXPOSE [8000/tcp]",
		"HEALTHCHECK &{Test:[CMD-SHELL curl -f http://localhost:8000/ || exit 1] Interval:30s Timeout:5s StartPeriod:0s StartInterval:0s Retries:3}",
		"USER app",
		"VOLUME [/data]",
		`ENTRYPOINT ["gunicorn" "-b" "0.0.0.0:8000"]`,
		`CMD ["app:app"]`,
	})

	res, err := GenerateDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)

	require.Equal(t, generatedDockerfileHeader+`
FROM scratch
# debian.sh --arch 'amd64' out/ 'bookworm' '@1736726400'
ENV PATH=/usr/local/bin:/usr/bin:/bin
RUN set -eux; 	apt-get update; 	apt-get install -y --no-install-recommends ca-certificates netbase tzdata; 	rm -rf /var/lib/apt/lists/*
ENV PYTHON_VERSION=3.12.8
CMD ["python3"]
WORKDIR /app
ARG APP_VERSION=1.0
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY --chown=app:app src/ ./src/
COPY <unresolvable-source heredoc:EOT> /etc/motd
RUN <<SLEEKO_EOF
set -e
useradd app
SLEEKO_EOF
LABEL org.opencontainers.image.title="My App"
EXPOSE 8000/tcp
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -f http://localhost:8000/ || exit 1
USER app
VOLUME ["/data"]
ENTRYPOINT ["gunicorn","-b","0.0.0.0:8000"]
CMD ["app:app"]
`, res.Dockerfile)

	f, err := dfparse.Parse(res.Dockerfile)
	require.NoError(t, err)

	contract, err := f.RuntimeContract()
	require.NoError(t, err)
	require.Equal(t, dfparse.RuntimeContract{
		Entrypoint: &dfparse.Command{Args: []string{"gunicorn", "-b", "0.0.0.0:8000"}},
		Cmd:        &dfparse.Command{Args: []string{"app:app"}},
		Expose:     []string{"8000/tcp"},
		Workdir:    "/app",
		User:       "app",
	}, contract)

	runs := f.RunCommands()
	require.Len(t, runs, 3)
	require.Equal(t, "set -e\nuseradd app\n", runs[2].Heredocs[0].Content)
}

// TestGenerateDockerfileFromSource_ClassicImage does the same for an image
// built with the classic builder, in the shape of the official nginx image.
func TestGenerateDockerfileFromSource_ClassicImage(t *testing.T) {
	src := historyImage(t, map[string]any{
		"Cmd":        []string{"nginx", "-g", "daemon off;"},
		"StopSignal": "SIGQUIT",
	}, []string{
		"/bin/sh -c #(nop) ADD file:5d673d25da3a14ce1f6cf66e4c7fd4f4b85a3759a9d93efb3fd9ff852b5b56e4 in / ",
		`/bin/sh -c #(nop)  CMD ["bash"]`,
		"/bin/sh -c #(nop)  LABEL maintainer=NGINX Docker Maintainers <docker-maint@nginx.com>",
		"/bin/sh -c #(nop)  ENV NGINX_VERSION=1.25.3",
		"|1 PKG_RELEASE=1~bookworm /bin/sh -c set -x && apt-get update && apt-get install -y nginx=${NGINX_VERSION}-${PKG_RELEASE}",
		"/bin/sh -c #(nop)  EXPOSE 80",
		"/bin/sh -c #(nop)  STOPSIGNAL SIGQUIT",
		`/bin/sh -c #(nop)  CMD ["nginx" "-g" "daemon off;"]`,
	})

	res, err := GenerateDockerfileFromSource(context.Background(), src)
	require.NoError(t, err)

	require.Equal(t, generatedDockerfileHeader+`
FROM scratch
ADD <unresolvable-source sha256:5d673d25da3a14ce1f6cf66e4c7fd4f4b85a3759a9d93efb3fd9ff852b5b56e4> /
CMD ["bash"]
LABEL maintainer="NGINX Docker Maintainers <docker-maint@nginx.com>"
ENV NGINX_VERSION=1.25.3
RUN set -x && apt-get update && apt-get install -y nginx=${NGINX_VERSION}-${PKG_RELEASE}
EXPOSE 80
STOPSIGNAL SIGQUIT
CMD ["nginx","-g","daemon off;"]
`, res.Dockerfile)

	f, err := dfparse.Parse(res.Dockerfile)
	require.NoError(t, err)
	contract, err := f.RuntimeContract()
	require.NoError(t, err)
	require.Equal(t, &dfparse.Command{Args: []string{"nginx", "-g", "daemon off;"}}, contract.Cmd)
}

// An ENV rebuilt from the final config, because history did not contribute
// one, is quoted the same way as one rendered from history.
func TestTrailingConfigLines_EnvValueWithSpaces(t *testing.T) {
	lines := trailingConfigLines(&ociContainerConfig{Env: []string{"GREETING=hello world", "A=1"}}, map[string]bool{})
	require.Equal(t, []string{`ENV GREETING="hello world"`, "ENV A=1"}, lines)
}
