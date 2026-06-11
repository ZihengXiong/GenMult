# 测试记录（Test Record）

记录最近一次全量验证通过的基线 commit 与各目录对应的测试套件。
**基线之后没有改动的目录不需要重复测试**；改动了哪些文件，就重跑对应套件
（用 `scripts/test-changed.sh` 自动判断并执行）。

<!-- last-verified-commit: 90bb31a9c1ae3e7bdfbcc98fef431947eaf7fe51 -->

## 最近一次全量验证

- **基线 commit**: `148b9ff` （branch `feat/agenthub-projection-idempotency`；全量套件实测于 `a0aa049`，其后到基线仅 docs/脚本改动，无需套件）
- **日期**: 2026-06-11
- **结论**: 全部通过

| 范围（文件夹） | 套件 / 命令 | 结果 |
|---|---|---|
| 全部 Go 包（`internal/`、`cmd/`、`db/` 等） | `go build ./...` && `go test ./...` | ✅ 全部通过 |
| 全部 Go 包 | `mise exec golangci-lint@2.10.1 -- golangci-lint run ./internal/... ./cmd/...` | ✅ 0 问题 |
| 前端/TS（`apps/`、`packages/`） | `pnpm vitest run`（需 mise PATH，见下） | ✅ 10 文件 97 用例 |
| 前端/TS | `pnpm eslint .` | ✅ 0 问题 |
| 真实 LLM 链路（`internal/agenthub/`） | `LIVE_LLM_TEST=1 go test ./internal/agenthub/ -run TestLive -v -count=1`（先 `set -a; source .env; set +a`） | ✅ 4/4（deepseek-v4-flash，花费可忽略） |

## 增量重测规则

1. 改动后运行 `scripts/test-changed.sh`：它对比上面的基线 commit（含未提交改动），
   自动只跑受影响的套件。
2. 全部通过后运行 `scripts/test-changed.sh --update` 把基线刷新为当前 HEAD
   （需要工作区干净），并更新本文件上方的表格日期。
3. 路径 → 套件映射：
   - `*.go` / `go.mod` / `go.sum` / `db/` / `sqlc.yaml` → Go build + test（Go 自带测试缓存，
     未变包会命中缓存，整体很快）+ golangci-lint（只 lint 改动的包）
   - `apps/` / `packages/` / 根部 `*.ts`、`*.mjs`、`package.json` 等 → vitest + eslint
   - 仅 `*.md` / `docs/` / `deliverables/` → 无需测试
4. 真实 LLM 测试（`TestLive*`）只在改动了 `internal/agenthub/llm_planner*.go`、
   `internal/agenthub/orchestrator/planner.go` 或 `internal/models/sdk.go` 时需要重跑；
   它有双重门控（`LIVE_LLM_TEST=1` + env 凭证），CI / pre-commit 自动跳过，不烧 token。
   **务必使用 flash/nano 档便宜模型**（默认读 `.env` 的 `MODEL_NAME=deepseek-v4-flash`）。

## 注意事项

- 直接跑 `pnpm` 可能因 corepack/Node 版本不匹配崩溃（`ERR_VM_DYNAMIC_IMPORT_CALLBACK_MISSING`）。
  用 husky 钩子同款 PATH：
  `export PATH="$HOME/.local/share/mise/shims:$HOME/.local/bin:$HOME/.npm-global/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"`
- husky pre-commit 已并行跑 check-go / check-go-test / check-web，提交本身就是一道增量门禁。
