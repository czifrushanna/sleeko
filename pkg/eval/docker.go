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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/czifrushanna/sleeko/pkg/score"
)

// DockerBuilder builds with `docker buildx build` into a Docker-format
// archive. The archive carries an OCI index, so the manifest digest and the
// compressed layer sizes — the schedule's image size — are read from it
// directly, without a registry; it is then loaded into the local Docker
// engine so the smoke test can run it.
type DockerBuilder struct {
	exec execFunc
}

func (d *DockerBuilder) run(ctx context.Context, what string, args ...string) (execResult, error) {
	run := d.exec
	if run == nil {
		run = runCommand
	}
	res, err := run(ctx, "", "docker", args...)
	return mustSucceed(res, err, "docker "+what)
}

// buildArgs is the buildx command line for a spec. --pull makes O′ and C
// both start from the base images as they are today, and attestations are
// left out so the archive holds just the image.
func buildArgs(spec BuildSpec, archive string) []string {
	args := []string{
		"buildx", "build",
		"--pull",
		"--provenance=false", "--sbom=false",
		"--platform", spec.Platform,
		"--file", spec.Dockerfile,
		"--output", fmt.Sprintf("type=docker,dest=%s,name=%s", archive, spec.Tag),
	}
	if spec.Target != "" {
		args = append(args, "--target", spec.Target)
	}
	keys := make([]string, 0, len(spec.BuildArgs))
	for k := range spec.BuildArgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--build-arg", k+"="+spec.BuildArgs[k])
	}
	return append(args, spec.ContextDir)
}

// Build builds the image, reads it from the archive and loads it.
func (d *DockerBuilder) Build(ctx context.Context, spec BuildSpec) (Image, error) {
	if err := os.MkdirAll(spec.OutDir, 0o755); err != nil {
		return Image{}, err
	}
	archive := filepath.Join(spec.OutDir, "image.tar")
	if _, err := d.run(ctx, "build", buildArgs(spec, archive)...); err != nil {
		return Image{}, err
	}

	img, err := readArchive(archive)
	if err != nil {
		return Image{}, fmt.Errorf("read image archive: %w", err)
	}
	img.Tag, img.Archive = spec.Tag, archive

	if _, err := d.run(ctx, "load", "load", "--quiet", "--input", archive); err != nil {
		return Image{}, err
	}
	return img, nil
}

