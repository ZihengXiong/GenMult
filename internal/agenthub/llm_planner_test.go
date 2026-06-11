package agenthub

import (
	"context"
	"strings"
	"testing"
	"time"

	orch "github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

func TestBuildPlannerUserPrompt_IncludesHistory(t *testing.T) {
	p := buildPlannerUserPrompt("做个功能", plannerAgents(), map[string]any{"room_history": "我：先讨论一下"})
	if !strings.Contains(p, "先讨论一下") || !strings.Contains(p, "做个功能") || !strings.Contains(p, "id=codex") {
		t.Errorf("prompt missing parts: %q", p)
	}
	if p2 := buildPlannerUserPrompt("x", plannerAgents(), nil); strings.Contains(p2, "群聊近期对话") {
		t.Errorf("unexpected history header without metadata: %q", p2)
	}
}

func plannerAgents() []orch.AgentDescriptor {
	return []orch.AgentDescriptor{
		{ID: "claude-code", ProviderName: "claudecode", Name: "Claude Code"},
		{ID: "codex", ProviderName: "codex", Name: "Codex"},
	}
}

func draftByKey(plan orch.Plan, key string) (orch.TaskDraft, bool) {
	for _, d := range plan.Tasks {
		if d.ClientKey == key {
			return d, true
		}
	}
	return orch.TaskDraft{}, false
}

func TestParsePlannerOutput_Valid(t *testing.T) {
	raw := `{"tasks":[
		{"key":"be","title":"后端","description":"实现接口","agent_id":"codex","depends_on":[]},
		{"key":"fe","title":"前端","description":"实现页面","agent_id":"claude-code","depends_on":["be"]}
	]}`
	plan, err := parsePlannerOutput(raw, plannerAgents())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if plan.PlannerVersion != llmPlannerVersion {
		t.Errorf("planner version: %s", plan.PlannerVersion)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(plan.Tasks))
	}
	be, ok := draftByKey(plan, "be")
	if !ok || be.ProviderName != "codex" || be.AssignedAgentID != "codex" {
		t.Errorf("be task wrong: %+v", be)
	}
	fe, ok := draftByKey(plan, "fe")
	if !ok || fe.ProviderName != "claudecode" || len(fe.DependsOn) != 1 || fe.DependsOn[0] != "be" {
		t.Errorf("fe task wrong: %+v", fe)
	}
}

func TestParsePlannerOutput_FencedWithProse(t *testing.T) {
	raw := "好的，这是计划：\n```json\n{\"tasks\":[{\"key\":\"t1\",\"title\":\"做事\",\"description\":\"x\",\"agent_id\":\"codex\"}]}\n```\n"
	plan, err := parsePlannerOutput(raw, plannerAgents())
	if err != nil {
		t.Fatalf("parse fenced: %v", err)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].ClientKey != "t1" {
		t.Errorf("unexpected plan: %+v", plan.Tasks)
	}
}

func TestParsePlannerOutput_UnknownAgentCoerced(t *testing.T) {
	raw := `{"tasks":[{"key":"t1","title":"做事","description":"x","agent_id":"ghost"}]}`
	plan, err := parsePlannerOutput(raw, plannerAgents())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// unknown agent id coerces to the first room agent
	if plan.Tasks[0].AssignedAgentID != "claude-code" || plan.Tasks[0].ProviderName != "claudecode" {
		t.Errorf("expected coercion to first agent, got %+v", plan.Tasks[0])
	}
}

func TestParsePlannerOutput_DropsDanglingDeps(t *testing.T) {
	raw := `{"tasks":[{"key":"t1","title":"a","description":"x","agent_id":"codex","depends_on":["nope","t1"]}]}`
	plan, err := parsePlannerOutput(raw, plannerAgents())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(plan.Tasks[0].DependsOn) != 0 {
		t.Errorf("expected dangling/self deps dropped, got %+v", plan.Tasks[0].DependsOn)
	}
}

func TestParsePlannerOutput_CycleRejected(t *testing.T) {
	raw := `{"tasks":[
		{"key":"a","title":"a","description":"x","agent_id":"codex","depends_on":["b"]},
		{"key":"b","title":"b","description":"y","agent_id":"codex","depends_on":["a"]}
	]}`
	if _, err := parsePlannerOutput(raw, plannerAgents()); err == nil {
		t.Error("expected cyclic plan to be rejected")
	}
}

func TestParsePlannerOutput_NoJSON(t *testing.T) {
	if _, err := parsePlannerOutput("抱歉我无法完成", plannerAgents()); err == nil {
		t.Error("expected error when no JSON object present")
	}
}

func TestMentionedAgents_SubsetAndConstraint(t *testing.T) {
	agents := plannerAgents()
	// Only Claude Code is @-mentioned → subset of one, in input order.
	got := orch.MentionedAgents("做个待办: 写前端,@Claude Code 写后端", agents)
	if len(got) != 1 || got[0].ID != "claude-code" {
		t.Fatalf("expected only claude-code mentioned, got %+v", got)
	}
	// No '@' at all → nil so the caller plans over all room agents.
	if got := orch.MentionedAgents("帮我做个待办工具", agents); got != nil {
		t.Errorf("expected nil for no mention, got %+v", got)
	}
	// Both mentioned → both, order preserved.
	both := orch.MentionedAgents("@Claude Code 写前端，@Codex 写后端", agents)
	if len(both) != 2 || both[0].ID != "claude-code" || both[1].ID != "codex" {
		t.Fatalf("expected both agents in order, got %+v", both)
	}
	c := buildMentionConstraint(both)
	if !strings.Contains(c, "id=claude-code") || !strings.Contains(c, "id=codex") || !strings.Contains(c, "并行") {
		t.Errorf("constraint missing mentioned ids / parallel hint: %q", c)
	}
}

