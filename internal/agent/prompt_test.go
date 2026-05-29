package agent

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateSystemPromptIncludesPlatformIdentitiesInChat(t *testing.T) {
	t.Parallel()

	prompt := GenerateSystemPrompt(SystemPromptParams{
		SessionType:               "chat",
		Now:                       time.Unix(1, 0).UTC(),
		Timezone:                  "UTC",
		PlatformIdentitiesSection: "## Platform Identities\n\n<identity channel=\"telegram\" username=\"@memoh\"/>",
	})

	if !strings.Contains(prompt, "## Platform Identities") {
		t.Fatalf("expected platform identities heading in prompt")
	}
	if !strings.Contains(prompt, `<identity channel="telegram" username="@memoh"/>`) {
		t.Fatalf("expected platform identity XML in prompt")
	}
}

func TestGenerateSystemPromptIncludesPlatformIdentitiesInDiscuss(t *testing.T) {
	t.Parallel()

	prompt := GenerateSystemPrompt(SystemPromptParams{
		SessionType:               "discuss",
		Now:                       time.Unix(1, 0).UTC(),
		Timezone:                  "UTC",
		PlatformIdentitiesSection: "## Platform Identities\n\n<identity channel=\"discord\" username=\"@memoh\"/>",
	})

	if !strings.Contains(prompt, "## Platform Identities") {
		t.Fatalf("expected platform identities heading in discuss prompt")
	}
	if !strings.Contains(prompt, `<identity channel="discord" username="@memoh"/>`) {
		t.Fatalf("expected platform identity XML in discuss prompt")
	}
}

func TestGenerateSystemPromptClarifiesSpeakerOwnedAccounts(t *testing.T) {
	t.Parallel()

	for _, sessionType := range []string{"chat", "discuss"} {
		prompt := GenerateSystemPrompt(SystemPromptParams{
			SessionType: sessionType,
			Now:         time.Unix(1, 0).UTC(),
			Timezone:    "UTC",
		})

		if !strings.Contains(prompt, `first-person words such as "I", "me", "my", "mine", "我的", and "我" refer to that message's `+"`sender`") {
			t.Fatalf("expected first-person speaker guidance in %s prompt", sessionType)
		}
		if !strings.Contains(prompt, "If the sender's account/link is missing or ambiguous, ask the sender for it before searching or acting.") {
			t.Fatalf("expected ambiguous external account guidance in %s prompt", sessionType)
		}
		if !strings.Contains(prompt, "Do not substitute a GitHub/OAuth/tool account") {
			t.Fatalf("expected tool account substitution warning in %s prompt", sessionType)
		}
		if !strings.Contains(prompt, "Only store or reuse a sender-to-account mapping after that same sender explicitly provides or confirms their own account.") {
			t.Fatalf("expected sender-account memory guard in %s prompt", sessionType)
		}
		if !strings.Contains(prompt, `If a message says "Alice is @alicehub", treat it as a claim about Alice.`) {
			t.Fatalf("expected named-person account claim guidance in %s prompt", sessionType)
		}
	}
}
