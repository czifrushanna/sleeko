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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/dfparse"
	"github.com/czifrushanna/sleeko/pkg/score"
)

// world is the fake environment. A Dockerfile drives what its image looks
// like through comment markers:
//
//	# size=1200                       compressed size (default 1000)
//	# vulns=CVE-1:critical:1.2,CVE-2:high:   findings (id:severity:fixed)
//	# FAILBUILD / # FAILSMOKE         the build / the smoke test fails
type world struct {
	t   *testing.T
	src string // the checkout

	mu         sync.Mutex
	content    map[string]string // digest -> Dockerfile
	builds     []string          // Dockerfiles built, in order
	scans      map[string]int    // digest -> scan calls
	smokes     int
	removed    []string
	checkouts  int
	checkoutFn func(repo string) error
	dbVersion  string
}

func newWorld(t *testing.T, original string) *world {
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "app", "Dockerfile"), []byte(original), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "app", "requirements.txt"), []byte("flask\n"), 0o644))
	return &world{t: t, src: src, content: map[string]string{}, scans: map[string]int{}, dbVersion: "db-1"}
}

func marker(dockerfile, name string) (string, bool) {
	for _, line := range strings.Split(dockerfile, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "# "+name); ok {
			return strings.TrimPrefix(v, "="), true
		}
	}
	return "", false
}

func (w *world) Checkout(_ context.Context, repo, _ string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.checkouts++
	if w.checkoutFn != nil {
		if err := w.checkoutFn(repo); err != nil {
			return "", err
		}
	}
	return w.src, nil
}

func (w *world) Build(ctx context.Context, spec BuildSpec) (Image, error) {
	data, err := os.ReadFile(spec.Dockerfile)
	if err != nil {
		return Image{}, err
	}
	df := string(data)

	w.mu.Lock()
	w.builds = append(w.builds, df)
	w.mu.Unlock()

	if _, slow := marker(df, "SLOWBUILD"); slow {
		<-ctx.Done()
		return Image{}, ctx.Err()
	}
	if _, oom := marker(df, "OOMBUILD"); oom {
		return Image{}, errors.New("failed to receive status: rpc error: code = Unavailable desc = error reading from server: EOF")
	}
	if _, fail := marker(df, "FAILBUILD"); fail {
		return Image{}, errors.New("RUN failed: exit code 1")
	}
	size := int64(1000)
	if v, ok := marker(df, "size"); ok {
		size, _ = strconv.ParseInt(v, 10, 64)
	}
	f, err := dfparse.Parse(df)
	require.NoError(w.t, err)
	rc, err := f.RuntimeContract()
	require.NoError(w.t, err)

	digest := "sha256:" + hashKey(df)
	w.mu.Lock()
	w.content[digest] = df
	w.mu.Unlock()
	return Image{Tag: spec.Tag, Digest: digest, Size: size, Contract: score.ContractFromDockerfile(rc)}, nil
}

func (w *world) Remove(_ context.Context, img Image) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.removed = append(w.removed, img.Tag)
	return nil
}

func (w *world) Scan(_ context.Context, img Image) (ScanOutput, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.scans[img.Digest]++

	var vulns []score.Vulnerability
	if v, ok := marker(w.content[img.Digest], "vulns"); ok && v != "" {
		for _, f := range strings.Split(v, ",") {
			parts := strings.Split(f, ":")
			vulns = append(vulns, score.Vulnerability{
				ID: parts[0], Package: "pkg-" + parts[0], Severity: score.ParseSeverity(parts[1]), FixedVersion: parts[2],
			})
		}
	}
	return ScanOutput{Result: score.NewScanResult(vulns), OS: "debian 12.8"}, nil
}

func (w *world) DBVersion(context.Context) (string, error) { return w.dbVersion, nil }

func (w *world) Smoke(_ context.Context, img Image, _ string, _ Smoke) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.smokes++
	if strings.Contains(w.content[img.Digest], "# FAILSMOKE") {
		return errors.New("exit code 1, want 0")
	}
	return nil
}

func (w *world) buildCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.builds)
}

