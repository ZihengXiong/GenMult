#!/usr/bin/env bash
# e2e-agenthub.sh — 真实服务的 AgentHub 端到端验证（本地、免 Docker、免 LLM）。
#
# 流程：构建 server → 隔离 sqlite 配置 → migrate → serve → 登录 →
# 建房间 → 发起带确认闸的 run → 断言轮询击不穿闸 → SSE 订阅 →
# confirm → 断言完成 + 推送送达 + 房间消息投影完整 → 清理。
#
# 用法：scripts/e2e-agenthub.sh   （全程 < 1 分钟，退出码 0 = PASS）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/agenthub-e2e.XXXXXX")"
PORT="${E2E_PORT:-18807}"
BASE="http://127.0.0.1:$PORT"
SERVER_PID=""
SSE_PID=""

cleanup() {
  [ -n "$SSE_PID" ] && kill "$SSE_PID" 2>/dev/null || true
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

fail() { echo "FAIL: $1" >&2; exit 1; }

echo "== build =="
go build -o "$WORK/memoh-server" ./cmd/agent

echo "== config + migrate =="
python3 - "$ROOT/conf/app.local.toml" "$WORK" "$PORT" <<'EOF'
import sys
src, work, port = open(sys.argv[1]).read(), sys.argv[2], sys.argv[3]
src = src.replace('addr = ":18731"', f'addr = "127.0.0.1:{port}"')
src = src.replace('path = "data/local/memoh.db"', f'path = "{work}/memoh.db"')
src = src.replace('data_root = "data/local"', f'data_root = "{work}/data"')
src = src.replace('runtime_dir = "data/runtime"', f'runtime_dir = "{work}/runtime"')
src = src.replace('metadata_root = "data/local/containers"', f'metadata_root = "{work}/containers"')
open(f'{work}/config.toml', 'w').write(src)
EOF
CONFIG_PATH="$WORK/config.toml" "$WORK/memoh-server" migrate up >"$WORK/migrate.log" 2>&1

echo "== serve =="
CONFIG_PATH="$WORK/config.toml" "$WORK/memoh-server" serve >"$WORK/server.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 30); do
  curl -sf "$BASE/ping" >/dev/null 2>&1 && break
  sleep 1
done
curl -sf "$BASE/ping" >/dev/null || { tail -20 "$WORK/server.log" >&2; fail "server did not become ready"; }

echo "== login =="
TOKEN=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('token') or d.get('access_token') or '')")
[ -n "$TOKEN" ] || fail "login returned no token"
AUTH=(-H "Authorization: Bearer $TOKEN")

echo "== create room =="
ROOM_ID=$(curl -s -X POST "$BASE/agent-hub/rooms" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"name":"e2e 自动验证"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
[ -n "$ROOM_ID" ] || fail "room creation failed"

echo "== subscribe SSE =="
curl -sN "$BASE/agent-hub/rooms/$ROOM_ID/runs/events" "${AUTH[@]}" >"$WORK/sse.log" 2>&1 &
SSE_PID=$!
sleep 1

echo "== start gated run =="
RUN_JSON=$(curl -s -X POST "$BASE/agent-hub/rooms/$ROOM_ID/runs" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"objective":"e2e 验证确认闸与推送","auto_dispatch":false}')
RUN_ID=$(echo "$RUN_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['run']['id'])")
echo "$RUN_JSON" | python3 -c "
import sys, json
d = json.load(sys.stdin)
assert d['run'].get('metadata', {}).get('await_confirmation') is True, 'hold flag missing'
" || fail "await_confirmation hold not set"

echo "== reconcile must not breach the gate =="
for _ in 1 2 3; do
  curl -s -X POST "$BASE/agent-hub/runs/$RUN_ID/reconcile" "${AUTH[@]}" >"$WORK/rec.json"
done
python3 - "$WORK/rec.json" <<'EOF'
import sys, json
d = json.load(open(sys.argv[1]))
attempts = d.get('attempts') or []
assert len(attempts) == 0, f'gate breached: {len(attempts)} attempts'
assert d['run']['status'] not in ('completed', 'failed', 'cancelled'), d['run']['status']
EOF
[ $? -eq 0 ] || fail "gate breached by reconcile polling"

echo "== confirm =="
curl -s -X POST "$BASE/agent-hub/runs/$RUN_ID/confirm" "${AUTH[@]}" >/dev/null
STATUS=""
for _ in $(seq 1 20); do
  STATUS=$(curl -s "$BASE/agent-hub/runs/$RUN_ID" "${AUTH[@]}" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['run']['status'])")
  [ "$STATUS" = "completed" ] && break
  sleep 1
done
[ "$STATUS" = "completed" ] || fail "run did not complete after confirm (status=$STATUS)"

echo "== assert push + projection =="
sleep 1
grep -q '"status":"completed"' "$WORK/sse.log" || fail "SSE never delivered the completed event"
MSGS=$(curl -s "$BASE/agent-hub/rooms/$ROOM_ID/messages" "${AUTH[@]}")
echo "$MSGS" | python3 -c "
import sys, json
titles = ' '.join(m['title'] + m['body'] for m in json.load(sys.stdin)['items'])
assert '任务规划' in titles, 'planning message missing'
assert '协作完成' in titles, 'completion message missing'
" || fail "room projection incomplete"

echo "PASS: gate held, confirm dispatched, SSE delivered, projection complete"
