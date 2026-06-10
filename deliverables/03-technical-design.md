# AgentHub 技术文档

> 面向答辩的"代码理解度"维度:讲清**架构选型**与**三条核心链路**。

## 1. 总体架构

AgentHub 构建在 memoh/Twilight 平台之上,新增「多 Agent 协作」能力。分层如下:

```
┌─────────────────────────────────────────────────────────────┐
│  Web 前端 (Vue 3 + <script setup>)                            │
│  apps/web/src/pages/agent-hub/index.vue                       │
│  - 会话列表 / 单聊 / 群聊 / 富媒体卡片 / 工作区面板            │
│  - @pinia/colada useQuery + 轮询驱动 run 进度                 │
└───────────────┬─────────────────────────────────────────────┘
                │ HTTP (REST, @memohai/sdk client)
┌───────────────▼─────────────────────────────────────────────┐
│  Go 后端 (echo + uber/fx 依赖注入)                            │
│  internal/handlers/agent_hub*.go   ← 路由层                   │
│  internal/agenthub/                 ← AgentHub 业务层          │
│    ├─ service.go              房间/消息 CRUD                  │
│    ├─ orchestrator_service.go 编排服务(装配 + 凭证 + 投影)   │
│    ├─ orchestrator_projector.go run 事件 → 房间消息           │
│    ├─ llm_planner.go          LLM 动态任务拆解                │
│    └─ orchestrator/           编排内核(DAG 引擎)            │
│         ├─ service.go    Reconcile/Dispatch 状态机           │
│         ├─ planner.go    RulePlanner(@mention 直分派)       │
│         ├─ providers.go  AgentProvider 注册表                │
│         ├─ state_machine.go 任务/run 状态转移               │
│         └─ {memory,sql}_store.go  事件/任务持久化           │
│  internal/agenthub/providers/   ← 统一适配器层               │
│    ├─ claudecode.go  Claude Code CLI (stream-json)          │
│    ├─ codex.go       Codex CLI                              │
│    ├─ memoh.go       Memoh 框架(接口反转注入)             │
│    ├─ clirunner.go   子进程生命周期 + NDJSON 流式解析       │
│    ├─ executor.go    HostExecutor / 桥接执行器              │
│    └─ workspace_resolver.go  共享工作目录解析               │
└───────────────┬─────────────────────────────────────────────┘
                │ sqlc 生成的 dbstore.Queries 接口
┌───────────────▼─────────────────────────────────────────────┐
│  存储: SQLite(开发) / PostgreSQL(生产) 双实现             │
│  CLI Agent 产物: 每房间共享工作目录(host /tmp/…/<roomID>) │
└─────────────────────────────────────────────────────────────┘
```

### 关键选型理由
- **uber/fx 依赖注入**:Provider、Resolver、Store 全部声明式装配,新增 Agent 适配器只需注册一个 `AgentProvider`,符合课题"统一适配器层屏蔽 API 差异"。
- **`dbstore.Queries` 接口 + sqlc**:同一份业务代码跑 SQLite/Postgres,开发零依赖、生产可扩展。
- **编排内核与平台解耦**:`internal/agenthub/orchestrator` 是纯领域内核(不 import 上层),通过**接口反转**注入 Memoh runtime,避免包循环(`flow → botruntime → providers`)。

## 2. 数据模型(核心三张)

- **Room(房间/会话)**:一个聊天会话,含成员 Agent 列表;单聊=1 成员,群聊=N 成员。
- **Message(消息)**:文本/代码/工具/附件;`metadata` 携带 pin、reply_to、attachments、event 投影来源。
- **Run / Task / RunEvent(编排)**:一次群聊编排 = 一个 Run;Run 拆成 Task DAG;执行过程产生有序 `RunEvent`(`run_planned / task_dispatched / agent_output / agent_tool_call / task_succeeded / run_status_changed …`)。

## 3. 核心链路一:单聊流式输出

```
用户发消息 → 前端 collectMainAgentReply(onProgress)
  → 后端 botruntime/cli.go 启动 claude CLI (stream-json)
  → clirunner 逐行解析 NDJSON → text/thinking/tool_use 事件
  → WebSocket 推送 → 前端 streamingReply 累积 → 实时草稿气泡(R5)
  → end 事件 → 落一条完整消息
```
要点:**生成中即时可见**,而非等全部完成才显示一条。

