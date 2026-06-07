package providers

import (
	"os"
	"strconv"
)

// ClaudeCodeConfig holds configuration for the Claude Code CLI provider.
type ClaudeCodeConfig struct {
	APIKey         string            `toml:"api_key"`    //nolint:gosec // intentional: struct field for config parsing
	AuthToken      string            `toml:"auth_token"` //nolint:gosec // intentional: struct field for config parsing
	BaseURL        string            `toml:"base_url"`
	PermissionMode string            `toml:"permission_mode"`
	MaxTurns       int               `toml:"max_turns"`
	Model          string            `toml:"model"`
	CustomEnv      map[string]string `toml:"custom_env"`
	AllowedTools   []string          `toml:"allowed_tools"`
	// MaxContextMessages caps how many recent history messages (user/assistant
	// mixed) are replayed into the prompt for multi-turn context. Distinct from
	// MaxTurns (the CLI --max-turns agent-iteration cap). Defaults to 15.
	// The json tag is required: per-bot overrides arrive as snake_case JSON via
	// provider_ext.claudecode and are merged with encoding/json.
	MaxContextMessages int `toml:"max_context_messages" json:"max_context_messages"`
}

type CodexConfig struct {
	APIKey  string `toml:"api_key"` //nolint:gosec // intentional: struct field for config parsing
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
	if c.ClaudeCode.MaxContextMessages <= 0 {
		c.ClaudeCode.MaxContextMessages = 15
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
