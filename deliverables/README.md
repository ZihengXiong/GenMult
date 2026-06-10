# AgentHub 交付物索引

本目录是《AgentHub-多 Agent 协作平台》课题的交付文档集。课题原始要求见仓库根的 `agenthub_description.pdf`。

## 文档清单

| 文件 | 对应交付物 | 主要服务的考察维度 |
|---|---|---|
| [`01-demo-runbook.md`](01-demo-runbook.md) | 可运行 Demo（运行说明 + 演示动线） | 功能完整度 25% |
| [`02-product-design.md`](02-product-design.md) | 产品设计文档（功能矩阵 + 亮点） | 生成效果质量 20% / 创新 10% |
| [`03-technical-design.md`](03-technical-design.md) | 技术文档（架构 + 三条核心链路 + API） | 代码理解度 15% |
| [`04-ai-collaboration.md`](04-ai-collaboration.md) | AI 协作开发记录（Spec/Skill/Rules + R1–R9） | **AI 协作能力 30%** |
| [`05-demo-video-script.md`](05-demo-video-script.md) | 3 分钟 Demo 视频脚本 | 生成效果质量 20% |

## 考察维度 → 交付物对照

| 维度 | 权重 | 靠哪些交付物得分 |
|---|---|---|
| AI 协作能力 | 30% | `04` + git log(R1–R9)+ `~/.claude/plans/` |
| 功能完整度 | 25% | `01` 跑通 + `02` 功能矩阵 |
| 生成效果质量 | 20% | Demo 视频 + `02` 亮点 |
| 代码理解度 | 15% | `03` + 答辩口述核心链路 |
| 创新与产品感 | 10% | `02` §4 亮点(R3/R5/R9/工作区) |

## 还需人工补的素材（非文档）
- **3 分钟 Demo 视频**:按 `05` 脚本录屏。
- **架构图配图**:`03` 里已有 ASCII 架构图,如需精美版可用 draw.io / excalidraw 重绘。
- **截图**:产品设计文档可补单聊流式、群聊编排、pin、工作区的实际截图。
- **(可选)启用 codex**:补强"2 个主流平台",见 `02` §5。
