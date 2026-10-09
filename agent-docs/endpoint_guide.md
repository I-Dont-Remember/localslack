# Endpoint Implementation Guide

Process for adding a new Slack API endpoint.

## Steps

1. **Identify the endpoint** — method name (e.g. `chat.postMessage`), HTTP path, rate-limit tier, required OAuth scopes. Reference Slack API docs.

2. **Write the failing test first** — handler test in `internal/api/` as an **external** package (`api_test`) using `storetest.NewApp(t)` (real temp SQLite, no mocks). Run `make test` and confirm it fails.

3. **Register the method** — add it to the `registry()` map in `internal/api/api.go` (handler shape `func(ctx, *call) (map[string]any, error)`; return domain errors, `api` renders them). Wiring lives only in `server.Build` (shared by `Run` and `storetest`). Add a conformance fixture in `tests/conformance/fixtures/` (see `RECORDING.md`) for the success and main error shapes.

4. **Implement the handler** — follow `agent-docs/coding_standards.md`:
   - Handlers receive `ctx` from the request; they never touch `http` types
   - Use `slog` for structured logging
   - Return Slack-shaped JSON (`"ok": true/false`)

5. **Rules go in `internal/service`, SQL in `internal/store`** (SQLite via `modernc.org/sqlite`, no ORM). `api`/`web` never import `store` (depguard). Map each `service.Err*` to a Slack code in one table; never `json.Marshal` a service type directly, use explicit DTO mappers.

6. **Tag with rate-limit tier (M5+)** — attach tier middleware so the rate limiter enforces it.

7. **Run `make check && make test`** — must pass before committing.

8. **Update smoke test** if applicable.

## Rules

- All responses: JSON with `"ok": true` on success, `"ok": false, "error": "<code>"` on failure.
- Real Slack API errors are HTTP **200** with `ok:false` (e.g. `channel_not_found`, `name_taken`). Only `not_implemented` is a deliberate 404.
- Handler tests for `api`/`web` must be external packages (`api_test`, `web_test`): `storetest` imports `server`, which imports them, so in-package tests would be an import cycle.
- Authorization is deliberately permissive for channels: no membership checks on read/post (`caller` is passed through so a rule can be added later); only im/mpim listings are filtered by membership.
- Unimplemented endpoints: return `{"ok": false, "error": "not_implemented"}` — HTTP 404 with that body (matches `h.notImplemented` in `internal/server`).
- Always accept POST: the Slack Python SDK sends POST even to GET-only endpoints.
- Rate-limit tiers: Default=60/min, Tier1=1, Tier2=20, Tier3=50, Tier4=100/min.
- Web UI errors: map new `service.Err*` in `web.fail` (HTTP status) as well as in `api/errors.go` (Slack code).
- Server hardening lives in `server/guard.go` (Host allowlist on loopback binds, cross-origin write refusal, 2MB body cap). Default bind is `127.0.0.1:3232`; expose deliberately with `LOCALSLACK_ADDR=0.0.0.0:3232`.