func (w *world) scanCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, c := range w.scans {
		n += c
	}
	return n
}

// fakeOptimizer returns a fixed candidate, or an error.
type fakeOptimizer struct {
	candidate string
	err       error
	calls     int
	mu        sync.Mutex
}

func (f *fakeOptimizer) Optimize(_ context.Context, in OptimizeInput) (OptimizeOutput, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if !strings.Contains(in.ScanReport, `"vulnerabilities"`) {
		return OptimizeOutput{}, errors.New("optimizer did not get a Harbor-shape report")
	}
	return OptimizeOutput{Candidate: f.candidate, LLMRounds: 1, Diagnostics: []string{"generate: note"}}, f.err
}

const originalDockerfile = `FROM python:3.11-slim
# vulns=CVE-1:critical:1.2,CVE-2:high:,CVE-3:low:
# size=1000
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
EXPOSE 8000
CMD ["python", "-m", "app"]
`

// candidate is the original with its markers replaced.
func candidate(markers string) string {
	lines := []string{"FROM python:3.12-slim", markers}
	for _, l := range strings.Split(originalDockerfile, "\n")[1:] {
		if !strings.HasPrefix(l, "# ") {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

var testItem = Item{
	ID: "flask-app", Source: "demo", Repo: "https://example.com/app.git", Commit: strings.Repeat("a", 40),
	Context: "app", Dockerfile: "app/Dockerfile", Platform: DefaultPlatform, DistroFamily: "debian",
	Smoke: Smoke{Command: []string{"python", "-c", "import app"}, Timeout: time.Minute},
}

type harness struct {
	w      *world
	opt    Optimizer
	cache  string
	work   string
	out    bytes.Buffer
	done   []Record
	reps   int
	runner *Runner
}

func newHarness(t *testing.T, opt Optimizer) *harness {
	return &harness{w: newWorld(t, originalDockerfile), opt: opt, cache: t.TempDir(), work: t.TempDir(), reps: 1}
}

func (h *harness) run(t *testing.T, items ...Item) []Record {
	t.Helper()
	if len(items) == 0 {
		items = []Item{testItem}
	}
	h.out.Reset()
	h.runner = &Runner{
		Source: h.w, Builder: h.w, Scanner: h.w, Smoker: h.w, Optimizer: h.opt,
		Cache:   &Cache{Dir: h.cache},
		WorkDir: h.work, Pipeline: "W0", Repetitions: h.reps, Concurrency: 2, BuildTimeout: 50 * time.Millisecond,
		Weights: score.DefaultWeights, SizeTolerance: score.DefaultSizeTolerance,
		Model: "test-model", PromptHash: "abc123",
		Out: NewRecordWriter(&h.out), Done: h.done,
	}
	require.NoError(t, h.runner.Run(context.Background(), items))
	recs, err := DecodeRecords(&h.out)
	require.NoError(t, err)
	return recs
}

func gateStatuses(rec Record) map[score.GateID]score.Status {
	out := map[score.GateID]score.Status{}
	for _, g := range rec.Gates {
		out[g.ID] = g.Status
	}
	return out
}

func allGates(g1, g2, g3, g4, g5, g6 score.Status) map[score.GateID]score.Status {
	return map[score.GateID]score.Status{
		score.G1Parse: g1, score.G2NoInvention: g2, score.G3Versions: g3,
		score.G4Build: g4, score.G5Behaviour: g5, score.G6NoNewSevere: g6,
	}
}

const pass, fail, notChecked = score.Pass, score.Fail, score.NotChecked

func TestRun_SuccessfulAttempt(t *testing.T) {
	h := newHarness(t, &fakeOptimizer{candidate: candidate("# vulns=CVE-2:high:,CVE-3:low:\n# size=900")})
	recs := h.run(t)
	require.Len(t, recs, 1)
	rec := recs[0]

	require.Equal(t, StatusMeasured, rec.Status, rec.Error)
	require.Equal(t, allGates(pass, pass, notChecked, pass, pass, pass), gateStatuses(rec))
	require.True(t, rec.Success)

	// V_O′ = 10 + 5 + 1 = 16, V_C = 5 + 1 = 6.
	require.Equal(t, &Components{
		VOriginal: 16, VCandidate: 6, DeltaV: 10.0 / 16, DeltaS: 0.1,
		FixRate: score.FixRate{Fixable: 1, Fixed: 1},
	}, rec.Components)

	require.Equal(t, "flask-app", rec.Item)
	require.Equal(t, "W0", rec.Pipeline)
	require.Equal(t, 1, rec.Repetition)
	require.Equal(t, "debian", rec.DistroFamily)
	require.Equal(t, int64(1000), rec.Original.Size)
	require.Equal(t, map[string]int{"Critical": 1, "High": 1, "Low": 1}, rec.Original.Severities)
	require.Equal(t, int64(900), rec.Candidate.Size)
	require.Equal(t, "debian 12.8", rec.Candidate.OS)
	require.Contains(t, rec.CandidateDockerfile, "FROM python:3.12-slim")
	require.Equal(t, 1, rec.LLMRounds)
	require.Equal(t, []string{"generate: note"}, rec.Diagnostics)
	require.Equal(t, "test-model", rec.Model)
	require.Equal(t, "abc123", rec.PromptHash)
	require.Equal(t, "db-1", rec.TrivyDB)

	// O′ and C are built, scanned and smoke-tested, and both cleaned up.
	require.Equal(t, 2, h.w.buildCount())
	require.Equal(t, 2, h.w.smokes)
	require.Len(t, h.w.removed, 2)
}

func TestRun_CandidateIsBuiltOutsideTheContext(t *testing.T) {
	h := newHarness(t, &fakeOptimizer{candidate: candidate("# vulns=")})
	require.NoError(t, os.WriteFile(filepath.Join(h.w.src, "app", "Dockerfile.dockerignore"), []byte("*.md\n"), 0o644))
	h.run(t)

	dir := filepath.Join(h.work, "candidates", "flask-app", "w0", "1")
	written, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	require.Contains(t, string(written), "FROM python:3.12-slim")
	ignore, err := os.ReadFile(filepath.Join(dir, "Dockerfile.dockerignore"))
	require.NoError(t, err)
	require.Equal(t, "*.md\n", string(ignore))

	entries, err := os.ReadDir(filepath.Join(h.w.src, "app"))
	require.NoError(t, err)
	require.Len(t, entries, 3, "nothing added to the build context")
}

func TestRun_GateFailures(t *testing.T) {
	cases := []struct {
		name       string
		candidate  string
		gates      map[score.GateID]score.Status
		components bool
		builds     int // O′ included
	}{
		{
			name:      "C does not parse: nothing else is checked",
			candidate: "FROM python:3.12\nCOPYY . .\n",
			gates:     allGates(fail, notChecked, notChecked, notChecked, notChecked, notChecked),
			builds:    1,
		},
		{
			name:      "C invents a source: not built",
			candidate: candidate("# vulns=") + "\nCOPY pyproject.toml .\n",
			gates:     allGates(pass, fail, notChecked, notChecked, notChecked, notChecked),
			builds:    1,
		},
		{
			name:      "C does not build",
			candidate: candidate("# FAILBUILD"),
			gates:     allGates(pass, pass, notChecked, fail, notChecked, notChecked),
			builds:    2,
		},
		{
			name:       "C fails its smoke test: still scored",
			candidate:  candidate("# vulns=\n# FAILSMOKE"),
			gates:      allGates(pass, pass, notChecked, pass, fail, pass),
			components: true,
			builds:     2,
		},
		{
			name:       "C changes the runtime contract",
			candidate:  strings.Replace(candidate("# vulns="), "EXPOSE 8000", "EXPOSE 9000", 1),
			gates:      allGates(pass, pass, notChecked, pass, fail, pass),
			components: true,
			builds:     2,
		},
		{
			name:       "C brings a new high CVE",
			candidate:  candidate("# vulns=CVE-9:high:"),
			gates:      allGates(pass, pass, notChecked, pass, pass, fail),
			components: true,
			builds:     2,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeOptimizer{candidate: c.candidate})
			recs := h.run(t)
			require.Len(t, recs, 1)
			rec := recs[0]

			require.Equal(t, StatusMeasured, rec.Status, rec.Error)
			require.Equal(t, c.gates, gateStatuses(rec))
			require.False(t, rec.Success)
			require.Equal(t, c.components, rec.Components != nil)
			require.Equal(t, c.builds, h.w.buildCount())
			require.NotEmpty(t, rec.CandidateDockerfile)
		})
	}
}

func TestRun_OptimizerFailure(t *testing.T) {
	h := newHarness(t, &fakeOptimizer{err: errors.New("llm gateway returned 500")})
	recs := h.run(t)
	require.Len(t, recs, 1)
	require.Equal(t, StatusOptimizerFailed, recs[0].Status)
	require.Equal(t, "llm gateway returned 500", recs[0].Error)
	require.False(t, recs[0].Success)
	require.Empty(t, recs[0].Gates)
	require.Equal(t, 1, h.w.buildCount(), "only O′")
}

func TestRun_BaselineProblems(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(h *harness)
		wantStatus string
		wantErr    string
	}{
		{"checkout fails: the harness's problem", func(h *harness) {
			h.w.checkoutFn = func(string) error { return errors.New("repository not found") }
		}, StatusInfraError, "checkout: repository not found"},
		{"O′ does not build: the item is broken", func(h *harness) {
			writeOriginal(h, originalDockerfile+"# FAILBUILD\n")
		}, StatusItemInvalid, "O′: build: RUN failed"},
		{"O′ build killed by the machine: retried later", func(h *harness) {
			writeOriginal(h, originalDockerfile+"# OOMBUILD\n")
		}, StatusInfraError, "O′: build: failed to receive status: rpc error: code = Unavailable"},
		{"O′ fails its own smoke test: the item is broken", func(h *harness) {
			writeOriginal(h, originalDockerfile+"# FAILSMOKE\n")
		}, StatusItemInvalid, "O′ fails its own smoke test"},
		{"the original does not parse: the item is broken", func(h *harness) {
			writeOriginal(h, "FROM\n")
		}, StatusItemInvalid, "original Dockerfile does not parse"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opt := &fakeOptimizer{candidate: candidate("# vulns=")}
			h := newHarness(t, opt)
			h.reps = 2
			c.setup(h)

			recs := h.run(t)
			require.Len(t, recs, 2, "one record per pending repetition")
			for _, rec := range recs {
				require.Equal(t, c.wantStatus, rec.Status)
				require.Contains(t, rec.Error, c.wantErr)
			}
			require.Zero(t, opt.calls, "the pipeline is not run without a valid O′")
		})
	}
}

