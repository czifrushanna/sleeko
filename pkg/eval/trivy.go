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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/czifrushanna/sleeko/pkg/score"
)

// DefaultTrivyImage pins the Trivy version: O′ and C must be scanned by the
// same one.
const DefaultTrivyImage = "aquasec/trivy:0.75.0"

// TrivyScanner scans image archives with Trivy, either the trivy binary or
// Trivy's container image. The vulnerability database lives in CacheDir and
// is never updated during a run (--skip-db-update), so every scan in a run
// uses the same one; download it beforehand with DownloadDB.
type TrivyScanner struct {
	// CacheDir holds Trivy's databases. Must be an absolute path.
	CacheDir string
	// Image runs Trivy in Docker from this image; empty uses the trivy
	// binary on PATH.
	Image string

	exec execFunc
}

const (
	containerCache = "/trivy-cache"
	containerInput = "/input"
)

// command builds the invocation. In container mode the cache and the
// archive's directory are mounted, and paths are rewritten to the mounts.
func (t *TrivyScanner) command(archive string, args ...string) (string, []string) {
	cache := t.CacheDir
	if t.Image == "" {
		if archive != "" {
			args = append(args, "--input", archive)
		}
		return "trivy", append(args, "--cache-dir", cache)
	}

	docker := []string{"run", "--rm", "-v", cache + ":" + containerCache}
	if archive != "" {
		docker = append(docker, "-v", filepath.Dir(archive)+":"+containerInput+":ro")
		args = append(args, "--input", containerInput+"/"+filepath.Base(archive))
	}
	docker = append(docker, t.Image)
	return "docker", append(append(docker, args...), "--cache-dir", containerCache)
}

func (t *TrivyScanner) run(ctx context.Context, what, archive string, args ...string) (execResult, error) {
	if !filepath.IsAbs(t.CacheDir) {
		return execResult{}, fmt.Errorf("trivy cache dir %q must be absolute", t.CacheDir)
	}
	run := t.exec
	if run == nil {
		run = runCommand
	}
	name, full := t.command(archive, args...)
	res, err := run(ctx, "", name, full...)
	return mustSucceed(res, err, what)
}

// DownloadDB downloads the vulnerability and Java databases into CacheDir.
func (t *TrivyScanner) DownloadDB(ctx context.Context) error {
	if _, err := t.run(ctx, "download vulnerability db", "", "image", "--download-db-only", "--quiet"); err != nil {
		return err
	}
	_, err := t.run(ctx, "download java db", "", "image", "--download-java-db-only", "--quiet")
	return err
}

// DBVersion is the database's UpdatedAt timestamp, which identifies it.
func (t *TrivyScanner) DBVersion(ctx context.Context) (string, error) {
	res, err := t.run(ctx, "trivy version", "", "version", "--format", "json")
	if err != nil {
		return "", err
	}
	var v struct {
		Version         string `json:"Version"`
		VulnerabilityDB *struct {
			UpdatedAt string `json:"UpdatedAt"`
		} `json:"VulnerabilityDB"`
	}
	if err := json.Unmarshal(res.Stdout, &v); err != nil {
		return "", fmt.Errorf("trivy version output: %w", err)
	}
	if v.VulnerabilityDB == nil || v.VulnerabilityDB.UpdatedAt == "" {
		return "", errors.New("no vulnerability database in the cache; run `sleeko-eval db` first")
	}
	return "trivy " + v.Version + " db " + v.VulnerabilityDB.UpdatedAt, nil
}

// Scan scans the image archive for vulnerabilities only.
func (t *TrivyScanner) Scan(ctx context.Context, img Image) (ScanOutput, error) {
	res, err := t.run(ctx, "trivy scan", img.Archive,
		"image", "--quiet", "--format", "json", "--scanners", "vuln",
		"--skip-db-update", "--skip-java-db-update")
	if err != nil {
		return ScanOutput{}, err
	}

	result, err := score.ParseTrivy(res.Stdout)
	if err != nil {
		return ScanOutput{}, err
	}
	var meta struct {
		Metadata struct {
			OS *struct {
				Family string `json:"Family"`
				Name   string `json:"Name"`
				EOSL   bool   `json:"EOSL"`
			} `json:"OS"`
		} `json:"Metadata"`
	}
	if err := json.Unmarshal(res.Stdout, &meta); err != nil {
		return ScanOutput{}, err
	}
	eosl := false
	out := ScanOutput{Result: result, EOSL: &eosl}
	if os := meta.Metadata.OS; os != nil {
		out.OS = os.Family + " " + os.Name
		eosl = os.EOSL
	}
	return out, nil
}
