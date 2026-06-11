package agenthub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	orch "github.com/ZihengXiong/GenMult/internal/agenthub/orchestrator"
)

const llmPlannerVersion = "llm-planner/v1"

// plannerResolveRetryInterval rate-limits how often the lazy planner re-probes
// the DB for a usable chat model while none is configured.
const plannerResolveRetryInterval = time.Minute

// lazyPlanner resolves the LLM planner on demand instead of once at process
// start, so a user who configures their first chat model (or fixes provider
// credentials) after boot gets LLM planning on their next run without a server
// restart. While no model is configured it serves the deterministic fallback
// and re-probes at most once per retryInterval; once resolution succeeds the
// planner is fixed for the process lifetime (same as the old eager behavior).
type lazyPlanner struct {
	mu            sync.Mutex
	resolve       func(ctx context.Context) orch.Planner
	fallback      orch.Planner
	resolved      orch.Planner
	retryInterval time.Duration
	lastAttempt   time.Time
	now           func() time.Time
	log           *slog.Logger
}

func newLazyPlanner(resolve func(ctx context.Context) orch.Planner, fallback orch.Planner, retryInterval time.Duration, log *slog.Logger) *lazyPlanner {
	if fallback == nil {
		fallback = orch.NewRulePlanner()
	}
	if retryInterval <= 0 {
		retryInterval = plannerResolveRetryInterval
	}
	if log == nil {
		log = slog.Default()
	}
	return &lazyPlanner{
		resolve:       resolve,
		fallback:      fallback,
		retryInterval: retryInterval,
		now:           time.Now,
		log:           log.With(slog.String("component", "agenthub_lazy_planner")),
	}
}

func (p *lazyPlanner) Plan(ctx context.Context, input orch.PlanInput) (orch.Plan, error) {
	return p.current(ctx).Plan(ctx, input)
}

// current returns the resolved LLM planner, attempting (rate-limited)
// resolution first, and the fallback while none is available. The mutex stays
// held across resolve — a cheap DB lookup — so concurrent runs never resolve
// twice.
func (p *lazyPlanner) current(ctx context.Context) orch.Planner {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.resolved != nil {
		return p.resolved
	}
	if p.resolve == nil {
		return p.fallback
	}
	now := p.now()
	if !p.lastAttempt.IsZero() && now.Sub(p.lastAttempt) < p.retryInterval {
		return p.fallback
	}
	p.lastAttempt = now
	if planner := p.resolve(ctx); planner != nil {
		p.resolved = planner
		p.log.Info("agenthub planner: LLM planner enabled (chat model resolved)")
		return p.resolved
	}
	p.log.Info("agenthub planner: no usable chat model yet, using rule planner",
		slog.Duration("next_probe_in", p.retryInterval))
	return p.fallback
}

// llmPlanner implements orchestrator.Planner by asking an LLM to decompose the
// objective into a task DAG over the room's available agents. Any failure (no
// model, call error, unparseable/invalid/cyclic output) falls back to the
// deterministic RulePlanner so a run is always plannable.
type llmPlanner struct {
	model    *sdk.Model
	fallback orch.Planner
	timeout  time.Duration
	log      *slog.Logger
}

func newLLMPlanner(model *sdk.Model, fallback orch.Planner, log *slog.Logger) *llmPlanner {
	if log == nil {
		log = slog.Default()
	}
	if fallback == nil {
		fallback = orch.NewRulePlanner()
	}
	return &llmPlanner{
		model:    model,
		fallback: fallback,
		timeout:  45 * time.Second,
		log:      log.With(slog.String("component", "agenthub_llm_planner")),
	}
}