// A candidate build the machine broke (BuildKit OOM-killed) is not the
// pipeline's G4 failure: it is retried.
func TestRun_CandidateBuildKilledByTheMachine(t *testing.T) {
	h := newHarness(t, &fakeOptimizer{candidate: candidate("# OOMBUILD")})
	recs := h.run(t)
	require.Len(t, recs, 1)
	require.Equal(t, StatusInfraError, recs[0].Status)
	require.Contains(t, recs[0].Error, "build candidate:")
	require.Empty(t, recs[0].Gates)
	require.False(t, recs[0].final(), "retried by a resumed run")
}

func TestRun_ResumeSkipsInvalidItems(t *testing.T) {
	opt := &fakeOptimizer{candidate: candidate("# vulns=")}
	h := newHarness(t, opt)
	h.done = []Record{{Item: "flask-app", Pipeline: "W0", Repetition: 1, Status: StatusItemInvalid}}
	checkouts := h.w.checkouts
	require.Empty(t, h.run(t))
	require.Equal(t, checkouts, h.w.checkouts, "an invalid item is not even checked out again")
}

func TestMachineFailure(t *testing.T) {
	require.True(t, machineFailure(errors.New("ERROR: failed to build: failed to receive status: rpc error: code = Unavailable desc = error reading from server: EOF")))
	require.True(t, machineFailure(errors.New("toomanyrequests: You have reached your pull rate limit")))
	require.False(t, machineFailure(errors.New(`process "/bin/sh -c curl -fsSL https://gone.example/x" did not complete successfully: exit code: 6`)))
	require.False(t, machineFailure(errors.New("build took longer than 10m0s")))
	require.False(t, machineFailure(nil))
}

