# Agent trace review — test-loop run (Oct 12–13, 2026)

Purpose: record why the test-loop run was slow/expensive, what we changed, and how to tell whether
it helped. Re-measure with `make trace-stats SESSION=<id> [VS=<id>]` (`scripts/trace_stats.py`).

Sessions reviewed: `db04cabb-…` (orchestrator + 24 subagents, the main subject), plus Oct 9–10
sessions `9450070d`, `07890024`, `c8791ed5` for recurring themes. Traces live in
`~/.claude/projects/-home-user-work-localslack/`.

## Summary

- 490M cache-read tokens, 12.7M cache-write, 0.79M output over ~10.7h wall clock.
- **~6.3h was idle**: the session limit was hit twice, ~20 min after a wide wave launched
  (7 agents at 18:47 UTC → limit 19:07; 5 agents at 22:22 → limit 22:46). Four streams died mid-task.
- Tool execution time was small (no command >77s except one hung pre-commit). Cost = **many turns
  over bloated context** (p50 134k, p95 321k, peak 406k tokens per turn).
- Wall clock is not the user's priority; token/rate-limit cost and output quality are weighted equally.

## Baseline metrics (session db04cabb, orchestrator + subagents)

| Metric | Baseline |
|---|---|
| Assistant turns | 3268 |
| Cache-read / cache-write / output tokens | 490.5M / 12.7M / 0.79M |
| Context per turn p50 / p95 / peak | 134k / 321k / 406k |
| Tool-result bytes p50 / p95 | 522 / 11,111 |
| Max concurrent subagents | 7 |
| Session-limit hits (rows) | 8 |
| Idle hours (gaps >30 min) | 6.3 |
| Bash calls / Read / Edit / Write | 1463 / 78 / 104 / 172 |
| Bash file-edit calls (sed -i, python heredoc, cat >, tee) | 407 |
| Worktree-guard refusals | 59 |
| `make check` / `test` / `verify` runs | 152 / 118 / 40 |

Earlier session `07890024` (Oct 9, M1): 160.8M cache-read, 95 Bash file-edit calls, 0 guard refusals
(no worktree fan-out), max concurrency 3.

Per-stream cost (cache-read, M): Wave1 A 68.5 · Wave2 G 74.2 · Gate-1 hardening 44.8 ·
membership-fidelity 40.1 · B2 37.7 · C 31.5 · Wave 0 part 2 26.2 · S 21.2 · X 18.1 · F 11.6 ·
B1 7.3 · E 6.9. Narrow streams (~40 calls, <125k context) cost ~7M; broad ones ~70M.

Correction note: first-pass analysis by subagents estimated 189 Bash edits / 124 guard refusals;
the scripted counts above (which dedupe to tool results) are authoritative.

## Findings → fixes

| # | Finding | Evidence | Fix |
|---|---|---|---|
| 1 | Wide fan-out hit the session limit twice | 7 then 5 parallel agents; 4 streams died; C, X never handed back | Concurrency cap 3, stream sizing (`agent-docs/orchestration.md`) |
| 2 | Context bloat via Bash `cat`/`sed -n`/heredoc edits | fix agents sent 312k/378k chars of results; up-front `cat` of ~150KB | Block hook, CLAUDE.md rule, `quiet.sh` |
| 3 | Worktree guard refusals on heredoc / `cd …;` chains | 59 refusals, each a wasted turn at ~200k context | Same hook; flat branch names |
| 4 | Large docs re-read by every agent | `plans/test-loop.md` (27KB) read 47×, `TESTING_API.md` (20KB) 44× ≈ 12% of cache reads | Briefs with line ranges; plans edited in place |
| 5 | Contract settled after code | Wave 0 coded 4 min after drafting; user redesigned errors/reset/URL override → align agent + later C4 | Contract gate before code waves |
| 6 | "Recommended" defaults favoured simplification; user chose fidelity | single-app slot, auto-join choices → 68M stream + 40M fix agent | CLAUDE.md fidelity-first rule |
| 7 | Sandbox/env friction | pre-commit read-only cache (one 300s hang), loopback denied, parallel golangci-lint lock, worktree based on `main`, `test-loop/x` ref clash, `rg -r` misuse | settings/pre-commit changes (user-applied), hook blocks `rg -r .`, orchestration rules |
| 8 | Generated/contract drift | `TestCompatibilityDocIsCurrent` failed in 7/8 agents; `COMPATIBILITY.md` conflicted on 5 merges | `make gen`, `.gitattributes`, merge script |
| 9 | Format/lint churn | gofumpt ×3 per agent; ~26 sed import edits; test-code lints (gochecknoglobals 7, errorlint 7, unparam 9, bodyclose 3, paralleltest 5, thelper 2) | post-edit formatter hook, test-idiom cheat sheet |
| 10 | Quality escapes found by reviewers, mostly mechanisable | flaky timing test; unused `StrictTokens`; missing depguard entries; duplicated long-poll parsing and `writeJSON` | ast-grep rules, meta-tests, `dupl` |
| 11 | Verbose output + repeated full-suite runs | 152 `make check`, 118 `make test`; pre-commit runs full `go test` on ~124 commits (~22 min); earlier session missed failures buried in verify output | `quiet.sh`, `make t`, `lint-changed`, scoped pre-commit test |

