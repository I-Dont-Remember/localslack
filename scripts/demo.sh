#!/usr/bin/env bash
# Hands-on demo: LocalSlack + two real Bolt apps (examples/demo-bolt) in a throwaway state dir.
#   scripts/demo.sh            run until Ctrl-C            (make demo)
#   scripts/demo.sh --check    scripted mention -> reply, then exit  (make demo-check)
# Env: DEMO_MODE=http|socket (default http), DEMO_ADDR (default 127.0.0.1:3232),
#      DEMO_PORT / ECHO_PORT (default 3000 / 3001; --check picks free ports).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
bin="$root/bin/localslack"
mode="${DEMO_MODE:-http}"
check=""
[ "${1:-}" = "--check" ] && check=1
free_port() { python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])'; }

if [ -n "$check" ]; then
	addr="127.0.0.1:$(free_port)"
	demo_port="$(free_port)"
	echo_port="$(free_port)"
else
	addr="${DEMO_ADDR:-127.0.0.1:3232}"
	demo_port="${DEMO_PORT:-3000}"
	echo_port="${ECHO_PORT:-3001}"
fi
url="http://$addr"
ex="$root/examples/demo-bolt"

make -C "$root" build >&2
tmp="$(mktemp -d)"
pids=()
cleanup() {
	trap - EXIT INT TERM
	for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
	wait 2>/dev/null || true
	rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

export XDG_STATE_HOME="$tmp/state" XDG_DATA_HOME="$tmp/data" XDG_RUNTIME_DIR="$tmp/run"
export LOCALSLACK_DB_PATH="$tmp/data/localslack.db" LOCALSLACK_SEED=demo LOCALSLACK_ADDR="$addr"
mkdir -p "$tmp/state" "$tmp/data" "$tmp/run"

"$bin" start >"$tmp/server.log" 2>&1 &
pids+=($!)
for _ in $(seq 1 50); do
	curl -sf "$url/_localslack/health" >/dev/null 2>&1 && break
	sleep 0.2
done
curl -sf "$url/_localslack/health" >/dev/null || { echo "server did not start:" >&2; cat "$tmp/server.log" >&2; exit 1; }

# Register both apps (no handshake yet: the Bolt apps need these credentials to boot).
demo_manifest="$ex/manifest.$mode.json"
echo_manifest="$ex/echo.manifest.$mode.json"
reg_args=(--addr "$addr" --no-verify)
if [ "$mode" = "http" ]; then
	"$bin" apps add "$demo_manifest" "${reg_args[@]}" --deliver-to "http://127.0.0.1:$demo_port" >/dev/null
	"$bin" apps add "$echo_manifest" "${reg_args[@]}" --deliver-to "http://127.0.0.1:$echo_port" >/dev/null
else
	"$bin" apps add "$demo_manifest" "${reg_args[@]}" >/dev/null
	"$bin" apps add "$echo_manifest" "${reg_args[@]}" >/dev/null
fi

start_app() { # <app name> <script> <port var> <port>
	(
		eval "$("$bin" apps env --addr "$addr" "$1")"
		export DEMO_MODE="$mode" "$3=$4"
		exec uv run --project "$root/tests/smoke" python "$ex/$2"
	) >"$tmp/$2.log" 2>&1 &
	pids+=($!)
}
start_app "Demo Bolt" app.py DEMO_PORT "$demo_port"
start_app "Echo Bolt" echo.py ECHO_PORT "$echo_port"

if [ "$mode" = "http" ]; then
	for port in "$demo_port" "$echo_port"; do
		for _ in $(seq 1 100); do
			(exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null && break
			sleep 0.2
		done
	done
	for app in $(curl -sf "$url/_localslack/apps" | jq -r '.items[].id'); do
		curl -sf -X POST "$url/_localslack/apps/$app/verify" | jq -r '"  verified=\(.verified) \(.name)"'
	done
else
	# Socket Mode: events only flow once each app has opened its WebSocket.
	for _ in $(seq 1 150); do
		[ "$(curl -sf "$url/_localslack/socket/connections" | jq '.items | length')" -ge 2 ] && break
		sleep 0.2
	done
	curl -sf "$url/_localslack/socket/connections" | jq -r '.items[] | "  socket \(.app_id) connected \(.id)"'
fi

echo
"$bin" apps list --addr "$addr"
echo
echo "LocalSlack UI:  $url/ui"
echo "Inspector:      $url/ui/inspector"
echo "Try, in #general: @demo-bolt ping  |  hello  |  echo hi  |  add a reaction"
if [ "$mode" = "socket" ]; then
	echo "Force a reconnect: curl -X POST $url/_localslack/socket/disconnect -d '{\"reason\":\"refresh_requested\"}'"
fi
echo "Logs: $tmp (removed on exit)"

if [ -n "$check" ]; then
	"$root/scripts/demo-check.sh" "$url" "$tmp"
	exit $?
fi
echo "Ctrl-C to stop."
wait
