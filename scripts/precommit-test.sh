#!/usr/bin/env bash
# pre-commit go-test hook: test only the packages of the staged files (full suite stays in `make verify` / CI).
# Usage: scripts/precommit-test.sh <staged files...>   (PRECOMMIT_TEST_DRY=1 prints the command)
set -euo pipefail
dirs=()
for f in "$@"; do
	case "$f" in
	go.mod | go.sum) dirs=(...); break ;;
	*.go) dirs+=("./$(dirname "$f")") ;;
	esac
done
[ ${#dirs[@]} -gt 0 ] || exit 0
if [ "${dirs[0]}" = "..." ]; then
	pkgs=(./...)
else
	mapfile -t pkgs < <(printf '%s\n' "${dirs[@]}" | sort -u)
fi
if [ -n "${PRECOMMIT_TEST_DRY:-}" ]; then
	echo "go test -shuffle=on ${pkgs[*]}"
	exit 0
fi
exec go test -shuffle=on "${pkgs[@]}"
