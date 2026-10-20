// Package adapter implements the sleeko optimizer adapter service: a
// standalone HTTP server speaking the Harbor optimizer-adapter REST contract
// (see pkg/api/v1). It pulls the artifact from the Harbor registry with the
// credential Harbor hands it, extracts the Dockerfile from the BuildKit
// provenance attestation (or reconstructs one from image history when no
// attestation exists) and optimizes it via the LLM gateway.
package adapter

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/pipeline"
)

const (
	envListenAddr     = "OPTIMIZER_ADAPTER_LISTEN_ADDR"
	envJobTTL         = "OPTIMIZER_ADAPTER_JOB_TTL"
	envMaxConcurrency = "OPTIMIZER_ADAPTER_MAX_CONCURRENCY"
	envAPIKey         = "LLMGW_API_KEY"
	envAPIBaseURL     = "LLMGW_API_BASE_URL"
	envModel          = "LLMGW_MODEL"
	envPrompt         = "OPTIMIZER_PROMPT"
	envMaxTokens      = "LLMGW_MAX_TOKENS"
	envContextWindow  = "LLMGW_CONTEXT_WINDOW"
	envPipeline       = "OPTIMIZER_PIPELINE"
	envRepairRounds   = "OPTIMIZER_MAX_REPAIR_ROUNDS"
	envWorkerTimeout  = "OPTIMIZER_ADAPTER_WORKER_TIMEOUT"
	envTemperature    = "LLMGW_TEMPERATURE"
	envSeed           = "LLMGW_SEED"

	defaultListenAddr     = ":8080"
	defaultJobTTL         = time.Hour
	defaultMaxConcurrency = 4
	defaultMaxTokens      = 4096
	// defaultContextWindow is deliberately conservative: it is smaller than the
	// window of any model the gateway is likely to serve, so the default never
	// overruns a real one. Condensing the scan report keeps a typical prompt far
	// inside it anyway (see pkg/scanreport), so this only bites on pathological
	// images - and then it trims the report rather than failing the request.
	defaultContextWindow = 32768
	// defaultWorkerTimeout bounds one optimization end to end: registry
	// pulls and every LLM call, repair rounds included.
	defaultWorkerTimeout = 5 * time.Minute
)

// Config carries the adapter service configuration, resolved from environment
// variables. The LLM gateway credential lives here — on the adapter deployment —
// not on harbor-core.
type Config struct {
	// ListenAddr is the address the HTTP server binds to.
	ListenAddr string
	// JobTTL is how long finished optimization results are kept in the in-memory store.
	JobTTL time.Duration
	// MaxConcurrency bounds the number of optimizations running at once.
	MaxConcurrency int
	// APIKey is the LLM gateway API key (required).
	APIKey string
	// APIBaseURL is the LLM gateway chat-completions endpoint. It is
	// deployment-specific and has no baked-in default, so it must be supplied via
	// LLMGW_API_BASE_URL.
	APIBaseURL string
	// Model is the LLM model identifier. It is deployment-specific and has no
	// baked-in default, so it must be supplied via LLMGW_MODEL.
	Model string
	// Prompt is the instruction text prepended to the Dockerfile before it is sent
	// to the LLM gateway. It defaults to dockerfile.DefaultPrompt; set
	// OPTIMIZER_PROMPT to override it with deployment-specific phrasing.
	Prompt string
	// MaxTokens is the completion budget sent to the gateway as max_tokens.
	MaxTokens int
	// ContextWindow is the model's total token window, used to bound the prompt:
	// the condensed vulnerability scan report is trimmed to whatever room the
	// instructions and the Dockerfile leave inside it. Set LLMGW_CONTEXT_WINDOW
	// to 0 to disable trimming entirely.
	ContextWindow int
	// MaxRepairRounds bounds the verify stage's repair rounds
	// (OPTIMIZER_MAX_REPAIR_ROUNDS); 0 only checks.
	MaxRepairRounds int
	// WorkerTimeout bounds one optimization end to end
	// (OPTIMIZER_ADAPTER_WORKER_TIMEOUT). Repair rounds are extra LLM calls,
	// so a pipeline with verify may need more than the default.
	WorkerTimeout time.Duration
	// Temperature (LLMGW_TEMPERATURE) and Seed (LLMGW_SEED) set the
	// gateway's sampling; unset, they are not sent and the gateway's defaults
	// apply. Setting both makes a call repeatable.
	Temperature *float64
	Seed        *int64
	// Pipeline is the ordered list of optimization stages run after extraction
	// (see pkg/pipeline), set as a comma-separated OPTIMIZER_PIPELINE. It defaults
	// to pipeline.DefaultNames(), the single LLM call.
	Pipeline []string
}

