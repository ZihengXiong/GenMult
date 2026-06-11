package orchestrator

import (
	"context"
	"fmt"
	"sort"
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
	// 10min/1-retry mirrors the LLM planner (llm_planner.go parsePlannerOutput):
	// a real agent "写前端/写后端" cannot finish in 90s, and a single retry rides
	// out transient model timeouts without cascading the whole run to failure.
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

	if direct := p.directMentionPlan(objective, input.Agents, timeout, maxRetries); len(direct.Tasks) > 0 {
		return direct, nil
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

func (*RulePlanner) directMentionPlan(objective string, agents []AgentDescriptor, timeout time.Duration, maxRetries int) Plan {
	if len(agents) == 0 {
		return Plan{}
	}
	type mentionedAgent struct {
		agent AgentDescriptor
		at    int
	}
	mentioned := make([]mentionedAgent, 0, len(agents))
	lowerObjective := strings.ToLower(objective)
	for _, agent := range agents {
		if idx := firstMentionIndex(lowerObjective, agent); idx >= 0 {
			mentioned = append(mentioned, mentionedAgent{agent: agent, at: idx})
		}
	}
	if len(mentioned) == 0 && len(agents) == 1 {
		mentioned = append(mentioned, mentionedAgent{agent: agents[0], at: 0})
	}
	if len(mentioned) == 0 {
		return Plan{}
	}
	sort.SliceStable(mentioned, func(i, j int) bool {
		return mentioned[i].at < mentioned[j].at
	})
	drafts := make([]TaskDraft, 0, len(mentioned))
	for i, item := range mentioned {
		name := firstNonEmpty(strings.TrimSpace(item.agent.Name), strings.TrimSpace(item.agent.ProviderName), strings.TrimSpace(item.agent.ID))
		title := "执行用户指定任务"
		if len(mentioned) > 1 {
			title = fmt.Sprintf("%s 执行分配任务", name)
		}
		// Mentioned agents run independently in parallel: "你写前端, cc 写后端" are
		// separate jobs, so one agent failing/timing out must not block the others.
		// (No DependsOn chain — that previously cascaded a single timeout into a
		// whole-run failure.)
		draft := TaskDraft{
			ClientKey:       fmt.Sprintf("direct-%d", i+1),
			Title:           title,
			Description:     objective,
			AssignedAgentID: item.agent.ID,
			ProviderName:    item.agent.ProviderName,
			Priority:        100 - i,
			Timeout:         timeout,
			MaxRetries:      maxRetries,
			Metadata:        map[string]any{"phase": "direct"},
		}
		drafts = append(drafts, draft)
	}
	return Plan{PlannerVersion: RulePlannerVersion, Tasks: drafts}
}

// MentionedAgents returns the subset of agents the objective explicitly
// @-mentions, preserving input order; nil when the objective contains no "@" so
// callers can plan over all room agents. It shares the rule planner's
// word-boundary-aware alias matching (firstMentionIndex), so e.g. "@ds2" never
// counts as a mention of an agent aliased "ds".
func MentionedAgents(objective string, agents []AgentDescriptor) []AgentDescriptor {
	lower := strings.ToLower(objective)
	if !strings.Contains(lower, "@") {
		return nil
	}
	out := make([]AgentDescriptor, 0, len(agents))
	for _, agent := range agents {
		if firstMentionIndex(lower, agent) >= 0 {
			out = append(out, agent)
		}
	}
	return out
}

func firstMentionIndex(lowerObjective string, agent AgentDescriptor) int {
	best := -1
	for _, alias := range agentMentionAliases(agent) {
		needle := "@" + strings.ToLower(alias)
		start := 0
		for {
			idx := strings.Index(lowerObjective[start:], needle)
			if idx < 0 {
				break
			}
			idx += start
			end := idx + len(needle)
			if end >= len(lowerObjective) || !isMentionChar(rune(lowerObjective[end])) {
				if best == -1 || idx < best {
					best = idx
				}
				break
			}
			start = end
		}
	}
	return best
}

func isMentionChar(r rune) bool {
	return r == '-' || r == '_' || r == '.' || r == ':' || r == '/' || r == '@' ||
		(r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func agentMentionAliases(agent AgentDescriptor) []string {
	seen := map[string]struct{}{}
	aliases := make([]string, 0, 6)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		lower := strings.ToLower(value)
		if _, ok := seen[lower]; ok {
			return
		}
		seen[lower] = struct{}{}
		aliases = append(aliases, value)
	}
	name := strings.TrimSpace(agent.Name)
	add(name)
	add(strings.TrimPrefix(agent.ID, "bot:"))
	add(agent.ID)
	add(agent.ProviderName)
	switch strings.ToLower(strings.TrimSpace(agent.ProviderName)) {
	case "memoh":
		if name == "" {
			add("ds")
		}
	case "claudecode":
		if name == "" {
			add("cc")
			add("claude")
			add("claude code")
		}
	case "codex":
		if name == "" {
			add("cx")
		}
	}
	return aliases
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
