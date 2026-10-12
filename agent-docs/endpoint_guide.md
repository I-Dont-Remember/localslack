# Endpoint Implementation Guide

Process for adding a new Slack API endpoint. Check `internal/capabilities/capabilities.toml` first
(see `capabilities.md`): if the method is already `in_progress`, coordinate instead of duplicating.

## Steps

1. **Claim and identify** — in `capabilities.toml` set the method `status = "in_progress"` with
   `plan = "agent-docs/plans/<file>.md"`. Note its docs tier and scopes (unverified until recorded).

2. **Write the failing test first** — handler test in `internal/api/` as an **external** package
   (`api_test`) using `storetest.NewApp(t)` (real temp SQLite, no mocks). Run `make test`, confirm it fails.

3. **Register the method** — add it to the `registry()` map in `internal/api/api.go` (handler shape
   `func(ctx, *call) (map[string]any, error)`; return domain errors, `api` renders them). Wiring lives
   only in `server.Build` (shared by `Run` and `storetest`).

4. **Implement** — follow `agent-docs/coding_standards.md`. Rules go in `internal/service`, SQL in
   `internal/store` (SQLite via `modernc.org/sqlite`, no ORM). `api`/`web` never import `store`
   (depguard). Map each `service.Err*` to a Slack code in `api/errors.go` (and `web.fail` for the UI);
   never `json.Marshal` a service type directly, use explicit DTO mappers.

5. **Add a conformance fixture** in `tests/conformance/fixtures/` for the success and main error
   shapes, with a truthful `source` (`llm` unless you cited docs or recorded it; see
   `tests/conformance/RECORDING.md`).

6. **Call it through the real SDK** in `tests/smoke` (write-then-read-back; this also derives the
   SDK-called column). Cover SDK pagination iterators if the method paginates.

7. **Update the manifest** — `status = "implemented"` (or `partial`), tier, scopes, and a
   `[deviations.<id>]` entry for anything you knowingly do differently from Slack (add a
   `@deviation("<id>")` strict-xfail test in `tests/smoke/known_deviations_test.py` when practical).
   Then `make docs`.

8. **`make verify`** must pass before committing.

## Rules

- All responses: JSON with `"ok": true` on success, `"ok": false, "error": "<code>"` on failure.
- Real Slack API errors are HTTP **200** with `ok:false` (e.g. `channel_not_found`, `name_taken`).
  LocalSlack's deliberate or infrastructural non-200s: 404 `not_implemented` (unknown method; Slack
  returns 200 `unknown_method`, see the `unknown-method-404` deviation), 403 `invalid_host` /
  `forbidden_origin` (guard), 405 `method_not_allowed`, 500 `internal_error` (unmapped fault), and
  later 429 for rate limits (M5). Under `/_localslack/` the same server-level failures use the control-API
  shape `{"error":{"code","message"}}` instead (`not_found`, `method_not_allowed`, `invalid_host`,
  `forbidden_origin`, `payload_too_large` 413, `internal_error`); one writer: `internal/respond`.
- Always accept POST: the Slack Python SDK sends POST even to GET-documented methods.
- Handler tests for `api`/`web` must be external packages (`api_test`, `web_test`): `storetest`
  imports `server`, which imports them, so in-package tests would be an import cycle.
- Authorization is deliberately permissive for channels: no membership checks on read/post (`caller`
  is passed through so a rule can be added later); only im/mpim listings are filtered by membership.
  This is a documented deviation (`no-membership-check`).
- Rate-limit tiers are the manifest's per-method `tier` (Slack tiers: 1 = 1/min, 2 = 20/min,
  3 = 50/min, 4 = 100/min; default 60/min). M5 enforces them; do not add a second table.
- Web UI errors: map new `service.Err*` in `web.fail` (HTTP status) as well as in `api/errors.go`.
- Server hardening lives in `server/guard.go` (Host allowlist on loopback binds, cross-origin write
  refusal, 2MB body cap). Default bind is `127.0.0.1:3232`; expose deliberately with
  `LOCALSLACK_ADDR=0.0.0.0:3232`.
