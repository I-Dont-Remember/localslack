# Endpoint Implementation Guide

Process for adding a new Slack API endpoint.

## Steps

1. **Identify the endpoint** — method name (e.g. `chat.postMessage`), HTTP path, rate-limit tier, required OAuth scopes. Reference Slack API docs.

2. **Write the failing test first** — handler unit test in `internal/server/`. Run `make test` and confirm it fails.

3. **Register the route** — add the route in the chi router in `internal/server/server.go`.

4. **Implement the handler** — follow `agent-docs/coding_standards.md`:
   - Access context via `r.Context()`
   - Use `slog` for structured logging
   - Return Slack-shaped JSON (`"ok": true/false`)

5. **Add repository layer if needed** — persistence via SQLite (`modernc.org/sqlite`, no ORM). Add to `internal/store/` (M1).

6. **Tag with rate-limit tier (M5+)** — attach tier middleware so the rate limiter enforces it.

7. **Run `make check && make test`** — must pass before committing.

8. **Update smoke test** if applicable.

## Rules

- All responses: JSON with `"ok": true` on success, `"ok": false, "error": "<code>"` on failure.
- Unimplemented endpoints: return `{"ok": false, "error": "not_implemented"}` — HTTP 404 with that body (matches `notImplementedHandler`).
- Always accept POST: the Slack Python SDK sends POST even to GET-only endpoints.
- Rate-limit tiers: Default=60/min, Tier1=1, Tier2=20, Tier3=50, Tier4=100/min.
