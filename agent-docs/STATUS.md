# Status
**Current milestone:** 1 — Vertical slice is done and merged to `main`. Enabling work (capability tracking) is on branch `capabilities-tracking`.
**Branch:** `capabilities-tracking`
**Active plan:** `plans/capabilities-tracking.md` (done, pending owner review/merge). Read `plans/README.md` first, then `workflow.md` and `capabilities.md`.
**Where to look for what exists:** `docs/COMPATIBILITY.md` (generated) / `internal/capabilities/capabilities.toml`. Claim feature work there (`status = "in_progress"` + `plan`).
**Priorities:** `ROADMAP.md` (ranked work, S/M/L sizing). **Next:** ROADMAP tier 1 (the test loop): plan M2 events in `plans/`; call log + reset (item 3) is an independent M that can start now; bolt-core methods and blocks (items 5-6) are enabling work, done as events need them. Open follow-ups: plan "Follow-ups" sections and `make evidence-gaps`.
**Verify everything:** `make verify` (setup once with `make setup`)
