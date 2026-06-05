package providers

import (
	"os"
	"strconv"
)

// ClaudeCodeConfig holds configuration for the Claude Code CLI provider.
type ClaudeCodeConfig struct {
	APIKey         string   `toml:"api_key"`
	AuthToken      string   `toml:"auth_token"`
	BaseURL        string   `toml:"base_url"`
	PermissionMode string   `toml:"permission_mode"`
	MaxTurns       int      `toml:"max_turns"`
	AllowedTools   []string `toml:"allowed_tools"`
	Model          string   `toml:"model"`
}

type CodexConfig struct {
	APIKey  string `toml:"api_key"`
	BaseURL string `toml:"base_url"`
	Sandbox string `toml:"sandbox"`
	Model   string `toml:"model"`
}

// ProviderConfigs aggregates all external provider configurations.
type ProviderConfigs struct {
	ClaudeCode ClaudeCodeConfig `toml:"claude_code"`
	Codex      CodexConfig      `toml:"codex"`
}

// FromEnvWithDefaults fills unset fields from environment variables and sets defaults.
func (c *ProviderConfigs) FromEnvWithDefaults() {
	if c.ClaudeCode.APIKey == "" {
		c.ClaudeCode.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if c.ClaudeCode.AuthToken == "" {
		c.ClaudeCode.AuthToken = os.Getenv("ANTHROPIC_AUTH_TOKEN")
	}
	if c.ClaudeCode.PermissionMode == "" {
		c.ClaudeCode.PermissionMode = "auto"
	}
	if c.ClaudeCode.MaxTurns <= 0 {
		if val := os.Getenv("CLAUDE_MAX_TURNS"); val != "" {
			if parsed, err := strconv.Atoi(val); err == nil {
				c.ClaudeCode.MaxTurns = parsed
			}
		}
		if c.ClaudeCode.MaxTurns <= 0 {
			c.ClaudeCode.MaxTurns = 15
		}
	}

	if c.Codex.APIKey == "" {
		c.Codex.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if c.Codex.Sandbox == "" {
		c.Codex.Sandbox = "workspace-write"
	}

	if c.ClaudeCode.Model == "" {
		c.ClaudeCode.Model = os.Getenv("CLAUDE_DEFAULT_MODEL")
	}
}
