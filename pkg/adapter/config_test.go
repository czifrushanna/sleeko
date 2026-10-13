package adapter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/czifrushanna/sleeko/pkg/dockerfile"
	"github.com/czifrushanna/sleeko/pkg/pipeline"
)

// setBaseEnv clears every adapter env var so each test starts from a known state
// regardless of what the surrounding environment has set.
func setBaseEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{envAPIKey, envModel, envAPIBaseURL, envPrompt, envListenAddr, envJobTTL, envMaxConcurrency, envMaxTokens, envContextWindow, envPipeline} {
		t.Setenv(k, "")
	}
}

// setRequiredEnv sets every required env var to a valid value; tests override
// individual ones to exercise the missing-value error paths.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envAPIKey, "key")
	t.Setenv(envModel, "some-model")
	t.Setenv(envAPIBaseURL, "https://example.test/v1/chat")
}

func TestLoadConfig_APIKeyRequired(t *testing.T) {
	setBaseEnv(t)
	setRequiredEnv(t)
	t.Setenv(envAPIKey, "")

	_, err := LoadConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), envAPIKey)
}

func TestLoadConfig_APIBaseURLRequired(t *testing.T) {
	setBaseEnv(t)
	setRequiredEnv(t)
	t.Setenv(envAPIBaseURL, "")

	_, err := LoadConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), envAPIBaseURL)
}

func TestLoadConfig_ModelRequired(t *testing.T) {
	setBaseEnv(t)
	setRequiredEnv(t)
	t.Setenv(envModel, "")

	_, err := LoadConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), envModel)
}

func TestLoadConfig_Defaults(t *testing.T) {
	setBaseEnv(t)
	setRequiredEnv(t)

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, "key", cfg.APIKey)
	require.Equal(t, "some-model", cfg.Model)
	require.Equal(t, "https://example.test/v1/chat", cfg.APIBaseURL)
	require.Equal(t, dockerfile.DefaultPrompt, cfg.Prompt)
	require.Equal(t, defaultListenAddr, cfg.ListenAddr)
	require.Equal(t, defaultJobTTL, cfg.JobTTL)
	require.Equal(t, defaultMaxConcurrency, cfg.MaxConcurrency)
	require.Equal(t, defaultMaxTokens, cfg.MaxTokens)
	require.Equal(t, defaultContextWindow, cfg.ContextWindow)
	require.Equal(t, pipeline.DefaultNames(), cfg.Pipeline)
}

func TestLoadConfig_PromptOverride(t *testing.T) {
	setBaseEnv(t)
	setRequiredEnv(t)
	t.Setenv(envPrompt, "some-prompt")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, "some-prompt", cfg.Prompt)
}

func TestLoadConfig_Overrides(t *testing.T) {
	setBaseEnv(t)
	setRequiredEnv(t)
	t.Setenv(envListenAddr, ":9090")
	t.Setenv(envJobTTL, "30m")
	t.Setenv(envMaxConcurrency, "8")
	t.Setenv(envMaxTokens, "2048")
	t.Setenv(envContextWindow, "131072")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, ":9090", cfg.ListenAddr)
	require.Equal(t, 30*time.Minute, cfg.JobTTL)
	require.Equal(t, 8, cfg.MaxConcurrency)
	require.Equal(t, 2048, cfg.MaxTokens)
	require.Equal(t, 131072, cfg.ContextWindow)
}

// A zero context window is a valid setting: it turns prompt trimming off.
func TestLoadConfig_ContextWindowZeroDisablesTrimming(t *testing.T) {
	setBaseEnv(t)
	setRequiredEnv(t)
	t.Setenv(envContextWindow, "0")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, 0, cfg.ContextWindow)
}

func TestLoadConfig_MaxTokensMustLeaveRoomForThePrompt(t *testing.T) {
	setBaseEnv(t)
	setRequiredEnv(t)
	t.Setenv(envMaxTokens, "32768")
	t.Setenv(envContextWindow, "32768")

	_, err := LoadConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), envMaxTokens)
	require.Contains(t, err.Error(), envContextWindow)
}

func TestLoadConfig_InvalidValues(t *testing.T) {
	cases := map[string]string{
		envJobTTL:         "not-a-duration",
		envMaxConcurrency: "-1",
		envMaxTokens:      "0",
		envContextWindow:  "-1",
	}
	for env, bad := range cases {
		t.Run(env, func(t *testing.T) {
			setBaseEnv(t)
			setRequiredEnv(t)
			t.Setenv(env, bad)

			_, err := LoadConfig()
			require.Error(t, err)
			require.Contains(t, err.Error(), env)
		})
	}
}

func TestLoadConfig_Pipeline(t *testing.T) {
	cases := []struct {
		name    string
		spec    string
		want    []string
		wantErr string
	}{
		{"single stage", "generate", []string{"generate"}, ""},
		{"spaces and empty entries are ignored", " generate , ", []string{"generate"}, ""},
		{"unknown stage refuses to start", "generate,verfiy", nil, `unknown stage "verfiy"`},
		{"only separators", ",", nil, "no stages configured"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setBaseEnv(t)
			setRequiredEnv(t)
			t.Setenv(envPipeline, c.spec)

			cfg, err := LoadConfig()
			if c.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), envPipeline)
				require.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, cfg.Pipeline)
		})
	}
}
