#!/usr/bin/env bash
# A/B replay: run a fixed task prompt headlessly in a throwaway worktree and score the transcript.
# Usage: scripts/agent-bench.sh <label> <task.md> <base-ref> [overlay-ref]
#   overlay-ref: after checkout, copy the tooling (CLAUDE.md, agent-docs, scripts, Makefile, lint config,
#   .claude/settings.json) from that ref - this is the "after" arm; omit for the "before" arm.
# Env: DRY=1 print plan only; BENCH_MODEL (default claude-sonnet-5-5); BENCH_PERMISSION_FLAGS
#   (default "--permission-mode acceptEdits"); results in tmp/bench/<label>.jsonl (+ stats).
# Costs real tokens (~7M cache-read for the B1 task per run). Run manually, never in CI.
set -euo pipefail
label=${1:?usage: agent-bench.sh <label> <task.md> <base-ref> [overlay-ref]}
task=${2:?task file}
base=${3:?base ref}
overlay=${4:-}
root="$(cd "$(dirname "$0")/.." && pwd)"
out="$root/tmp/bench"
wt="$out/wt-$label"
model=${BENCH_MODEL:-claude-sonnet-5-5}
perm=${BENCH_PERMISSION_FLAGS:---permission-mode acceptEdits}
tooling=(CLAUDE.md agent-docs scripts Makefile rules rule-tests sgconfig.yml .golangci.yml .pre-commit-config.yaml .gitattributes .claude/settings.json)

cmd="cd $wt && claude -p \"\$(cat $task)\" --model $model $perm --output-format stream-json --verbose > $out/$label.jsonl"
if [ -n "${DRY:-}" ]; then
	echo "worktree: git worktree add -b bench/$label $wt $base"
	[ -n "$overlay" ] && echo "overlay: git checkout $overlay -- ${tooling[*]}"
	echo "run: $cmd"
	echo "score: python3 scripts/trace_stats.py $out/$label.jsonl"
	exit 0
fi

mkdir -p "$out"
git -C "$root" worktree add -b "bench/$label" "$wt" "$base"
if [ -n "$overlay" ]; then
	git -C "$wt" checkout "$overlay" -- "${tooling[@]}" 2>/dev/null || true
fi
(cd "$wt" && claude -p "$(cat "$task")" --model "$model" $perm --output-format stream-json --verbose >"$out/$label.jsonl")
python3 "$root/scripts/trace_stats.py" "$out/$label.jsonl" | tee "$out/$label.stats.txt"
echo "final state: cd $wt && make verify   (record green/red + diff size in the review history table)"
