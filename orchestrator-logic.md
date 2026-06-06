# AgentHub Orchestrator 逻辑说明

## 一句话概括

用户在 AgentHub 房间里发一个目标（objective），Orchestrator 把它拆成一张 **任务 DAG**，按依赖顺序把每个任务派给 Agent 执行，执行完的结果解锁下游任务，直到整张 DAG 跑完或失败。

---

## 核心流程

```
用户发送目标
    │
    ▼
┌─────────────┐
│  StartRun   │  入口
└─────┬───────┘
      │
      ▼
┌─────────────┐     Planner 把目标拆成 TaskDraft[]
│   Plan      │     当前用的 RulePlanner，固定产出 5 个节点：
└─────┬───────┘     plan → backend ─┐
      │                              ├→ verify → finalize
      │             plan → frontend ┘
      ▼
┌─────────────┐     TaskDraft[] 写入 Store，生成真实 Task + TaskDependency
│ CreateTasks │     所有 task 初始状态 = pending
└─────┬───────┘
      │
      ▼
┌──────────────┐    扫描 DAG，没有前置依赖的 task → ready
│markRunnable  │    有前置依赖失败的 task → blocked
└─────┬────────┘
      │
      ▼
┌──────────────┐    ReconcileRun 是一个循环（不是单次调用）
│ Reconcile    │◀─────────────────────────┐
│   Loop       │                          │
└─────┬────────┘                          │
      │                                   │
      ▼                                   │
┌──────────────┐    按优先级排序 ready task，  │
│dispatchReady │    受并发上限约束后逐个派发     │
└─────┬────────┘                          │
      │                                   │
      ▼                                   │
┌──────────────┐    调用 AgentProvider.Execute │
│executeAttempt│    同步模式：阻塞等结果        │
└─────┬────────┘    异步模式：goroutine 执行   │
      │                                   │
      ▼                                   │
┌──────────────┐    标记 task succeeded/failed │
│completeTask  │    如果可重试 → task 回到 ready│
│  Attempt     │                          │
└─────┬────────┘                          │
      │         同步模式：循环回到顶部 ────────┘
      │         异步模式：goroutine 调 ReconcileRun
      ▼
┌──────────────┐
│recomputeRun  │    所有 task succeeded → run completed
│  Status      │    有 failed 且无 running/pending → run failed
└──────────────┘    还有 running → collecting
                    还有 ready → dispatching（继续循环）
```

---

## 状态机

### Run 状态

```
draft → planning → dispatching ⇄ collecting → completed
                                             → failed
           任意非终态 ──────────────────────→ cancelled
```

### Task 状态

```
pending → ready → running → succeeded
                          → failed ──→ ready（重试）
pending → blocked（上游失败）
任意非终态 → cancelled
```

### Attempt 状态

```
running → succeeded / failed / timed_out / cancelled
```

三层状态各司其职：Run 是全局进度，Task 是 DAG 节点，Attempt 是单次执行记录。

---

## 关键机制

### 1. DAG 依赖推进

`markRunnableTasks` 扫描所有 pending task：

- 所有前置依赖都 succeeded → 标记为 **ready**
- 任一前置依赖 failed/blocked/cancelled → 标记为 **blocked**
- 还有前置依赖在跑 → 保持 pending

这保证了拓扑排序执行，且失败能向下游传播。

### 2. 并发控制

两个维度限流：

- `MaxParallelPerRun = 3`：一个 run 里最多同时 3 个 task 在跑
- `MaxParallelPerAgent = 1`：同一个 agent 同时只执行 1 个 task

`dispatchReadyTasks` 按优先级排序 ready task，在容量内逐个派发。

### 3. 重试

每个 task 有 `MaxRetries`（额外重试次数）。Provider 返回 `Retryable: true` 且还有预算时：

```
task: running → failed → ready（回到待派发队列）
```

下次 reconcile 循环会重新 dispatch 它。`AttemptCount` 记录已执行次数，每次 dispatch 前递增。

### 4. 超时

两层超时：

- **执行时**：`executeAttempt` 用 `context.WithTimeout` 包裹 provider 调用，超时直接标记 `timed_out`
- **恢复时**：`failTimedOutTasks` 在 reconcile 开头扫描所有 running task，如果最新 attempt 的 `startedAt + timeout < now`，强制标记超时（用于异步模式或崩溃恢复）

### 5. 同步 vs 异步派发

- `DispatchAsync: false`（默认）：`executeAttempt` 在当前 goroutine 阻塞执行。完成后 reconcile loop 的下一轮自动推进 DAG。调用栈是平的（循环，不是递归）。
- `DispatchAsync: true`：`executeAttempt` 在 goroutine 里跑，完成后 goroutine 自己调 `ReconcileRun` 推进后续任务。

