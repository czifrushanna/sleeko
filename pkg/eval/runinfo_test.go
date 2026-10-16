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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/score"
)

func testInfo() RunInfo {
	return RunInfo{
		StartedAt: time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC), Code: "abc123", Pipeline: "W0", Stages: "generate",
		Model: "m", MaxTokens: 4096, ContextWindow: 32768, PromptHash: "p1", Prompt: "optimize:\n",
		Manifest: "eval/benchmark/v1.yaml", ManifestSHA256: "m1", Items: []string{"a", "b"},
		Repetitions: 3, Concurrency: 3, BuildTimeout: "10m0s", MaxOriginalAge: "12h0m0s",
		SizeTolerance: 0.1, Weights: score.DefaultWeights, TrivyImage: DefaultTrivyImage, TrivyDB: "db1",
	}
}

func TestAppendRunInfo(t *testing.T) {
	file := filepath.Join(t.TempDir(), "w0.runs.jsonl")
	first := testInfo()
	require.NoError(t, AppendRunInfo(file, first, false))

	// A resume with only harmless differences is recorded.
	resume := testInfo()
	resume.StartedAt = first.StartedAt.Add(2 * time.Hour)
	resume.Concurrency = 1
	resume.Items = []string{"b"}
	require.NoError(t, AppendRunInfo(file, resume, false))

	// A resume that would mix experiments is refused, naming the differences.
	changed := testInfo()
	changed.Code = "def456-dirty"
	changed.PromptHash = "p2"
	err := AppendRunInfo(file, changed, false)
	require.ErrorContains(t, err, "code: abc123 → def456-dirty")
	require.ErrorContains(t, err, "prompt_hash: p1 → p2")

	infos, err := ReadRunInfo(file)
	require.NoError(t, err)
	require.Len(t, infos, 2, "the refused run is not recorded")
	require.Equal(t, first, infos[0])

	require.NoError(t, AppendRunInfo(file, changed, true), "--allow-mixed-runs")
	infos, err = ReadRunInfo(file)
	require.NoError(t, err)
	require.Len(t, infos, 3)

	none, err := ReadRunInfo(filepath.Join(t.TempDir(), "missing"))
	require.NoError(t, err)
	require.Nil(t, none)
}
