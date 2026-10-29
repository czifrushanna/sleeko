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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/czifrushanna/sleeko/pkg/score"
)

// The study's scans keep the vulnerable packages of every image, not the
// full inventory, so they cannot tell whether a package C reports a new
// vulnerability in was already in O′ unreported. An inventory rebuilds O′
// and lists every package in it, with the same scanner and database.

// InstalledPackage is one package of an image's inventory, its version
// written the way the scanner writes InstalledVersion in findings.
type InstalledPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Inventory is one rebuilt O′: its full package list and its scan.
type Inventory struct {
	Item     string             `json:"item"`
	Digest   string             `json:"digest"`
	OS       string             `json:"os"`
	EOSL     bool               `json:"eosl"`
	TrivyDB  string             `json:"trivy_db"`
	BuiltAt  time.Time          `json:"built_at"`
	Packages []InstalledPackage `json:"packages"`
	Scan     score.ScanResult   `json:"scan"`
}

// trivyPackages reads the package lists of a Trivy JSON report written with
// --list-all-pkgs.
func trivyPackages(data []byte) ([]InstalledPackage, error) {
	var r struct {
		Results []struct {
			Packages []struct {
				Name    string `json:"Name"`
				Version string `json:"Version"`
				Release string `json:"Release"`
				Epoch   int    `json:"Epoch"`
			} `json:"Packages"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("decode trivy packages: %w", err)
	}
	var out []InstalledPackage
	for _, res := range r.Results {
		for _, p := range res.Packages {
			out = append(out, InstalledPackage{Name: p.Name, Version: fullVersion(p.Epoch, p.Version, p.Release)})
		}
	}
	return out, nil
}

// fullVersion joins epoch, version and release the way Trivy writes a
// finding's InstalledVersion.
func fullVersion(epoch int, version, release string) string {
	v := version
	if release != "" {
		v += "-" + release
	}
	if epoch > 0 {
		v = strconv.Itoa(epoch) + ":" + v
	}
	return v
}

// ScanInventory scans an image archive like Scan, and also lists every
// package in it.
func (t *TrivyScanner) ScanInventory(ctx context.Context, img Image) (ScanOutput, []InstalledPackage, error) {
	res, err := t.run(ctx, "trivy inventory scan", img.Archive,
		"image", "--quiet", "--format", "json", "--scanners", "vuln", "--list-all-pkgs",
		"--skip-db-update", "--skip-java-db-update")
	if err != nil {
		return ScanOutput{}, nil, err
	}
	out, err := parseScan(res.Stdout)
	if err != nil {
		return ScanOutput{}, nil, err
	}
	pkgs, err := trivyPackages(res.Stdout)
	return out, pkgs, err
}

// BuildInventory checks out an item, builds O′ exactly as the runner does
// (same context, build arguments, platform, target and build limit), and
// takes its inventory. The image is removed afterwards.
func BuildInventory(ctx context.Context, it Item, src Source, b Builder, t *TrivyScanner, workDir, dbVersion string, timeout time.Duration) (Inventory, error) {
	dir, err := src.Checkout(ctx, it.Repo, it.Commit)
	if err != nil {
		return Inventory{}, fmt.Errorf("checkout: %w", err)
	}
	r := &Runner{Builder: b, BuildTimeout: timeout, WorkDir: workDir}
	img, err := r.build(ctx, r.spec(it, dir, filepath.Join(dir, it.Dockerfile), "inventory", "o"))
	if err != nil {
		return Inventory{}, fmt.Errorf("build: %w", err)
	}
	defer r.remove(img)
	scan, pkgs, err := t.ScanInventory(ctx, img)
	if err != nil {
		return Inventory{}, fmt.Errorf("scan: %w", err)
	}
	inv := Inventory{Item: it.ID, Digest: img.Digest, OS: scan.OS, TrivyDB: dbVersion, BuiltAt: time.Now().UTC(), Packages: pkgs, Scan: scan.Result}
	if scan.EOSL != nil {
		inv.EOSL = *scan.EOSL
	}
	return inv, nil
}

// WriteInventory saves an inventory as DIR/<item>.json.
func WriteInventory(dir string, inv Inventory) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(inv, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, inv.Item+".json"), data, 0o644)
}

// LoadInventories reads every DIR/<item>.json, keyed by item.
func LoadInventories(dir string) (map[string]Inventory, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := map[string]Inventory{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var inv Inventory
		if err := json.Unmarshal(data, &inv); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out[inv.Item] = inv
	}
	return out, nil
}