### 6. 事件流

每次状态变更都 `AppendEvent` 到 `agent_hub_run_events`（append-only，带单调递增 seq）。前端可以用 `ListEvents(afterSeq)` 做增量轮询，实现时间线回放。事件类型覆盖：run 创建/计划/状态变更、task 创建/就绪/派发/成功/失败/阻塞/重试/超时/取消。

### 7. 幂等性

每个 attempt 有 `idempotencyKey = sha256(runID:taskID:attemptNo:description)`，用于去重（比如崩溃恢复后不重复执行相同 attempt）。

---

## 分层架构

```
Handler 层     agent_hub_orchestrator.go    HTTP 路由 + 参数校验 + 错误映射
     │
     ▼
Service 层     orchestrator_service.go      房间权限校验 + 封装调用
     │
     ▼
Core 层        orchestrator/service.go      状态机 + DAG 推进 + 派发逻辑
     │
     ▼
Store 层       memory_store.go              测试用内存实现
               sql_store.go                 Postgres/SQLite 双实现（CAS 保护）
```

Core 层不依赖 HTTP 框架或具体数据库，只依赖 `Store`、`Planner`、`AgentProvider` 三个接口。这意味着：

- 换 LLM Planner 替换 RulePlanner，不改 dispatcher
- 接入 Claude Code / Codex 只需实现 `AgentProvider` 接口
- 换存储只需实现 `Store` 接口

---

## RulePlanner 产出的 DAG

当前是硬编码的 5 节点 DAG，未来会被 LLM Planner 替换：

```
plan（理解目标、形成计划）
  ├──→ backend（后端/接口实现）──┐
  └──→ frontend（前端/交互实现）─┤
                                └──→ verify（验证测试）──→ finalize（汇总交付）
```

`pickAgent` 按关键词匹配把房间里的 agent 分配到对应节点。没有匹配到就用第一个 agent 兜底，全都没有就用 `noop` provider。

---

## 文件清单

| 文件 | 职责 |
|------|------|
| `orchestrator/doc.go` | 包说明与职责边界 |
| `orchestrator/types.go` | Run/Task/Attempt/Event/Plan/Snapshot 领域模型 |
| `orchestrator/interfaces.go` | `Planner`、`AgentProvider`、`Store` 接口定义 |
| `orchestrator/errors.go` | 统一错误定义 + 终态判断辅助函数 |
| `orchestrator/state_machine.go` | Run/Task 状态迁移规则 |
| `orchestrator/planner.go` | `RulePlanner`（规则化拆任务，5 节点 DAG） |
| `orchestrator/providers.go` | `ProviderRegistry` + `NoopProvider` |
| `orchestrator/service.go` | 核心编排逻辑（拆任务→派发→回收→状态推进→reconcile 循环） |
| `orchestrator/memory_store.go` | 内存版 Store（测试/本地开发） |
| `orchestrator/sql_store.go` | SQL 持久化 Store（Postgres + SQLite 双实现，CAS 保护） |
| `orchestrator/service_test.go` | 核心流程单测（DAG、成功链路、重试、阻塞、取消、超时） |
| `orchestrator_service.go` | AgentHub 应用层封装（权限校验 + 编排服务调用） |
| `handlers/agent_hub_orchestrator.go` | HTTP API 层（6 个端点） |

---

## HTTP API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/agent-hub/rooms/:room_id/runs` | 创建并启动一个编排 run |
| GET | `/agent-hub/runs/:run_id` | 获取 run 快照（run + tasks + deps + attempts） |
| POST | `/agent-hub/runs/:run_id/reconcile` | 手动触发 reconcile（推进状态） |
| POST | `/agent-hub/runs/:run_id/cancel` | 取消 run 及其所有非终态 task |
| GET | `/agent-hub/runs/:run_id/events` | 获取事件流（支持 `after_seq` 增量查询） |
| POST | `/agent-hub/runs/reconcile-active` | 批量 reconcile 当前用户的所有活跃 run |

---

## 数据库表

| 表名 | 说明 |
|------|------|
| `agent_hub_runs` | Run 主表（目标、状态、planner 版本、元数据） |
| `agent_hub_tasks` | Task 节点（标题、描述、分配 agent、优先级、重试配置） |
| `agent_hub_task_deps` | DAG 依赖边（task_id depends_on depends_on_task_id） |
| `agent_hub_task_attempts` | 执行记录（provider、输入/输出、错误信息、幂等键） |
| `agent_hub_run_events` | Append-only 事件日志（seq 单调递增，支持增量查询） |
