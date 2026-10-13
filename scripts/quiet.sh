#!/usr/bin/env bash
# Run a command with compact output: one "ok" line on success, only failure signal on failure.
# Usage: scripts/quiet.sh <name> <cmd...>   (V=1 passes output through untouched)
# Full output always goes to ${QUIET_LOG_DIR:-tmp/logs}/<name>.log
set -uo pipefail
[ $# -ge 2 ] || { echo "usage: $0 <name> <cmd...>" >&2; exit 2; }
name=$1
shift
if [ -n "${V:-}" ]; then exec "$@"; fi

dir="${QUIET_LOG_DIR:-tmp/logs}"
mkdir -p "$dir"
log="$dir/$name.log"
start=$SECONDS
"$@" >"$log" 2>&1
code=$?
if [ "$code" -eq 0 ]; then
	echo "ok $name ($((SECONDS - start))s, log: $log)"
	exit 0
fi

echo "FAILED $name (exit $code, log: $log)" >&2
# Failure signal first (test failures, races, panics, compiler/lint locations, pytest errors), then the tail.
rg -n --no-heading -e '^(--- FAIL|FAIL\b|panic:|WARNING: DATA RACE|FAILED|ERROR|E  )' \
	-e '^\S+\.(go|py):[0-9]+(:[0-9]+)?:' -e '^Error:' "$log" | head -60 >&2
echo "--- last 40 lines ---" >&2
tail -n 40 "$log" >&2
exit "$code"
