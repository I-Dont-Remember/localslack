# Plans

One file per milestone, checkbox tasks. Source of truth for resuming work after a stall.

## Protocol
- Start: read `../STATUS.md` -> active plan -> any `## Handoff` section -> first unchecked task.
- Tick the box in the same commit that completes the task.
- Stopping mid-task (rate limit etc): append `## Handoff` to the plan: what you were doing, next command, gotchas. Remove it when resumed.
- Rescoping: edit the plan + `docs/DESIGN_DOC.md` together, then run `make debt` and launch the `dry-consolidator` agent on the code the change affects (earlier patterns often no longer fit).
- New plans open with `## Fit with existing code` (<=3 lines): which existing patterns this extends; whether it makes code redundant or creates a third copy of something; what the design would be if planned from the start.
- Edit plans **in place**. Never append "supersede conflicting text above" sections: every agent reads the whole plan, so contradictions cost tokens and cause mistakes. Fold amendments into the original text before the next wave.
- Fan-out waves follow `../orchestration.md` (concurrency cap, stream briefs, contract gate).

## Using the arch reviewer
`arch-reviewer` agent (`.claude/agents/arch-reviewer.md`): run it at the points the plan names, fold findings into the plan, tick the box.

## Plans
- `m0.5-hardening.md` — tooling + test harness hardening (DONE, merged)
- `m1-vertical-slice.md` — SQLite + core Slack endpoints + htmx UI + e2e (DONE, merged)
- `capabilities-tracking.md` — capability manifest, evidence provenance, real-client (SDK/Bolt) coverage (DONE)
- `test-loop.md` — ROADMAP tier 1 + tier 2: events, call log/reset, faults, scenarios, inspector (Waves 0-3 DONE; owner QA pending). Has stacked amendment sections - read "Progress" and the last section first; do not copy this layout.
