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

// sleeko-eval measures sleeko's optimization pipelines over a benchmark.
//
//	sleeko-eval db     [--work DIR] [--trivy-image IMAGE]
//	sleeko-eval run    --manifest FILE --pipeline STAGES|noop [--name NAME] [--reps N] [--out FILE] ...
//	sleeko-eval report [--out FILE] RESULTS.jsonl...
//	sleeko-eval failures [--gate G] [--out DIR] RESULTS.jsonl...
//	sleeko-eval compare --a W0 --b W0+W1 [--out FILE] RESULTS.jsonl...
//
// `db` downloads Trivy's vulnerability database once; every later scan uses
// that copy unchanged. `run` appends one JSONL record per attempt and skips
// attempts the output file already settles, so an interrupted run resumes.
// The LLM gateway settings are the adapter's environment variables
// (LLMGW_*, OPTIMIZER_PROMPT), so a pipeline runs exactly as it would there.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/czifrushanna/sleeko/pkg/adapter"
	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/eval"
	"github.com/czifrushanna/sleeko/pkg/lookup"
	"github.com/czifrushanna/sleeko/pkg/pipeline"
	"github.com/czifrushanna/sleeko/pkg/score"
	"github.com/czifrushanna/sleeko/pkg/verify"
)

const usage = `usage:
  sleeko-eval db     [flags]                 download the Trivy database
  sleeko-eval run    [flags]                 measure a pipeline over the benchmark
  sleeko-eval report [flags] RESULTS.jsonl...  write the markdown report
  sleeko-eval failures [flags] RESULTS.jsonl... list failed attempts, save them as fixtures
  sleeko-eval compare [flags] RESULTS.jsonl...  McNemar and Wilcoxon of pipeline B against A

Run 'sleeko-eval <command> -h' for the flags of a command.
`

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "db":
		err = runDB(ctx, os.Args[2:])
	case "run":
		err = runEval(ctx, os.Args[2:])
	case "report":
		err = runReport(os.Args[2:])
	case "failures":
		err = runFailures(os.Args[2:])
	case "compare":
		err = runCompare(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("sleeko-eval: %v", err)
	}
}

// commonFlags are shared by db and run.
type commonFlags struct {
	work       string
	trivyImage string
}

func (c *commonFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&c.work, "work", defaultWorkDir(), "working directory: checkouts, caches, the Trivy database, image archives")
	fs.StringVar(&c.trivyImage, "trivy-image", eval.DefaultTrivyImage, "Trivy container image; empty uses the trivy binary on PATH")
}

// defaultWorkDir is outside the repository, so checkouts and image archives
// never end up in a build context or in git.
func defaultWorkDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "sleeko-eval")
	}
	return filepath.Join(os.TempDir(), "sleeko-eval")
}

func (c *commonFlags) scanner() (*eval.TrivyScanner, string, error) {
	work, err := filepath.Abs(c.work)
	if err != nil {
		return nil, "", err
	}
	cache := filepath.Join(work, "trivy-cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return nil, "", err
	}
	return &eval.TrivyScanner{CacheDir: cache, Image: c.trivyImage}, work, nil
}

func runDB(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("db", flag.ExitOnError)
	var common commonFlags
	common.register(fs)
	replace := fs.Bool("replace", false, "replace a database that is already there")
	fs.Parse(args)

	scanner, _, err := common.scanner()
	if err != nil {
		return err
	}
	// Every pipeline of a study must be scanned with the same database, so
	// an existing one is only replaced on purpose.
	if version, err := scanner.DBVersion(ctx); err == nil && !*replace {
		return fmt.Errorf("a database is already there (%s); comparisons need the same one, pass --replace to download a new one anyway", version)
	}
	if err := scanner.DownloadDB(ctx); err != nil {
		return err
	}
	version, err := scanner.DBVersion(ctx)
	if err != nil {
		return err
	}
	log.Printf("database ready: %s", version)
	return nil
}

