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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// execResult is a finished command.
type execResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// execFunc runs a command. It returns an error only when the command could
// not be run at all or the context ended; a non-zero exit is in ExitCode.
type execFunc func(ctx context.Context, dir, name string, args ...string) (execResult, error)

// waitDelay bounds how long a cancelled command may keep its output open.
const waitDelay = 10 * time.Second

func runCommand(ctx context.Context, dir, name string, args ...string) (execResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	// Cancelling must stop the command's children too: `docker buildx` runs
	// as a plugin child of `docker`, and killing only the parent leaves the
	// build running and holding the output open, so Wait would block until
	// it finished on its own, past any timeout.
	killProcessGroup(cmd)
	cmd.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()

	res := execResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return res, ctx.Err()
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	case err != nil:
		return res, err
	}
	return res, nil
}

// mustSucceed turns a non-zero exit into an error carrying the end of the
// command's output, where tools put the reason.
func mustSucceed(res execResult, err error, what string) (execResult, error) {
	if err != nil {
		return res, fmt.Errorf("%s: %w", what, err)
	}
	if res.ExitCode != 0 {
		return res, fmt.Errorf("%s: exit code %d: %s", what, res.ExitCode, tail(res.Stderr, res.Stdout))
	}
	return res, nil
}

// tail returns the last lines of the first non-empty output, so an error
// message carries the reason without the whole build log.
func tail(outputs ...[]byte) string {
	const maxLines = 15
	for _, out := range outputs {
		s := strings.TrimSpace(string(out))
		if s == "" {
			continue
		}
		lines := strings.Split(s, "\n")
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
		return strings.Join(lines, "\n")
	}
	return "(no output)"
}
