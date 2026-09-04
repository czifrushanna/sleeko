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

package dockerfile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultPrompt_MatchesScanReportFormat guards against the default prompt
// silently drifting out of sync with the headings pkg/scanreport actually
// renders - the prompt's reading instructions are useless to the model if
// they describe a format the summary no longer produces.
func TestDefaultPrompt_MatchesScanReportFormat(t *testing.T) {
	require.NotEmpty(t, DefaultPrompt)
	require.Contains(t, DefaultPrompt, "Vulnerability scan summary:")
	require.Contains(t, DefaultPrompt, "Packages with a fix available")
	require.Contains(t, DefaultPrompt, "No fix available")
	require.Contains(t, DefaultPrompt, "(+")
}

// TestDefaultPrompt_EndsReadyForConcatenation guards the adapter's
// concatenation convention: OptimizeDockerfile appends the Dockerfile
// directly onto the prompt with no separator (see llm.go), so the prompt
// must supply its own trailing newline after the "Dockerfile:" label.
func TestDefaultPrompt_EndsReadyForConcatenation(t *testing.T) {
	require.True(t, strings.HasSuffix(DefaultPrompt, "Dockerfile:\n\n"),
		"DefaultPrompt must end with \"Dockerfile:\\n\\n\" so the recovered Dockerfile is concatenated cleanly")
}

// TestDefaultPrompt_LeavesRoomForScanReport guards against the default prompt
// growing large enough to crowd the condensed scan report below its floor at
// the adapter's default context window.
func TestDefaultPrompt_LeavesRoomForScanReport(t *testing.T) {
	budget := Budget{MaxTokens: 4096, ContextWindow: 32768}
	got := budget.scanReportBudget(DefaultPrompt, "FROM scratch\n")
	require.Greater(t, got, minScanReportChars*4,
		"default prompt leaves too little room for the condensed scan report")
}
