package orchestrator

import (
	"context"
	"strings"
	"time"
)

const RulePlannerVersion = "rule-planner/v1"

// RulePlanner is a deterministic baseline planner. It gives AgentHub a stable
// P0 flow without depending on an LLM planner; a future LLM planner can replace
// this interface without changing dispatcher/collector logic.
type RulePlanner struct {
	DefaultTimeout    time.Duration
	DefaultMaxRetries int
}

func NewRulePlanner() *RulePlanner {
	return &RulePlanner{DefaultTimeout: 10 * time.Minute, DefaultMaxRetries: 1}
}

func (p *RulePlanner) Plan(_ context.Context, input PlanInput) (Plan, error) {
	objective := strings.TrimSpace(input.Objective)
	if objective == "" || strings.TrimSpace(input.RoomID) == "" {
		return Plan{}, ErrInvalidInput
	}
	timeout := p.DefaultTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	maxRetries := p.DefaultMaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	analysis := p.pickAgent(input.Agents, "plan", "analysis", "orchestrate")
	backend := p.pickAgent(input.Agents, "backend", "api", "server", "codex", "code")
	frontend := p.pickAgent(input.Agents, "frontend", "ui", "web", "claude", "code")
	qa := p.pickAgent(input.Agents, "test", "qa", "review")
	merge := analysis
	if merge.ID == "" {
		merge = qa
	}

	drafts := []TaskDraft{
		{
			ClientKey:       "plan",
			Title:           "理解目标并形成执行计划",
			Description:     "梳理用户目标、约束、交付物和验收标准。目标：" + objective,
			AssignedAgentID: analysis.ID,
			ProviderName:    analysis.ProviderName,
			Priority:        100,
			Timeout:         timeout,
			MaxRetries:      maxRetries,
			Metadata:        map[string]any{"phase": "planning"},
		},
		{
			ClientKey:       "backend",
			Title:           "实现后端/接口相关工作",
			Description:     "基于计划处理服务端、数据模型、接口或执行逻辑相关事项。",
			AssignedAgentID: backend.ID,
			ProviderName:    backend.ProviderName,
			Priority:        80,
			Timeout:         timeout,
			MaxRetries:      maxRetries,
			DependsOn:       []string{"plan"},
			Metadata:        map[string]any{"phase": "implementation", "track": "backend"},
		},
		{
			ClientKey:       "frontend",
			Title:           "实现前端/交互相关工作",
			Description:     "基于计划处理页面、消息卡片、交互或展示相关事项。",
			AssignedAgentID: frontend.ID,
			ProviderName:    frontend.ProviderName,
			Priority:        80,
			Timeout:         timeout,
			MaxRetries:      maxRetries,
			DependsOn:       []string{"plan"},
			Metadata:        map[string]any{"phase": "implementation", "track": "frontend"},
		},
		{
			ClientKey:       "verify",
			Title:           "验证、测试并汇总风险",
			Description:     "检查实现结果，运行可用验证，输出失败原因、风险和下一步建议。",
			AssignedAgentID: qa.ID,
			ProviderName:    qa.ProviderName,
			Priority:        60,
			Timeout:         timeout,
			MaxRetries:      maxRetries,
			DependsOn:       []string{"backend", "frontend"},
			Metadata:        map[string]any{"phase": "verification"},
		},
		{
			ClientKey:       "finalize",
			Title:           "回收结果并生成最终交付说明",
			Description:     "综合所有任务输出，形成面向用户的最终说明、产物列表和未解决事项。",
			AssignedAgentID: merge.ID,
			ProviderName:    merge.ProviderName,
			Priority:        40,
			Timeout:         timeout,
			MaxRetries:      maxRetries,
			DependsOn:       []string{"verify"},
			Metadata:        map[string]any{"phase": "finalize"},
		},
	}
	return Plan{PlannerVersion: RulePlannerVersion, Tasks: drafts}, nil
}

func (*RulePlanner) pickAgent(agents []AgentDescriptor, keywords ...string) AgentDescriptor {
	for _, agent := range agents {
		haystack := strings.ToLower(strings.Join(append([]string{agent.ID, agent.Name, agent.ProviderName}, agent.Capabilities...), " "))
		for _, keyword := range keywords {
			if strings.Contains(haystack, strings.ToLower(keyword)) {
				return agent
			}
		}
	}
	if len(agents) > 0 {
		return agents[0]
	}
	return AgentDescriptor{ID: "orchestrator", ProviderName: "noop"}
}
