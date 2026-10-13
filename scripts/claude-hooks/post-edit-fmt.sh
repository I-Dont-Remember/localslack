#!/usr/bin/env bash
# PostToolUse(Edit|Write): silently format the edited file (gofumpt/goimports via golangci-lint, ruff for Python).
# Never blocks: formatting problems are left for `make check` to report.
set -uo pipefail
f=$(jq -r '.tool_input.file_path // empty' 2>/dev/null)
[ -n "$f" ] && [ -f "$f" ] || exit 0
root="${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
case "$f" in
*.go)
	if [[ "$f" == "$root"/* ]]; then
		(cd "$root" && golangci-lint fmt "$f") >/dev/null 2>&1
	else
		gofmt -w "$f" >/dev/null 2>&1
	fi
	;;
*.py)
	(cd "$root" && uv run --project tests/smoke ruff format "$f") >/dev/null 2>&1
	;;
esac
exit 0
