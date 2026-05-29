# GenMult

GenMult is a self-hosted multi-agent workspace for building, running, and coordinating AI assistants across web, desktop, and external chat channels.

It started from the idea of giving every agent its own long-running context, tools, files, memory, and delivery channels. The current project has grown into a full-stack system with a Go backend, a Vue web client, an Electron desktop app, containerized agent workspaces, AgentHub collaboration rooms, and channel adapters such as WhatsApp.

## What GenMult Does

- Run multiple AI agents with separate identities, models, settings, memory, and permissions.
- Chat with agents from the web UI, desktop shell, local sessions, and connected channels.
- Use AgentHub rooms to coordinate humans and agents in the same workspace.
- Give agents isolated containers for file operations, command execution, MCP tools, and longer-running work.
- Keep conversation history, memory, scheduled tasks, and channel routes in a database.
- Configure providers, models, channels, storage, containers, and tools from a graphical interface.
- Connect external channels including Telegram, Discord, Feishu/Lark, DingTalk, WeChat-family adapters, Matrix, Email, and WhatsApp.

## Main Components

| Component | Stack | Default Port | Purpose |
| --- | --- | --- | --- |
| Server | Go, Echo, Twilight AI SDK | 8080 | REST API, auth, database, agents, channels, containers |
| Web | Vue 3, Vite, Pinia, Tailwind CSS | 8082 | Browser-based control panel and chat workspace |
| Desktop | Electron, electron-vite | local app | Desktop shell that reuses the web client and local server |
| Database | PostgreSQL or SQLite | varies | Users, bots, rooms, messages, memory, providers, settings |
| Vector / Search | Qdrant, sparse search | optional | Long-term memory retrieval and hybrid search |
| Containers | Docker, containerd, Kubernetes, Apple Virtualization | varies | Isolated workspaces for agent tools and files |

## Features

### Agent Workspace

- Multi-agent and multi-user chat.
- Per-agent model, provider, memory, channel, and permission settings.
- AgentHub rooms for shared collaboration, mentions, room messages, and agent membership.
- Long-term memory providers with history compaction and retrieval hooks.
- Scheduled tasks, heartbeat sessions, discuss mode, and subagent workflows.

### Channels

- Built-in web and local chat.
- WhatsApp support with QR login, media handling, group routing, and bot mention behavior.
- Channel adapters for Telegram, Discord, Feishu/Lark, DingTalk, QQ, Matrix, Misskey, WeCom, WeChat, WeChat Official Account, and Email.
- Source-aware access control for deciding who can trigger which agent.

### Tools And Runtime

- MCP support for external tool servers.
- Container-backed command execution and file editing.
- File manager, attachments, media assets, and browser-facing workspace views.
- Provider configuration for OpenAI-compatible APIs, Anthropic, Google, GitHub Copilot, Codex-style clients, TTS, and search providers.

### Apps

- Web dashboard for agent management, chat, settings, channels, and AgentHub.
- Electron desktop app for a local client experience.
- Docker Compose deployment for server, web, database, migrations, and optional memory services.

## Quick Start

Requirements:

- Docker and Docker Compose
- Git

Clone the repository:

```bash
git clone https://github.com/ZihengXiong/GenMult.git
cd GenMult
```

Create a local config file:

```bash
cp conf/app.docker.toml config.toml
```

Edit `config.toml` for your admin account, database, model providers, container runtime, and channel settings.

Start the stack:

```bash
docker compose up -d
```

Open the web client:

```text
http://localhost:8082
```

The backend API listens on:

```text
http://localhost:8080
```

## Desktop Development

Install frontend dependencies:

```bash
corepack enable
pnpm install
```

Run the desktop app in development mode:

```bash
pnpm --filter @memohai/desktop dev
```

Build desktop packages:

```bash
pnpm --filter @memohai/desktop build
```

The desktop package still reuses the web renderer internally, so web UI changes are shared by both browser and desktop builds.

## Web Development

Run the web client:

```bash
pnpm --filter @memohai/web dev
```

Build the web client:

```bash
pnpm --filter @memohai/web build
```

## Backend Development

Run the Go server locally:

```bash
go run ./cmd/agent
```

Regenerate database code after SQL changes:

```bash
mise run sqlc-generate
```

Generate OpenAPI and TypeScript SDK artifacts after API changes:

```bash
mise run swagger-generate
mise run sdk-generate
```

## Repository Layout

```text
cmd/                 Go entry points for server, bridge, MCP, and CLI
internal/            Backend domain modules
apps/web/            Vue web client
apps/desktop/        Electron desktop shell
packages/            Shared UI, SDK, icons, and config packages
db/                  PostgreSQL and SQLite migrations and queries
conf/                Example configuration and provider templates
docker/              Production Dockerfiles and nginx config
docs/                Documentation site
deploy/              Kubernetes deployment examples
```

## Security Notes

- Do not commit `.env`, `config.toml`, keys, local database files, generated backups, or deployment secrets.
- Channel credentials and model provider API keys should stay in local config, environment variables, or a secret manager.
- Each agent workspace should be treated as an execution boundary. Review container privileges before exposing a deployment publicly.
- For production deployments, change the default admin password, enable HTTPS at the reverse proxy, and restrict inbound ports.

## Current Status

GenMult is under active development. Some internal package names and historical paths may still use the original module naming while the product identity moves toward GenMult.

## License

See [LICENSE](./LICENSE).
