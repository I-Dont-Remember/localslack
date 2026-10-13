#!/usr/bin/env bash
# Tests for scripts/quiet.sh and scripts/claude-hooks/*. Run: make hooks-test
set -uo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
fails=0

# check <name> <want_exit> <want_substr_or_empty> <got_exit> <got_output>
check() {
	local name=$1 want=$2 sub=$3 code=$4 out=$5
	if [ "$code" != "$want" ] || { [ -n "$sub" ] && [[ "$out" != *"$sub"* ]]; }; then
		echo "FAIL $name: exit=$code (want $want), want substring '$sub', output: $out" >&2
		fails=$((fails + 1))
	fi
}

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
export QUIET_LOG_DIR="$work/logs"
export HOOK_EVENT_LOG="$work/events.jsonl"

# --- quiet.sh ---
out=$("$root/scripts/quiet.sh" demo bash -c 'echo noisy; echo noisy2'); code=$?
check "quiet success is one line" 0 "ok demo" "$code" "$out"
[[ "$out" == *noisy* ]] && check "quiet success hides output" 0 "" 1 "$out"

out=$("$root/scripts/quiet.sh" demo bash -c 'echo "--- FAIL: TestX"; echo chatter; exit 3' 2>&1); code=$?
check "quiet failure keeps exit code and fail lines" 3 "--- FAIL: TestX" "$code" "$out"

out=$(V=1 "$root/scripts/quiet.sh" demo bash -c 'echo raw'); code=$?
check "V=1 passes output through" 0 "raw" "$code" "$out"

[ -s "$work/logs/demo.log" ] || check "quiet writes full log" 0 "" 1 "missing log"

# --- hooks ---
hook() { # hook <script> <json>; prints output, returns exit code
	printf '%s' "$2" | "$root/scripts/claude-hooks/$1" 2>&1
}
bash_json() { jq -nc --arg c "$1" '{tool_name:"Bash",tool_input:{command:$c}}'; }

while IFS='|' read -r want sub cmd; do
	[ -z "$want" ] && continue
	out=$(hook pre-bash-guard.sh "$(bash_json "$cmd")"); code=$?
	check "guard: $cmd" "$want" "$sub" "$code" "$out"
done <<'CASES'
2|Edit tool|sed -i 's/a/b/' internal/x.go
2|Edit tool|python3 - <<'E'\nprint(1)\nE
2|Write tool|cat > internal/x.go <<'E'\npackage x\nE
2|Write tool|cat >> internal/x.go <<'E'\nfoo\nE
2|Write tool|tee internal/x.go
2|sleep|sleep 120
2|-r|rg -n foo -r . --glob '!tmp'
0||sed -i 's/a/b/' tmp/scratch.txt
0||python3 - <<'E'\nprint(1)\nE > $TMPDIR/out
0||make check
0||go test ./internal/service
0||sleep 2
0||rg -n foo internal
CASES

edit_json() { jq -nc --arg p "$1" '{tool_name:"Edit",tool_input:{file_path:$p}}'; }
printf 'package x\nfunc  F( ) {}\n' >"$work/bad.go"
hook post-edit-fmt.sh "$(edit_json "$work/bad.go")" >/dev/null; code=$?
check "fmt hook exits 0" 0 "" "$code" ""
grep -q 'func F()' "$work/bad.go" || check "fmt hook formats go" 0 "" 1 "$(cat "$work/bad.go")"
hook post-edit-fmt.sh "$(edit_json "$work/notes.md")" >/dev/null; code=$?
check "fmt hook ignores other files" 0 "" "$code" ""

if [ "$fails" -gt 0 ]; then
	echo "$fails hooks/quiet test(s) failed" >&2
	exit 1
fi
echo "hooks-test ok"
