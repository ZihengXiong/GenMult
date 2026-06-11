package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// clearProviderEnv blanks every env var FromEnvWithDefaults reads, so tests
// are insulated from the developer's real shell/.env configuration.
func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_MAX_TURNS", "CLAUDE_DEFAULT_MODEL",
		"OPENAI_API_KEY", "OPENAI_BASE_URL", "DEEPSEEK_API_KEY", "DEEPSEEK_API_URL",
	} {
		t.Setenv(k, "")
	}
}

func TestFromEnvWithDefaults_CodexPrefersOpenAIKey(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("DEEPSEEK_API_KEY", "sk-deepseek")

	var cfg ProviderConfigs
	cfg.FromEnvWithDefaults()

	assert.Equal(t, "sk-openai", cfg.Codex.APIKey)
	assert.Empty(t, cfg.Codex.BaseURL, "OPENAI_API_KEY must not trigger the DeepSeek base-URL fallback")
}

// TestFromEnvWithDefaults_CodexDeepSeekFallback: the codex provider's
// missing-key error names DEEPSEEK_API_KEY as accepted, so it must actually be
// read — and since a DeepSeek key is useless against api.openai.com, the base
// URL must follow it to DeepSeek's OpenAI-compatible endpoint when nothing
// else configures one.
func TestFromEnvWithDefaults_CodexDeepSeekFallback(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("DEEPSEEK_API_KEY", "sk-deepseek")
	t.Setenv("DEEPSEEK_API_URL", "https://api.deepseek.com/")

	var cfg ProviderConfigs
	cfg.FromEnvWithDefaults()

	assert.Equal(t, "sk-deepseek", cfg.Codex.APIKey)
	assert.Equal(t, "https://api.deepseek.com/v1", cfg.Codex.BaseURL)
}

func TestFromEnvWithDefaults_CodexDeepSeekRespectsExplicitBaseURL(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("DEEPSEEK_API_KEY", "sk-deepseek")
	t.Setenv("DEEPSEEK_API_URL", "https://api.deepseek.com")
	t.Setenv("OPENAI_BASE_URL", "https://proxy.example.com/v1")

	var cfg ProviderConfigs
	cfg.FromEnvWithDefaults()

	assert.Equal(t, "sk-deepseek", cfg.Codex.APIKey)
	assert.Empty(t, cfg.Codex.BaseURL, "OPENAI_BASE_URL is already honored by CodexEnv at exec time; config must not override it")
}
