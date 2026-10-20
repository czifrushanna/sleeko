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

package eval

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/pipeline"
	"github.com/czifrushanna/sleeko/pkg/score"
)

// recordingExec records command lines and answers from a script.
type recordingExec struct {
	calls  [][]string
	answer func(args []string) (execResult, error)
}

func (r *recordingExec) run(ctx context.Context, _ string, name string, args ...string) (execResult, error) {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	if r.answer == nil {
		return execResult{}, nil
	}
	return r.answer(call)
}

func TestBuildArgs(t *testing.T) {
	args := buildArgs(BuildSpec{
		ContextDir: "/src/app", Dockerfile: "/work/c/Dockerfile", Platform: "linux/amd64",
		Target: "runtime", Tag: "sleeko-eval/x:c-w0-1", BuildArgs: map[string]string{"B": "2", "A": "1"},
	}, "/work/img/image.tar")
	require.Equal(t, []string{
		"buildx", "build", "--pull", "--provenance=false", "--sbom=false",
		"--platform", "linux/amd64", "--file", "/work/c/Dockerfile",
		"--output", "type=docker,dest=/work/img/image.tar,name=sleeko-eval/x:c-w0-1",
		"--target", "runtime",
		"--build-arg", "A=1", "--build-arg", "B=2",
		"/src/app",
	}, args)
}

// writeArchive writes a Docker-format archive with an OCI index, as
// `buildx build --output type=docker` does, with the given layer sizes.
func writeArchive(t *testing.T, file string, config map[string]any, layerSizes []int64, manifests int) string {
	t.Helper()
	f, err := os.Create(file)
	require.NoError(t, err)
	defer f.Close()
	tw := tar.NewWriter(f)

	add := func(name string, data []byte) {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(data)
		require.NoError(t, err)
	}
	blob := func(data []byte) string {
		sum := sha256.Sum256(data)
		h := hex.EncodeToString(sum[:])
		add("blobs/sha256/"+h, data)
		return "sha256:" + h
	}

	cfg, _ := json.Marshal(config)
	cfgDigest := blob(cfg)
	var layers []map[string]any
	for i, size := range layerSizes {
		// Layer blobs are stand-ins: the reader takes sizes from the manifest.
		layers = append(layers, map[string]any{"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip", "digest": blob([]byte{byte(i)}), "size": size})
	}
	manifest, _ := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"config":        map[string]any{"digest": cfgDigest, "size": len(cfg)},
		"layers":        layers,
	})
	mDigest := blob(manifest)
	var entries []map[string]any
	for i := 0; i < manifests; i++ {
		entries = append(entries, map[string]any{"digest": mDigest, "size": len(manifest)})
	}
	index, _ := json.Marshal(map[string]any{"schemaVersion": 2, "manifests": entries})
	add("index.json", index)
	add("manifest.json", []byte(`[]`))
	require.NoError(t, tw.Close())
	return mDigest
}

func TestReadArchive(t *testing.T) {
	file := filepath.Join(t.TempDir(), "image.tar")
	digest := writeArchive(t, file, map[string]any{"config": map[string]any{
		"Entrypoint":   []string{"/server"},
		"Cmd":          nil,
		"ExposedPorts": map[string]any{"9090/tcp": map[string]any{}, "8080/tcp": map[string]any{}},
		"WorkingDir":   "/app",
		"User":         "65532",
	}}, []int64{3630321, 174}, 1)

	img, err := readArchive(file)
	require.NoError(t, err)
	require.Equal(t, digest, img.Digest)
	require.Equal(t, int64(3630321+174), img.Size)
	require.Equal(t, score.Contract{
		Entrypoint: []string{"/server"},
		Expose:     []string{"8080/tcp", "9090/tcp"},
		Workdir:    "/app",
		User:       "65532",
	}, img.Contract)
}

func TestReadArchive_Rejects(t *testing.T) {
	dir := t.TempDir()

	multi := filepath.Join(dir, "multi.tar")
	writeArchive(t, multi, map[string]any{}, []int64{1}, 2)
	_, err := readArchive(multi)
	require.ErrorContains(t, err, "2 manifests")

	plain := filepath.Join(dir, "plain.tar")
	f, err := os.Create(plain)
	require.NoError(t, err)
	tw := tar.NewWriter(f)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: 2}))
	_, err = tw.Write([]byte("[]"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, f.Close())
	_, err = readArchive(plain)
	require.ErrorContains(t, err, "no index.json")

	_, err = readArchive(filepath.Join(dir, "missing.tar"))
	require.Error(t, err)
}

