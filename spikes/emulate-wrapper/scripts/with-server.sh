#!/usr/bin/env bash
# Run a command against the emulate-wrapper on a free port. Exports LOCALSLACK_URL.
# Usage: scripts/with-server.sh <cmd...>   Env: LOCALSLACK_SEED, UNKNOWN_METHOD, LOCALSLACK_CMD (server command; default node src/server.mjs)
set -euo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"
[ $# -gt 0 ] || { echo "usage: $0 <cmd...>" >&2; exit 2; }
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')"
export PORT="$port" HOST=127.0.0.1 LOCALSLACK_URL="http://127.0.0.1:$port" NO_PROXY="127.0.0.1,localhost" no_proxy="127.0.0.1,localhost"
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy ALL_PROXY all_proxy  # SDK async client ignores NO_PROXY
export LOCALSLACK_SEED="${LOCALSLACK_SEED:-with-server-seed}"
log="$(mktemp)"
${LOCALSLACK_CMD:-node "$here/src/server.mjs"} >"$log" 2>&1 &
pid=$!
trap 'kill $pid 2>/dev/null || true; wait $pid 2>/dev/null || true; rm -f "$log"' EXIT
for _ in $(seq 1 50); do
	curl -sf "$LOCALSLACK_URL/_localslack/health" >/dev/null 2>&1 && break
	kill -0 $pid 2>/dev/null || { cat "$log" >&2; exit 1; }
	sleep 0.1
done
rc=0
"$@" || rc=$?
[ "$rc" -eq 0 ] || { echo "--- server log ---" >&2; cat "$log" >&2; }
exit "$rc"
