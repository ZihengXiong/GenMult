# 产品设计文档

## 1. 问题定义

现有 AI 对话工具存在以下痛点：

- **多平台消息碎片化**：用户在 Telegram、Discord、飞书、钉钉、微信等平台分散使用 AI，无法统一管理
- **Agent 能力单一**：大多数方案只支持单模型单对话，缺少多 Agent 协作、工具调用、代码执行等能力
- **缺乏持久化工作空间**：对话结束即丢失上下文，无法持续迭代文件、代码和项目

## 2. 用户画像

| 角色 | 需求 | 使用场景 |
|------|------|----------|
| 开发者 | 代码审查、调试、自动化任务 | 多 Agent 协作编码 + CLI 执行 |
| 内容创作者 | 写作辅助、翻译、摘要 | 跨平台消息管理 + 记忆上下文 |
| 团队管理者 | 自动化日报、定时任务 | 定时触发 + 心跳检测 |
| 研究人员 | 文献检索、数据分析 | 工具集成 + MCP 协议 |

## 3. IM 核心体验

### 3.1 多平台消息聚合

系统通过 Channel Adapter 架构统一接入：
- Telegram、Discord、飞书、钉钉、微信、企业微信、Matrix、Email、QQ
- 每个平台对应一个 Adapter，实现统一的消息收发接口
- Channel Identity 机制实现跨平台用户身份映射

### 3.2 AI Agent 对话

- 每个 Bot 独立容器隔离，拥有独立的文件系统和执行环境
- 支持三种 Agent 框架：Memoh（内置）、Claude Code、Codex
- 长期记忆系统（向量语义搜索 + BM25）
- 流式响应 + Thinking Block 可视化

## 4. 页面流与信息架构

```
首页
├── 聊天列表（Sessions）
│   ├── 对话消息流
│   │   ├── 消息气泡（用户/助手）
│   │   ├── Agent 活动面板（思考 + 工具调用）
│   │   ├── 产物预览卡片（iframe/代码/图片）
│   │   └── 附件区域
│   └── 侧边栏（文件/MCP/Skills/Sessions）
├── Bot 管理
│   ├── 创建 Bot（表单/对话引导）
│   └── Bot 设置（模型/工具/权限/框架配置）
├── Agent Hub
│   ├── Room 列表（多 Agent 协作空间）
│   ├── Orchestrator（DAG 任务引擎）
│   └── Agent 成员管理
└── 系统设置
    ├── Provider 管理（模型供应商）
    ├── Channel 配置
    └── 用户管理
```

## 5. 功能清单

### 核心功能
- [x] 多平台 IM 接入（9+ 平台）
- [x] 多框架 Agent 支持（Memoh/Claude Code/Codex）
- [x] 容器化工作空间（文件编辑 + 命令执行）
- [x] 长期记忆系统（多 Provider）
- [x] 流式对话 + 工具调用可视化
- [x] MCP 协议集成

### WS3 产物预览
- [x] 网页预览卡片（iframe 沙箱）
- [x] Diff 卡片 + 一键应用
- [x] 全屏 Monaco 代码查看器
- [x] 部署状态卡片（接口预留）

### WS4 多 Agent 接入
- [x] 自建 Agent（自定义 SystemPrompt + 工具集）
- [x] 对话式创建 Wizard
- [x] 能力标签系统
- [x] 双适配器层设计文档

### 自动化与调度
- [x] 定时任务（Cron）
- [x] 心跳检测
- [x] 后台任务管理

## 6. 创新点

1. **容器化 Agent 隔离**：每个 Bot 独立容器，通过 gRPC Bridge (UDS) 通信，安全可控
2. **双适配器层架构**：AgentProvider（任务执行）+ BotRuntime（聊天流），解耦编排与对话
3. **Orchestrator DAG 引擎**：LLM Planner 自动分解任务，支持依赖关系和重试
4. **跨平台身份聚合**：Channel Identity 统一身份，一个用户在所有平台保持上下文连续
5. **AI 协作开发**：项目本身使用 Claude Code + Codex 协作开发，.agents/ 规则约束 AI 行为