func TestDockerBuilder(t *testing.T) {
	out := t.TempDir()
	rec := &recordingExec{answer: func(args []string) (execResult, error) {
		if args[1] == "buildx" {
			dest := strings.TrimPrefix(strings.Split(args[11], ",")[1], "dest=")
			writeArchive(t, dest, map[string]any{"config": map[string]any{"User": "app"}}, []int64{100}, 1)
		}
		return execResult{}, nil
	}}
	b := &DockerBuilder{exec: rec.run}
	img, err := b.Build(context.Background(), BuildSpec{ContextDir: "/src", Dockerfile: "/src/Dockerfile", Platform: "linux/amd64", Tag: "sleeko-eval/x:o-1", OutDir: out})
	require.NoError(t, err)
	require.Equal(t, "sleeko-eval/x:o-1", img.Tag)
	require.Equal(t, int64(100), img.Size)
	require.Equal(t, "app", img.Contract.User)
	require.Equal(t, []string{"docker", "load", "--quiet", "--input", filepath.Join(out, "image.tar")}, rec.calls[1])

	require.NoError(t, b.Remove(context.Background(), img))
	require.Equal(t, []string{"docker", "image", "rm", "--force", "sleeko-eval/x:o-1"}, rec.calls[2])
	_, err = os.Stat(out)
	require.True(t, os.IsNotExist(err), "the archive directory is removed")
}

func TestDockerBuilder_BuildFailureCarriesTheReason(t *testing.T) {
	rec := &recordingExec{answer: func([]string) (execResult, error) {
		return execResult{ExitCode: 1, Stderr: []byte("#8 ERROR: process \"/bin/sh -c pip install nope\" did not complete\n")}, nil
	}}
	_, err := (&DockerBuilder{exec: rec.run}).Build(context.Background(), BuildSpec{OutDir: t.TempDir(), Platform: "linux/amd64"})
	require.ErrorContains(t, err, "docker build: exit code 1")
	require.ErrorContains(t, err, "pip install nope")
}

func TestDockerSmoker(t *testing.T) {
	img := Image{Tag: "sleeko-eval/x:c-w0-1"}
	cases := []struct {
		name     string
		smoke    Smoke
		exitCode int
		wantArgs []string
		wantErr  string
	}{
		{
			name:     "command passes",
			smoke:    Smoke{Command: []string{"python", "-c", "import app"}},
			wantArgs: []string{"sleeko-eval/x:c-w0-1", "python", "-c", "import app"},
		},
		{
			name:     "entrypoint override and expected non-zero exit",
			smoke:    Smoke{Entrypoint: "/bin/sh", Command: []string{"-c", "exit 3"}, ExitCode: 3},
			exitCode: 3,
			wantArgs: []string{"--entrypoint", "/bin/sh", "sleeko-eval/x:c-w0-1", "-c", "exit 3"},
		},
		{
			name:     "unexpected exit code",
			smoke:    Smoke{},
			exitCode: 1,
			wantArgs: []string{"sleeko-eval/x:c-w0-1"},
			wantErr:  "exit code 1, want 0: boom",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := &recordingExec{answer: func([]string) (execResult, error) {
				return execResult{ExitCode: c.exitCode, Stderr: []byte("boom")}, nil
			}}
			err := (&DockerSmoker{exec: rec.run}).Smoke(context.Background(), img, "linux/amd64", c.smoke)
			if c.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, c.wantErr)
			}
			call := rec.calls[0]
			require.Equal(t, []string{"docker", "run", "--rm", "--name"}, call[:4])
			require.Equal(t, []string{"--platform", "linux/amd64"}, call[5:7])
			require.Equal(t, c.wantArgs, call[7:])
		})
	}
}

func TestDockerSmoker_Timeout(t *testing.T) {
	rec := &recordingExec{answer: func(args []string) (execResult, error) {
		if args[1] == "run" {
			time.Sleep(50 * time.Millisecond)
			return execResult{}, context.DeadlineExceeded
		}
		return execResult{}, nil
	}}
	err := (&DockerSmoker{exec: rec.run}).Smoke(context.Background(), Image{Tag: "x"}, "linux/amd64", Smoke{Timeout: 10 * time.Millisecond})
	require.EqualError(t, err, "timed out after 10ms")
	require.Equal(t, []string{"docker", "rm", "--force", rec.calls[0][4]}, rec.calls[1], "the container is removed")
}

const trivyOutput = `{
  "SchemaVersion": 2,
  "Metadata": {"OS": {"Family": "debian", "Name": "12.8"}},
  "Results": [{"Target": "x", "Vulnerabilities": [
    {"VulnerabilityID": "CVE-1", "PkgName": "libssl3", "InstalledVersion": "1", "FixedVersion": "2", "Severity": "HIGH"}
  ]}]
}`

