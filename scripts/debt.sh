#!/usr/bin/env bash
# Consolidation targets: near-duplicates, dead code, tracked repeated patterns, size outliers, churn.
# Report only (always exits 0). Env: DEBT_ROOT (default repo), DEBT_BASE (default main),
# DEBT_TOP (lines per section, default 10), DEBT_DUPL (dupl token threshold, default 50),
# DEBT_TESTS=1 (include _test.go in duplicates).
set -uo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"
root="${DEBT_ROOT:-$here}"
base="${DEBT_BASE:-main}"
top="${DEBT_TOP:-10}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cd "$root" || exit 0

echo "== Near-duplicates (dupl >= ${DEBT_DUPL:-50} tokens; the lint gate is 80)"
cat >"$tmp/dupl.yml" <<E
version: "2"
linters:
  default: none
  enable: [dupl]
  settings:
    dupl:
      threshold: ${DEBT_DUPL:-50}
E
golangci-lint run -c "$tmp/dupl.yml" ./... 2>&1 | rg 'lines are duplicate' |
	sed -E 's#^(\.\./)+[^ ]*/'"$(basename "$root")"'/##; s# \(dupl\)$##' >"$tmp/dupl.txt"
[ -n "${DEBT_TESTS:-}" ] || { rg -v '_test\.go' "$tmp/dupl.txt" >"$tmp/dupl2.txt"; mv "$tmp/dupl2.txt" "$tmp/dupl.txt"; }
echo "$(wc -l <"$tmp/dupl.txt") hits (each pair listed twice); files with most hits:"
cut -d: -f1 "$tmp/dupl.txt" | sort | uniq -c | sort -rn | head -"$top"
echo "--- first hits"
head -"$top" "$tmp/dupl.txt"

echo
echo "== art-dupl (semantic clones; //art-dupl:accept honored)"
ad="$(cd "$here" && mise which art-dupl 2>/dev/null || command -v art-dupl)"
if [ -x "$ad" ]; then
	scan=""
	for d in internal cmd; do [ -d "$d" ] && scan="$scan ./$d"; done
	# shellcheck disable=SC2086
	"$ad" --no-color -q --exclude-pattern '*_test.go' ${scan:-.} </dev/null 2>&1 | rg -v '^\s*(📖|[0-9]+ files|✅|$)' | head -$((top * 4))
else
	echo "(art-dupl not installed: mise install)"
fi

echo
echo "== Accepted-clone markers (//art-dupl:accept <reason> (expires=YYYY-MM-DD))"
today="$(date +%F)"
rg -n --no-heading --glob '*.go' --glob '!.claude/**' --glob '!tmp/**' 'art-dupl:accept' . 2>/dev/null |
	awk -v today="$today" -F: '{
		sub(/^\.\//, "", $1); loc = $1 ":" $2; text = $0; n++
		if (match(text, /expires=[0-9-]+/)) { d = substr(text, RSTART + 8, RLENGTH - 8); if (d < today) print "EXPIRED " loc " (" d ")" }
		else print "NO-EXPIRY " loc
	} END { print n + 0 " markers" }'

echo
echo "== Dead code (unreachable from main packages; test-only helpers show up here too)"
{
	# pinned in mise.toml; resolve the real binary from the repo (shims only work inside it)
	dc="$(cd "$here" && mise which deadcode 2>/dev/null || command -v deadcode)"
	if [ -x "$dc" ]; then
		"$dc" ./...
	else # fallback: needs network on first use
		GOSUMDB=off go run golang.org/x/tools/cmd/deadcode@v0.36.0 ./...
	fi
} 2>&1 | rg -v '^go: ' | head -"$top"

echo
echo "== Tracked repeated patterns (report-rules/, not enforced)"
if [ -f sgconfig.report.yml ]; then
	ast-grep scan -c sgconfig.report.yml --json=stream --globs '!.claude/**' --globs '!tmp/**' --globs '!**/*_test.go' . 2>/dev/null | jq -r '.ruleId' | sort | uniq -c | sort -rn
else
	echo "(no sgconfig.report.yml)"
fi

echo
echo "== Size outliers (non-test .go)"
find . -name '*.go' ! -name '*_test.go' ! -path './tmp/*' ! -path './.git/*' ! -path './.claude/*' -print0 | xargs -0 wc -l 2>/dev/null |
	rg -v ' total$' | sort -rn | head -"$top"
echo "--- longest functions"
find . -name '*.go' ! -name '*_test.go' ! -path './tmp/*' ! -path './.git/*' ! -path './.claude/*' -print0 | xargs -0 awk '
	/^func / { start = FNR; name = $0 }
	/^}/ && start { printf "%d %s:%d %s\n", FNR - start + 1, FILENAME, start, substr(name, 1, 60); start = 0 }' |
	sort -rn | head -"$top"

echo
echo "== Churn vs $base"
if git rev-parse --verify -q "$base" >/dev/null; then
	git diff --shortstat "$base"...HEAD
	git log --numstat --format='@%h' "$base"..HEAD 2>/dev/null | awk '
		/^@/ { if (seen) { c++; if (d >= a) neg++ } seen = 1; a = d = 0; next }
		$1 != "-" && NF { a += $1; d += $2 }
		END { if (seen) { c++; if (d >= a) neg++ } printf "%d commits, %d with deletions >= additions\n", c, neg }'
else
	echo "(base $base not found)"
fi
exit 0