func writeOriginal(h *harness, content string) {
	require.NoError(h.w.t, os.WriteFile(filepath.Join(h.w.src, "app", "Dockerfile"), []byte(content), 0o644))
}

func TestRun_CachesBaselineAndScans(t *testing.T) {
	h := newHarness(t, NoopOptimizer{})
	h.reps = 3
	recs := h.run(t)
	require.Len(t, recs, 3)

	// O′ is built once; each no-op candidate is built, but has O′'s digest,
	// so it is never scanned again.
	require.Equal(t, 4, h.w.buildCount())
	require.Equal(t, 1, h.w.scanCount())

	// A second run of another configuration reuses O′ from the cache.
	h.runner.Pipeline = "W0+W1"
	h.out.Reset()
	require.NoError(t, h.runner.Run(context.Background(), []Item{testItem}))
	require.Equal(t, 4+3, h.w.buildCount(), "no O′ rebuild")
	require.Equal(t, 1, h.w.scanCount())

	// A new Trivy database invalidates both caches.
	h.w.dbVersion = "db-2"
	h.runner.Pipeline = "W0+W1+W2"
	require.NoError(t, h.runner.Run(context.Background(), []Item{testItem}))
	require.Equal(t, 4+3+4, h.w.buildCount())
	require.Equal(t, 2, h.w.scanCount())
}

