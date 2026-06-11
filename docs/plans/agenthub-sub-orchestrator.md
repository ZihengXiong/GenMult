# Plan: Sub-Orchestrator（嵌套编排 / 任务再分解）

> 状态：评估完成（2026-06-11），**暂不实现**——结论与触发条件见「执行决策」。
> 数据模型已就绪：`tasks.parent_task_id` / `TaskDraft.ParentClientKey` 已存在并持久化。

## 目标

让一个任务本身可以再分解为子 DAG（例如"做一个电商网站"→ 前端任务再拆为
登录页/商品页/购物车），对标 LangGraph subgraphs / AutoGen nested chats /
CrewAI hierarchical process。

## 两种设计

### 方案 A：子 Run 链接（推荐）

复合任务由 `suborchestrator` provider 执行：它对任务描述再次调用 planner，
**创建一个新的 Run**（metadata 携带 `parent_run_id` / `parent_task_id`），
轮询/订阅其终态；子 Run 完成 → 复合任务 attempt 成功，子 Run 的产出汇总为
attempt 输出（喂给下游 Upstream）。

- 优点：**零引擎改动**——复用 StartRun/Reconcile/重试/退避/取消/投影的全部
  既有语义；子 Run 在房间里有自己的任务面板视图；幂等天然成立（attempt
  幂等键挂 parent task）。
- 缺点：跨 Run 取消需要级联（父 Run 取消 → 找 child runs 取消）；房间消息
  时间线会交织两个 Run 的投影（可按 run_id 分组渲染解决）。
- 必做防护：**深度限制**（metadata `orchestration_depth`，≥2 拒绝再分解，
  防递归爆炸）；子 Run 任务数上限；planner 对"是否值得再分解"的判断要保守
  （默认不分解，仅当任务描述明显含多个可并行子目标）。

### 方案 B：In-Run 动态扩展

复合任务执行时向**同一 Run** 追加子任务（`CreateTasks` 二次调用 +
`ParentClientKey`），父任务变成 join 节点，子任务全部成功后父任务才算成功。

- 优点：单一 Run 视图、依赖图完整。
- 缺点：**动引擎核心不变量**——完成/失败/重试传播、run 状态重算、扩展的
  幂等（重试不得重复造子任务）、取消传播、`maxIterations` 与并发上限的
  交互，全部要重新论证。这是仓库测试最密集、最不能回归的代码。

## 执行决策（为什么现在不做）

1. **演示价值低**：当前房间是 2-4 个 agent 的扁平协作，LLM planner 已经把
   目标拆成 per-agent 子提示（1-6 个任务），没有出现过"单任务还需要再拆"
   的真实用例。
2. **风险不对称**：方案 B 动引擎核心；方案 A 虽零引擎改动，但取消级联、
   消息交织、深度防护都需要产品确认交互形态（子 Run 在 UI 怎么呈现）。
3. 与其余已落地能力（确认闸、摘要记忆、SSE 推送）不同，本项没有"小而完整
   的安全切片"——最小可用版本也要跨 provider/UI/取消语义三个面。

**触发条件**（满足任一再启动，按方案 A 实施）：
- 出现真实用例：单个任务的产出明显需要多 agent 并行子协作；
- 产品确认子 Run 的 UI 呈现方式（嵌套面板 or 按 run 分组的时间线）。

## 方案 A 实施清单（触发后）

- [ ] `suborchestrator` provider：plan→StartRun(child)→等待终态→汇总输出
- [ ] 深度限制 + 子任务数上限 + 保守分解判定（单测全覆盖）
- [ ] CancelRun 级联：父 Run 取消时按 `parent_run_id` 取消子 Run
- [ ] 投影：子 Run 消息带 parent 标记，前端按 run 分组渲染
- [ ] 端到端测试：两层 DAG 全链路（含子 Run 失败→父任务失败传播）
