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
//
// `db` downloads Trivy's vulnerability database once; every later scan uses
// that copy unchanged. `run` appends one JSONL record per attempt and skips
// attempts the output file already settles, so an interrupted run resumes.
// The LLM gateway settings are the adapter's environment variables
// (LLMGW_*, OPTIMIZER_PROMPT), so a pipeline runs exactly as it would there.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/czifrushanna/sleeko/pkg/adapter"
	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/eval"
	"github.com/czifrushanna/sleeko/pkg/pipeline"
	"github.com/czifrushanna/sleeko/pkg/score"
)

const usage = `usage:
  sleeko-eval db     [flags]                 download the Trivy database
  sleeko-eval run    [flags]                 measure a pipeline over the benchmark
  sleeko-eval report [flags] RESULTS.jsonl...  write the markdown report

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
	fs.Parse(args)

	scanner, _, err := common.scanner()
	if err != nil {
		return err
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

	optimizer, model, promptHash, err := buildOptimizer(*stages)
	if err != nil {
		return err
	}

	scanner, work, err := common.scanner()
	if err != nil {
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
		Source:        &eval.GitSource{Dir: filepath.Join(work, "src")},
		Builder:       &eval.DockerBuilder{},
		Scanner:       scanner,
		Smoker:        &eval.DockerSmoker{},
		Optimizer:     optimizer,
		Cache:         &eval.Cache{Dir: filepath.Join(work, "cache")},
		WorkDir:       work,
		Pipeline:      *name,
		Repetitions:   *reps,
		Concurrency:   *concurrency,
		Weights:       score.DefaultWeights,
		SizeTolerance: *tolerance,
		Model:         model,
		PromptHash:    promptHash,
		Out:           eval.NewRecordWriter(f),
		Done:          done,
		Logf:          log.Printf,
	}
	return r.Run(ctx, selected)
}

// buildOptimizer returns the optimizer for --pipeline, with the model and
// prompt hash to record. A real pipeline reads the adapter's configuration.
func buildOptimizer(stages string) (eval.Optimizer, string, string, error) {
	if stages == "noop" {
		return eval.NoopOptimizer{}, "", "", nil
	}

	names, err := pipeline.ParseNames(stages)
	if err != nil {
		return nil, "", "", fmt.Errorf("--pipeline: %w", err)
	}
	cfg, err := adapter.LoadConfig()
	if err != nil {
		return nil, "", "", err
	}
	p, err := pipeline.New(names, pipeline.Deps{
		LLM: pipeline.LLM{
			APIBaseURL: cfg.APIBaseURL,
			APIKey:     cfg.APIKey,
			Model:      cfg.Model,
			Prompt:     cfg.Prompt,
			Budget:     dockerfile.Budget{MaxTokens: cfg.MaxTokens, ContextWindow: cfg.ContextWindow},
		},
	})
	if err != nil {
		return nil, "", "", err
	}
	return &eval.PipelineOptimizer{Pipeline: p}, cfg.Model, eval.PromptHash(cfg.Prompt), nil
}

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	out := fs.String("out", "", "write the markdown here instead of stdout")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New("give at least one results file")
	}

	var records []eval.Record
	for _, file := range fs.Args() {
		recs, err := eval.ReadRecords(file)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		if recs == nil {
			return fmt.Errorf("%s: no records", file)
		}
		records = append(records, recs...)
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