func runEval(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var common commonFlags
	common.register(fs)
	manifestFile := fs.String("manifest", "", "benchmark manifest (YAML)")
	items := fs.String("items", "", "comma-separated item ids to run (default: all)")
	stages := fs.String("pipeline", "", `pipeline stages as in OPTIMIZER_PIPELINE (e.g. "generate"), or "noop" for C = O`)
	name := fs.String("name", "", "pipeline name in the records (default: the --pipeline value)")
	reps := fs.Int("reps", 3, "repetitions per item")
	out := fs.String("out", "results.jsonl", "JSONL file to append records to")
	concurrency := fs.Int("concurrency", 1, "items processed at once")
	buildTimeout := fs.Duration("build-timeout", 0, "limit for each image build, e.g. 10m (default: none)")
	maxOriginalAge := fs.Duration("max-original-age", 0, "rebuild O′ when its cached build is older than this, e.g. 12h (default: any age)")
	allowMixed := fs.Bool("allow-mixed-runs", false, "resume into --out even if code, pipeline, model, prompt, manifest or Trivy DB changed")
	hadolintImage := fs.String("hadolint-image", verify.DefaultHadolintImage, "Hadolint container image for the verify stage's lint check; empty uses hadolint on PATH, or skips lint without it")
	tolerance := fs.Float64("size-tolerance", score.DefaultSizeTolerance, "size tolerance of the success rule")
	fs.Parse(args)

	if *manifestFile == "" || *stages == "" {
		return errors.New("--manifest and --pipeline are required")
	}
	if *name == "" {
		*name = *stages
	}

	manifest, err := eval.LoadManifest(*manifestFile)
	if err != nil {
		return err
	}
	var ids []string
	if *items != "" {
		ids = strings.Split(*items, ",")
	}
	selected, err := manifest.Select(ids)
	if err != nil {
		return err
	}

	optimizer, llm, vcfg, err := buildOptimizer(*stages, *hadolintImage)
	if err != nil {
		return err
	}

	scanner, work, err := common.scanner()
	if err != nil {
		return err
	}
	dbVersion, err := scanner.DBVersion(ctx)
	if err != nil {
		return err
	}

	// Everything that produced the records goes next to them, once per
	// invocation; a resume that would mix experiments is refused.
	base := strings.TrimSuffix(*out, filepath.Ext(*out))
	manifestData, err := os.ReadFile(*manifestFile)
	if err != nil {
		return err
	}
	selectedIDs := make([]string, 0, len(selected))
	for _, it := range selected {
		selectedIDs = append(selectedIDs, it.ID)
	}
	info := eval.RunInfo{
		StartedAt:       time.Now().UTC(),
		Code:            codeVersion(),
		Pipeline:        *name,
		Stages:          *stages,
		Model:           llm.Model,
		APIBaseURL:      llm.APIBaseURL,
		MaxTokens:       llm.Budget.MaxTokens,
		ContextWindow:   llm.Budget.ContextWindow,
		Prompt:          llm.Prompt,
		MaxRepairRounds: vcfg.maxRepairRounds,
		Lint:            vcfg.lint,
		Manifest:        *manifestFile,
		ManifestSHA256:  fmt.Sprintf("%x", sha256.Sum256(manifestData)),
		Items:           selectedIDs,
		Repetitions:     *reps,
		Concurrency:     *concurrency,
		BuildTimeout:    buildTimeout.String(),
		MaxOriginalAge:  maxOriginalAge.String(),
		SizeTolerance:   *tolerance,
		Weights:         score.DefaultWeights,
		TrivyImage:      common.trivyImage,
		TrivyDB:         dbVersion,
		Docker:          dockerVersion(ctx),
	}
	if llm.Prompt != "" {
		info.PromptHash = eval.PromptHash(llm.Prompt)
	}
	if err := eval.AppendRunInfo(base+".runs.jsonl", info, *allowMixed); err != nil {
		return err
	}

	done, err := eval.ReadRecords(*out)
	if err != nil {
		return fmt.Errorf("read %s: %w", *out, err)
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	r := &eval.Runner{
		Source:         &eval.GitSource{Dir: filepath.Join(work, "src")},
		Builder:        &eval.DockerBuilder{},
		Scanner:        scanner,
		Smoker:         &eval.DockerSmoker{},
		Optimizer:      optimizer,
		Cache:          &eval.Cache{Dir: filepath.Join(work, "cache")},
		WorkDir:        work,
		Pipeline:       *name,
		Repetitions:    *reps,
		Concurrency:    *concurrency,
		BuildTimeout:   *buildTimeout,
		MaxOriginalAge: *maxOriginalAge,
		Weights:        score.DefaultWeights,
		SizeTolerance:  *tolerance,
		Model:          llm.Model,
		PromptHash:     info.PromptHash,
		ScanDir:        base + "-scans",
		Out:            eval.NewRecordWriter(f),
		Done:           done,
		Logf:           log.Printf,
	}
	return r.Run(ctx, selected)
}

// verifyConfig is what the run records about the verify stage.
type verifyConfig struct {
	maxRepairRounds int
	lint            string
}

// buildOptimizer returns the optimizer for --pipeline and the LLM and verify
// settings it uses (empty for noop). A real pipeline reads the adapter's
// configuration, so it runs exactly as it would there; only the lint check
// differs, running Hadolint from its container image, which the adapter's
// image cannot (D2).
func buildOptimizer(stages, hadolintImage string) (eval.Optimizer, pipeline.LLM, verifyConfig, error) {
	if stages == "noop" {
		return eval.NoopOptimizer{}, pipeline.LLM{}, verifyConfig{}, nil
	}

	names, err := pipeline.ParseNames(stages)
	if err != nil {
		return nil, pipeline.LLM{}, verifyConfig{}, fmt.Errorf("--pipeline: %w", err)
	}
	cfg, err := adapter.LoadConfig()
	if err != nil {
		return nil, pipeline.LLM{}, verifyConfig{}, err
	}
	llm := pipeline.LLM{
		APIBaseURL: cfg.APIBaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		Prompt:     cfg.Prompt,
		Budget:     dockerfile.Budget{MaxTokens: cfg.MaxTokens, ContextWindow: cfg.ContextWindow},
	}

	verifier := &verify.Verifier{Lookups: &lookup.Client{}}
	var vcfg verifyConfig
	for _, n := range names {
		if n != pipeline.VerifyStageName {
			continue
		}
		vcfg.maxRepairRounds = cfg.MaxRepairRounds
		switch h := verify.FindHadolint(); {
		case hadolintImage != "":
			verifier.Lint = &verify.Hadolint{Command: []string{"docker", "run", "--rm", "-i", hadolintImage, "hadolint"}}
			vcfg.lint = hadolintImage
		case h != nil:
			verifier.Lint = h
			vcfg.lint = h.Command[0]
		}
	}

	p, err := pipeline.New(names, pipeline.Deps{LLM: llm, Verifier: verifier, MaxRepairRounds: cfg.MaxRepairRounds})
	if err != nil {
		return nil, pipeline.LLM{}, verifyConfig{}, err
	}
	return &eval.PipelineOptimizer{Pipeline: p}, llm, vcfg, nil
}

// codeVersion is the commit this binary was built from, as `go build`
// stamps it, with "-dirty" for uncommitted changes.
func codeVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	var rev, modified string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if rev == "" {
		return "unknown (not built from a git checkout with go build)"
	}
	if modified == "true" {
		rev += "-dirty"
	}
	return rev
}

