# AI 协作开发规范与记录

> 对应考察维度"AI 协作能力(30%)":沉淀出与 AI 协作的 **Spec / Skill / Rules** 等规范,并以真实开发记录佐证。

本项目全程采用 **Claude Code 结对开发**。下面给出我们沉淀的协作方法论,以及可验证的过程记录。

## 1. Rules:与 AI 协作的硬约束

集中写在 `~/.claude/CLAUDE.md`(每次会话强制加载),贯穿全程:

```
- 不要轻易简化程序;如需简化,必须提前商量
- 如无必要,勿增实体(最小改动)
- 使用 Python 时使用虚拟环境(无则用 conda base)
```

加上每个任务回合的局部约束(写进对话与 plan):
- **不改 `chat`,只修 `agenthub`**:只复用 `flow.Resolver`,不动 `internal/conversation/flow` 内部。
- **凭证只从「通用设置」走 DeepSeek**(base_url + token),不依赖 `ANTHROPIC_API_KEY`,不用 codex。
- **完成一个 commit 一次**:每个回合独立可回溯。

> 价值:Rules 把"人对工程的判断"固化下来,约束 AI 不越界、不偷懒、不擅自简化——这是协作质量的地基。

## 2. Spec / Plan:规格先行的回合制

复杂改动一律**先出 plan 再写码**。plan 固定四段式结构:**根因(代码追踪确认)→ 设计(改动清单 + 复用现成件)→ 验证(如何证明修复)→ 风险**。

样例见 `~/.claude/plans/plan-resilient-sutherland.md`(R4 凭证复用):它先用代码行号定位"单聊能跑、编排报错"的根因,再列最小改动,再给出可执行的验证步骤与反证实验。

> 价值:Spec 把"要做什么、为什么、怎么验证"讲清楚后再动手,避免 AI 一上来乱写;也直接成为技术文档的素材。

## 3. Skill:固化的协作工具链

| Skill | 用途 | 在本项目的作用 |
|---|---|---|
| **advisor 复核** | 动手前/收尾前请更强模型审一遍 | R9 由 advisor 纠正"实时气泡只放 thinking、不放 text",避免与投影消息重复渲染 |
| **memory 持久化** | 跨会话沉淀非显然事实 | 记录编排架构、单聊历史不加载根因、stream-json 多轮语义 |
| **/verify 浏览器实测** | 用 Playwright 真跑 UI 验证 | R8 验证文件/终端;R9 实测"运行中实时气泡"22/22 采样命中 |
| **pre-commit 门禁** | golangci-lint + go test + eslint | 每次 commit 前自动跑,保证不带病入库 |

## 4. 协作回合记录(可验证)

整个 AgentHub 以**编号回合(R1–R9)**推进,每回合一个独立 commit,git log 即开发记录:

| 回合 | Commit | 内容 |
|---|---|---|
| R1–R3 | `f2bd672` | Memoh 编排 provider、按框架分派、pin 长期上下文 |
| R4 | `b3ae212` | 编排 claudecode 复用「通用设置」DeepSeek 凭证 |
| R4.1 | `144608d` | 编排 CLI agent 共享每房间工作目录 |
| R5 | `850999c` | 主 Agent 回复逐字流式显示 |
| R6 | `ad27637` | 激活 LLM Planner,动态多 Agent 分派(去硬编码) |
| R7 | `247adc0` | 编排 cc 用 bot 自身模型+凭证(修空产出) |
| R8 | `0da297a` | 群聊工作区文件浏览 + 终端接线 |
| R9 | `ea6e394` | 群聊编排 run 运行中实时过程气泡 |

每个回合的标准闭环:
```
调研定位根因 → advisor 复核方案 → 最小改动实现 → /verify 实测 → pre-commit 门禁 → commit
```

## 5. 一个能讲的协作实例:R9

> 适合答辩时口述,体现"人 + AI 如何分工纠偏"。

1. **现象**:群聊编排时,任务执行的几十秒里房间是静默的,末尾才一次性冒结果。
2. **AI 调研**:dump 213 个 `agent_output` 事件,发现事件其实是**增量到达**的(27 秒跨度),只是 Projector 只把最终 text 投成一条消息,thinking/工具调用被丢弃。
3. **advisor 纠偏**:原方案想把累积文本也塞进实时气泡;advisor 指出 text 已被 Projector 投成真实消息,实时气泡再放 text 会**重复渲染**——改为只放 thinking + 工具指示。
4. **最小实现**:纯前端,复用已有 `GET /runs/:id/events` 端点 + R5 的气泡组件,无后端改动、无新实体(守"勿增实体")。
5. **实测**:Playwright 起一次真实 cc 任务,确认气泡在任务 `running` 期间持续刷新(22/22 采样命中 thinking),填上静默空档。

## 6. 交付时如何呈现这 30%

- 附 **git log**(R1–R9)+ 本文件 + `~/.claude/plans/` 里的 plan 截图。
- 现场口述 §5 的 R9 实例:**人定约束、AI 调研、advisor 纠偏、verify 闭环**——这是别人最难复制的协作深度。
