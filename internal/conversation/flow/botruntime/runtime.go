// Package botruntime introduces a conversational runtime abstraction so a bot
// can be backed by different agent frameworks. The built-in memoh runtime wraps
// the in-process agent.Agent (default, unchanged behavior); the CLI-backed
// runtimes (claudecode, codex) drive their respective CLIs while emitting the
// same agent.StreamEvent stream the resolver already consumes.
package botruntime

import (
	"context"
	"time"

	agentpkg "github.com/ZihengXiong/GenMult/internal/agent"
)

// RunInput is the framework-agnostic turn input. It carries the existing
// agent.RunConfig because the memoh runtime needs it verbatim, while the CLI
// runtimes consume only a framework-agnostic subset (Query, System, Identity,
// Messages).
type RunInput struct {
	Config agentpkg.RunConfig
}

// BotRuntime drives a single conversational turn for a bot. Implementations
// emit agent.StreamEvent so the resolver's existing streaming, persistence,
// idle-timeout, and terminal-snapshot machinery works unchanged across
// frameworks.
type BotRuntime interface {
	// Name returns the framework identifier ("memoh", "claudecode", "codex").
	Name() string
	// Stream runs the turn and streams events; the channel is closed when done.
	Stream(ctx context.Context, in RunInput) <-chan agentpkg.StreamEvent
	// Generate runs the turn and returns the final result.
	Generate(ctx context.Context, in RunInput) (*agentpkg.GenerateResult, error)
	IdleTimeout() time.Duration
}

// Registry resolves a framework identifier to its BotRuntime, falling back to a
// default (memoh) when the framework is empty or unregistered.
type Registry struct {
	runtimes map[string]BotRuntime
	fallback BotRuntime
}

// NewRegistry builds a registry. The first runtime is used as the fallback.
func NewRegistry(fallback BotRuntime, others ...BotRuntime) *Registry {
	r := &Registry{runtimes: make(map[string]BotRuntime)}
	if fallback != nil {
		r.fallback = fallback
		r.runtimes[fallback.Name()] = fallback
	}
	for _, rt := range others {
		if rt == nil {
			continue
		}
		r.runtimes[rt.Name()] = rt
	}
	return r
}

// Add registers additional runtimes, keyed by Name. The fallback is unchanged.
func (r *Registry) Add(runtimes ...BotRuntime) {
	if r == nil {
		return
	}
	if r.runtimes == nil {
		r.runtimes = make(map[string]BotRuntime)
	}
	for _, rt := range runtimes {
		if rt == nil {
			continue
		}
		r.runtimes[rt.Name()] = rt
	}
}

// Resolve returns the runtime for a framework, or the fallback when the
// framework is empty or unknown.
func (r *Registry) Resolve(framework string) BotRuntime {
	if r == nil {
		return nil
	}
	if rt, ok := r.runtimes[framework]; ok {
		return rt
	}
	return r.fallback
}
