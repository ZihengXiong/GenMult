package agenthub

import (
	"context"
	"testing"

	orch "github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

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
