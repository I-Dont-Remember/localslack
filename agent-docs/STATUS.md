# Status
**Current milestone:** 1 — Vertical slice is done and merged to `main`. Capability tracking is done and merged.
**Branch:** `test-loop` (integration)
**Active plan:** `plans/test-loop.md` (integration branch `test-loop`; Waves 0-3 done: events HTTP + Socket Mode, call log/reset, faults, blocks, access, header fidelity, scenarios/snapshots/diff, inspector, demo; final arch review done, plus membership methods (`conversations.join/leave/invite/kick`), channel-members helper, app_mention membership rule, snapshot `app_bots`; next: owner QA Gates 1-2 via `docs/QA.md`, then owner merges to `main`). Contract: `docs/TESTING_API.md`. Read `plans/README.md` first, then `workflow.md` and `capabilities.md`.
**Where to look for what exists:** `docs/COMPATIBILITY.md` (generated) / `internal/capabilities/capabilities.toml`. Claim feature work there (`status = "in_progress"` + `plan`).
**Priorities:** `ROADMAP.md` (ranked work, S/M/L sizing). Tier 1 and tier 2 (items 1-10) are done or partly done on `test-loop`; remaining gaps are listed there (message `team` field, multipart, channel-name posting, DM-by-user-id, interactivity/slash commands over socket). **Next:** owner QA (`docs/QA.md`), then item 11 (first real recordings, `make evidence-gaps`) and tier 3. Open follow-ups: plan "Follow-ups" sections.
**Verify everything:** `make verify` (setup once with `make setup`)
