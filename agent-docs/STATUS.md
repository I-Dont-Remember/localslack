# Status
**Current milestone:** 1 — Vertical slice is done and merged to `main`. Enabling work (capability tracking) is on branch `capabilities-tracking`.
**Branch:** `capabilities-tracking`
**Active plan:** `plans/capabilities-tracking.md` (done, pending owner review/merge). Read `plans/README.md` first, then `workflow.md` and `capabilities.md`.
**Where to look for what exists:** `docs/COMPATIBILITY.md` (generated) / `internal/capabilities/capabilities.toml`. Claim feature work there (`status = "in_progress"` + `plan`).
**Next:** merge, then M2 (Events API over HTTP + Bolt event delivery), see docs/DESIGN_DOC.md. Open follow-ups: plan "Follow-ups" sections and `make evidence-gaps`.
**Verify everything:** `make verify` (setup once with `make setup`)