### Reviewer findings and what could have caught them deterministically
- C1/C2 reset re-join and ordering, event_id and PUT/PATCH semantics, membership fidelity: needed
  upfront context (fidelity principle, reset invariants) — not lintable.
- C3 `StrictTokens` parsed but unused → config-consumed meta-test.
- C4 `/_localslack` errors Slack-shaped → route-contract test (the user suggested this mid-run).
- Missing depguard for `service`, `socket`, `seed`, `respond` → depguard coverage meta-test.
- Flaky `TestEngineTimeoutThenRetryReason` (indexing `requests()[n]`, short timeouts) →
  `no-index-recorded-requests` / `no-time-sleep-in-tests` rules, `make stress`.
- Duplicated helpers across streams → `dupl`, shared helpers in Wave 0.

## Orchestration timeline (UTC)

16:44 explore + plan · 17:44 contract drafted, Wave 0 coded · 18:22–18:29 user redesigns contract →
align agent · 18:47 Wave 1 (7 agents) · 19:07 limit #1 · idle 2h23 · 21:31 merges · 22:21 Wave 2
(5 agents) · 22:46 limit #2 · idle 3h25 · 02:31 merges, final review · 03:24 done.
A leftover Monitor delayed one handback ~27 min (user: "are we still waiting?").

## User corrections worth encoding
Overrode two "Recommended" options for fidelity; asked for RESTful status codes; wanted sticky URL
override and no `--keep-logs`; supplied the route-contract guardrail idea; asked for sandbox
guidance; earlier: "no commits on M1 yet", "You've stopped - please continue", "a couple errors
hiding in [make verify output]".

## How to know it helped (and how not to fool ourselves)

1. **Mechanism metrics** (low noise; prove the fix engages): Bash file-edit calls 407 → ~0, guard
   refusals 59 → <10, max concurrency 7 → ≤3, `TestCompatibilityDocIsCurrent` failures, merge
   conflicts on generated files, sandbox errors, tool-result bytes p95. Hooks log each block to
   `tmp/logs/hook-events.jsonl`: a hook that never fires is dead weight; one followed by an identical
   retry has a bad message.
2. **Normalized outcomes** (directional, noisy at small n): cache-read per merged commit, context
   p50/p95, turns per stream, session-limit hits per 10 commits, idle hours, reviewer findings per
   wave, fix-ups to code merged <48h earlier, user corrections.
3. **Controlled A/B replay** (the only causal evidence): same task prompt and model against the
   pre-change and post-change refs, 2 runs each, scored with `trace_stats.py`. Costs ~30–40M tokens;
   run manually. Pass bar: median tokens −25% or better, refusals ~0, `make verify` green.

Running the A/B replay (manual, ~7M cache-read tokens per run for the B1 task):

```
scripts/agent-bench.sh before-1 agent-docs/bench/b1-read-methods.md 9d20c27            # old tooling
scripts/agent-bench.sh after-1  agent-docs/bench/b1-read-methods.md 9d20c27 agent-loop-tooling  # new tooling overlaid
make trace-stats SESSION=tmp/bench/before-1.jsonl VS=tmp/bench/after-1.jsonl
```

Repeat each arm twice, then run `make verify` in each `tmp/bench/wt-*` worktree and record green/red
and diff size next to the token numbers. `DRY=1` prints the plan without spending tokens. The "after"
arm only includes the hooks once `.claude/settings.json` on the overlay ref has them wired
(`scripts/claude-hooks/INSTALL.md`). Clean up with `git worktree remove` + `git branch -D bench/<label>`.

Caveats: small n; model/harness versions confound comparisons (record them per run); different
tasks have different scope, so never compare raw totals across tasks.

### Consolidation baseline (`make debt`, added 2026-10-13)

Raw LOC growth is expected for a large Slack emulation, so track duplication and repeated patterns
instead. Rationale: requirements shift mid-build, leaving near-copies a full-requirements design would
have unified; the `dry-consolidator` agent (own `consolidate-*` branch) consolidates them at checkpoints.

| Date | Ref | dupl>=50 hits (non-test, pairs x2) | deadcode unreachable | decode-guard | consolidator branch |
|---|---|---|---|---|---|
| 2026-10-13 | 9c3ca39 | 8 | 39 (some test-only helpers) | 5 | n/a (baseline) |
| 2026-10-13 | d48536a | 6 | n/a | 5 | `consolidate-agent-loop-tooling`: 1 commit, 6 copies -> `Tx.execOne`, net -36 LOC, ~38k tokens |

Trial finding: `url-param-read` (30) and `hand-rolled-json-decode` (3) were dropped as noise (already
one-liners / test helpers); the agent correctly skipped pairs that fail the rule of three.

Per dry-consolidator run also record: merged or dropped by the owner, clusters removed, LOC delta, tokens
(`make trace-stats`). Branches that keep getting dropped mean the brief or the agent needs retuning.
Judge the trend per feature, not absolute counts.

### History (append a row per measured session)

| Date | Session | Task | cache-read | turns | Bash-edit | refusals | max conc. | limit hits | notes |
|---|---|---|---|---|---|---|---|---|---|
| 2026-10-12 | db04cabb | test-loop waves 0–3 | 490.5M | 3268 | 407 | 59 | 7 | 8 | baseline |