func (p *llmPlanner) Plan(ctx context.Context, input orch.PlanInput) (orch.Plan, error) {
	objective := strings.TrimSpace(input.Objective)
	if objective == "" || strings.TrimSpace(input.RoomID) == "" {
		return orch.Plan{}, orch.ErrInvalidInput
	}
	if p.model == nil {
		return p.fallback.Plan(ctx, input)
	}
	// When the user explicitly @-mentions specific agents they've chosen *who*
	// participates, but for a big objective ("做个待办: 写前端, cc 写后端") the main
	// agent should still split it into a tailored sub-prompt per mentioned agent
	// rather than passing the raw objective verbatim to each. So we narrow the
	// candidate agents to the mentioned subset and let the LLM plan over them; the
	// rule planner's (parallel) directMentionPlan stays as the fallback below.
	planAgents := input.Agents
	mentionConstraint := ""
	if mentioned := orch.MentionedAgents(objective, input.Agents); len(mentioned) > 0 {
		planAgents = mentioned
		mentionConstraint = buildMentionConstraint(mentioned)
		p.log.Info("llm planner: explicit @mention present, planning over mentioned subset",
			slog.String("room_id", input.RoomID), slog.Int("mentioned", len(mentioned)))
	}

	cctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	raw, err := sdk.GenerateText(cctx,
		sdk.WithModel(p.model),
		sdk.WithSystem(plannerSystemPrompt+mentionConstraint),
		sdk.WithMessages([]sdk.Message{sdk.UserMessage(buildPlannerUserPrompt(objective, planAgents, input.Metadata))}),
		sdk.WithMaxTokens(2048),
		sdk.WithTemperature(0.2),
	)
	if err != nil {
		p.log.Warn("llm planner call failed; falling back to rule planner",
			slog.String("room_id", input.RoomID), slog.Any("error", err))
		return p.fallback.Plan(ctx, input)
	}

	plan, perr := parsePlannerOutput(raw, planAgents)
	if perr != nil || len(plan.Tasks) == 0 {
		p.log.Warn("llm planner output invalid; falling back to rule planner",
			slog.String("room_id", input.RoomID), slog.Any("error", perr))
		return p.fallback.Plan(ctx, input)
	}

	p.log.Info("llm planner produced a plan",
		slog.String("room_id", input.RoomID), slog.Int("tasks", len(plan.Tasks)))
	return plan, nil
}

const plannerSystemPrompt = `你是 AgentHub 的主控编排器（Orchestrator）。把用户目标拆解成可分派给子 Agent 的任务有向无环图（DAG）。
严格只输出一个 JSON 对象，形如 {"tasks":[{"key":"","title":"","description":"","agent_id":"","depends_on":[]}]}，不要输出任何额外文字或 markdown 代码块。
字段说明：
- key：任务唯一短标识（英文/数字）。
- title：简短中文标题。
- description：给执行 Agent 的清晰、可独立执行的中文指令。
- agent_id：必须从"可用 Agent"列表的 id 中选择最合适的一个。
- depends_on：该任务依赖的其它任务 key 数组，无依赖则为空数组。
规则：按能力把任务分给最合适的 Agent；可并行的任务不要相互依赖；任务数量与目标复杂度匹配（简单目标 1-2 个，复杂目标可到 5-6 个），不要为简单目标制造无谓步骤；至少有一个无依赖的起始任务；依赖关系不能成环。`

func buildPlannerUserPrompt(objective string, agents []orch.AgentDescriptor, metadata map[string]any) string {
	var b strings.Builder
	if hist, ok := metadata["room_history"].(string); ok {
		if h := strings.TrimSpace(hist); h != "" {
			b.WriteString("群聊近期对话（用于理解意图）：\n")
			b.WriteString(h)
			b.WriteString("\n\n")
		}
	}
	b.WriteString("用户目标：\n")
	b.WriteString(objective)
	b.WriteString("\n\n可用 Agent：\n")
	if len(agents) == 0 {
		b.WriteString("- id=orchestrator 名称=Orchestrator 能力=任务拆解、调度\n")
	}
	for _, a := range agents {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = a.ID
		}
		caps := strings.Join(a.Capabilities, "、")
		fmt.Fprintf(&b, "- id=%s 名称=%s 能力=%s\n", a.ID, name, caps)
	}
	b.WriteString("\n只输出 JSON。")
	return b.String()
}

type plannerTaskJSON struct {
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	AgentID     string   `json:"agent_id"`
	DependsOn   []string `json:"depends_on"`
}

type plannerOutputJSON struct {
	Tasks []plannerTaskJSON `json:"tasks"`
}

