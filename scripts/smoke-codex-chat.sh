#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://127.0.0.1:18080}"
ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin123}"
OPENAI_BASE_URL="${OPENAI_BASE_URL:?OPENAI_BASE_URL is required}"
OPENAI_API_KEY="${OPENAI_API_KEY:?OPENAI_API_KEY is required}"
MODEL_NAME="${MODEL_NAME:?MODEL_NAME is required}"
PROMPT="${PROMPT:-Reply with exactly: CODEx smoke ok}"
NO_PROXY_ARG=(--noproxy '*')

require_bin() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_bin curl
require_bin python3

login_json="$(
  curl "${NO_PROXY_ARG[@]}" -sS \
    -X POST "$API_URL/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$ADMIN_USERNAME\",\"password\":\"$ADMIN_PASSWORD\"}"
)"

token="$(printf '%s' "$login_json" | python3 -c 'import sys, json; print(json.load(sys.stdin)["access_token"])')"
stamp="$(date +%s)"
provider_name="deepseek-openai-$stamp"
bot_name="codex-smoke-$stamp"

auth_header=(-H "Authorization: Bearer $token")

provider_json="$(
  curl "${NO_PROXY_ARG[@]}" -sS \
    -X POST "$API_URL/providers" \
    "${auth_header[@]}" \
    -H 'Content-Type: application/json' \
    -d "{
      \"name\": \"$provider_name\",
      \"client_type\": \"openai-completions\",
      \"config\": {
        \"api_key\": \"$OPENAI_API_KEY\",
        \"base_url\": \"$OPENAI_BASE_URL\"
      }
    }"
)"
provider_id="$(printf '%s' "$provider_json" | python3 -c 'import sys, json; print(json.load(sys.stdin)["id"])')"

provider_test_json="$(
  curl "${NO_PROXY_ARG[@]}" -sS \
    -X POST "$API_URL/providers/$provider_id/test" \
    "${auth_header[@]}"
)"

model_json="$(
  curl "${NO_PROXY_ARG[@]}" -sS \
    -X POST "$API_URL/models" \
    "${auth_header[@]}" \
    -H 'Content-Type: application/json' \
    -d "{
      \"model_id\": \"$MODEL_NAME\",
      \"name\": \"$MODEL_NAME\",
      \"provider_id\": \"$provider_id\",
      \"type\": \"chat\",
      \"config\": {}
    }"
)"
model_id="$(printf '%s' "$model_json" | python3 -c 'import sys, json; print(json.load(sys.stdin)["id"])')"

bot_json="$(
  curl "${NO_PROXY_ARG[@]}" -sS \
    -X POST "$API_URL/bots" \
    "${auth_header[@]}" \
    -H 'Content-Type: application/json' \
    -d "{
      \"display_name\": \"$bot_name\",
      \"is_active\": true,
      \"acl_preset\": \"allow_all\",
      \"framework\": \"codex\"
    }"
)"
bot_id="$(printf '%s' "$bot_json" | python3 -c 'import sys, json; print(json.load(sys.stdin)["id"])')"

curl "${NO_PROXY_ARG[@]}" -sS \
  -X PUT "$API_URL/bots/$bot_id/settings" \
  "${auth_header[@]}" \
  -H 'Content-Type: application/json' \
  -d "{\"chat_model_id\":\"$model_id\"}" >/dev/null

curl "${NO_PROXY_ARG[@]}" -sS \
  -X POST "$API_URL/bots/$bot_id/web/messages" \
  "${auth_header[@]}" \
  -H 'Content-Type: application/json' \
  -d "{
    \"message\": {
      \"text\": \"$PROMPT\"
    }
  }" >/dev/null

assistant_text=""
for _ in $(seq 1 20); do
  sleep 1
  messages_json="$(
    curl "${NO_PROXY_ARG[@]}" -sS \
      "$API_URL/bots/$bot_id/messages?limit=20" \
      "${auth_header[@]}"
  )"
  assistant_text="$(
    printf '%s' "$messages_json" | python3 -c '
import json, sys
items = (json.load(sys.stdin).get("items") or [])
assistant = ""
for item in items:
    if item.get("role") != "assistant":
        continue
    content = item.get("content") or {}
    parts = content.get("content") or []
    texts = [part.get("text", "") for part in parts if part.get("type") == "text" and part.get("text")]
    if texts:
        assistant = texts[-1]
print(assistant)
'
  )"
  if [ -n "$assistant_text" ]; then
    break
  fi
done

if [ -z "$assistant_text" ]; then
  echo "assistant reply not found in bot history" >&2
  exit 1
fi

echo "provider_id=$provider_id"
echo "provider_test=$provider_test_json"
echo "model_id=$model_id"
echo "bot_id=$bot_id"
echo "assistant=$assistant_text"
