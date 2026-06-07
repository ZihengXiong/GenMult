package botruntime

import (
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/ZihengXiong/GenMult/internal/agent"
	"github.com/ZihengXiong/GenMult/internal/agenthub/providers"
)

func ri(cfg agentpkg.RunConfig) RunInput { return RunInput{Config: cfg} }

func TestClaudeCurrentTurn_QuerySet(t *testing.T) {
	in := ri(agentpkg.RunConfig{
		Query: "  current question  ",
		Messages: []sdk.Message{
			sdk.UserMessage("hi"),
			sdk.AssistantMessage("hello"),
		},
	})
	cur, hist := claudeCurrentTurn(in)
	if cur != "current question" {
		t.Fatalf("current = %q, want trimmed Query", cur)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2 (all Messages are history)", len(hist))
	}
}

func TestClaudeCurrentTurn_PipelineNoQuery(t *testing.T) {
	in := ri(agentpkg.RunConfig{
		Messages: []sdk.Message{
			sdk.UserMessage("older"),
			sdk.AssistantMessage("reply"),
			sdk.UserMessage("the current one"),
		},
	})
	cur, hist := claudeCurrentTurn(in)
	if cur != "the current one" {
		t.Fatalf("current = %q, want last user message", cur)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2 (current excluded)", len(hist))
	}
	for _, m := range hist {
		if extractMsgText(m) == "the current one" {
			t.Fatalf("current turn leaked into history")
		}
	}
}

func TestFormatHistoryTranscript_TruncateAndLabel(t *testing.T) {
	msgs := []sdk.Message{
		sdk.UserMessage("[Alice] one"),
		sdk.AssistantMessage("two"),
		sdk.UserMessage("[Bob] three"),
		sdk.AssistantMessage("four"),
	}
	got := formatHistoryTranscript(msgs, 2) // keep last 2
	if strings.Contains(got, "one") || strings.Contains(got, "two") {
		t.Fatalf("expected only the last 2 messages, got: %s", got)
	}
	if !strings.Contains(got, "[Bob] three") {
		t.Fatalf("user turn should be verbatim (with sender prefix): %s", got)
	}
	if !strings.Contains(got, "Assistant: four") {
		t.Fatalf("assistant turn should be labeled: %s", got)
	}
	if !strings.Contains(got, "--- Conversation History ---") ||
		!strings.Contains(got, "--- End of History ---") {
		t.Fatalf("missing transcript markers: %s", got)
	}
}

func TestFormatHistoryTranscript_Empty(t *testing.T) {
	if got := formatHistoryTranscript(nil, 15); got != "" {
		t.Fatalf("nil messages: want empty, got %q", got)
	}
	if got := formatHistoryTranscript([]sdk.Message{sdk.UserMessage("   ")}, 15); got != "" {
		t.Fatalf("blank content: want empty, got %q", got)
	}
}

func TestFormatHistoryTranscript_NoLimit(t *testing.T) {
	msgs := []sdk.Message{
		sdk.UserMessage("a"),
		sdk.AssistantMessage("b"),
		sdk.UserMessage("c"),
	}
	got := formatHistoryTranscript(msgs, 0) // 0 == no limit
	for _, want := range []string{"a", "Assistant: b", "c"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
}

func TestClaudeAppendSystem(t *testing.T) {
	in := ri(agentpkg.RunConfig{
		System: "You are helpful.",
		Query:  "what is my name?",
		Messages: []sdk.Message{
			sdk.UserMessage("[Alice] hi"),
			sdk.AssistantMessage("hello"),
		},
	})
	out := claudeAppendSystem(in, providers.ClaudeCodeConfig{MaxContextMessages: 15})
	if !strings.Contains(out, "You are helpful.") {
		t.Fatalf("system preamble missing: %s", out)
	}
	if !strings.Contains(out, "[Alice] hi") || !strings.Contains(out, "Assistant: hello") {
		t.Fatalf("history missing from append-system: %s", out)
	}
	if strings.Contains(out, "what is my name?") {
		t.Fatalf("current turn must NOT be in append-system (it goes to stdin): %s", out)
	}
}

func TestClaudeCurrentUserNDJSON_Shape(t *testing.T) {
	line := claudeCurrentUserNDJSON(ri(agentpkg.RunConfig{Query: "what is my name?"}))
	if !strings.HasSuffix(line, "\n") {
		t.Fatalf("NDJSON line must end with newline")
	}
	var got struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &got); err != nil {
		t.Fatalf("invalid NDJSON: %v (line=%s)", err, line)
	}
	if got.Type != "user" || got.Message.Role != "user" {
		t.Fatalf("bad envelope: %+v", got)
	}
	if len(got.Message.Content) != 1 ||
		got.Message.Content[0].Type != "text" ||
		got.Message.Content[0].Text != "what is my name?" {
		t.Fatalf("bad content: %+v", got.Message.Content)
	}
}

func TestClaudeCurrentUserNDJSON_EmptyWhenNoTurn(t *testing.T) {
	if got := claudeCurrentUserNDJSON(ri(agentpkg.RunConfig{})); got != "" {
		t.Fatalf("no current turn: want empty, got %q", got)
	}
}
