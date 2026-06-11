#!/usr/bin/env sh
# test-changed.sh — 按改动增量重测（配套 TESTING.md 的测试记录）。
#
# 对比 TESTING.md 中记录的“最近一次全量验证通过”的基线 commit（外加未提交改动），
# 推断哪些测试套件需要重跑并执行；没改的范围直接跳过。
#
# 用法：
#   scripts/test-changed.sh            # 检测改动并运行所需套件
#   scripts/test-changed.sh --dry-run  # 只打印将要运行的套件
#   scripts/test-changed.sh --update   # （工作区干净且测试已过后）把基线刷新为 HEAD
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
RECORD="TESTING.md"

BASELINE="$(sed -n 's/.*last-verified-commit: \([0-9a-f]\{40\}\).*/\1/p' "$RECORD" | head -n1)"
if [ -z "$BASELINE" ]; then
  echo "error: no last-verified-commit marker in $RECORD" >&2
  exit 1
fi

if [ "${1:-}" = "--update" ]; then
  if [ -n "$(git status --porcelain)" ]; then
    echo "error: working tree not clean; commit first, then --update" >&2
    exit 1
  fi
  HEAD_SHA="$(git rev-parse HEAD)"
  sed -i '' "s/last-verified-commit: [0-9a-f]\{40\}/last-verified-commit: $HEAD_SHA/" "$RECORD"
  echo "baseline updated: $HEAD_SHA (remember to update the date/table in $RECORD and commit)"
  exit 0
fi

# Changed files: committed since baseline + staged + unstaged + untracked.
CHANGED="$( { git diff --name-only "$BASELINE"...HEAD 2>/dev/null || git diff --name-only "$BASELINE" HEAD; \
              git diff --name-only; git diff --name-only --cached; \
              git ls-files --others --exclude-standard; } | sort -u )"
if [ -z "$CHANGED" ]; then
  echo "no changes since baseline ${BASELINE%????????????????????????????????} — nothing to test"
  exit 0
fi

echo "changed since baseline:"
echo "$CHANGED" | sed 's/^/  /'

NEED_GO=0; NEED_WEB=0; NEED_LIVE=0
for f in $CHANGED; do
  case "$f" in
    *.md|docs/*|deliverables/*|assets/*|scripts/*|.husky/*|spec/*) ;;  # docs/tooling — no suites
    *.go|go.mod|go.sum|db/*|sqlc.yaml) NEED_GO=1 ;;
    apps/*|packages/*|*.ts|*.mjs|*.vue|package.json|pnpm-lock.yaml|eslint.config.mjs|vitest.config.ts|tsconfig.json) NEED_WEB=1 ;;
    *) NEED_GO=1; NEED_WEB=1 ;;                                   # unknown — be safe
  esac
  case "$f" in
    internal/agenthub/llm_planner*.go|internal/agenthub/orchestrator/planner.go|internal/models/sdk.go) NEED_LIVE=1 ;;
  esac
done

echo ""
echo "suites to run: go=$NEED_GO web=$NEED_WEB live-llm=$NEED_LIVE(manual)"
if [ "${1:-}" = "--dry-run" ]; then
  exit 0
fi

FAIL=0
if [ "$NEED_GO" = 1 ]; then
  echo ""; echo "== go build + test =="
  go build ./... || FAIL=1
  go test ./... || FAIL=1
  echo "== golangci-lint =="
  mise exec golangci-lint@2.10.1 -- golangci-lint run ./internal/... ./cmd/... || FAIL=1
fi
if [ "$NEED_WEB" = 1 ]; then
  echo ""; echo "== vitest + eslint =="
  export PATH="$HOME/.local/share/mise/shims:$HOME/.local/bin:$HOME/.npm-global/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"
  pnpm vitest run || FAIL=1
  pnpm eslint . || FAIL=1
fi
if [ "$NEED_LIVE" = 1 ]; then
  echo ""
  echo "note: LLM 规划路径有改动，建议手动跑真实 API 测试（花费极小，flash 模型）："
  echo "  set -a; . ./.env; set +a; LIVE_LLM_TEST=1 go test ./internal/agenthub/ -run TestLive -v -count=1"
fi

if [ "$FAIL" = 1 ]; then
  echo ""; echo "RESULT: FAILED — fix before updating the baseline"; exit 1
fi
echo ""; echo "RESULT: OK — after committing, refresh the baseline with: scripts/test-changed.sh --update"
