package settings

import "testing"

func TestIsCodexChatModelClientType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		clientType string
		want       bool
	}{
		{name: "openai codex allowed", clientType: "openai-codex", want: true},
		{name: "trimmed openai codex allowed", clientType: " openai-codex ", want: true},
		{name: "openai completions rejected", clientType: "openai-completions", want: false},
		{name: "github copilot rejected", clientType: "github-copilot", want: false},
		{name: "empty rejected", clientType: "", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isCodexChatModelClientType(tc.clientType); got != tc.want {
				t.Fatalf("isCodexChatModelClientType(%q) = %v, want %v", tc.clientType, got, tc.want)
			}
		})
	}
}
