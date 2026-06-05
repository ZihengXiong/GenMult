package botruntime

import (
	"context"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
)

type RunInput struct {
	Config agentpkg.RunConfig
}

type BotRuntime interface {
	Name() string
	Stream(ctx context.Context, in RunInput) <-chan agentpkg.StreamEvent
	Generate(ctx context.Context, in RunInput) (*agentpkg.GenerateResult, error)
	IdleTimeout() time.Duration
}

type Registry struct {
	runtimes map[string]BotRuntime
	fallback BotRuntime
}

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

func (r *Registry) Resolve(framework string) BotRuntime {
	if r == nil {
		return nil
	}
	if rt, ok := r.runtimes[framework]; ok {
		return rt
	}
	return r.fallback
}
