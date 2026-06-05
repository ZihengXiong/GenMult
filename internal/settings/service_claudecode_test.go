package settings

import "testing"

func TestIsClaudeCodeChatModelClientType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		clientType string
		want       bool
	}{
		{name: "anthropic allowed", clientType: "anthropic-messages", want: true},
		{name: "trimmed anthropic allowed", clientType: " anthropic-messages ", want: true},
		{name: "openai codex rejected", clientType: "openai-codex", want: false},
		{name: "openai completions rejected", clientType: "openai-completions", want: false},
		{name: "empty rejected", clientType: "", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isClaudeCodeChatModelClientType(tc.clientType); got != tc.want {
				t.Fatalf("isClaudeCodeChatModelClientType(%q) = %v, want %v", tc.clientType, got, tc.want)
			}
		})
	}
}