func TestTrivyScanner_Container(t *testing.T) {
	rec := &recordingExec{answer: func(args []string) (execResult, error) {
		if args[len(args)-3] == "version" || strings.Contains(strings.Join(args, " "), " version ") {
			return execResult{Stdout: []byte(`{"Version":"0.75.0","VulnerabilityDB":{"UpdatedAt":"2026-10-13T13:06:33Z"}}`)}, nil
		}
		return execResult{Stdout: []byte(trivyOutput)}, nil
	}}
	s := &TrivyScanner{CacheDir: "/work/trivy-cache", Image: DefaultTrivyImage, exec: rec.run}

	version, err := s.DBVersion(context.Background())
	require.NoError(t, err)
	require.Equal(t, "trivy 0.75.0 db 2026-10-13T13:06:33Z", version)
	require.Equal(t, []string{"docker", "run", "--rm", "-v", "/work/trivy-cache:/trivy-cache", DefaultTrivyImage,
		"version", "--format", "json", "--cache-dir", "/trivy-cache"}, rec.calls[0])

	out, err := s.Scan(context.Background(), Image{Archive: "/work/images/x/o-1/image.tar"})
	require.NoError(t, err)
	require.Equal(t, "debian 12.8", out.OS)
	require.Len(t, out.Result.Vulnerabilities, 1)
	require.Equal(t, []string{"docker", "run", "--rm",
		"-v", "/work/trivy-cache:/trivy-cache", "-v", "/work/images/x/o-1:/input:ro", DefaultTrivyImage,
		"image", "--quiet", "--format", "json", "--scanners", "vuln", "--skip-db-update", "--skip-java-db-update",
		"--input", "/input/image.tar", "--cache-dir", "/trivy-cache"}, rec.calls[1])

	require.NoError(t, s.DownloadDB(context.Background()))
	require.Contains(t, rec.calls[2], "--download-db-only")
	require.Contains(t, rec.calls[3], "--download-java-db-only")
}

func TestTrivyScanner_Binary(t *testing.T) {
	rec := &recordingExec{answer: func([]string) (execResult, error) { return execResult{Stdout: []byte(trivyOutput)}, nil }}
	s := &TrivyScanner{CacheDir: "/cache", exec: rec.run}
	_, err := s.Scan(context.Background(), Image{Archive: "/a/image.tar"})
	require.NoError(t, err)
	require.Equal(t, []string{"trivy", "image", "--quiet", "--format", "json", "--scanners", "vuln",
		"--skip-db-update", "--skip-java-db-update", "--input", "/a/image.tar", "--cache-dir", "/cache"}, rec.calls[0])
}

func TestTrivyScanner_Errors(t *testing.T) {
	_, err := (&TrivyScanner{CacheDir: "relative"}).DBVersion(context.Background())
	require.ErrorContains(t, err, "must be absolute")

	noDB := &recordingExec{answer: func([]string) (execResult, error) {
		return execResult{Stdout: []byte(`{"Version":"0.75.0"}`)}, nil
	}}
	_, err = (&TrivyScanner{CacheDir: "/c", exec: noDB.run}).DBVersion(context.Background())
	require.ErrorContains(t, err, "sleeko-eval db")

	failing := &recordingExec{answer: func([]string) (execResult, error) {
		return execResult{ExitCode: 1, Stderr: []byte("FATAL: no such file")}, nil
	}}
	_, err = (&TrivyScanner{CacheDir: "/c", exec: failing.run}).Scan(context.Background(), Image{Archive: "/a/x.tar"})
	require.ErrorContains(t, err, "FATAL: no such file")
}

func TestGitSource(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()

	// A local repository with two commits.
	repo := t.TempDir()
	git := func(args ...string) string {
		res, err := runCommand(ctx, repo, "git", args...)
		res, err = mustSucceed(res, err, "git")
		require.NoError(t, err)
		return strings.TrimSpace(string(res.Stdout))
	}
	git("init", "--quiet")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")
	git("config", "uploadpack.allowAnySHA1InWant", "true")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "Dockerfile"), []byte("FROM alpine:3.19\n"), 0o644))
	git("add", ".")
	git("commit", "--quiet", "-m", "one")
	first := git("rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "Dockerfile"), []byte("FROM alpine:3.20\n"), 0o644))
	git("commit", "--quiet", "-am", "two")

	src := &GitSource{Dir: t.TempDir()}
	dir, err := src.Checkout(ctx, "file://"+repo, first)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	require.Equal(t, "FROM alpine:3.19\n", string(data), "the pinned commit, not the latest")

	// Reused as it is; a modified checkout is replaced.
	again, err := src.Checkout(ctx, "file://"+repo, first)
	require.NoError(t, err)
	require.Equal(t, dir, again)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("tampered\n"), 0o644))
	_, err = src.Checkout(ctx, "file://"+repo, first)
	require.NoError(t, err)
	data, _ = os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.Equal(t, "FROM alpine:3.19\n", string(data))

	_, err = src.Checkout(ctx, "file://"+repo, strings.Repeat("0", 40))
	require.ErrorContains(t, err, "git fetch")
}

