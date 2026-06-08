# 3 分钟 Demo 脚本

## 开场（0:00 - 0:20）

**画面**：Memoh Web UI 首页

> "这是 Memoh —— 一个多成员、容器化的 AI Agent 平台。它让你通过 IM 和 Web 统一管理多个 AI 助手。"

## Part 1：创建 Agent（0:20 - 0:50）

**操作**：
1. 点击「创建 Bot」
2. 切换到「Guided」模式
3. 对话式创建：输入名称 → 选择框架 → 填写 System Prompt → 选择能力标签
4. 点击创建

**话术**：
> "支持三种框架：内置 Memoh Agent、Claude Code、Codex。每个 Bot 有独立容器和记忆系统。"

## Part 2：多平台对话（0:50 - 1:30）

**操作**：
1. 在 Web UI 中发送一条消息
2. 展示 Agent 思考过程（Thinking Block 展开/折叠）
3. 展示工具调用（文件读写、命令执行、diff 视图）
4. 点击 diff 卡片的「Apply」按钮
5. 点击「View」打开全屏 Monaco 编辑器

**话术**：
> "Agent 的推理过程和工具调用完全透明。产物可以一键应用，也可以在全屏编辑器中查看修改。"

## Part 3：多 Agent 协作（1:30 - 2:10）

**操作**：
1. 进入 Agent Hub 页面
2. 展示一个 Room，包含 Claude Code 和 Codex 两个 Agent
3. 发送一个任务目标
4. 展示 Orchestrator DAG 任务图（tasks 状态变化）
5. 展示 Agent 并行执行

**话术**：
> "Agent Hub 的 Orchestrator 引擎会自动分解任务为 DAG，调度多个 Agent 并行执行。"

## Part 4：跨平台 + AI 协作（2:10 - 2:50）

**操作**：
1. 展示 Bot 设置页面 → Channel 接入（Telegram/Discord）
2. 展示 Bot 的能力标签和自定义 System Prompt
3. 快速展示项目的 AGENTS.md 文件
4. 展示 git log 中 AI 贡献的 commit

**话术**：
> "项目本身就是用 Claude Code + Codex 协作开发的。AGENTS.md 文件约束了 AI 的行为规范，确保代码质量。"

## 结尾（2:50 - 3:00）

**画面**：回到首页

> "Memoh：多平台消息聚合、容器化 Agent 隔离、多 Agent 编排协作。"

---

## 录制清单

- [ ] 确保 dev 环境运行（`mise run dev`）
- [ ] 创建至少 2 个不同框架的 Bot（memoh + claudecode）
- [ ] 准备一个 Agent Hub Room（含 2+ Agent）
- [ ] 准备一个可展示的对话历史（含工具调用、diff、thinking）
- [ ] 接入至少一个 IM 平台用于展示（或展示配置页面）
- [ ] 录制工具：OBS / QuickTime / Loom
- [ ] 分辨率：1920x1080，帧率 30fps
- [ ] 测试 Demo 流程 2-3 次确保流畅
- [ ] 准备好 AGENTS.md 和 git log 页面
