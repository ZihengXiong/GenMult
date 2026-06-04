package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	memohcopilot "github.com/ZihengXiong/GenMult/internal/copilot"
	"github.com/ZihengXiong/GenMult/internal/db/postgres/sqlc"
	dbstore "github.com/ZihengXiong/GenMult/internal/db/store"
	"github.com/ZihengXiong/GenMult/internal/models"
)

const openAIAuthClaimPath = "https://api.openai.com/auth"

type ModelCredentials struct {
	APIKey         string //nolint:gosec // runtime credential material used to construct SDK providers
	CodexAccountID string
}

func SupportsOpenAICodexOAuth(provider sqlc.Provider) bool {
	return supportsOAuth(provider)
}

func (s *Service) ResolveModelCredentials(ctx context.Context, provider sqlc.Provider) (ModelCredentials, error) {
	switch models.ClientType(provider.ClientType) {
	case models.ClientTypeGitHubCopilot:
		githubToken, err := s.GetValidAccessToken(ctx, provider.ID.String())
		if err != nil {
			return ModelCredentials{}, err
		}
		copilotToken, err := memohcopilot.ResolveToken(ctx, githubToken)
		if err != nil {
			return ModelCredentials{}, err
		}
		return ModelCredentials{APIKey: copilotToken}, nil

	case models.ClientTypeOpenAICodex:
		token, err := s.GetValidAccessToken(ctx, provider.ID.String())
		if err != nil {
			return ModelCredentials{}, err
		}
		accountID, err := codexAccountIDFromToken(token)
		if err != nil {
			return ModelCredentials{}, err
		}
		return ModelCredentials{
			APIKey:         token,
			CodexAccountID: accountID,
		}, nil

	default:
		apiKey := ProviderConfigString(provider, "api_key")
		return ModelCredentials{
			APIKey: apiKey,
		}, nil
	}
}

func codexAccountIDFromToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid oauth access token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode oauth token payload: %w", err)
	}
	var claims struct {
		OpenAIAuth struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse oauth token payload: %w", err)
	}
	accountID := strings.TrimSpace(claims.OpenAIAuth.ChatGPTAccountID)
	if accountID == "" {
		return "", fmt.Errorf("oauth access token missing %s.chatgpt_account_id", openAIAuthClaimPath)
	}
	return accountID, nil
}

// ResolveAPIKeyForFramework resolves the API key of the active provider matching the given framework ("claudecode" or "codex").
func ResolveAPIKeyForFramework(ctx context.Context, queries dbstore.Queries, framework string) (string, error) {
	providersList, err := queries.ListProviders(ctx)
	if err != nil {
		return "", fmt.Errorf("list providers: %w", err)
	}

	var match *sqlc.Provider
	for _, p := range providersList {
		if !p.Enable {
			continue
		}
		if framework == "claudecode" && p.ClientType == string(models.ClientTypeAnthropicMessages) {
			pCopy := p
			match = &pCopy
			break
		}
		if framework == "codex" && (p.ClientType == string(models.ClientTypeOpenAICompletions) || p.ClientType == string(models.ClientTypeOpenAIResponses)) {
			pCopy := p
			match = &pCopy
			break
		}
	}

	if match == nil {
		switch framework {
		case "claudecode":
			return "", errors.New("anthropic provider not configured or disabled in database")
		case "codex":
			return "", errors.New("openai provider not configured or disabled in database")
		}
		return "", fmt.Errorf("provider for framework %q not configured", framework)
	}

	s := NewService(nil, queries, "")
	creds, err := s.ResolveModelCredentials(ctx, *match)
	if err != nil || creds.APIKey == "" {
		return "", fmt.Errorf("provider configured but API key is missing or invalid: %w", err)
	}

	return creds.APIKey, nil
}

