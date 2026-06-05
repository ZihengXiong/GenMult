package models

import "testing"

func TestUseNativeCodexBackend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		apiKey         string
		codexAccountID string
		want           bool
	}{
		{name: "jwt token uses native backend", apiKey: "a.b.c", want: true},
		{name: "account id forces native backend", apiKey: "plain-api-key", codexAccountID: "acct_123", want: true},
		{name: "plain api key uses responses backend", apiKey: "plain-api-key", want: false},
		{name: "empty credentials do not use native backend", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := useNativeCodexBackend(tt.apiKey, tt.codexAccountID); got != tt.want {
				t.Fatalf("useNativeCodexBackend() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSDKProviderOpenAICodexSelectsBackendByCredentialShape(t *testing.T) {
	t.Parallel()

	native := NewSDKProvider("https://chatgpt.com/backend-api", "a.b.c", "", ClientTypeOpenAICodex, 0, nil)
	if got := native.Name(); got != "openai-codex" {
		t.Fatalf("expected native codex provider, got %q", got)
	}

	compat := NewSDKProvider("https://www.autodl.art/api/v1", "plain-api-key", "", ClientTypeOpenAICodex, 0, nil)
	if got := compat.Name(); got != "openai-responses" {
		t.Fatalf("expected responses-compatible provider, got %q", got)
	}
}

func TestNewSDKChatModelOpenAICodexSelectsBackendByCredentialShape(t *testing.T) {
	t.Parallel()

	native := NewSDKChatModel(SDKModelConfig{
		ModelID:    "gpt-5.3-codex",
		ClientType: string(ClientTypeOpenAICodex),
		APIKey:     "a.b.c",
		BaseURL:    "https://chatgpt.com/backend-api",
	})
	if got := native.Provider.Name(); got != "openai-codex" {
		t.Fatalf("expected native codex provider, got %q", got)
	}

	compat := NewSDKChatModel(SDKModelConfig{
		ModelID:    "gpt-5.3-codex",
		ClientType: string(ClientTypeOpenAICodex),
		APIKey:     "plain-api-key",
		BaseURL:    "https://www.autodl.art/api/v1",
	})
	if got := compat.Provider.Name(); got != "openai-responses" {
		t.Fatalf("expected responses-compatible provider, got %q", got)
	}
}
