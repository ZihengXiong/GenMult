package agenthub

// Live tests that exercise the LLM planner and the orchestration engine against
// a real chat-completions API (DeepSeek / OpenAI compatible). They cost real
// tokens, so they are double-gated: LIVE_LLM_TEST=1 must be set explicitly AND
// credentials must be present in the environment — `go test ./...` (CI, husky
// pre-commit) skips them. Run with:
//
//	set -a; source .env; set +a
//	LIVE_LLM_TEST=1 go test ./internal/agenthub/ -run TestLive -v -count=1
//
// Keep the model a cheap flash/nano tier (LIVE_LLM_MODEL or MODEL_NAME).

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orch "github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
	"github.com/ZihengXiong/GenMult/internal/agenthub/providers"
	"github.com/ZihengXiong/GenMult/internal/models"
)

// liveModel resolves credentials/model from the environment (mirroring the
// repo's .env contract) and skips the test when live testing is not enabled.
func liveModel(t *testing.T) *sdk.Model {
	t.Helper()
	if os.Getenv("LIVE_LLM_TEST") == "" {
		t.Skip("live LLM test skipped: set LIVE_LLM_TEST=1 (calls a real API and costs tokens)")
	}
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		t.Skip("live LLM test skipped: no DEEPSEEK_API_KEY / OPENAI_API_KEY in environment")
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		if ds := os.Getenv("DEEPSEEK_API_URL"); ds != "" {
			baseURL = strings.TrimRight(ds, "/") + "/v1"
		}
	}
	modelID := os.Getenv("LIVE_LLM_MODEL")
	if modelID == "" {
		modelID = os.Getenv("MODEL_NAME")
	}
	if modelID == "" {
		// Cheap flash-tier default, matching the repo .env's MODEL_NAME.
		modelID = "deepseek-v4-flash"
	}
	t.Logf("live LLM: model=%s base_url=%s", modelID, baseURL)
	return models.NewSDKChatModel(models.SDKModelConfig{
		ModelID:    modelID,
		ClientType: string(models.ClientTypeOpenAICompletions),
		APIKey:     apiKey,
		BaseURL:    baseURL,
	})
}

func liveCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestLiveModelSanity is the cheapest possible credential / endpoint check, so
// a misconfigured key fails here with a clear error instead of inside the
// planner's silent rule-planner fallback.
func TestLiveModelSanity(t *testing.T) {
	model := liveModel(t)
	out, err := sdk.GenerateText(liveCtx(t),
		sdk.WithModel(model),
		sdk.WithMessages([]sdk.Message{sdk.UserMessage("只回复两个字母：OK")}),
		sdk.WithMaxTokens(16),
	)
	require.NoError(t, err)
	require.NotEmpty(t, strings.TrimSpace(out))
	t.Logf("model replied: %q", out)
}

// TestLiveLLMPlannerProducesDAG verifies the real model, under the planner
// system prompt, produces a parseable DAG: tasks assigned only to roster
// agents, with the sequential objective expressed as a dependency.
func TestLiveLLMPlannerProducesDAG(t *testing.T) {
	model := liveModel(t)
	p := newLLMPlanner(model, orch.NewRulePlanner(), slog.Default())
	agents := []orch.AgentDescriptor{
		{ID: "bot:researcher", ProviderName: "stub", Name: "调研员", Capabilities: []string{"调研", "analysis"}},
		{ID: "bot:writer", ProviderName: "stub", Name: "写作者", Capabilities: []string{"写作", "总结"}},
	}

	plan, err := p.Plan(liveCtx(t), orch.PlanInput{
		RoomID:    "room-live-dag",
		Objective: "先调研三种常见的 Go 并发模式并整理要点，然后基于调研要点写一篇简短的中文总结",
		Agents:    agents,
	})
	require.NoError(t, err)
	require.Equal(t, llmPlannerVersion, plan.PlannerVersion,
		"plan must come from the LLM planner, not the rule-planner fallback (check API errors in the log)")
	require.NotEmpty(t, plan.Tasks)

	validAgent := map[string]bool{"bot:researcher": true, "bot:writer": true}
	hasDep := false
	for _, task := range plan.Tasks {
		assert.True(t, validAgent[task.AssignedAgentID], "task %q assigned to unknown agent %q", task.ClientKey, task.AssignedAgentID)
		assert.NotEmpty(t, strings.TrimSpace(task.Description), "task %q must carry an executable instruction", task.ClientKey)
		if len(task.DependsOn) > 0 {
			hasDep = true
		}
		t.Logf("task key=%s agent=%s deps=%v title=%s", task.ClientKey, task.AssignedAgentID, task.DependsOn, task.Title)
	}
	assert.True(t, hasDep, "“先调研、然后总结” must yield at least one dependency edge")
}

