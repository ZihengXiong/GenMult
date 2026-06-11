# Codex 适配器启用验证结论（2026-06-11）

实测环境：codex-cli 0.139.0（npm @openai/codex），DeepSeek API（.env 凭证）。

## 实测结论

1. **环境变量注入失效（已修复）**：codex ≥0.139 完全忽略 `OPENAI_BASE_URL` /
   `OPENAI_API_KEY` 环境变量——实测它直接使用了本机的 ChatGPT 登录态。
   适配器原来靠 `CodexEnv` 导出环境变量配置端点的方式从根上不生效。
   修复：`CodexBuildArgs` 在配置了 `base_url` 时注入 `-c` 配置覆盖
   （`model_providers.custom.{name,base_url,env_key}` + `model_provider`），
   API key 仍经 `OPENAI_API_KEY` 环境变量传递（`env_key` 指向它）。
   实测注入生效：请求落在配置的 host 上。

2. **DeepSeek 当前无法支撑 codex（上游限制，无法在本仓库内修复）**：
   - codex 只讲 Responses API：`POST {base_url}/responses`；
   - DeepSeek `/v1` 无该端点（实测 404）；
   - `wire_api = "chat"`（chat-completions 模式）已被 codex 上游移除
     （实测报错并指向 openai/codex#7782）。

## 启用路径（满足其一）

- 使用真实 OpenAI key（api.openai.com 原生支持 /responses）；
- 在 DeepSeek 前架一个 chat→responses 翻译代理（如 LiteLLM proxy 的
  `/v1/responses` 端点）并把 bot 的 codex `base_url` 指向代理。
  注意 `scripts/dev_codex_relay.py` 只是透明转发，不做协议翻译，不能用于此。

## 当前行为

满足启用路径前，codex 任务会以清晰错误失败（errorDetailFromStderr 会把
404/认证细节带回 run 事件），编排会按不可重试处理——不会静默空转。
