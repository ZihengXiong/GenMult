# Agent Framework 对比

AgentHub 目前支持三种 agent 框架，各自的运行机制和能力不同。

## 对比

| | memoh | claude code | codex |
|---|---|---|---|
| **运行方式** | in-process Go | CLI subprocess | — |
| **多轮上下文** | 天然支持（Config.Messages） | `--append-system-prompt` 注入历史 | — |
| **thinking / reasoning** | 取决于模型 `reasoning_effort` 配置 | 取决于模型配置 | — |
| **show_thinking_stream** | 支持（通用 flag） | 支持（通用 flag） | — |
| **工具调用** | 支持 | 支持（Claude CLI 内置） | — |
| **session 复用** | `ensureSession`（正常聊天路径） | `metadata.agent_sessions[agentId]` | — |
| **AgentHub 多轮修复** | 同上，session 存 room metadata | 同左 | — |

## 说明

### memoh
原生 Go 实现，直接消费 `Config.Messages`，和普通聊天共用同一 resolver 路径。thinking 内容经 `ConvertModelMessagesToUIAssistantMessages` 流到前端，`TextContent()` 在构建下轮上下文时自动剔除 reasoning part，不会污染历史。

### claude code
以 CLI subprocess 运行（`claude -p --input-format stream-json --output-format stream-json --verbose`），无状态，每次冷启动。多轮历史通过 `--append-system-prompt` 注入 transcript，session 持久化在房间 `metadata.agent_sessions` 里。thinking 内容同样从 WebSocket 流出，存进消息 `metadata.thinking`，不进 `body`。

### codex
待补充。
