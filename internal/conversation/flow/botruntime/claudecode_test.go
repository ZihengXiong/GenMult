package botruntime

import (
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
)

func TestClaudeEnvUsesAuthTokenForThirdPartyBaseURL(t *testing.T) {
	t.Parallel()

	env := claudeEnv(ClaudeCodeConfig{
		APIKey:  "test-key",
		BaseURL: "https://api.deepseek.com/anthropic",
	})

	foundAuthToken := false
	foundAPIKey := false
	for _, item := range env {
		if item == "ANTHROPIC_AUTH_TOKEN=test-key" {
			foundAuthToken = true
		}
		if item == "ANTHROPIC_API_KEY=test-key" {
			foundAPIKey = true
		}
	}
	if !foundAuthToken {
		t.Fatalf("expected ANTHROPIC_AUTH_TOKEN to be set for third-party base url, got %#v", env)
	}
	if foundAPIKey {
		t.Fatalf("did not expect ANTHROPIC_API_KEY for third-party base url, got %#v", env)
	}
}

func TestParseClaudeEventAssistantText(t *testing.T) {
	t.Parallel()

	event, err := parseClaudeEvent([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"},{"type":"text","text":" world"}]}}`))
	if err != nil {
		t.Fatalf("parseClaudeEvent() error = %v", err)
	}
	if event.Type != "text" {
		t.Fatalf("expected text event, got %q", event.Type)
	}
	if event.Content != "hello world" {
		t.Fatalf("expected merged text, got %q", event.Content)
	}
}

func TestParseClaudeEventResultError(t *testing.T) {
	t.Parallel()

	event, err := parseClaudeEvent([]byte(`{"type":"result","is_error":true,"result":"bad auth"}`))
	if err != nil {
		t.Fatalf("parseClaudeEvent() error = %v", err)
	}
	if event.Type != "error" {
		t.Fatalf("expected error event, got %q", event.Type)
	}
	if event.Content != "bad auth" {
		t.Fatalf("expected error content, got %q", event.Content)
	}
}

func TestPromptForUsesConversationHistory(t *testing.T) {
	t.Parallel()

	prompt := promptFor(RunInput{
		Config: agentpkg.RunConfig{
			System: "system prompt",
			Messages: []sdk.Message{
				sdk.UserMessage("first question"),
				sdk.AssistantMessage("first answer"),
				sdk.UserMessage("second question"),
			},
		},
	})

	for _, want := range []string{
		"system prompt",
		"USER: first question",
		"ASSISTANT: first answer",
		"USER: second question",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got %q", want, prompt)
		}
	}
}
