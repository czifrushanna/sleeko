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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/czifrushanna/sleeko/pkg/dfparse"
	"github.com/czifrushanna/sleeko/pkg/score"
)

// Runner measures one pipeline configuration over benchmark items.
type Runner struct {
	Source    Source
	Builder   Builder
	Scanner   Scanner
	Smoker    Smoker
	Optimizer Optimizer
	Cache     *Cache

	// WorkDir holds the candidate Dockerfiles and image archives.
	WorkDir string
	// Pipeline names the configuration in records ("W0", "noop", ...).
	Pipeline string
	// Repetitions is the number of attempts per item.
	Repetitions int
	// Concurrency bounds how many items are processed at once.
	Concurrency int
	// BuildTimeout bounds each image build; 0 means no limit. An O′ build
	// that runs out is an infra error (the item is too slow for the
	// benchmark), a candidate's is a G4 failure.
	BuildTimeout time.Duration

	Weights       score.Weights
	SizeTolerance float64

	// Model and PromptHash are copied into every record.
	Model      string
	PromptHash string

	Out *RecordWriter
	// Done holds the records of earlier runs; attempts they settle are
	// skipped.
	Done []Record
	// Logf reports progress; nil discards it.
	Logf func(format string, args ...any)

	// now is time.Now, replaceable in tests.
	now func() time.Time
}

// Run measures every item, Concurrency at a time. It returns the first
// error that stops the run itself (writing a record); per-attempt failures
// are recorded, not returned.
func (r *Runner) Run(ctx context.Context, items []Item) error {
	if r.Repetitions < 1 {
		return errors.New("repetitions must be at least 1")
	}
	if r.now == nil {
		r.now = time.Now
	}

	dbVersion, err := r.Scanner.DBVersion(ctx)
	if err != nil {
		return fmt.Errorf("trivy database version: %w", err)
	}

	done := map[attemptKey]bool{}
	for i := range r.Done {
		if r.Done[i].final() {
			done[r.Done[i].key()] = true
		}
	}

	concurrency := max(r.Concurrency, 1)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var once sync.Once
	var runErr error

	for _, it := range items {
		var todo []int
		for rep := 1; rep <= r.Repetitions; rep++ {
			if !done[attemptKey{it.ID, r.Pipeline, rep}] {
				todo = append(todo, rep)
			}
		}
		if len(todo) == 0 {
			r.logf("%s: all %d repetitions already recorded, skipping", it.ID, r.Repetitions)
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(it Item, todo []int) {
			defer func() { <-sem; wg.Done() }()
			if err := r.runItem(ctx, it, todo, dbVersion); err != nil {
				once.Do(func() { runErr = err })
			}
		}(it, todo)
	}
	wg.Wait()
	return runErr
}

func (r *Runner) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
	}
}

// runItem measures O′ (or takes it from the cache) and then each pending
// repetition. Only a failure to write a record is returned.
func (r *Runner) runItem(ctx context.Context, it Item, reps []int, dbVersion string) error {
	started := r.now()
	newRecord := func(rep int) Record {
		return Record{
			Item: it.ID, Pipeline: r.Pipeline, Repetition: rep, DistroFamily: it.DistroFamily,
			Model: r.Model, PromptHash: r.PromptHash, TrivyDB: dbVersion, StartedAt: started,
		}
	}

	src, err := r.Source.Checkout(ctx, it.Repo, it.Commit)
	if err != nil {
		return r.infraErrors(newRecord, reps, fmt.Errorf("checkout: %w", err))
	}
	original, err := os.ReadFile(filepath.Join(src, it.Dockerfile))
	if err != nil {
		return r.infraErrors(newRecord, reps, fmt.Errorf("read original Dockerfile: %w", err))
	}
	oFile, err := dfparse.Parse(string(original))
	if err != nil {
		return r.infraErrors(newRecord, reps, fmt.Errorf("original Dockerfile does not parse: %w", err))
	}

	base, err := r.baseline(ctx, it, src, dbVersion)
	if err != nil {
		return r.infraErrors(newRecord, reps, fmt.Errorf("O′: %w", err))
	}
	if base.SmokeError != "" {
		// A smoke test O′ fails would fail every candidate's G5: the item
		// is broken, not the pipeline.
		return r.infraErrors(newRecord, reps, fmt.Errorf("O′ fails its own smoke test: %s", base.SmokeError))
	}
	report, err := base.Scan.HarborReport()
	if err != nil {
		return r.infraErrors(newRecord, reps, fmt.Errorf("O′ scan report: %w", err))
	}

	for _, rep := range reps {
		rec := newRecord(rep)
		rec.StartedAt = r.now()
		orig := base.Image
		rec.Original = &orig
		r.attempt(ctx, it, src, oFile, string(original), string(report), base, &rec)
		r.logf("%s #%d: %s success=%v %s", it.ID, rep, rec.Status, rec.Success, rec.Error)
		if err := r.Out.Write(rec); err != nil {
			return fmt.Errorf("write record: %w", err)
		}
	}
	return nil
}

