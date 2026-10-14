# Consolidation decisions

Candidates the `dry-consolidator` agent (or the owner) looked at and **declined**, so the next pass doesn't
re-argue them. Read before consolidating; add a row for every candidate you skip. Newest first.

Each row needs a concrete reason and a **Revisit when** condition (a count, a divergence, a requirement).
"Reviewed" is not a reason. A row is stale if its code moved or the condition is met: delete it or act on it.
Inline `//art-dupl:accept <reason> (expires=YYYY-MM-DD)` is only for a clone a tool keeps flagging that is
intentional; `make lint` rejects markers without a reason and expiry, `make debt` lists expired ones.

| Date | Where | Decision | Reason | Revisit when |
|---|---|---|---|---|
| 2026-10-13 | `admin/events.go` redeliver / redeliverDelivery | keep | 2 copies, not diverged; already share `redeliverBody.request()` + `respondRedelivery` | a third redeliver variant appears |
| 2026-10-13 | `api` conversationsLeave / conversationsArchive | keep | 2 copies of 6 lines; a helper adds code | a third conversation action copies the shape |
| 2026-10-13 | `Store.IsEmpty` vs `Tx.IsEmpty` | keep | 2 copies, not diverged | either changes |
| 2026-10-13 | `chi.URLParam(r, "x")` reads (30) | keep | already a one-liner; a typed wrapper removes nothing | path params gain validation/typing |
| 2026-10-13 | `if !e.decode(...) { return }` guards (5, `admin/events.go`) | owner question | a wrapper is restructuring, not deletion | owner asks for a handler wrapper |
| 2026-10-13 | `api.MethodNames` (deadcode) | keep | documented capabilities sync point (`internal/CLAUDE.md`) | the sync moves |
| 2026-10-13 | capabilities `Validate*`/`MarshalFixture` (deadcode) | owner question | only tests call them; extension point or delete with their tests? | owner answers |
| 2026-10-13 | `store` exec errors via `execErr` | keep separate | routing UPDATE/DELETE through `execErr` turns unique violations into `ErrDuplicate` (behavior change); `Tx.execOne` is the shared path | error mapping is redesigned |
