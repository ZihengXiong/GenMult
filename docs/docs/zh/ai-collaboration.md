# AI 协作开发记录

## 1. 协作工具链

本项目使用以下 AI 协作工具进行开发：

| 工具 | 用途 | 贡献 |
|------|------|------|
| **Claude Code** (Anthropic) | 代码编写、架构设计、代码审查 | 后端 Go + 前端 Vue 组件 |
| **Codex** (OpenAI) | 代码生成、重构 | 辅助代码生成 |
| **Claude** (对话) | 需求分析、方案讨论、文档撰写 | 设计决策、文档 |

### AI 贡献统计

基于 git 历史，normalman743 与 Claude 合作编写的 commit 占总 commit 数的 ~85%。主要贡献分布：

- **后端 Agent 集成**：Claude Code/Codex CLI bridge、多轮对话支持、tool streaming
- **前端 UI 组件**：Agent Hub 改版、Bot 设置、产物预览卡片
- **基础设施**：DevEnv 修复、Docker toolkit、lint 清理
- **文档**：AGENTS.md、架构文档、API 文档

## 2. .agents/ 目录与 Teamwork 模式

项目根目录下的 `.agents/` 目录包含 AI Agent 的 skill 定义：

```
.agents/
└── skills/
    ├── humanizer/      # 文本人性化处理 skill
    ├── humanizer-zh/   # 中文文本处理 skill
    └── twilight-ai/    # Twilight AI SDK 集成 skill
```

这些 skill 作为 bot 容器内的可复用能力模块，定义了：
- Skill 名称和描述（YAML frontmatter）
- 执行逻辑（Markdown 格式的指令）
- 输入/输出规范

### Orchestrator/Sentinel Teamwork 模式

Orchestrator DAG 引擎实现了多 Agent 协作模式：
- **Planner Agent**：分解用户目标为任务 DAG
- **Worker Agents**：独立执行各自的任务节点
- **Sentinel Loop Detection**：`internal/agent/sential.go` 实现循环检测，防止 Agent 陷入重复行为

## 3. CLAUDE.md / AGENTS.md 规则约束

### AGENTS.md

`AGENTS.md` 是项目的核心约束文件，包含：
- 完整的项目结构映射（200+ 行）
- 技术栈声明（Go + Vue 3 + Echo + sqlc + Pinia Colada）
- 数据库开发规则（PG/SQLite 双后端同步、迁移规范）
- API 开发工作流（handlers → swagger → sdk → frontend）
- Agent 开发约束（in-process agent、ToolProvider 接口、prompt 模板）

### 作用机制

当 Claude Code 在项目中工作时：
1. 自动读取 `AGENTS.md`，理解项目架构和约束
2. 遵循迁移规则（双后端同步、增量+全量更新）
3. 使用正确的代码模式（sqlc 生成、Vue Composition API）
4. 不修改自动生成的文件（除非需要临时编辑以保持编译通过）

## 4. Prompt → 评审回路 → 迭代

### 协作模式

```
用户需求
    ↓
Claude 分析代码库 (Explore agent)
    ↓
生成实施计划 (Plan agent)
    ↓
用户审批计划
    ↓
Claude 执行实现
    ↓
用户审查代码变更
    ↓
修正/迭代
    ↓
提交
```

### 关键实践

1. **Plan-first 模式**：非平凡任务先进入 Plan 模式，探索代码库后设计方案，用户批准后再实现
2. **增量提交**：每完成一个逻辑单元就标记 task 完成，保持可见进度
3. **并行探索**：使用多个 Explore agent 并行研究代码库的不同区域
4. **上下文管理**：通过 task list 跟踪进度，避免上下文丢失

## 5. 本次规划会话协作示例

### WS3-5 实施过程

1. **需求输入**：用户提供 WS3/WS4/WS5 三个工作流的详细需求
2. **代码探索**：Claude 并行读取 ~15 个关键文件，理解现有架构
3. **计划制定**：生成包含执行顺序、文件清单、验证方式的完整计划
4. **逐步执行**：
   - WS4.1：后端 DB 迁移 + types + service + prompt（先行，因为其他任务依赖）
   - WS4.3：替换硬编码 capabilities
   - WS3.1-3.3：前端组件并行创建
   - WS3.2：后端 apply-edit 端点
   - WS4.2：对话式创建 wizard
   - WS5：文档并行编写

### 量化数据

| 指标 | 数值 |
|------|------|
| 新建文件 | ~12 个 |
| 修改文件 | ~20 个 |
| DB 迁移 | 4 个文件（PG 增量 + SQLite 增量 + 双端 canonical 更新） |
| 新 Vue 组件 | 4 个 |
| 新 Go handler | 1 个文件（2 个端点） |
| 文档 | 4 篇 |

## 6. Spec/Skill/Rules 沉淀

### 可复用 Skill

1. **humanizer** (`.agents/skills/humanizer/`): 将 AI 生成的文本转化为更自然的人类表达
2. **twilight-ai** (`.agents/skills/twilight-ai/`): Twilight AI SDK 集成指导

### 项目规则沉淀

- `AGENTS.md`：完整的项目结构和开发规范（已存在且维护良好）
- 迁移规则：PG/SQLite 双后端同步，canonical schema 始终更新
- API 工作流：handler → swagger → sdk → frontend 的自动化链路