// TestRun_NoopMeasuresNoDifference is the unit-level version of the
// no-op sanity check: with C = O every gate passes, ΔV = 0 and ΔS = 0, and
// with no improvement the attempt is not a success.
func TestRun_NoopMeasuresNoDifference(t *testing.T) {
	h := newHarness(t, NoopOptimizer{})
	recs := h.run(t)
	require.Len(t, recs, 1)
	rec := recs[0]

	require.Equal(t, allGates(pass, pass, notChecked, pass, pass, pass), gateStatuses(rec))
	require.Zero(t, rec.Components.DeltaV)
	require.Zero(t, rec.Components.DeltaS)
	require.Equal(t, score.FixRate{Fixable: 1, Fixed: 0}, rec.Components.FixRate)
	require.False(t, rec.Success)
	require.Equal(t, rec.Original.Digest, rec.Candidate.Digest)
}

func TestRun_Resume(t *testing.T) {
	opt := &fakeOptimizer{candidate: candidate("# vulns=")}
	h := newHarness(t, opt)
	h.reps = 3
	h.done = []Record{
		{Item: "flask-app", Pipeline: "W0", Repetition: 1, Status: StatusMeasured},
		{Item: "flask-app", Pipeline: "W0", Repetition: 2, Status: StatusInfraError},
		{Item: "flask-app", Pipeline: "other", Repetition: 3, Status: StatusMeasured},
	}

	recs := h.run(t)
	var reps []int
	for _, r := range recs {
		reps = append(reps, r.Repetition)
	}
	require.Equal(t, []int{2, 3}, reps, "measured attempts are skipped, infra errors retried, other pipelines ignored")
	require.Equal(t, 2, opt.calls)

	// Everything settled: nothing runs, not even the checkout.
	h.done = append(h.done, recs...)
	checkouts := h.w.checkouts
	require.Empty(t, h.run(t))
	require.Equal(t, checkouts, h.w.checkouts)
}

func TestRun_ManyItemsConcurrently(t *testing.T) {
	h := newHarness(t, NoopOptimizer{})
	h.reps = 2
	var items []Item
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		it := testItem
		it.ID = id
		items = append(items, it)
	}
	recs := h.run(t, items...)
	require.Len(t, recs, 10)
	seen := map[attemptKey]bool{}
	for _, r := range recs {
		seen[r.key()] = true
	}
	require.Len(t, seen, 10)
}

func TestRun_RequiresRepetitions(t *testing.T) {
	r := &Runner{Repetitions: 0}
	require.Error(t, r.Run(context.Background(), nil))
}

