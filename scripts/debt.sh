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
echo "== Dead code (unreachable from main packages; test-only helpers show up here too)"
GOSUMDB=off go run golang.org/x/tools/cmd/deadcode@v0.36.0 ./... 2>&1 | rg -v '^go: ' | head -"$top" ||
	echo "(deadcode unavailable)"

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
