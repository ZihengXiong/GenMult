# GenMult

GenMult 是一个自托管的多 Agent 工作台，用来创建、运行和协作管理多个 AI 助手。它同时提供 Web 客户端、桌面端、后端服务、AgentHub 协作房间、长期记忆、容器工作区和多渠道接入能力。

这个项目现在已经不只是一个普通聊天机器人管理面板。它更像一个可以长期运行的 AI 工作环境：每个 Agent 都可以有自己的身份、模型、工具、文件系统、记忆、权限和外部消息渠道。

## GenMult 能做什么

- 创建多个 Agent，并为每个 Agent 单独配置模型、记忆、权限和渠道。
- 通过 Web、桌面端、本地会话和外部聊天平台与 Agent 对话。
- 使用 AgentHub 房间把人类成员和多个 Agent 放在同一个协作空间里。
- 为 Agent 分配隔离容器，让它们可以执行命令、读写文件、运行 MCP 工具和处理长期任务。
- 持久化保存会话历史、长期记忆、定时任务、渠道路由和配置。
- 在图形界面里管理模型供应商、机器人、渠道、存储、容器和工具。
- 接入 WhatsApp、Telegram、Discord、飞书、钉钉、微信系渠道、Matrix、邮件等外部渠道。

## 核心组件

| 组件 | 技术栈 | 默认端口 | 作用 |
| --- | --- | --- | --- |
| Server | Go, Echo, Twilight AI SDK | 8080 | REST API、鉴权、数据库、Agent、渠道、容器管理 |
| Web | Vue 3, Vite, Pinia, Tailwind CSS | 8082 | 浏览器里的控制台和聊天工作区 |
| Desktop | Electron, electron-vite | 本地应用 | 复用 Web 客户端的桌面端 |
| Database | PostgreSQL 或 SQLite | 视配置而定 | 用户、Agent、房间、消息、记忆、供应商、设置 |
| Vector / Search | Qdrant、稀疏检索服务 | 可选 | 长期记忆检索和混合搜索 |
| Containers | Docker、containerd、Kubernetes、Apple Virtualization | 视配置而定 | Agent 的隔离工具和文件工作区 |

## 功能概览

### Agent 工作区

- 多 Agent、多用户聊天。
- 每个 Agent 都有独立的模型、供应商、记忆、渠道和权限配置。
- AgentHub 支持协作房间、成员管理、消息流、Agent 加入和提及。
- 长期记忆、历史压缩和检索注入。
- 定时任务、心跳任务、讨论模式和子 Agent 工作流。

### 多渠道接入

- 内置 Web 和本地聊天。
- WhatsApp 支持扫码登录、媒体消息、群聊路由和提及触发。
- 支持 Telegram、Discord、飞书、钉钉、QQ、Matrix、Misskey、企业微信、微信、微信公众号和邮件。
- 支持按来源、身份、渠道和会话范围配置访问控制。

### 工具和运行环境

- 支持 MCP 外部工具服务。
- 支持容器内命令执行、文件编辑和工作区管理。
- 支持附件、媒体资产、文件管理器和可视化工作区。
- 支持 OpenAI 兼容接口、Anthropic、Google、GitHub Copilot、Codex 风格客户端、TTS 和搜索供应商。

### 客户端

- Web 控制台：管理 Agent、聊天、设置、渠道和 AgentHub。
- Electron 桌面端：提供本地客户端体验。
- Docker Compose：用于部署 server、web、数据库、迁移服务和可选记忆服务。

## 快速开始

需要先安装：

- Docker 和 Docker Compose
- Git

克隆仓库：

```bash
git clone https://github.com/ZihengXiong/GenMult.git
cd GenMult
```

创建本地配置：

```bash
cp conf/app.docker.toml config.toml
```

然后编辑 `config.toml`，配置管理员账号、数据库、模型供应商、容器运行时和渠道参数。

启动服务：

```bash
docker compose up -d
```

打开 Web 客户端：

```text
http://localhost:8082
```

后端 API 默认地址：

```text
http://localhost:8080
```

## 桌面端开发

安装前端依赖：

```bash
corepack enable
pnpm install
```

启动桌面端开发模式：

```bash
pnpm --filter @memohai/desktop dev
```

构建桌面端：

```bash
pnpm --filter @memohai/desktop build
```

桌面端目前复用 Web 渲染层，所以 Web 界面的改动也会同步反映到桌面端。

## Web 开发

启动 Web 开发服务：

```bash
pnpm --filter @memohai/web dev
```

构建 Web：

```bash
pnpm --filter @memohai/web build
```

## 后端开发

本地运行 Go 后端：

```bash
go run ./cmd/agent
```

修改 SQL 后重新生成数据库代码：

```bash
mise run sqlc-generate
```

修改 API 后重新生成 OpenAPI 和前端 SDK：

```bash
mise run swagger-generate
mise run sdk-generate
```

## 目录结构

```text
cmd/                 Go 服务入口、bridge、MCP 和 CLI
internal/            后端核心模块
apps/web/            Vue Web 客户端
apps/desktop/        Electron 桌面端
packages/            共享 UI、SDK、图标和配置包
db/                  PostgreSQL 和 SQLite 迁移与查询
conf/                配置模板和模型供应商模板
docker/              生产 Dockerfile 和 nginx 配置
docs/                文档站点
deploy/              Kubernetes 部署示例
```

## 安全说明

- 不要提交 `.env`、`config.toml`、密钥、本地数据库、生成备份或部署口令。
- 渠道凭据和模型 API Key 应保存在本地配置、环境变量或 Secret Manager 中。
- Agent 容器拥有执行能力，生产环境部署前要认真检查容器权限和对外暴露端口。
- 对外部署时请修改默认管理员密码，并在反向代理层配置 HTTPS。

## 当前状态

GenMult 仍在快速开发中。部分内部包名和历史路径可能暂时保留原始命名，后续会逐步整理为 GenMult 的项目身份。

## 许可证

见 [LICENSE](./LICENSE)。
