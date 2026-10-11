#!/usr/bin/env bash
# Usage: cold-start.sh <cmd...>  -> ms from spawn to first 200 on /_localslack/health
set -euo pipefail
port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')"
export PORT="$port" HOST=127.0.0.1 LOCALSLACK_SEED=cold
start=$(date +%s%N)
"$@" >/tmp/claude-0/cold.log 2>&1 &
pid=$!
trap 'kill $pid 2>/dev/null || true' EXIT
for _ in $(seq 1 600); do
	curl -sf "http://127.0.0.1:$port/_localslack/health" >/dev/null 2>&1 && { echo "$(( ($(date +%s%N) - start) / 1000000 )) ms"; exit 0; }
	kill -0 $pid 2>/dev/null || { cat /tmp/claude-0/cold.log; exit 1; }
	sleep 0.01
done
echo timeout; cat /tmp/claude-0/cold.log; exit 1
