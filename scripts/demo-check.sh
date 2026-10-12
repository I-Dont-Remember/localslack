#!/usr/bin/env bash
# Scripted assertion for `make demo-check`: a human mentions the demo bot and gets "pong" in a thread.
# Usage: demo-check.sh <localslack url> [log dir]
set -euo pipefail

url="$1"
logs="${2:-}"
api="$url/api"

demo="$(curl -sf "$url/_localslack/apps" | jq -c '.items[] | select(.name == "Demo Bolt")')"
app_id="$(jq -r .id <<<"$demo")"
bot_user="$(jq -r .bot_user_id <<<"$demo")"
bot_token="$(jq -r .credentials.bot_token <<<"$demo")"
human="$(curl -sf -H "Authorization: Bearer $bot_token" "$api/users.list" | jq -r '[.members[] | select(.is_bot | not)][0].id')"
human_auth="Authorization: Bearer xoxp-$human"
general="$(curl -sf -H "$human_auth" "$api/conversations.list?types=public_channel" | jq -r '.channels[] | select(.name == "general") | .id')"

sent="$(curl -sf -H "$human_auth" --data-urlencode "channel=$general" --data-urlencode "text=<@$bot_user> ping" "$api/chat.postMessage")"
ts="$(jq -r .ts <<<"$sent")"

fail() { echo "demo-check FAILED: $1" >&2; [ -n "$logs" ] && tail -n 30 "$logs"/*.log >&2 || true; exit 1; }

curl -sf "$url/_localslack/deliveries?app_id=$app_id&event_type=app_mention&state=delivered&min=1&timeout_ms=15000" |
	jq -e '.items | length >= 1' >/dev/null || fail "app_mention was never delivered"
for _ in $(seq 1 50); do
	reply="$(curl -sf -H "$human_auth" "$api/conversations.replies?channel=$general&ts=$ts" | jq -r '[.messages[] | select(.text == "pong")] | length')"
	[ "$reply" -ge 1 ] && { echo "demo-check OK: mention delivered, bot replied \"pong\" in the thread"; exit 0; }
	sleep 0.2
done
fail "no pong reply in the thread"
