# Orchestrating parallel agents

Why this exists: the test-loop run (7 then 5 parallel agents) hit the session limit twice and sat
idle ~6h; cost came from large contexts x many turns, not command time. Evidence and metrics:
`reviews/2026-10-13-agent-trace-review.md`. The goal is a marathon, not a sprint.

## Rules

1. **Max 3 implementer agents at once.** Queue the rest. Run streams that touch the same hot files
   (`api.go` method list, `server/build.go`, `capabilities.toml`) one after another.
2. **Size a stream like B1/E: ~40-60 tool calls, one or two packages.** Parser + dispatcher + UI in
   one stream (Wave1 A, ~70M cache-read tokens) is three streams. Split it.
3. **Settle contracts before code.** HTTP shapes, CLI flags, reset semantics: give the owner a
   one-page decisions list, wait for answers, *then* start the first code wave. Never code in the
   turn the contract is drafted. Prefer fidelity to real Slack; simplifications need owner sign-off.
4. **Land shared seams first (Wave 0):** helpers, registration points, test utilities. Duplicated
   helpers (long-poll parsing, `writeJSON`) were only found at review.
5. **Write each brief in <=1.5k chars:** goal; exact files and extension points; doc line ranges to
   read (not "read the whole plan"); acceptance tests; inner-loop commands; "commit, hand back, leave
   no background Monitors running".
6. **Worktrees:** branch from the current integration branch (check `git branch --show-current`),
   flat names (`w1-events`, not `test-loop/w1-events` - it collides with the `test-loop` ref).
   The Agent tool's `isolation: worktree` branches from an old default commit, not the current
   branch (and the agent then can't read the branch's `.claude/` files): create the worktree
   yourself, `git worktree add -b w1-events tmp/w1-events <integration-branch>`, and give the agent
   its absolute path (`git -C`/`make -C`, no `cd x;` chains).
   Inside a worktree use Read/Edit/Write; shell heredocs and `cd x; ...` chains get refused.
7. **Merge with `scripts/merge-stream.sh <branch>`.** It regenerates `docs/COMPATIBILITY.md` on
   conflict, aborts on real conflicts, then runs `make check test`.
8. **A "stopped with background work" notice is not a handback.** Check the worktree's `git log` /
   `git status` rather than waiting.
9. **Consolidate after each wave merges:** run `make debt BASE=<wave start>` and launch one
   `dry-consolidator` (`.claude/agents/dry-consolidator.md`) on the merged result; parallel streams are the
   main source of near-copies. It works on its own `consolidate-*` branch, counts toward the cap of 3.
10. **Rate-limit checkpoints:** before launching a wave, estimate tokens (streams x ~10-40M). If a
   limit hits, agents commit WIP and write `## Handoff` in the plan (see `plans/README.md`).

## Inner loop for every agent

```
make t PKG=./internal/x [RUN=TestY]   # fast, package-scoped
make lint-changed                     # BASE=<parent-branch> if stacked
make check                            # before each commit (quiet; V=1 for raw output)
make verify                           # ONCE before handback
```

`make check|test|smoke|e2e` print one `ok` line on success and only failures otherwise; the full
log is in `tmp/logs/<name>.log`. Flaky? `make stress PKG=... RUN=TestY`.
