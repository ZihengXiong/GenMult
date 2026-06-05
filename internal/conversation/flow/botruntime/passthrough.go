package botruntime

import (
	"context"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
)

type passthroughRuntime struct {
	name  string
	agent *agentpkg.Agent
}

func NewPassthroughRuntime(name string, agent *agentpkg.Agent) BotRuntime {
	return passthroughRuntime{name: name, agent: agent}
}

func (p passthroughRuntime) Name() string { return p.name }

func (p passthroughRuntime) Stream(ctx context.Context, in RunInput) <-chan agentpkg.StreamEvent {
	return p.agent.Stream(ctx, in.Config)
}

func (p passthroughRuntime) Generate(ctx context.Context, in RunInput) (*agentpkg.GenerateResult, error) {
	return p.agent.Generate(ctx, in.Config)
}

func (p passthroughRuntime) IdleTimeout() time.Duration { return 0 }
