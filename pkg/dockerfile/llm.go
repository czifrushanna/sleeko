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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/czifrushanna/sleeko/pkg/scanreport"
)

// Message is one turn of a chat with the gateway.
type Message struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

type chatMessage = Message

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
		Delta   chatMessage `json:"delta"`
	} `json:"choices"`
}

// scanReportHeading introduces the condensed scan report in the prompt, after
// the Dockerfile.
const scanReportHeading = "\n\nVulnerability scan summary:\n"

// Budget bounds what is sent to the gateway.
type Budget struct {
	// MaxTokens is the completion budget, sent to the gateway as max_tokens.
	// Zero omits the field, which leaves the gateway to derive one - and a
	// gateway that derives it by subtracting the prompt from the context window
	// answers an oversized prompt with a negative-max_tokens 400 rather than a
	// legible context-window error.
	MaxTokens int
	// ContextWindow is the model's total token window. Zero disables prompt
	// trimming, so the scan report is sent however long it is.
	ContextWindow int
}

const (
	// charsPerToken converts the token budget into the character budget the scan
	// report is measured against. Prose runs nearer 4 characters per token, but
	// the summary is not prose: a line like
	//
	//	zlib1g 1:1.2.13.dfsg-1 -> 1:1.2.13.dfsg-1+deb12u1 (3 Critical, 7 High)
	//
	// is mostly punctuated version strings, which tokenize close to one token
	// per two characters. Guessing high here would put a trimmed report back
	// over the window, so guess low: on a report small enough not to need
	// trimming the constant costs nothing at all.
	charsPerToken = 2
	// promptOverheadTokens covers the chat template, role markers and whatever
	// else the gateway wraps around the message - none of which appears in the
	// strings measured here.
	promptOverheadTokens = 512
	// minScanReportChars is the floor the report is squeezed to when the prompt
	// prefix and Dockerfile alone leave no room. Trimming the report further
	// cannot rescue an oversized prefix, and sending no vulnerability
	// information at all would silently change the task.
	minScanReportChars = 256
)

// scanReportBudget returns the character budget for the condensed scan report,
// or 0 for "unlimited" when no context window is configured.
func (b Budget) scanReportBudget(promptPrefix, dockerfile string) int {
	if b.ContextWindow <= 0 {
		return 0
	}

	tokens := b.ContextWindow - b.MaxTokens - promptOverheadTokens
	budget := tokens*charsPerToken - len(promptPrefix) - len(dockerfile) - len(scanReportHeading)
	if budget < minScanReportChars {
		return minScanReportChars
	}

	return budget
}

// OptimizeDockerfile sends the recovered Dockerfile, together with a condensed
// form of the artifact's vulnerability scan report, to an LLM gateway and returns
// the optimized result. promptPrefix is the instruction text prepended to the
// Dockerfile content; callers default it to DefaultPrompt and let deployments
// override it (see pkg/adapter/config.go). scanReport is the raw vulnerability
// scan report JSON for the artifact; callers are expected to only call this
// once a report exists. It is
// condensed to a per-package summary and fitted to budget before being added to
// the prompt - a full report for a real base image is larger than any model's
// context window (see pkg/scanreport).
func OptimizeDockerfile(ctx context.Context, apiBaseURL, apiKey, model, promptPrefix, dockerfile, scanReport string, budget Budget) (string, error) {
	prompt := BuildPrompt(promptPrefix, dockerfile, scanReport, budget)
	return Chat(ctx, apiBaseURL, apiKey, model, []Message{{Role: "user", Content: prompt}}, budget.MaxTokens)
}

// BuildPrompt is the optimization prompt: the instructions, the Dockerfile,
// and the scan report condensed and fitted to the budget.
func BuildPrompt(promptPrefix, dockerfile, scanReport string, budget Budget) string {
	condensed := scanreport.Condense(scanReport, budget.scanReportBudget(promptPrefix, dockerfile))
	return promptPrefix + dockerfile + scanReportHeading + condensed
}

// Chat sends a conversation to the gateway and returns the reply, streamed
// and joined. A follow-up turn (a repair round) sends the whole conversation
// so far, ending with the new user message. maxTokens of 0 omits the field.
func Chat(ctx context.Context, apiBaseURL, apiKey, model string, messages []Message, maxTokens int) (string, error) {
	reqBody := chatRequest{
		Model:     model,
		Messages:  messages,
		Stream:    true,
		MaxTokens: maxTokens,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBaseURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("llm gateway returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var optimized strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var parsed chatResponse
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			continue
		}
		if len(parsed.Choices) == 0 {
			continue
		}

		choice := parsed.Choices[0]
		if choice.Delta.Content != "" {
			optimized.WriteString(choice.Delta.Content)
			continue
		}
		if choice.Message.Content != "" {
			optimized.WriteString(choice.Message.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	result := strings.TrimSpace(optimized.String())
	if result == "" {
		return "", fmt.Errorf("llm gateway returned no optimized content")
	}

	return result, nil
}
