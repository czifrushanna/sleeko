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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/czifrushanna/sleeko/pkg/score"
)

// Cache stores measured results as JSON files, so O′ is built and scanned
// once per (commit, build parameters, Trivy DB) across runs and pipeline
// configurations, and an image is scanned once per (digest, Trivy DB).
type Cache struct {
	Dir string
}

// baseline is everything measured about O′ that the attempts need.
type baseline struct {
	Image ImageInfo        `json:"image"`
	Scan  score.ScanResult `json:"scan"`
	// Contract is O′'s runtime contract from its image config.
	Contract score.Contract `json:"contract"`
	// SmokeError is why O′ failed its smoke test; empty when it passed.
	SmokeError string `json:"smoke_error,omitempty"`
}

// baselineKey identifies an O′ build: everything that determines it.
func baselineKey(it Item, dbVersion string) string {
	parts := []string{it.Repo, it.Commit, it.Context, it.Dockerfile, it.Platform, it.Target, dbVersion}
	args := make([]string, 0, len(it.BuildArgs))
	for k, v := range it.BuildArgs {
		args = append(args, k+"="+v)
	}
	sort.Strings(args)
	parts = append(parts, args...)
	return hashKey(parts...)
}

func scanKey(digest, dbVersion string) string {
	return hashKey(digest, dbVersion)
}

func hashKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Cache) file(kind, key string) string {
	return filepath.Join(c.Dir, kind, key+".json")
}

// get loads a cached value; ok is false when there is none.
func (c *Cache) get(kind, key string, v any) (bool, error) {
	if c == nil {
		return false, nil
	}
	data, err := os.ReadFile(c.file(kind, key))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, v)
}

// put stores a value, writing a temporary file first so a crash never leaves
// a truncated entry behind.
func (c *Cache) put(kind, key string, v any) error {
	if c == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	file := c.file(kind, key)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	tmp := file + ".tmp" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}