func TestPipelineOptimizer(t *testing.T) {
	var offset int64
	ok := pipeline.NewPipeline(stageFunc(func(st *pipeline.State) error {
		offset = st.SeedOffset
		st.Candidate = "FROM alpine:3.21\n"
		st.LLMRounds = 2
		st.Diagnostics = append(st.Diagnostics, pipeline.Diagnostic{Stage: "verify", Message: "dropped RUN"})
		return nil
	}))
	out, err := (&PipelineOptimizer{Pipeline: ok}).Optimize(context.Background(), OptimizeInput{Dockerfile: "FROM alpine:3.20\n", Repetition: 3})
	require.NoError(t, err)
	require.Equal(t, int64(3), offset, "the repetition becomes the seed offset")
	require.Equal(t, OptimizeOutput{Candidate: "FROM alpine:3.21\n", LLMRounds: 2, Diagnostics: []string{"verify: dropped RUN"}}, out)

	empty := pipeline.NewPipeline(stageFunc(func(*pipeline.State) error { return nil }))
	_, err = (&PipelineOptimizer{Pipeline: empty}).Optimize(context.Background(), OptimizeInput{})
	require.EqualError(t, err, "pipeline produced no candidate")

	failing := pipeline.NewPipeline(stageFunc(func(st *pipeline.State) error {
		st.LLMRounds = 1
		return errors.New("gateway down")
	}))
	out, err = (&PipelineOptimizer{Pipeline: failing}).Optimize(context.Background(), OptimizeInput{})
	require.ErrorContains(t, err, "gateway down")
	require.Equal(t, 1, out.LLMRounds, "rounds are reported for failed runs too")

	out, err = NoopOptimizer{}.Optimize(context.Background(), OptimizeInput{Dockerfile: "FROM x\n"})
	require.NoError(t, err)
	require.Equal(t, "FROM x\n", out.Candidate)

	require.Len(t, PromptHash("p"), 16)
	require.NotEqual(t, PromptHash("a"), PromptHash("b"))
}

type stageFunc func(*pipeline.State) error

func (stageFunc) Name() string                                      { return "test" }
func (f stageFunc) Run(_ context.Context, st *pipeline.State) error { return f(st) }

func TestDockerSmoker_RunFor(t *testing.T) {
	cases := []struct {
		name    string
		state   string // what docker inspect prints
		wantErr string
	}{
		{"still running", "true 0", ""},
		{"exited with the expected code", "false 0", ""},
		{"exited with another code", "false 1", "exited with code 1 within 10ms, want it running or exit code 0: listen tcp :80: bind: permission denied"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := &recordingExec{answer: func(args []string) (execResult, error) {
				switch args[1] {
				case "inspect":
					return execResult{Stdout: []byte(c.state + "\n")}, nil
				case "logs":
					return execResult{Stderr: []byte("listen tcp :80: bind: permission denied")}, nil
				}
				return execResult{}, nil
			}}
			err := (&DockerSmoker{exec: rec.run}).Smoke(context.Background(), Image{Tag: "img"}, "linux/amd64",
				Smoke{RunFor: 10 * time.Millisecond, Command: []string{"serve"}})
			if c.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, c.wantErr)
			}

			name := rec.calls[0][4]
			require.Equal(t, []string{"docker", "run", "--detach", "--name", name, "--platform", "linux/amd64", "img", "serve"}, rec.calls[0])
			require.Equal(t, []string{"docker", "rm", "--force", name}, rec.calls[len(rec.calls)-1], "always removed")
		})
	}
}

func TestDockerSmoker_RunForStartFailure(t *testing.T) {
	rec := &recordingExec{answer: func(args []string) (execResult, error) {
		if args[1] == "run" {
			return execResult{ExitCode: 125, Stderr: []byte("exec format error")}, nil
		}
		return execResult{}, nil
	}}
	err := (&DockerSmoker{exec: rec.run}).Smoke(context.Background(), Image{Tag: "img"}, "linux/amd64", Smoke{RunFor: time.Second})
	require.ErrorContains(t, err, "exec format error")
	require.Equal(t, "rm", rec.calls[len(rec.calls)-1][1])
}

// A cancelled command must return promptly even when a child it started
// keeps running, as docker's buildx plugin does.
func TestRunCommand_CancelKillsChildren(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := runCommand(ctx, "", "sh", "-c", "sleep 30 & wait")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 5*time.Second, "the child must not keep Wait blocked")
}