// Remove deletes the image from the engine and its archive.
func (d *DockerBuilder) Remove(ctx context.Context, img Image) error {
	var errs []error
	if img.Tag != "" {
		if _, err := d.run(ctx, "remove image", "image", "rm", "--force", img.Tag); err != nil {
			errs = append(errs, err)
		}
	}
	if img.Archive != "" {
		if err := os.RemoveAll(filepath.Dir(img.Archive)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type ociDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type ociIndex struct {
	Manifests []ociDescriptor `json:"manifests"`
}

type ociManifest struct {
	Config ociDescriptor   `json:"config"`
	Layers []ociDescriptor `json:"layers"`
}

type imageConfig struct {
	Config struct {
		Entrypoint   []string       `json:"Entrypoint"`
		Cmd          []string       `json:"Cmd"`
		ExposedPorts map[string]any `json:"ExposedPorts"`
		WorkingDir   string         `json:"WorkingDir"`
		User         string         `json:"User"`
	} `json:"config"`
}

// maxMetadataBlob bounds the archive entries kept in memory while reading:
// the index, manifest and config are small, the layers are not needed.
const maxMetadataBlob = 4 << 20

// readArchive reads the manifest digest, the compressed size and the
// runtime contract from a Docker-format archive with an OCI index.
func readArchive(file string) (Image, error) {
	f, err := os.Open(file)
	if err != nil {
		return Image{}, err
	}
	defer f.Close()

	small := map[string][]byte{}
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Image{}, err
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Size > maxMetadataBlob {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return Image{}, err
		}
		small[strings.TrimPrefix(hdr.Name, "./")] = data
	}

	blob := func(desc ociDescriptor, v any) error {
		algo, hex, ok := strings.Cut(desc.Digest, ":")
		if !ok {
			return fmt.Errorf("malformed digest %q", desc.Digest)
		}
		data, ok := small[filepath.ToSlash(filepath.Join("blobs", algo, hex))]
		if !ok {
			return fmt.Errorf("blob %s not in archive", desc.Digest)
		}
		return json.Unmarshal(data, v)
	}

	var index ociIndex
	data, ok := small["index.json"]
	if !ok {
		return Image{}, errors.New("no index.json: not an OCI-compatible archive")
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return Image{}, fmt.Errorf("index.json: %w", err)
	}
	if len(index.Manifests) != 1 {
		return Image{}, fmt.Errorf("index.json lists %d manifests, want 1 (one platform, no attestations)", len(index.Manifests))
	}

	var manifest ociManifest
	if err := blob(index.Manifests[0], &manifest); err != nil {
		return Image{}, fmt.Errorf("manifest: %w", err)
	}
	var cfg imageConfig
	if err := blob(manifest.Config, &cfg); err != nil {
		return Image{}, fmt.Errorf("config: %w", err)
	}

	var size int64
	for _, l := range manifest.Layers {
		size += l.Size
	}
	ports := make([]string, 0, len(cfg.Config.ExposedPorts))
	for p := range cfg.Config.ExposedPorts {
		ports = append(ports, p)
	}
	sort.Strings(ports)

	return Image{
		Digest: index.Manifests[0].Digest,
		Size:   size,
		Contract: score.Contract{
			Entrypoint: cfg.Config.Entrypoint,
			Cmd:        cfg.Config.Cmd,
			Expose:     ports,
			Workdir:    cfg.Config.WorkingDir,
			User:       cfg.Config.User,
		},
	}, nil
}

// DockerSmoker runs smoke tests with `docker run`.
type DockerSmoker struct {
	exec execFunc
}

// Smoke runs the image with the smoke command and compares the exit code,
// or with RunFor checks that it starts and keeps running. The container gets
// a unique name so it can always be removed.
func (d *DockerSmoker) Smoke(ctx context.Context, img Image, platform string, smoke Smoke) error {
	run := d.exec
	if run == nil {
		run = runCommand
	}

	name := "sleeko-smoke-" + hashKey(img.Tag, time.Now().String())[:12]
	runArgs := func(mode string) []string {
		args := []string{"run", mode, "--name", name, "--platform", platform}
		if smoke.Entrypoint != "" {
			args = append(args, "--entrypoint", smoke.Entrypoint)
		}
		args = append(args, img.Tag)
		return append(args, smoke.Command...)
	}

	if smoke.RunFor > 0 {
		return d.smokeRunning(ctx, run, name, runArgs("--detach"), smoke)
	}
	args := runArgs("--rm")

	timeout := smoke.Timeout
	if timeout <= 0 {
		timeout = DefaultSmokeTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := run(runCtx, "", "docker", args...)
	if runCtx.Err() != nil && ctx.Err() == nil {
		run(context.Background(), "", "docker", "rm", "--force", name)
		return fmt.Errorf("timed out after %s", timeout)
	}
	if err != nil {
		return err
	}
	if res.ExitCode != smoke.ExitCode {
		return fmt.Errorf("exit code %d, want %d: %s", res.ExitCode, smoke.ExitCode, tail(res.Stderr, res.Stdout))
	}
	return nil
}

// smokeRunning starts the container detached, waits RunFor, and passes if it
// is still running or has exited with the expected code. The container is
// removed whatever happens.
func (d *DockerSmoker) smokeRunning(ctx context.Context, run execFunc, name string, args []string, smoke Smoke) error {
	defer run(context.Background(), "", "docker", "rm", "--force", name)

	res, err := run(ctx, "", "docker", args...)
	if _, err := mustSucceed(res, err, "docker run"); err != nil {
		return err
	}
	select {
	case <-time.After(smoke.RunFor):
	case <-ctx.Done():
		return ctx.Err()
	}

	res, err = run(ctx, "", "docker", "inspect", "--format", "{{.State.Running}} {{.State.ExitCode}}", name)
	if res, err = mustSucceed(res, err, "docker inspect"); err != nil {
		return err
	}
	running, code, _ := strings.Cut(strings.TrimSpace(string(res.Stdout)), " ")
	if running == "true" || code == fmt.Sprint(smoke.ExitCode) {
		return nil
	}
	logs, _ := run(context.Background(), "", "docker", "logs", "--tail", "15", name)
	return fmt.Errorf("exited with code %s within %s, want it running or exit code %d: %s",
		code, smoke.RunFor, smoke.ExitCode, tail(logs.Stderr, logs.Stdout))
}
