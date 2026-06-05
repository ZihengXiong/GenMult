package botruntime

import (
	"context"
	"testing"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
)

type stubRuntime struct {
	name string
}

func (s stubRuntime) Name() string { return s.name }

func (s stubRuntime) Stream(_ context.Context, _ RunInput) <-chan agentpkg.StreamEvent { return nil }

func (s stubRuntime) Generate(_ context.Context, _ RunInput) (*agentpkg.GenerateResult, error) {
	return nil, nil
}

func (s stubRuntime) IdleTimeout() time.Duration { return 0 }

func TestRegistryResolve(t *testing.T) {
	memoh := stubRuntime{name: "memoh"}
	codex := stubRuntime{name: "codex"}

	reg := NewRegistry(memoh, codex)

	if got := reg.Resolve("codex"); got == nil || got.Name() != "codex" {
		t.Fatalf("expected codex runtime, got %#v", got)
	}
	if got := reg.Resolve("missing"); got == nil || got.Name() != "memoh" {
		t.Fatalf("expected fallback runtime, got %#v", got)
	}
}
