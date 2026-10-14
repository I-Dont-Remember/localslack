#!/usr/bin/env bash
# Merge a finished stream branch into the current branch. Conflicts in generated files are
# regenerated; any other conflict aborts the merge and is reported.
# Usage: scripts/merge-stream.sh <branch>
# Env: GEN_CMD (default `make gen`), VERIFY_CMD (default `make check test`; `true` to skip)
set -uo pipefail
branch=${1:?usage: merge-stream.sh <branch>}
gen=${GEN_CMD:-make gen}
verify=${VERIFY_CMD:-make check test}
generated=(docs/COMPATIBILITY.md)

if git merge --no-ff --no-edit "$branch"; then
	echo "merged $branch cleanly"
else
	mapfile -t conflicts < <(git diff --name-only --diff-filter=U)
	others=()
	for f in "${conflicts[@]}"; do
		is_gen=""
		for g in "${generated[@]}"; do [ "$f" = "$g" ] && is_gen=1; done
		[ -n "$is_gen" ] || others+=("$f")
	done
	if [ ${#others[@]} -gt 0 ]; then
		git merge --abort
		echo "merge of $branch aborted; resolve by hand (conflicts: ${others[*]})" >&2
		exit 1
	fi
	bash -c "$gen" && git add "${generated[@]}" && git commit --no-edit -q || exit 1
	echo "merged $branch; regenerated ${generated[*]}"
fi
bash -c "$verify"