func TestSanitizeTag(t *testing.T) {
	require.Equal(t, "w0-w1", sanitizeTag("W0+W1"))
	require.Equal(t, "generate-verify", sanitizeTag("generate,verify"))
}

func TestRun_BuildTimeout(t *testing.T) {
	h := newHarness(t, &fakeOptimizer{candidate: candidate("# SLOWBUILD")})
	h.reps = 1
	recs := h.run(t)
	require.Equal(t, StatusMeasured, recs[0].Status)
	require.Equal(t, fail, gateStatuses(recs[0])[score.G4Build])
	require.Contains(t, recs[0].Gates[3].Reason, "took longer than")

	// A slow O′ makes the item unusable rather than failing the pipeline.
	slow := newHarness(t, NoopOptimizer{})
	writeOriginal(slow, originalDockerfile+"# SLOWBUILD\n")
	recs = slow.run(t)
	require.Equal(t, StatusItemInvalid, recs[0].Status)
	require.Contains(t, recs[0].Error, "took longer than 50ms")
}

func TestRun_MaxOriginalAge(t *testing.T) {
	h := newHarness(t, NoopOptimizer{})
	clock := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	run := func() {
		t.Helper()
		h.done = nil
		h.out.Reset()
		h.runner = &Runner{
			Source: h.w, Builder: h.w, Scanner: h.w, Smoker: h.w, Optimizer: h.opt,
			Cache: &Cache{Dir: h.cache}, WorkDir: h.work, Pipeline: "W0", Repetitions: 1,
			Weights: score.DefaultWeights, SizeTolerance: score.DefaultSizeTolerance,
			MaxOriginalAge: 12 * time.Hour, Out: NewRecordWriter(&h.out),
			now: func() time.Time { return clock },
		}
		require.NoError(t, h.runner.Run(context.Background(), []Item{testItem}))
	}

	run()
	require.Equal(t, 2, h.w.buildCount(), "O′ and C")

	clock = clock.Add(3 * time.Hour) // a resume, or the next pipeline of the session
	run()
	require.Equal(t, 3, h.w.buildCount(), "O′ reused")

	clock = clock.Add(2 * 24 * time.Hour) // a session days later
	run()
	require.Equal(t, 5, h.w.buildCount(), "O′ rebuilt")
}

func TestRun_SavesTheScansItRefersTo(t *testing.T) {
	h := newHarness(t, &fakeOptimizer{candidate: candidate("# vulns=CVE-2:high:")})
	h.reps = 2
	scans := filepath.Join(t.TempDir(), "w0-scans")

	run := func() []Record {
		h.out.Reset()
		h.runner = &Runner{
			Source: h.w, Builder: h.w, Scanner: h.w, Smoker: h.w, Optimizer: h.opt,
			Cache: &Cache{Dir: h.cache}, WorkDir: h.work, ScanDir: scans, Pipeline: "W0", Repetitions: h.reps,
			Weights: score.DefaultWeights, SizeTolerance: score.DefaultSizeTolerance, Out: NewRecordWriter(&h.out),
		}
		require.NoError(t, h.runner.Run(context.Background(), []Item{testItem}))
		recs, err := DecodeRecords(&h.out)
		require.NoError(t, err)
		return recs
	}
	recs := run()
	require.Len(t, recs, 2)

	for _, digest := range []string{recs[0].Original.Digest, recs[0].Candidate.Digest} {
		data, err := os.ReadFile(filepath.Join(scans, strings.ReplaceAll(digest, ":", "-")+".json"))
		require.NoError(t, err, digest)
		require.Contains(t, string(data), "CVE-")
	}
	entries, err := os.ReadDir(scans)
	require.NoError(t, err)
	require.Len(t, entries, 2, "one file per image, no temporary files left")

	// O′ from the cache in a later run is saved too.
	scans = filepath.Join(t.TempDir(), "w0+w1-scans")
	h.done = nil
	run()
	_, err = os.Stat(filepath.Join(scans, strings.ReplaceAll(recs[0].Original.Digest, ":", "-")+".json"))
	require.NoError(t, err)
}