// dockerVersion is the Docker server and buildx version, for the record.
func dockerVersion(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").Output()
	if err != nil {
		return ""
	}
	v := "server " + strings.TrimSpace(string(out))
	if bx, err := exec.CommandContext(ctx, "docker", "buildx", "version").Output(); err == nil {
		v += ", " + strings.TrimSpace(string(bx))
	}
	return v
}

// readAll reads and concatenates records from results files.
func readAll(files []string) ([]eval.Record, error) {
	var records []eval.Record
	for _, file := range files {
		recs, err := eval.ReadRecords(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if recs == nil {
			return nil, fmt.Errorf("%s: no records", file)
		}
		records = append(records, recs...)
	}
	return records, nil
}

func runCompare(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)
	a := fs.String("a", "", "baseline pipeline name, e.g. W0")
	b := fs.String("b", "", "pipeline compared against it, e.g. W0+W1")
	out := fs.String("out", "", "write the markdown here instead of stdout")
	fs.Parse(args)
	if *a == "" || *b == "" || fs.NArg() == 0 {
		return errors.New("usage: sleeko-eval compare --a W0 --b W0+W1 RESULTS.jsonl...")
	}
	records, err := readAll(fs.Args())
	if err != nil {
		return err
	}
	var w io.Writer = os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	return eval.WriteComparison(w, eval.Compare(records, *a, *b))
}

func runFailures(args []string) error {
	fs := flag.NewFlagSet("failures", flag.ExitOnError)
	gate := fs.String("gate", "", "only attempts failing this gate, e.g. G2_no_invention (default: any)")
	out := fs.String("out", "", "also save each failure as a fixture directory here")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New("give at least one results file")
	}
	records, err := readAll(fs.Args())
	if err != nil {
		return err
	}
	failures := eval.Failures(records, score.GateID(*gate))
	if err := eval.WriteFailures(os.Stdout, failures); err != nil {
		return err
	}
	if *out != "" {
		if err := eval.SaveFailures(*out, failures); err != nil {
			return err
		}
		log.Printf("saved %d failures under %s", len(failures), *out)
	}
	return nil
}

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	out := fs.String("out", "", "write the markdown here instead of stdout")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New("give at least one results file")
	}

	records, err := readAll(fs.Args())
	if err != nil {
		return err
	}

	var w io.Writer = os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	return eval.WriteMarkdown(w, eval.BuildReport(records))
}
