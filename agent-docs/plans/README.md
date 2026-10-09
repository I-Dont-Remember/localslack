# Plans

One file per milestone, checkbox tasks. Source of truth for resuming work after a stall.

## Protocol
- Start: read `../STATUS.md` -> active plan -> any `## Handoff` section -> first unchecked task.
- Tick the box in the same commit that completes the task.
- Stopping mid-task (rate limit etc): append `## Handoff` to the plan: what you were doing, next command, gotchas. Remove it when resumed.
- Rescoping: edit the plan + `docs/DESIGN_DOC.md` together.

## Using the arch reviewer
`arch-reviewer` agent (`.claude/agents/arch-reviewer.md`): run it at the points the plan names, fold findings into the plan, tick the box.

## Plans
- `m0.5-hardening.md` — tooling + test harness hardening (DONE, merged)
- `m1-vertical-slice.md` — SQLite + core Slack endpoints + htmx UI + e2e