## 4. 核心链路二:群聊 Orchestrator 自动分派

```
@主Agent / 发起任务
  → StartRun(objective, 房间成员作为 AgentDescriptor)
  → Planner 决策:
       · 显式 @某Agent → RulePlanner 直接分派(directMentionPlan)
       · 否则 → LLM Planner 把目标拆成 Task DAG(llm_planner.go)
  → ReconcileRun 状态机循环:markRunnable → dispatch → 各 Provider.Execute
       · DispatchAsync=true,任务并行,完成后回调 ReconcileRun 推进 DAG
  → 每个 Provider 把产出写成 RunEvent
  → Projector(orchestrator_projector.go)把事件投影成房间消息
       · run_planned → "任务规划" 消息
       · agent_output(text) → Agent 产出消息
       · task_succeeded → 聚合结果,run_status_changed → "协作完成"
  → 前端 1.5s 轮询 reconcile + 拉 messages 刷新
```
要点:**用户只描述目标,系统自动拆解、分派、聚合**(失败任务降级、不阻断其余)。

## 5. 核心链路三:CLI Agent 凭证 / 工作目录

- **凭证统一来源**:`ResolveCredentialsForFramework(queries, "claudecode")` 从「通用设置」里启用的 Anthropic 兼容 Provider 取 `{base_url, token, model}`,单聊与编排**同源**;支持第三方端点(base_url≠官方时用 `ANTHROPIC_AUTH_TOKEN` Bearer)。
- **每个 bot 的模型覆盖**:从 bot 的 `provider_ext["claudecode"]` 读取它在 1:1 聊天用的 DeepSeek 模型,传给 CLI 的 `--model`,避免 CLI 默认 Claude 模型 → DeepSeek 端点不服务 → 空产出(R7)。
- **群内共享工作目录**:`SharedRoomWorkDir(roomID) = os.TempDir()/memoh_agenthub_rooms/<roomID>`,同房间所有 CLI Agent 在同一目录协作,一个写的文件另一个可读(R4.1);prompt 里显式注明该目录。

## 6. REST API 一览

房间 / 消息(`internal/handlers/agent_hub.go`):
```
GET    /agent-hub/rooms
POST   /agent-hub/rooms
GET    /agent-hub/rooms/:room_id
PUT    /agent-hub/rooms/:room_id
DELETE /agent-hub/rooms/:room_id
POST   /agent-hub/rooms/:room_id/agents
DELETE /agent-hub/rooms/:room_id/agents/:agent_id
GET    /agent-hub/rooms/:room_id/messages
POST   /agent-hub/rooms/:room_id/messages
DELETE /agent-hub/rooms/:room_id/messages/:message_id
GET    /agent-hub/rooms/:room_id/files            # 工作区文件列表 (R8)
GET    /agent-hub/rooms/:room_id/files/content    # 读文件 (R8)
POST   /agent-hub/rooms/:room_id/exec             # 工作区终端 (R8)
```
编排 run(`internal/handlers/agent_hub_orchestrator.go`):
```
POST   /agent-hub/rooms/:room_id/runs             # 发起编排
GET    /agent-hub/rooms/:room_id/runs/latest      # 房间最近一次 run
GET    /agent-hub/runs/:run_id                    # run 快照
POST   /agent-hub/runs/:run_id/reconcile          # 推进/轮询
POST   /agent-hub/runs/:run_id/cancel
GET    /agent-hub/runs/:run_id/events             # 原始事件流(R9 实时气泡用)
POST   /agent-hub/runs/reconcile-active
```

## 7. 适配器层:新增一个 Agent 平台

实现 `orchestrator.AgentProvider`(`Name / Capabilities / Execute`)并注册进 registry 即可。CLI 类(claudecode/codex)复用 `clirunner.go` 的子进程 + NDJSON 流式骨架,只需提供 `BuildArgs / ParseEvent`;框架类(memoh)用接口反转注入 runtime。这就是课题"统一适配器层"的落点。

## 8. 已实现能力 ↔ 课题核心功能对照

见 `02-product-design.md` 的功能矩阵。