// parsePlannerOutput converts raw LLM text into a validated orchestrator.Plan.
// It is defensive: strips markdown fences/prose, coerces unknown agent ids to a
// default room agent, drops dangling dependency references, and rejects cyclic
// graphs (returning an error so the caller falls back to the rule planner).
func parsePlannerOutput(raw string, agents []orch.AgentDescriptor) (orch.Plan, error) {
	jsonText := extractJSONObject(raw)
	if jsonText == "" {
		return orch.Plan{}, errors.New("no JSON object found in planner output")
	}
	var out plannerOutputJSON
	if err := json.Unmarshal([]byte(jsonText), &out); err != nil {
		return orch.Plan{}, fmt.Errorf("unmarshal planner output: %w", err)
	}
	if len(out.Tasks) == 0 {
		return orch.Plan{}, errors.New("planner produced no tasks")
	}

	providerByID := make(map[string]string, len(agents))
	for _, a := range agents {
		providerByID[a.ID] = a.ProviderName
	}
	defaultAgentID, defaultProvider := "orchestrator", "noop"
	if len(agents) > 0 {
		defaultAgentID = agents[0].ID
		defaultProvider = agents[0].ProviderName
	}

	drafts := make([]orch.TaskDraft, 0, len(out.Tasks))
	seen := make(map[string]struct{}, len(out.Tasks))
	for i, t := range out.Tasks {
		key := strings.TrimSpace(t.Key)
		if key == "" {
			key = fmt.Sprintf("task-%d", i+1)
		}
		if _, dup := seen[key]; dup {
			key = fmt.Sprintf("%s-%d", key, i+1)
		}
		seen[key] = struct{}{}

		title := strings.TrimSpace(t.Title)
		if title == "" {
			title = key
		}
		agentID := strings.TrimSpace(t.AgentID)
		provider, ok := providerByID[agentID]
		if !ok || agentID == "" {
			agentID, provider = defaultAgentID, defaultProvider
		}

		drafts = append(drafts, orch.TaskDraft{
			ClientKey:       key,
			Title:           title,
			Description:     strings.TrimSpace(t.Description),
			AssignedAgentID: agentID,
			ProviderName:    provider,
			Priority:        100 - i,
			Timeout:         10 * time.Minute,
			MaxRetries:      1,
			DependsOn:       t.DependsOn,
			Metadata:        map[string]any{"planner": "llm"},
		})
	}

	// Keep only dependency references that point at real task keys (and never at
	// self), so the resulting plan always passes orchestrator.validatePlan.
	validKeys := make(map[string]struct{}, len(drafts))
	for _, d := range drafts {
		validKeys[d.ClientKey] = struct{}{}
	}
	for i := range drafts {
		cleaned := make([]string, 0, len(drafts[i].DependsOn))
		for _, dep := range drafts[i].DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" || dep == drafts[i].ClientKey {
				continue
			}
			if _, ok := validKeys[dep]; ok {
				cleaned = append(cleaned, dep)
			}
		}
		drafts[i].DependsOn = cleaned
	}

	if hasDependencyCycle(drafts) {
		return orch.Plan{}, errors.New("planner produced a cyclic task graph")
	}

	return orch.Plan{PlannerVersion: llmPlannerVersion, Tasks: drafts}, nil
}

// extractJSONObject pulls the first {...} JSON object out of an LLM response,
// tolerating leading prose and ```json fences.
func extractJSONObject(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
	}
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end < 0 || end < start {
		return ""
	}
	return s[start : end+1]
}

// hasDependencyCycle reports whether the task DAG drafts contain a cycle, via
// Kahn's algorithm over edges dep -> task.
func hasDependencyCycle(drafts []orch.TaskDraft) bool {
	indeg := make(map[string]int, len(drafts))
	adj := make(map[string][]string, len(drafts))
	for _, d := range drafts {
		if _, ok := indeg[d.ClientKey]; !ok {
			indeg[d.ClientKey] = 0
		}
	}
	for _, d := range drafts {
		for _, dep := range d.DependsOn {
			adj[dep] = append(adj[dep], d.ClientKey)
			indeg[d.ClientKey]++
		}
	}
	queue := make([]string, 0, len(indeg))
	for k, v := range indeg {
		if v == 0 {
			queue = append(queue, k)
		}
	}
	visited := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		visited++
		for _, m := range adj[n] {
			indeg[m]--
			if indeg[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	return visited != len(indeg)
}

// buildMentionConstraint appends an instruction to the planner system prompt so
// the LLM only assigns work to the user-mentioned agents and writes an
// independent, parallelizable sub-prompt for each.
func buildMentionConstraint(mentioned []orch.AgentDescriptor) string {
	names := make([]string, 0, len(mentioned))
	for _, a := range mentioned {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = a.ID
		}
		names = append(names, fmt.Sprintf("%s(id=%s)", name, a.ID))
	}
	return "\n\n约束：用户已用 @ 显式点名以下 Agent，只能把任务分配给他们（agent_id 必须取自此列表）：" +
		strings.Join(names, "、") +
		"。为每个被点名的 Agent 写清晰的子任务指令。依赖关系按语义判断：" +
		"如果某个 Agent 的任务需要用到另一个 Agent 的产出（例如转述、基于、汇总、审阅其回答/代码），" +
		"必须用 depends_on 表达这种依赖，以保证下游能拿到上游的输出；只有彼此真正独立时（如分别写前端/后端）才不加依赖、让它们并行。"
}
