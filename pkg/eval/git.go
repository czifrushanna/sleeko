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
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// GitSource checks repositories out with the git CLI, one directory per
// (repository, commit) under Dir, reused across items and runs.
type GitSource struct {
	Dir string

	exec  execFunc
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (g *GitSource) run(ctx context.Context, dir string, args ...string) (execResult, error) {
	run := g.exec
	if run == nil {
		run = runCommand
	}
	res, err := run(ctx, dir, "git", args...)
	return mustSucceed(res, err, "git "+args[0])
}

// lock serialises work on one checkout directory, so two items from the
// same repository and commit do not clone over each other.
func (g *GitSource) lock(dir string) func() {
	g.mu.Lock()
	if g.locks == nil {
		g.locks = map[string]*sync.Mutex{}
	}
	l, ok := g.locks[dir]
	if !ok {
		l = &sync.Mutex{}
		g.locks[dir] = l
	}
	g.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// Checkout fetches just the commit (servers such as GitHub allow fetching a
// commit by its SHA) and checks it out detached. A directory already at the
// commit is reused as it is.
func (g *GitSource) Checkout(ctx context.Context, repo, commit string) (string, error) {
	dir := filepath.Join(g.Dir, hashKey(repo)[:16], commit)
	defer g.lock(dir)()

	if res, err := g.run(ctx, dir, "rev-parse", "HEAD"); err == nil && strings.TrimSpace(string(res.Stdout)) == commit {
		if res, err := g.run(ctx, dir, "status", "--porcelain"); err == nil && len(res.Stdout) == 0 {
			return dir, nil
		}
	}

	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	steps := [][]string{
		{"init", "--quiet"},
		{"remote", "add", "origin", repo},
		{"fetch", "--quiet", "--depth", "1", "origin", commit},
		{"checkout", "--quiet", "--detach", "FETCH_HEAD"},
	}
	for _, args := range steps {
		if _, err := g.run(ctx, dir, args...); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}