// TestMentionedAgents_PrefixCollision: an agent whose alias is a prefix of
// another's must not be counted when only the longer alias is mentioned.
// (Regression: the LLM planner used a bare strings.Contains, so "@ds2" also
// "mentioned" an agent aliased "ds".)
func TestMentionedAgents_PrefixCollision(t *testing.T) {
	agents := []orch.AgentDescriptor{
		{ID: "bot:ds", Name: "ds", ProviderName: "memoh"},
		{ID: "bot:ds2", Name: "ds2", ProviderName: "memoh"},
	}
	got := orch.MentionedAgents("@ds2 帮我总结这份文档", agents)
	if len(got) != 1 || got[0].ID != "bot:ds2" {
		t.Fatalf("expected only ds2 mentioned, got %+v", got)
	}
	// Mentioning the short alias must still work.
	short := orch.MentionedAgents("@ds 帮我总结这份文档", agents)
	if len(short) != 1 || short[0].ID != "bot:ds" {
		t.Fatalf("expected only ds mentioned, got %+v", short)
	}
}

func TestLLMPlanner_NilModelFallsBack(t *testing.T) {
	p := newLLMPlanner(nil, orch.NewRulePlanner(), nil)
	plan, err := p.Plan(context.Background(), orch.PlanInput{
		RoomID:    "room-1",
		Objective: "实现一个登录功能",
		Agents:    plannerAgents(),
	})
	if err != nil {
		t.Fatalf("nil-model plan: %v", err)
	}
	if plan.PlannerVersion != orch.RulePlannerVersion {
		t.Errorf("expected rule-planner fallback, got version %s", plan.PlannerVersion)
	}
	if len(plan.Tasks) == 0 {
		t.Error("expected fallback plan to have tasks")
	}
}

// sentinelPlanner lets lazy-planner tests observe which planner served a call.
type sentinelPlanner struct{ version string }

func (p sentinelPlanner) Plan(context.Context, orch.PlanInput) (orch.Plan, error) {
	return orch.Plan{PlannerVersion: p.version, Tasks: []orch.TaskDraft{{ClientKey: "t", Title: "t"}}}, nil
}

// TestLazyPlanner_UpgradesWhenModelAppears: rule fallback while no model is
// configured, rate-limited re-probing, upgrade on success, then sticky.
func TestLazyPlanner_UpgradesWhenModelAppears(t *testing.T) {
	resolveCalls := 0
	var available orch.Planner
	lazy := newLazyPlanner(func(context.Context) orch.Planner {
		resolveCalls++
		return available
	}, sentinelPlanner{version: "fallback"}, time.Minute, nil)

	now := time.Now()
	lazy.now = func() time.Time { return now }
	input := orch.PlanInput{RoomID: "r", Objective: "o"}

	// No model yet → fallback, one probe.
	plan, err := lazy.Plan(context.Background(), input)
	if err != nil || plan.PlannerVersion != "fallback" {
		t.Fatalf("expected fallback plan, got %v / %v", plan.PlannerVersion, err)
	}
	if resolveCalls != 1 {
		t.Fatalf("expected 1 resolve call, got %d", resolveCalls)
	}

	// Within the retry interval: no re-probe even across many calls.
	for i := 0; i < 5; i++ {
		if _, err := lazy.Plan(context.Background(), input); err != nil {
			t.Fatal(err)
		}
	}
	if resolveCalls != 1 {
		t.Fatalf("probing not rate-limited: %d resolve calls", resolveCalls)
	}

	// Model appears; after the interval the next Plan upgrades.
	available = sentinelPlanner{version: "llm"}
	now = now.Add(2 * time.Minute)
	plan, err = lazy.Plan(context.Background(), input)
	if err != nil || plan.PlannerVersion != "llm" {
		t.Fatalf("expected llm plan after upgrade, got %v / %v", plan.PlannerVersion, err)
	}
	if resolveCalls != 2 {
		t.Fatalf("expected 2 resolve calls, got %d", resolveCalls)
	}

	// Sticky: no further resolution attempts once resolved.
	available = nil
	now = now.Add(time.Hour)
	plan, _ = lazy.Plan(context.Background(), input)
	if plan.PlannerVersion != "llm" {
		t.Fatalf("resolved planner must be sticky, got %v", plan.PlannerVersion)
	}
	if resolveCalls != 2 {
		t.Fatalf("resolved planner must stop probing, got %d resolve calls", resolveCalls)
	}
}

// TestLazyPlanner_NilResolveServesFallback guards the degenerate wiring.
func TestLazyPlanner_NilResolveServesFallback(t *testing.T) {
	lazy := newLazyPlanner(nil, sentinelPlanner{version: "fallback"}, 0, nil)
	plan, err := lazy.Plan(context.Background(), orch.PlanInput{RoomID: "r", Objective: "o"})
	if err != nil || plan.PlannerVersion != "fallback" {
		t.Fatalf("expected fallback, got %v / %v", plan.PlannerVersion, err)
	}
}
