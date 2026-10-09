> **Status (2026-10-09):** historical. Tier 1/2 items were resolved or re-planned in M0.5; remaining work lives in `plans/m1-vertical-slice.md` and `docs/DESIGN_DOC.md` milestones. Do not treat as a todo list.

# Testing Infrastructure Architecture Review
_Generated: 2026-03-26_

Architecture review of Milestone 0.5 testing infrastructure against `docs/DESIGN_DOC.md`.

---

## Summary

Solid foundation, critical gaps in execution. Three test layers exist (unit, conformance, smoke) with Makefile automation, but the conformance fixture format is incompatible with real API responses, smoke tests are empty placeholders, and persistence validation is absent entirely.

---

## Status by Category

### 🟢 Satisfied

- **Unit test tooling** — shuffle, race, Makefile wired correctly
- **Makefile automation** — `make test/conformance/smoke/test-all` all exist
- **File structure** — clean, extensible layout

---

### 🔴 Missing Entirely

**1. Conformance fixture format lacks sanitization support**

Current schema does exact JSON comparison. Real Slack API responses contain timestamps, IDs, and other volatile fields. Design doc (line 222) explicitly requires sanitization: _"we don't want it to fail because a date field had different days."_

The fixture format needs sanitization metadata — field ignore lists, pattern matchers for IDs/timestamps — before real recorded snapshots can be added without flakiness.

**2. Smoke tests are empty placeholders**

Design doc (lines 206–210) requires smoke tests proving these work:
- `auth.test`
- `conversations.create`
- `chat.postMessage`
- `conversations.list`
- Basic Bolt app integration
- Basic Socket Mode app integration
- (eventually) rate-limiting tier check

Two placeholder tests exist; none of the above are covered.

**3. Persistence validation entirely absent**

Design doc (line 266): smoke tests must prove data is actually persisted — not just that endpoints return plausible-looking responses. No tests for this exist anywhere in the suite.

**4. Zero OpenAPI spec validation infrastructure**

Design doc describes three conformance sub-strategies (lines 216–237):
1. Recorded snapshots — runner exists, 1 dummy fixture, format blocks real data
2. Real SDK apps (Bolt/Socket Mode E2E workflows) — nothing
3. OpenAPI spec validation — nothing

---

### 🟡 Partial

- **Conformance snapshots** — runner + 1 dummy fixture exist, but format blocks real API data
- **Smoke SDK integration** — harness starts/stops server correctly, but no real SDK tests written
- **Unit test parallelization** — `-shuffle=on` configured, but `t.Setenv()` in `xdg_test.go` blocks actual parallel execution

---

## Gap Summary Table

| Test Layer | Design Doc | Current | Gap |
|---|---|---|---|
| Unit tests | Simple, parallel, shuffle | 9 tests, shuffle ✓, parallel blocked | Minor |
| Smoke: SDK integration | auth, conversations, chat + Bolt + Socket | 0 implemented | Major |
| Smoke: persistence | Validate data is stored | 0 tests | Critical |
| Conformance: snapshots | Real API responses (sanitized) | 1 dummy fixture | Critical: format issue |
| Conformance: SDK apps | E2E workflows | 0 | Major |
| Conformance: OpenAPI | Shape validation | 0 | Major |
| Fixture sanitization | Field-level ignore rules | None | Critical |

---

## Prioritized Action List

### Tier 1 — Blocking (complete Milestone 0.5 properly)

1. **Extend conformance fixture schema** with sanitization support (field ignore lists, ID/timestamp pattern matchers)
2. **Implement smoke tests** for `auth.test`, `conversations.create`, `conversations.list`, `chat.postMessage` — with persistence validation (write then read back)
3. **Add Bolt app smoke test**
4. **Document fixture recording process** so future agents/developers have a clear path to add snapshots

### Tier 2 — High value (early Milestone 1)

5. **Fix unit test parallelization** — refactor `xdg_test.go` to avoid `t.Setenv()` blocking `t.Parallel()`
6. **Add Socket Mode smoke test**
7. **Add OpenAPI spec validator** — download Slack's published spec, wire up shape validation

### Tier 3 — Polish

8. **Rate-limiting tier check** in smoke tests
9. **Build out conformance snapshots** for core endpoints as they are implemented (conversations, chat, users, etc.)
