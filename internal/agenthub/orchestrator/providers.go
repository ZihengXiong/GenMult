package orchestrator

import (
	"context"
	"strings"
	"sync"
)

// ProviderRegistry resolves provider names used by planned tasks.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]AgentProvider
	fallback  AgentProvider
}

func NewProviderRegistry(providers ...AgentProvider) *ProviderRegistry {
	r := &ProviderRegistry{providers: make(map[string]AgentProvider)}
	for _, provider := range providers {
		r.Register(provider)
	}
	return r
}

func (r *ProviderRegistry) Register(provider AgentProvider) {
	if provider == nil || strings.TrimSpace(provider.Name()) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	name := strings.TrimSpace(provider.Name())
	r.providers[name] = provider
	if r.fallback == nil {
		r.fallback = provider
	}
}

func (r *ProviderRegistry) Resolve(name string) (AgentProvider, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if provider, ok := r.providers[strings.TrimSpace(name)]; ok {
		return provider, true
	}
	if r.fallback != nil && strings.TrimSpace(name) == "" {
		return r.fallback, true
	}
	return nil, false
}

// NoopProvider is useful for local demos and tests. It marks work successful
// without touching external systems.
type NoopProvider struct{}

func (NoopProvider) Name() string { return "noop" }

func (NoopProvider) Capabilities() []string { return []string{"plan", "code", "test", "review"} }

func (NoopProvider) Execute(_ context.Context, req ExecuteTaskRequest) (ExecuteTaskResult, error) {
	return ExecuteTaskResult{
		Output: map[string]any{
			"summary":  "completed by noop provider",
			"task_id":  req.Task.ID,
			"task":     req.Task.Title,
			"attempt":  req.AttemptNo,
			"provider": NoopProvider{}.Name(),
		},
		Summary:   "completed by noop provider",
		Retryable: false,
	}, nil
}
