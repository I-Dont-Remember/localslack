#!/usr/bin/env bash
# Run a command against an isolated live LocalSlack (temp state dir, free port).
# Usage: scripts/with-server.sh <cmd...>   (exports LOCALSLACK_URL)
# Env: SKIP_BUILD=1 to reuse ./bin/localslack.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
bin="$root/bin/localslack"
[ $# -gt 0 ] || { echo "usage: $0 <cmd...>" >&2; exit 2; }
[ -n "${SKIP_BUILD:-}" ] || make -C "$root" build >&2

tmp="$(mktemp -d)"
export XDG_STATE_HOME="$tmp/state" XDG_DATA_HOME="$tmp/data" XDG_RUNTIME_DIR="$tmp/run"
export LOCALSLACK_DB_PATH="$tmp/data/localslack.db"
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')"
export LOCALSLACK_URL="http://127.0.0.1:$port"
log="$XDG_STATE_HOME/localslack/server.log"

cleanup() {
	"$bin" stop >/dev/null 2>&1 || true
	rm -rf "$tmp"
}
trap cleanup EXIT

"$bin" start -d --addr "127.0.0.1:$port" >&2

ready=""
for _ in $(seq 1 50); do
	if curl -sf "$LOCALSLACK_URL/_localslack/health" >/dev/null 2>&1; then ready=1; break; fi
	sleep 0.2
done
if [ -z "$ready" ]; then
	echo "server did not become healthy at $LOCALSLACK_URL; log:" >&2
	cat "$log" >&2 || true
	exit 1
fi

rc=0
"$@" || rc=$?
if [ "$rc" -ne 0 ]; then
	echo "--- server log ($log) ---" >&2
	cat "$log" >&2 || true
fi
exit "$rc"
