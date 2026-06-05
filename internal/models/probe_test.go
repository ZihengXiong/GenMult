package models

import (
	"context"
	"testing"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestResolveModelCredentialsOpenAICodexUsesConfigAPIKeyFallback(t *testing.T) {
	t.Parallel()

	service := &Service{}
	provider := sqlc.Provider{
		ClientType: string(ClientTypeOpenAICodex),
		Config:     []byte(`{"api_key":"codex-config-key","codex_account_id":"acct_config"}`),
	}

	creds, err := service.resolveModelCredentials(context.Background(), provider)
	if err != nil {
		t.Fatalf("expected config fallback to succeed, got %v", err)
	}
	if creds.APIKey != "codex-config-key" {
		t.Fatalf("expected api key from config, got %q", creds.APIKey)
	}
	if creds.CodexAccountID != "acct_config" {
		t.Fatalf("expected account id from config, got %q", creds.CodexAccountID)
	}
}