// TestLiveLLMPlannerMentionSubset verifies that when the user @-mentions
// specific agents, the live planner assigns work to exactly that subset (the
// mention constraint added to the system prompt).
func TestLiveLLMPlannerMentionSubset(t *testing.T) {
	model := liveModel(t)
	p := newLLMPlanner(model, orch.NewRulePlanner(), slog.Default())
	agents := []orch.AgentDescriptor{
		{ID: "bot:qianduan", ProviderName: "stub", Name: "前端", Capabilities: []string{"前端", "code"}},
		{ID: "bot:houduan", ProviderName: "stub", Name: "后端", Capabilities: []string{"后端", "code"}},
		{ID: "bot:wendang", ProviderName: "stub", Name: "文档", Capabilities: []string{"写作"}},
	}

	plan, err := p.Plan(liveCtx(t), orch.PlanInput{
		RoomID:    "room-live-mention",
		Objective: "@qianduan 实现登录页面的 UI，@houduan 实现登录接口",
		Agents:    agents,
	})
	require.NoError(t, err)
	require.Equal(t, llmPlannerVersion, plan.PlannerVersion,
		"plan must come from the LLM planner, not the rule-planner fallback (check API errors in the log)")
	require.NotEmpty(t, plan.Tasks)

	assigned := map[string]int{}
	for _, task := range plan.Tasks {
		assigned[task.AssignedAgentID]++
		assert.NotEqual(t, "bot:wendang", task.AssignedAgentID, "un-mentioned agent must not receive tasks")
		t.Logf("task key=%s agent=%s deps=%v title=%s", task.ClientKey, task.AssignedAgentID, task.DependsOn, task.Title)
	}
	assert.Positive(t, assigned["bot:qianduan"], "mentioned 前端 agent must receive at least one task")
	assert.Positive(t, assigned["bot:houduan"], "mentioned 后端 agent must receive at least one task")
}

// liveStubProvider executes planned tasks instantly while recording the exact
// prompt each agent would receive (via the shared PromptWithContext path), so
// the end-to-end test can verify upstream output really reaches dependents.
type liveStubProvider struct {
	mu      sync.Mutex
	prompts map[string]string // task ID -> rendered prompt
}

func (*liveStubProvider) Name() string { return "stub" }

func (*liveStubProvider) Capabilities() []string { return []string{"plan", "code", "调研", "写作"} }

func (p *liveStubProvider) Execute(_ context.Context, req orch.ExecuteTaskRequest) (orch.ExecuteTaskResult, error) {
	prompt := providers.PromptWithContext(req)
	p.mu.Lock()
	p.prompts[req.Task.ID] = prompt
	p.mu.Unlock()
	out := "【" + req.Task.AssignedAgentID + "】已完成任务：" + req.Task.Title
	return orch.ExecuteTaskResult{Output: map[string]any{"raw_output": out}, Summary: "stub done"}, nil
}

// TestLiveOrchestratorRunEndToEnd drives the full engine — live LLM planning,
// task/dependency persistence, dispatch ordering, upstream-output injection —
// with only the per-task execution stubbed out.
func TestLiveOrchestratorRunEndToEnd(t *testing.T) {
	model := liveModel(t)
	stub := &liveStubProvider{prompts: map[string]string{}}
	engine := orch.NewService(
		orch.NewMemoryStore(),
		newLLMPlanner(model, orch.NewRulePlanner(), slog.Default()),
		orch.NewProviderRegistry(stub),
		slog.New(slog.DiscardHandler),
		orch.Config{DispatchAsync: false},
	)

	agents := []orch.AgentDescriptor{
		{ID: "bot:researcher", ProviderName: "stub", Name: "调研员", Capabilities: []string{"调研", "analysis"}},
		{ID: "bot:writer", ProviderName: "stub", Name: "写作者", Capabilities: []string{"写作", "总结"}},
	}
	snap, err := engine.StartRun(liveCtx(t), orch.StartRunInput{
		RoomID:       "room-live-e2e",
		Objective:    "先调研三种常见的 Go 并发模式并整理要点，然后基于调研要点写一篇简短的中文总结",
		CreatedBy:    "live-test",
		Agents:       agents,
		AutoDispatch: true,
	})
	require.NoError(t, err)
	require.Equal(t, orch.RunStatusCompleted, snap.Run.Status, "synchronous run must complete")
	require.Equal(t, llmPlannerVersion, snap.Run.PlannerVersion,
		"run must be planned by the LLM planner, not the rule-planner fallback")
	require.NotEmpty(t, snap.Tasks)
	for _, task := range snap.Tasks {
		assert.Equal(t, orch.TaskStatusSucceeded, task.Status, "task %q must succeed", task.Title)
	}

	// Every dependent task's prompt must embed its upstream agent's output.
	require.NotEmpty(t, snap.Dependencies, "the sequential objective must produce a dependency edge")
	for _, dep := range snap.Dependencies {
		prompt := stub.prompts[dep.TaskID]
		require.NotEmpty(t, prompt, "dependent task %s was never executed", dep.TaskID)
		assert.Contains(t, prompt, "上游协作 Agent 的产出", "dependent task prompt must carry the upstream block")
		assert.Contains(t, prompt, "已完成任务", "dependent task prompt must contain the upstream stub output")
	}
	t.Logf("run completed: %d tasks, %d dependencies", len(snap.Tasks), len(snap.Dependencies))
}
