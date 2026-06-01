package botruntime

import (
	"context"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/bots"
)

// memohRuntime is the built-in framework. It is a thin pass-through to the
// in-process agent.Agent, so existing behavior is preserved byte-for-byte.
type memohRuntime struct {
	agent *agentpkg.Agent
}

// NewMemohRuntime wraps the in-process agent as a BotRuntime.
func NewMemohRuntime(agent *agentpkg.Agent) BotRuntime {
	return memohRuntime{agent: agent}
}

func (m memohRuntime) Name() string { return bots.FrameworkMemoh }

func (m memohRuntime) Stream(ctx context.Context, in RunInput) <-chan agentpkg.StreamEvent {
	return m.agent.Stream(ctx, in.Config)
}

func (m memohRuntime) Generate(ctx context.Context, in RunInput) (*agentpkg.GenerateResult, error) {
	return m.agent.Generate(ctx, in.Config)
}
