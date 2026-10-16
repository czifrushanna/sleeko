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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/score"
)

func TestLineDiff(t *testing.T) {
	require.Equal(t,
		"- FROM python:3.11\n+ FROM python:3.12-slim\n  WORKDIR /app\n- RUN make\n  COPY . .\n+ COPY pyproject.toml .\n",
		LineDiff("FROM python:3.11\nWORKDIR /app\nRUN make\nCOPY . .\n", "FROM python:3.12-slim\nWORKDIR /app\nCOPY . .\nCOPY pyproject.toml .\n"))
	require.Equal(t, "  FROM a\n", LineDiff("FROM a\n", "FROM a"))
}

func TestFailures(t *testing.T) {
	g := func(id score.GateID, status score.Status, reason string) score.Gate {
		return score.Gate{ID: id, Status: status, Reason: reason}
	}
	invented := Record{Item: "a", Pipeline: "W0", Repetition: 2, Status: StatusMeasured,
		OriginalDockerfile: "FROM x\nCOPY . .\n", CandidateDockerfile: "FROM x\nCOPY pyproject.toml .\n",
		Gates: []score.Gate{g(score.G1Parse, score.Pass, ""), g(score.G2NoInvention, score.Fail, "context sources not in the original: pyproject.toml (line 2)")}}
	records := []Record{
		invented,
		{Item: "a", Pipeline: "W0", Repetition: 1, Status: StatusMeasured, Gates: []score.Gate{g(score.G4Build, score.Fail, "exit code 1")}},
		// Superseded by a later record of the same attempt.
		{Item: "b", Pipeline: "W0", Repetition: 1, Status: StatusMeasured, Gates: []score.Gate{g(score.G6NoNewSevere, score.Fail, "CVE-1")}},
		{Item: "b", Pipeline: "W0", Repetition: 1, Status: StatusMeasured, Gates: []score.Gate{g(score.G6NoNewSevere, score.Pass, "")}},
		{Item: "c", Pipeline: "W0", Repetition: 1, Status: StatusOptimizerFailed},
	}

	all := Failures(records, "")
	require.Len(t, all, 2)
	require.Equal(t, 1, all[0].Record.Repetition, "sorted by item, then repetition")

	g2 := Failures(records, score.G2NoInvention)
	require.Len(t, g2, 1)

	var buf bytes.Buffer
	require.NoError(t, WriteFailures(&buf, g2))
	require.Equal(t, "=== a  W0 #2\nG2_no_invention: context sources not in the original: pyproject.toml (line 2)\n"+
		"  FROM x\n- COPY . .\n+ COPY pyproject.toml .\n\n", buf.String())

	dir := t.TempDir()
	require.NoError(t, SaveFailures(dir, g2))
	cand, err := os.ReadFile(filepath.Join(dir, "a-w0-2", "candidate.Dockerfile"))
	require.NoError(t, err)
	require.Equal(t, invented.CandidateDockerfile, string(cand))
	gates, err := os.ReadFile(filepath.Join(dir, "a-w0-2", "gates.json"))
	require.NoError(t, err)
	require.Contains(t, string(gates), `"G2_no_invention"`)
}