func (r *Runner) infraErrors(newRecord func(int) Record, reps []int, cause error) error {
	for _, rep := range reps {
		rec := newRecord(rep)
		rec.Status = StatusInfraError
		rec.Error = cause.Error()
		if err := r.Out.Write(rec); err != nil {
			return fmt.Errorf("write record: %w", err)
		}
	}
	r.logf("%v", cause)
	return nil
}

// baseline returns O′, building, scanning and smoke-testing it unless the
// cache already has it.
func (r *Runner) baseline(ctx context.Context, it Item, src, dbVersion string) (*baseline, error) {
	key := baselineKey(it, dbVersion)
	var b baseline
	if ok, err := r.Cache.get("baseline", key, &b); err != nil {
		return nil, err
	} else if ok {
		r.logf("%s: O′ from cache (built %s)", it.ID, b.Image.BuiltAt.Format(time.RFC3339))
		return &b, nil
	}

	r.logf("%s: building O′", it.ID)
	img, err := r.build(ctx, r.spec(it, src, filepath.Join(src, it.Dockerfile), "o", key[:12]))
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	defer r.remove(img)

	scan, err := r.scan(ctx, img, dbVersion)
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	b = baseline{
		Image: ImageInfo{
			Digest: img.Digest, Size: img.Size, Severities: severityCounts(scan.Result),
			OS: scan.OS, User: img.Contract.User, BuiltAt: r.now(),
		},
		Scan:     scan.Result,
		Contract: img.Contract,
	}
	if err := r.Smoker.Smoke(ctx, img, it.Platform, it.Smoke); err != nil {
		b.SmokeError = err.Error()
	}
	if err := r.Cache.put("baseline", key, b); err != nil {
		return nil, err
	}
	return &b, nil
}

// scan scans an image, once per digest and database.
func (r *Runner) scan(ctx context.Context, img Image, dbVersion string) (ScanOutput, error) {
	key := scanKey(img.Digest, dbVersion)
	var out ScanOutput
	if ok, err := r.Cache.get("scan", key, &out); err != nil {
		return out, err
	} else if ok {
		return out, nil
	}
	out, err := r.Scanner.Scan(ctx, img)
	if err != nil {
		return out, err
	}
	return out, r.Cache.put("scan", key, out)
}

func (r *Runner) spec(it Item, src, dockerfile, kind, suffix string) BuildSpec {
	tag := fmt.Sprintf("sleeko-eval/%s:%s-%s", it.ID, kind, suffix)
	return BuildSpec{
		ContextDir: filepath.Join(src, it.Context),
		Dockerfile: dockerfile,
		BuildArgs:  it.BuildArgs,
		Platform:   it.Platform,
		Target:     it.Target,
		Tag:        tag,
		OutDir:     filepath.Join(r.WorkDir, "images", it.ID, kind+"-"+suffix),
	}
}

// build runs one build under BuildTimeout.
func (r *Runner) build(ctx context.Context, spec BuildSpec) (Image, error) {
	if r.BuildTimeout <= 0 {
		return r.Builder.Build(ctx, spec)
	}
	buildCtx, cancel := context.WithTimeout(ctx, r.BuildTimeout)
	defer cancel()
	img, err := r.Builder.Build(buildCtx, spec)
	if err != nil && buildCtx.Err() != nil && ctx.Err() == nil {
		return img, fmt.Errorf("build took longer than %s", r.BuildTimeout)
	}
	return img, err
}

func (r *Runner) remove(img Image) {
	// Cleanup must not be cut short by a cancelled run.
	if err := r.Builder.Remove(context.Background(), img); err != nil {
		r.logf("remove %s: %v", img.Tag, err)
	}
}

