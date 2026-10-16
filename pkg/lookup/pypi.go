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

package lookup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/czifrushanna/sleeko/pkg/versions"
)

var pep503 = regexp.MustCompile(`[-_.]+`)

// normalizeName is PEP 503's normalised project name.
func normalizeName(name string) string {
	return strings.ToLower(pep503.ReplaceAllString(name, "-"))
}

// PipHas reports whether PyPI has a release of the project at the version
// (by PEP 440 equality, so "1.0" matches "1.0.0") with at least one file
// that has not been yanked. A project PyPI does not know has no versions.
func (c *Client) PipHas(ctx context.Context, project, version string) (bool, error) {
	want, err := versions.ParsePEP440(version)
	if err != nil {
		return false, err
	}
	releases, err := c.pypiReleases(ctx, project)
	if err != nil {
		return false, err
	}
	for _, r := range releases {
		if v, err := versions.ParsePEP440(r); err == nil && v.Compare(want) == 0 {
			return true, nil
		}
	}
	return false, nil
}

// pypiReleases lists the project's releases that have unyanked files.
func (c *Client) pypiReleases(ctx context.Context, project string) ([]string, error) {
	name := normalizeName(project)
	defer c.keyLock("pypi:" + name)()

	c.mu.Lock()
	if rs, ok := c.pypi[name]; ok {
		c.mu.Unlock()
		return rs, nil
	}
	c.mu.Unlock()

	u := or(c.PyPI, "https://pypi.org/pypi") + "/" + name + "/json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var releases []string
	switch resp.StatusCode {
	case http.StatusOK:
		var doc struct {
			Releases map[string][]struct {
				Yanked bool `json:"yanked"`
			} `json:"releases"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&doc); err != nil {
			return nil, fmt.Errorf("%s: %w", u, err)
		}
		for version, files := range doc.Releases {
			for _, f := range files {
				if !f.Yanked {
					releases = append(releases, version)
					break
				}
			}
		}
	case http.StatusNotFound:
	default:
		return nil, fmt.Errorf("%s: %s", u, resp.Status)
	}

	c.mu.Lock()
	if c.pypi == nil {
		c.pypi = map[string][]string{}
	}
	c.pypi[name] = releases
	c.mu.Unlock()
	return releases, nil
}
