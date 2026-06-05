package botruntime

import (
	"context"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/bots"
)

type memohRuntime struct {
	agent *agentpkg.Agent
}

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

func (m memohRuntime) IdleTimeout() time.Duration { return 0 }
