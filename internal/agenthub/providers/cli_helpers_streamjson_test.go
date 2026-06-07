package providers

import (
	"strings"
	"testing"
)

func TestClaudeBuildArgsStreamJSON(t *testing.T) {
	cfg := ClaudeCodeConfig{PermissionMode: "auto", MaxTurns: 7, Model: "sonnet"}
	args := ClaudeBuildArgsStreamJSON(cfg, "SYSTEM PROMPT")
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"--input-format stream-json",
		"--output-format stream-json",
		"--verbose",
		"--permission-mode auto",
		"--max-turns 7",
		"--model sonnet",
		"--append-system-prompt SYSTEM PROMPT",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in args: %q", want, joined)
		}
	}
	if args[0] != "-p" {
		t.Fatalf("first arg should be -p, got %q", args[0])
	}
}

func TestClaudeBuildArgsStreamJSON_NoAppendWhenBlank(t *testing.T) {
	args := ClaudeBuildArgsStreamJSON(ClaudeCodeConfig{}, "   ")
	for _, a := range args {
		if a == "--append-system-prompt" {
			t.Fatalf("blank appendSystem should not add --append-system-prompt: %v", args)
		}
	}
	// Defaults still applied when MaxTurns/PermissionMode unset.
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--max-turns 15") || !strings.Contains(joined, "--permission-mode auto") {
		t.Fatalf("expected defaults, got: %s", joined)
	}
}
