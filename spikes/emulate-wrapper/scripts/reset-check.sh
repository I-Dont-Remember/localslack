#!/usr/bin/env bash
# P1 extra: after POST /_localslack/reset the same calls yield the same IDs/ts (fresh slack module instance resets tsCounter).
set -euo pipefail
cd "$(dirname "$0")/.."
run() {
	local H="Authorization: Bearer xoxb-x" c
	c=$(curl -s -H "$H" -d name=reset-probe "$LOCALSLACK_URL/api/conversations.create" | python3 -c 'import sys,json;print(json.load(sys.stdin)["channel"]["id"])')
	curl -s -H "$H" -d "channel=$c" -d text=x "$LOCALSLACK_URL/api/chat.postMessage" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["ts"])'
	echo "$c"
}
first=$(run); curl -s -XPOST "$LOCALSLACK_URL/_localslack/reset" >/dev/null; second=$(run)
echo "first : $(echo $first)"; echo "second: $(echo $second)"
[ "$first" = "$second" ] && echo "RESET-DETERMINISTIC" || { echo "RESET-DIVERGED"; exit 1; }
