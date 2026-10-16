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
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/czifrushanna/sleeko/pkg/pipeline"
	"github.com/czifrushanna/sleeko/pkg/score"
)

// Record statuses.
const (
	// StatusMeasured: the candidate went through the gates and was scored.
	StatusMeasured = "measured"
	// StatusOptimizerFailed: the pipeline under evaluation returned no
	// candidate. It counts as an unsuccessful attempt.
	StatusOptimizerFailed = "optimizer_failed"
	// StatusInfraError: the harness could not measure the attempt: a
	// checkout, a scan, or a build the machine broke (BuildKit killed by the
	// OOM killer, a registry rate limit, a full disk). It says nothing about
	// the pipeline or the item: the report leaves it out, and a resumed run
	// retries it.
	StatusInfraError = "infra_error"
	// StatusItemInvalid: the item itself cannot be measured today: its
	// original Dockerfile does not parse, O′ does not build (or takes longer
	// than the build timeout), or O′ fails its own smoke test. The report
	// leaves it out, and a resumed run does not retry it.
	StatusItemInvalid = "item_invalid"
)

// Record is one attempt: one pipeline run on one item, measured.
type Record struct {
	Item         string `json:"item"`
	Pipeline     string `json:"pipeline"`
	Repetition   int    `json:"repetition"`
	DistroFamily string `json:"distro_family"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`

	Gates      []score.Gate `json:"gates,omitempty"`
	Success    bool         `json:"success"`
	Components *Components  `json:"components,omitempty"`

	Original  *ImageInfo `json:"original,omitempty"`
	Candidate *ImageInfo `json:"candidate,omitempty"`
	// OriginalDockerfile and CandidateDockerfile are kept so failures can be
	// inspected, cited and turned into test fixtures.
	OriginalDockerfile  string `json:"original_dockerfile,omitempty"`
	CandidateDockerfile string `json:"candidate_dockerfile,omitempty"`

	Diagnostics []string `json:"diagnostics,omitempty"`
	// Verification is W1's verify rounds: round 0 checks the generated
	// candidate, round n the n-th repair. Empty for pipelines without verify.
	Verification []pipeline.VerifyRound `json:"verification,omitempty"`
	LLMRounds    int                    `json:"llm_rounds"`
	// LatencyMS is the pipeline's run time, in milliseconds.
	LatencyMS  int64  `json:"latency_ms"`
	Model      string `json:"model,omitempty"`
	PromptHash string `json:"prompt_hash,omitempty"`
	TrivyDB    string `json:"trivy_db,omitempty"`

	StartedAt time.Time `json:"started_at"`
}

// Components are the metric components. They are recorded whenever C was
// built and scanned, also for attempts a gate failed; the report only uses
// them for successful attempts, as the schedule defines them.
type Components struct {
	VOriginal  int           `json:"v_original"`
	VCandidate int           `json:"v_candidate"`
	DeltaV     float64       `json:"delta_v"`
	DeltaS     float64       `json:"delta_s"`
	FixRate    score.FixRate `json:"fix_rate"`
}

// ImageInfo describes O′ or C.
type ImageInfo struct {
	Digest     string         `json:"digest"`
	Size       int64          `json:"size"`
	Severities map[string]int `json:"severities"`
	// OS is what the scanner detected; empty when it found no distribution
	// (an image built FROM scratch).
	OS string `json:"os,omitempty"`
	// User is the image config's USER, inherited ones included; empty means
	// root.
	User string `json:"user,omitempty"`
	// BuiltAt is when the image was built; O′ may come from the cache.
	BuiltAt time.Time `json:"built_at"`
}

func severityCounts(r score.ScanResult) map[string]int {
	out := map[string]int{}
	for sev, n := range r.Counts() {
		out[sev.String()] = n
	}
	return out
}

// attemptKey identifies an attempt across runs, for resuming.
type attemptKey struct {
	item, pipeline string
	repetition     int
}

func (r *Record) key() attemptKey {
	return attemptKey{r.Item, r.Pipeline, r.Repetition}
}

// final reports whether the record settles its attempt, so a resumed run
// skips it. Infrastructure errors do not: the attempt is retried. An invalid
// item does: rebuilding a broken O′ would only fail again.
func (r *Record) final() bool {
	return r.Status == StatusMeasured || r.Status == StatusOptimizerFailed || r.Status == StatusItemInvalid
}

// ReadRecords reads a JSONL file of records. A missing file reads as empty,
// so a first run and a resumed one start the same way.
func ReadRecords(file string) ([]Record, error) {
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return DecodeRecords(f)
}

// DecodeRecords reads records, one JSON object per line.
func DecodeRecords(r io.Reader) ([]Record, error) {
	var out []Record
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for line := 1; sc.Scan(); line++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}

// RecordWriter appends records to a JSONL file, one complete line per
// record, safe for concurrent use. Each record is flushed as it is written,
// so an interrupted run loses at most the attempt in progress.
type RecordWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewRecordWriter writes to w.
func NewRecordWriter(w io.Writer) *RecordWriter {
	return &RecordWriter{w: w}
}

// Write appends one record.
func (w *RecordWriter) Write(rec Record) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.w.Write(append(line, '\n'))
	return err
}