// LoadConfig resolves the adapter configuration from the environment.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		ListenAddr:      defaultListenAddr,
		JobTTL:          defaultJobTTL,
		MaxConcurrency:  defaultMaxConcurrency,
		MaxTokens:       defaultMaxTokens,
		ContextWindow:   defaultContextWindow,
		APIKey:          os.Getenv(envAPIKey),
		APIBaseURL:      os.Getenv(envAPIBaseURL),
		Model:           os.Getenv(envModel),
		Prompt:          dockerfile.DefaultPrompt,
		Pipeline:        pipeline.DefaultNames(),
		MaxRepairRounds: pipeline.DefaultMaxRepairRounds,
		WorkerTimeout:   defaultWorkerTimeout,
	}

	if v := os.Getenv(envPrompt); v != "" {
		cfg.Prompt = v
	}

	if v := os.Getenv(envRepairRounds); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid %s %q: must be a non-negative integer", envRepairRounds, v)
		}
		cfg.MaxRepairRounds = n
	}

	if v := os.Getenv(envWorkerTimeout); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("invalid %s %q: must be a positive duration like 10m", envWorkerTimeout, v)
		}
		cfg.WorkerTimeout = d
	}

	if v := os.Getenv(envTemperature); v != "" {
		t, err := strconv.ParseFloat(v, 64)
		if err != nil || t < 0 || t > 2 {
			return nil, fmt.Errorf("invalid %s %q: must be a number from 0 to 2", envTemperature, v)
		}
		cfg.Temperature = &t
	}

	if v := os.Getenv(envSeed); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid %s %q: must be a non-negative integer", envSeed, v)
		}
		cfg.Seed = &n
	}

	if v := os.Getenv(envPipeline); v != "" {
		names, err := pipeline.ParseNames(v)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: %w", envPipeline, v, err)
		}
		cfg.Pipeline = names
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%s is required", envAPIKey)
	}

	if cfg.APIBaseURL == "" {
		return nil, fmt.Errorf("%s is required", envAPIBaseURL)
	}

	if cfg.Model == "" {
		return nil, fmt.Errorf("%s is required", envModel)
	}

	if v := os.Getenv(envListenAddr); v != "" {
		cfg.ListenAddr = v
	}

	if v := os.Getenv(envJobTTL); v != "" {
		ttl, err := time.ParseDuration(v)
		if err != nil || ttl <= 0 {
			return nil, fmt.Errorf("invalid %s %q: must be a positive duration like 1h", envJobTTL, v)
		}
		cfg.JobTTL = ttl
	}

	if v := os.Getenv(envMaxConcurrency); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid %s %q: must be a positive integer", envMaxConcurrency, v)
		}
		cfg.MaxConcurrency = n
	}

	if v := os.Getenv(envMaxTokens); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid %s %q: must be a positive integer", envMaxTokens, v)
		}
		cfg.MaxTokens = n
	}

	if v := os.Getenv(envContextWindow); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid %s %q: must be a non-negative integer (0 disables prompt trimming)", envContextWindow, v)
		}
		cfg.ContextWindow = n
	}

	// A completion budget at or above the whole window leaves no room for the
	// prompt, so every request would be rejected: catch it at startup rather
	// than once per optimization.
	if cfg.ContextWindow > 0 && cfg.MaxTokens >= cfg.ContextWindow {
		return nil, fmt.Errorf("%s (%d) must be smaller than %s (%d)",
			envMaxTokens, cfg.MaxTokens, envContextWindow, cfg.ContextWindow)
	}

	return cfg, nil
}
