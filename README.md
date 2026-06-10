# AgentHub — Multi-Agent Collaboration Platform

> AI Full-Stack Challenge entry · Team **启灵 (Qiling)** — 齐紫瀚, 熊子恒, 张桐铖
>
> 中文版:[README_CN.md](README_CN.md)

AgentHub makes "driving a team of AI agents" feel as natural as a group chat: agents are room members; @-mention the lead agent to auto-decompose and dispatch work, or @ a specific member to assign it directly. Artifacts land in a shared room workspace, and the whole collaboration is visible in the chat stream in real time. The platform plugs in multiple agents through one adapter layer — **memoh** (built-in framework agent in this repo), **Claude Code**, and **Codex** (CLI-backed) — coordinated by an Orchestrator DAG engine.

This repository is built on the memoh full-stack platform (Go backend + Vue web + Electron desktop + containerized agent workspaces). AgentHub is its multi-agent collaboration capability; memoh is one of the agents that run inside it.

---

## 📦 Deliverables (3 documents)

The three final documents live in [`deliverables/`](deliverables/) as PDFs:

| Document | File | Contents |
|---|---|---|
| Product Design | [`agenthub-product-design.pdf`](deliverables/agenthub-product-design.pdf) | Positioning, the group-chat mental model, multi-agent failure modes & mitigations, target users & scenarios, core UX decisions, feature matrix, roadmap |
| Technical Design | [`agenthub-technical-design.pdf`](deliverables/agenthub-technical-design.pdf) | Layered architecture, data model (full DDL), orchestration state machine & scheduling, two-tier Planner, unified adapter layer, credential chain, event projection, three core sequence flows, HTTP API, test gates |
| AI Collaboration Record | [`agenthub-ai-collaboration.pdf`](deliverables/agenthub-ai-collaboration.pdf) | Human–AI turn-based model, Rules / Spec / Plan / Skill conventions, three deep collaboration cases, real session archive (19 sessions / 277 user turns / 2582 tool calls) |

### Rubric navigation

| Dimension | Weight | Where to verify |
|---|---|---|
| AI collaboration | 30% | [AI Collaboration Record](deliverables/agenthub-ai-collaboration.pdf) + git log (R1–R9 feature commits) |
| Feature completeness | 25% | Run the demo below; feature matrix in the [Product Design](deliverables/agenthub-product-design.pdf) |
| Output quality | 20% | Demo video + screenshots below |
| Code understanding | 15% | [Technical Design](deliverables/agenthub-technical-design.pdf) (architecture / state machines / sequences) + `internal/agenthub/**` |
| Innovation & product sense | 10% | Product doc: failure-mode mitigations, pinned long-term context, shared workspace, live process bubbles |

---

## 🎬 Demo video (3 minutes)

Quark drive: **<https://pan.quark.cn/s/4e9ebc5c07b7>** ("启灵-agent")

---

## 🖼 Screenshots

| Single chat: token streaming + thinking / tool trace | Group orchestration: task-plan card |
|---|---|
| ![Single-chat streaming](deliverables/screenshots/01-single-streaming.png) | ![Task plan](deliverables/screenshots/03-run-plan.png) |
| **Live process bubble of a running task** | **Shared room workspace: in-browser terminal** |
| ![Live bubble](deliverables/screenshots/04-run-live-bubble.png) | ![Workspace terminal](deliverables/screenshots/08-workspace-terminal.png) |

---

## ▶️ Run the demo (local)

Prerequisites: **Docker / Docker Compose** and **[mise](https://mise.jdx.dev/)** (manages tasks plus Go / Node / pnpm / sqlc tool versions).

```bash
# 1. Install dependencies and toolchain
mise run setup

# 2. Start the dev environment (docker compose, SQLite, auto-build)
mise run dev
```

Then open the web console:

```text
http://localhost:19082
```

> The web port can be overridden via `MEMOH_SQLITE_DEV_WEB_PORT` (default 19082).
> Stop with `mise run dev:down:sqlite`; list all tasks (including `dev:postgres`) with `mise tasks`.

The demo walkthrough (single-chat token streaming, live process bubbles during group-chat orchestration, pinned long-term context, and the shared workspace file browser + in-browser terminal) is described in the Product / Technical documents.

---

## Project overview

- **Backend** Go (Echo) · **Frontend** Vue 3 + Vite · **Desktop** Electron · **Storage** SQLite / PostgreSQL · **Retrieval** Qdrant
- **Agent integration**: memoh (built-in framework agent) / Claude Code / Codex (CLI-backed), one adapter layer + Orchestrator DAG engine, LLM + rule-based two-tier Planner
- **Collaboration**: AgentHub rooms, @-mentions, shared workspace, pinned long-term context, event sourcing, live in-progress process bubbles, failure degradation, and crash self-healing

Full architecture, data model, and core sequence flows are in [`deliverables/agenthub-technical-design.pdf`](deliverables/agenthub-technical-design.pdf).

## License

See [LICENSE](./LICENSE).
