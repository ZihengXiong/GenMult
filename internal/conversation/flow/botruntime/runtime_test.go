package botruntime

import (
	"context"
	"testing"
	"time"

	agentpkg "github.com/ZihengXiong/GenMult/internal/agent"
)

type stubRuntime struct{ name string }

func (s stubRuntime) Name() string { return s.name }
func (stubRuntime) Stream(context.Context, RunInput) <-chan agentpkg.StreamEvent {
	ch := make(chan agentpkg.StreamEvent)
	close(ch)
	return ch
}

func (stubRuntime) Generate(context.Context, RunInput) (*agentpkg.GenerateResult, error) {
	return &agentpkg.GenerateResult{}, nil
}
func (stubRuntime) IdleTimeout() time.Duration { return 0 }

func TestRegistryResolve(t *testing.T) {
	memoh := stubRuntime{name: "memoh"}
	claude := stubRuntime{name: "claudecode"}
	reg := NewRegistry(memoh, claude)

	cases := map[string]string{
		"memoh":      "memoh",
		"claudecode": "claudecode",
		"":           "memoh", // empty falls back
		"unknown":    "memoh", // unknown falls back
	}
	for framework, want := range cases {
		got := reg.Resolve(framework)
		if got == nil {
			t.Fatalf("framework %q: got nil runtime", framework)
		}
		if got.Name() != want {
			t.Errorf("framework %q: got %q, want %q", framework, got.Name(), want)
		}
	}
}

func TestRegistryAdd(t *testing.T) {
	reg := NewRegistry(stubRuntime{name: "memoh"})
	reg.Add(stubRuntime{name: "codex"})
	if got := reg.Resolve("codex"); got == nil || got.Name() != "codex" {
		t.Fatalf("expected codex runtime after Add, got %v", got)
	}
	// Fallback unchanged.
	if got := reg.Resolve("nope"); got.Name() != "memoh" {
		t.Errorf("expected memoh fallback, got %q", got.Name())
	}
}
