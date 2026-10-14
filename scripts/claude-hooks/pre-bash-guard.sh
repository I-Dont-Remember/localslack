#!/usr/bin/env bash
# PreToolUse(Bash): block shell-based file editing/reading patterns that bloat context and trip
# the worktree guard. Exit 2 = block, stderr goes to the agent. Scratch paths (tmp/, $TMPDIR) are exempt.
set -uo pipefail
cmd=$(jq -r '.tool_input.command // empty' 2>/dev/null)
[ -n "$cmd" ] || exit 0

log="${HOOK_EVENT_LOG:-${CLAUDE_PROJECT_DIR:-.}/tmp/logs/hook-events.jsonl}"
block() { # block <rule> <message>
	mkdir -p "$(dirname "$log")" 2>/dev/null
	jq -nc --arg r "$1" --arg t "$(date -u +%FT%TZ)" '{ts:$t,hook:"pre-bash-guard",rule:$r}' >>"$log" 2>/dev/null
	echo "Blocked ($1): $2" >&2
	exit 2
}

if [[ "$cmd" =~ (^|[^[:alnum:]_])(tmp/|/tmp/|\$TMPDIR|\$\{TMPDIR\}) ]]; then exit 0; fi

if [[ "$cmd" =~ (^|[;&|[:space:]])sed[[:space:]]([^|;\&]*[[:space:]])?(-[[:alpha:]]*i[[:alpha:]]*|--in-place)([[:space:]]|$) ]]; then
	block sed-i "use the Edit tool for file changes (one targeted edit beats sed -i; imports are fixed by the post-edit formatter)."
fi
if [[ "$cmd" =~ python3?[[:space:]]+-[[:space:]]*\<\< ]]; then
	block python-heredoc "use the Edit tool for file changes; python heredocs re-send whole files and trip the worktree guard."
fi
if [[ "$cmd" =~ (^|[;&|[:space:]])cat[[:space:]]+\>\>?[[:space:]]*[^[:space:]] ]]; then
	block cat-redirect "use the Write tool to create files (Edit to append)."
fi
if [[ "$cmd" =~ (^|[;&|[:space:]])tee[[:space:]]+(-a[[:space:]]+)?[^[:space:]]*\.(go|py|md|toml|ya?ml|sh)([[:space:]]|$) ]]; then
	block tee "use the Write tool to create files."
fi
if [[ "$cmd" =~ (^|[;&|[:space:]])sleep[[:space:]]+([6-9]|[1-9][0-9]+)([[:space:]]|$) ]]; then
	block sleep "no long sleeps; run the command in the foreground or use Monitor with an until-loop."
fi
if [[ "$cmd" =~ (^|[;&|[:space:]])rg[[:space:]]([^|;\&]*[[:space:]])?-r[[:space:]]+\. ]]; then
	block rg-r "rg -r is --replace (it then waits on stdin), rg is already recursive: drop '-r .'."
fi
exit 0
