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
	BaseURL        string
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
		if apiKey := strings.TrimSpace(ProviderConfigString(provider, "api_key")); apiKey != "" {
			return ModelCredentials{
				APIKey:         apiKey,
				CodexAccountID: strings.TrimSpace(ProviderConfigString(provider, "codex_account_id")),
			}, nil
		}

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
		baseURL := ProviderConfigString(provider, "base_url")
		return ModelCredentials{
			APIKey:  apiKey,
			BaseURL: baseURL,
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

// ResolveCredentialsForFramework resolves the credentials of the active provider matching the given framework ("claudecode" or "codex").
//
// NOTE(design): a CLI framework does not strictly depend on a same-typed
// provider. Claude Code in particular can authenticate via per-bot
// provider_ext["claudecode"] (APIKey / AuthToken / BaseURL — see
// mergeClaudeCodeConfig in botruntime/cli.go), which supports both the official
// Anthropic API and third-party Anthropic-compatible endpoints (auth token +
// custom base_url configured in the bot's agent settings). This function is the
// fallback credential source: it locates an enabled anthropic-typed provider
// (codex: openai-typed) and uses its key when the bot has no provider_ext
// override.
//
// CAVEAT(current): runtime still calls this unconditionally (cli.go run ->
// resolveCreds) and it errors when no matching provider exists, so today a
// provider_ext-only bot ALSO needs a matching enabled provider present. The
// "claudecode doesn't depend on an anthropic provider" goal is therefore only
// partially realized; to fully decouple, skip/relax this lookup when the bot's
// provider_ext already carries APIKey/AuthToken+BaseURL.
func ResolveCredentialsForFramework(ctx context.Context, queries dbstore.Queries, framework string) (ModelCredentials, error) {
	providersList, err := queries.ListProviders(ctx)
	if err != nil {
		return ModelCredentials{}, fmt.Errorf("list providers: %w", err)
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
			return ModelCredentials{}, errors.New("anthropic provider not configured or disabled in database")
		case "codex":
			return ModelCredentials{}, errors.New("openai provider not configured or disabled in database")
		}
		return ModelCredentials{}, fmt.Errorf("provider for framework %q not configured", framework)
	}

	s := NewService(nil, queries, "")
	creds, err := s.ResolveModelCredentials(ctx, *match)
	if err != nil || creds.APIKey == "" {
		return ModelCredentials{}, fmt.Errorf("provider configured but API key is missing or invalid: %w", err)
	}

	return creds, nil
}
