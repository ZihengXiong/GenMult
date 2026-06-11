# Plan: AgentHub 房间会话摘要记忆（Conversation Summary Memory）

> 状态：Phase 0 已实现（2026-06-11）；Phase 1 已实现（2026-06-11，用户确认后落地，
> 见 internal/agenthub/summary_memory.go；房间 metadata `summary_memory: false` 可关闭；
> 验证清单全部勾掉，含 LIVE_LLM_TEST 真实折叠链路）。Phase 2 待产品需要。

## 背景与对标

当前编排上下文 = 置顶消息（按 ID 解析，长期）+ 最近消息窗口（newest-N，字符预算
12000）。窗口之外的历史对 agent 完全不可见。

业界对应做法：

| 框架 | 机制 | 要点 |
|---|---|---|
| LangChain | `ConversationSummaryBufferMemory` | 超出 token 预算的旧消息折叠为 LLM 生成的递增摘要 |
| LlamaIndex | `ChatSummaryMemoryBuffer` | 同上，摘要 + 最近原文混合 |
| LangGraph | checkpointer + 手动 summarize 节点 | 摘要是显式图节点，可控可测 |

## 分阶段设计

### Phase 0 —— 截断可见性（已实现，零成本）

历史窗口因预算丢弃旧消息时，在转写头部加显式标记
`（更早的对话因长度限制未包含，以下仅为最近 N 条消息）`。
agent 不再把窗口误当全部对话；用户可用 pin 补充关键旧信息。

### Phase 1 —— 按需递增摘要（待触发）

- **存储**：`agent_hub_rooms.metadata` 增加两个键（无 schema 迁移）：
  `history_summary`（string）与 `history_summary_through`（最后被摘要的 message id）。
- **触发**：仅当 `recentRoomHistory` 实际发生截断、且存在未摘要的被丢弃消息时，
  异步（run 启动后、不阻塞派发）调用一次廉价模型（`deepseek-v4-flash`，
  max_tokens ≤ 512）把 `[history_summary_through+1, 窗口起点)` 的消息折叠进摘要。
- **注入**：摘要块插在 pinned 之后、recent 窗口之前，预算 ≤ 2000 字符。
- **幂等/并发**：摘要更新走 room metadata 乐观更新；失败只记日志（best-effort，
  与 pinned/recent 同语义）。
- **成本**：每房间每次窗口推进最多一次 flash 调用；空闲房间零调用。

### Phase 2 —— 摘要质量与可见性（可选）

- 摘要在前端房间设置里可见/可清除；
- run 事件流增加 `history_summarized` 事件，便于审计。

## 执行决策（为什么现在停在 Phase 0）

1. **成本约束**：用户明确说明 API 余额有限；Phase 1 会在用户无感知的情况下产生
   LLM 调用。违背最小花费原则的「静默扣费」需要产品层确认（至少要有开关）。
2. **场景匹配度**：当前房间是短生命周期的协作演示，置顶 + 12000 字符窗口已覆盖
   实际需求；没有出现过「窗口外信息导致任务失败」的真实案例。
3. **风险面**：Phase 1 触达 run 启动路径与房间元数据写入，回归面大于收益。

**触发条件**（满足任一即启动 Phase 1）：
- 出现真实用例：长会话中 agent 因缺失窗口外上下文而给出错误结果；
- 产品确认增加「房间摘要记忆」开关（默认关）。

## 验证清单（Phase 1 实施时）

- [ ] 截断未发生时零 LLM 调用（单测，mock 模型计数）
- [ ] 摘要递增正确推进 `history_summary_through`（不重复摘要）
- [ ] 注入顺序 pinned → summary → recent，总预算不超
- [ ] 摘要调用失败不影响 run（best-effort 降级）
- [ ] LIVE_LLM_TEST 真实链路验证一次（flash 模型）
