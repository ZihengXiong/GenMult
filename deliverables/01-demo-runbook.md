# AgentHub Demo 运行手册

> 目标:任何人按本手册能在本地把 AgentHub 跑起来,并复现答辩 Demo 的完整链路。

## 1. 环境要求

- macOS / Linux,已安装 **Docker**(Desktop 即可)与 [**mise**](https://mise.jdx.dev/)
- 端口占用:`19082`(Web)、`19080`(Server API)、`17333/17334`(Qdrant)
- 一个 **DeepSeek**(或任意 Anthropic 兼容)API Key —— cc(Claude Code)Agent 走它

## 2. 一键启动(SQLite 开发栈)

```bash
# 仓库根目录
mise run dev          # = docker compose -f devenv/docker-compose.sqlite.yml up --build
```

首次构建约 3–5 分钟。起来后会有四个容器:

| 容器 | 作用 | 端口 |
|---|---|---|
| `memoh-dev-sqlite-web` | 前端(Vite 热更新) | `19082 → 8082` |
| `memoh-dev-sqlite-server` | Go 后端 / Orchestrator / CLI Agent 宿主 | `19080 → 8080` |
| `memoh-dev-sqlite-sparse` | 稀疏检索服务 | `19085` |
| `memoh-dev-sqlite-qdrant` | 向量库 | `17333` |

常用运维:

```bash
mise run dev:logs:sqlite                 # 跟日志
mise run dev:restart:sqlite -- server    # 只重启后端
mise run dev:restart:sqlite -- web       # 只重启前端
mise run dev:down:sqlite                 # 停掉整套
```

> 前端是 Vite 热更新(仓库以卷挂载进容器),改 `.vue` 立即生效,无需重启。

## 3. 登录

浏览器打开 **http://localhost:19082** ,用默认管理员账号登录:

```
用户名:admin
密码:  admin123
```

## 4. Demo 前置:配置 cc 的凭证(只需一次)

cc(Claude Code Agent)在宿主用 `claude` CLI 跑,凭证从「**通用设置**」里读,不依赖任何环境变量。

1. 进入 **通用设置 → 模型/Provider**
2. 新增一个 **Anthropic 类型** Provider:
   - `base_url`: `https://api.deepseek.com/anthropic`
   - `auth_token`: 你的 DeepSeek Key
   - 选一个 DeepSeek 模型(如 `deepseek-chat`)并启用
3. ds(Memoh Agent)用同一套 Memoh 框架配置(项目内已就绪)

> 验证凭证生效:在单聊里 @cc 发一句"你好",有真实回复即 OK。

## 5. Demo 脚本(答辩演示动线,约 3 分钟)

> 建议提前建好一个含 **cc + ds** 两个成员的群(本仓库已有现成房间 `123131`)。

### ① 单聊 + 流式 + 富媒体(体验,~40s)
1. 新建对话 → 选 **Claude Code** → 发:`用 Rust 写一个快速排序并加单元测试`
2. **看点**:回复**实时逐字流式**渲染(R5)、代码块高亮 + 一键复制、`查看思考过程` 折叠区。

### ② 群聊编排 + 自动分派(核心,~70s)
1. 进入群 `123131`(成员 cc + ds)
2. 在输入框发:`@主Agent 做一个待办列表小工具:ds 写前端,cc 写后端 Rust 接口`
   (或点「发起任务」直接给目标)
3. **看点**:
   - Orchestrator 输出**任务规划**消息(拆成多个子任务 DAG,R6 动态分派)
   - 每个子任务 `▶️ 开始执行` → 运行中出现**实时过程气泡**(thinking/工具调用,R9)
   - cc / ds **依次在聊天流里给出各自产出**
   - 结束有 `✅ 本次多 Agent 协作已完成` 聚合汇报

### ③ 长期上下文 pin(亮点,~20s)
1. 把某条关键约定(如"所有代码必须用 Rust")**置顶**(消息悬浮 → 📌)
2. 再发一轮任务,**看点**:被 pin 的内容作为长期上下文注入,Agent 持续遵守(R3)

### ④ 共享工作区:文件 + 终端(亮点,~30s)
1. 群聊右上角点 **文件** → 浏览 cc/ds 在**共享工作目录**里写的产物(R4.1 + R8)
2. 点 **终端** → 跑 `ls -la`、`cat <文件>`,在浏览器里直接看工作区

### ⑤(可选)富媒体卡片
- Diff 视图卡片、网页/Artifact 预览卡片、工具调用渲染,在上面的产出里自然出现。

## 6. 录屏建议(3 分钟视频)

- 分辨率拉到能看清代码;按 §5 的 ①→④ 走一遍。
- 旁白主线:**"IM 范式 → 单聊流式 → 群聊 Orchestrator 自动分派 → 实时过程可见 → 共享工作区"**。
- 详细分镜见 `05-demo-video-script.md`。

## 7. 故障速查

| 现象 | 原因 / 处理 |
|---|---|
| cc 回复空 / 报 `ANTHROPIC_API_KEY` | 通用设置里没配 Anthropic 兼容 Provider,见 §4 |
| 群聊 run 卡 `dispatching` 不动 | 前端轮询驱动 reconcile;确认停在该房间页面;或 `mise run dev:restart:sqlite -- server` |
| 端口被占 | 关掉占用 `19082/19080` 的进程,或改 compose 端口 |
| 前端改了不生效 | 一般会热更新;必要时 `mise run dev:restart:sqlite -- web` |
