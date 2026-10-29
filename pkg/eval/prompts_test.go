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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/pipeline"
	"github.com/czifrushanna/sleeko/pkg/score"
	"github.com/czifrushanna/sleeko/pkg/verify"
)

func TestReconstructCalls(t *testing.T) {
	fixable := vuln("CVE-1", "openssl", "3.5.0-r0", score.SeverityHigh)
	fixable.FixedVersion = "3.5.1-r0"
	o := scanOf("alpine 3.24.2", false, fixable)
	r := Record{
		Pipeline: "W0+W1", Item: "a", Repetition: 1, LLMRounds: 2, LatencyMS: 1500,
		OriginalDockerfile: "FROM alpine:3.24\n", CandidateDockerfile: "FROM alpine:3.24.3\n",
		Verification: []pipeline.VerifyRound{{Round: 0, Result: verify.Result{Findings: []verify.Finding{{Check: "x", Severity: verify.Fail, Message: "bad tag"}}}}},
	}
	ac, err := ReconstructCalls(r, o, "PROMPT\n", dockerfile.Budget{MaxTokens: 100, ContextWindow: 4000})
	require.NoError(t, err)
	require.Len(t, ac.Calls, 2)
	require.True(t, strings.HasPrefix(ac.Calls[0].Input, "PROMPT\nFROM alpine:3.24\n"))
	require.Contains(t, ac.Calls[0].Input, "openssl")
	require.True(t, ac.Calls[0].Approximate) // its answer was repaired, so the recorded one is the final
	require.True(t, strings.HasPrefix(ac.Calls[1].Input, ac.Calls[0].Input+"FROM alpine:3.24.3\n"))
	require.Contains(t, ac.Calls[1].Input, "bad tag")
	require.Equal(t, "FROM alpine:3.24.3\n", ac.Calls[1].Output)

	none, err := ReconstructCalls(Record{LLMRounds: 0}, o, "P", dockerfile.Budget{})
	require.NoError(t, err)
	require.Empty(t, none.Calls)
}