// attempt runs the pipeline once and measures the candidate, filling rec.
// Gates are evaluated in order; once one makes the rest impossible (no
// parse, invented sources, no build), the remaining ones are NotChecked.
func (r *Runner) attempt(ctx context.Context, it Item, src string, oFile *dfparse.File, original, report string, base *baseline, rec *Record) {
	start := r.now()
	out, err := r.Optimizer.Optimize(ctx, OptimizeInput{Dockerfile: original, ScanReport: report})
	rec.LatencyMS = r.now().Sub(start).Milliseconds()
	rec.LLMRounds = out.LLMRounds
	rec.Diagnostics = out.Diagnostics
	if err != nil {
		rec.Status = StatusOptimizerFailed
		rec.Error = err.Error()
		return
	}
	rec.CandidateDockerfile = out.Candidate
	rec.Status = StatusMeasured

	gates := map[score.GateID]score.Gate{}
	finish := func() {
		for _, id := range score.AllGates {
			g, ok := gates[id]
			if !ok {
				g = score.Gate{ID: id, Status: score.NotChecked}
			}
			rec.Gates = append(rec.Gates, g)
		}
		if rec.Components != nil {
			rec.Success = score.Success(rec.Gates, rec.Components.DeltaV, rec.Components.DeltaS, r.SizeTolerance)
		}
	}
	defer finish()

	g1, cFile := score.CheckParse(out.Candidate)
	gates[g1.ID] = g1
	if cFile == nil {
		return
	}
	g2 := score.CheckNoInvention(oFile, cFile)
	gates[g2.ID] = g2
	if g2.Status == score.Fail {
		return
	}
	g3 := score.CheckVersions(false, nil)
	gates[g3.ID] = g3

	dockerfile, err := r.writeCandidate(it, src, out.Candidate, rec.Repetition)
	if err != nil {
		rec.Status, rec.Error = StatusInfraError, fmt.Sprintf("write candidate: %v", err)
		return
	}
	img, buildErr := r.build(ctx, r.spec(it, src, dockerfile, "c-"+sanitizeTag(r.Pipeline), fmt.Sprint(rec.Repetition)))
	gates[score.G4Build] = score.CheckBuild(buildErr)
	if buildErr != nil {
		return
	}
	defer r.remove(img)

	scan, err := r.scan(ctx, img, rec.TrivyDB)
	if err != nil {
		// The candidate built; failing to scan it is the harness's problem.
		rec.Status, rec.Error = StatusInfraError, fmt.Sprintf("scan candidate: %v", err)
		return
	}
	rec.Candidate = &ImageInfo{
		Digest: img.Digest, Size: img.Size, Severities: severityCounts(scan.Result),
		OS: scan.OS, User: img.Contract.User, BuiltAt: r.now(),
	}

	gates[score.G5Behaviour] = score.CheckBehaviour(r.Smoker.Smoke(ctx, img, it.Platform, it.Smoke), base.Contract, img.Contract)
	gates[score.G6NoNewSevere] = score.CheckNoNewSevere(base.Scan, scan.Result)

	vO, vC := r.Weights.V(base.Scan), r.Weights.V(scan.Result)
	dS, err := score.DeltaS(base.Image.Size, img.Size)
	if err != nil {
		rec.Status, rec.Error = StatusInfraError, err.Error()
		return
	}
	rec.Components = &Components{
		VOriginal: vO, VCandidate: vC,
		DeltaV:  score.DeltaV(vO, vC),
		DeltaS:  dS,
		FixRate: score.ComputeFixRate(base.Scan, scan.Result),
	}
}

// writeCandidate writes C outside the build context, so the file cannot end
// up in the image and make C differ from O′ by more than the Dockerfile. A
// Dockerfile-specific ignore file (<Dockerfile>.dockerignore) is copied
// alongside, since BuildKit looks for it next to the Dockerfile.
func (r *Runner) writeCandidate(it Item, src, content string, rep int) (string, error) {
	dir := filepath.Join(r.WorkDir, "candidates", it.ID, sanitizeTag(r.Pipeline), fmt.Sprint(rep))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Base(it.Dockerfile)
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return "", err
	}

	ignore, err := os.ReadFile(filepath.Join(src, it.Dockerfile) + ".dockerignore")
	switch {
	case err == nil:
		if err := os.WriteFile(file+".dockerignore", ignore, 0o644); err != nil {
			return "", err
		}
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}
	return file, nil
}

// sanitizeTag makes a pipeline name usable in an image tag and a path.
func sanitizeTag(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}
