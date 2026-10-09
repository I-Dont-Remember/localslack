# Changelog

## Milestone 1 — Vertical slice (API + htmx UI over SQLite)
Playable end to end: `make run`, open http://localhost:3232, or point the Slack SDK at `/api/`. SQLite store (modernc + goose, writer/reader pools, WAL), deterministic seed, `service` layer (auth, names, monotonic ts, threads, DMs, cursor pagination), Slack methods `auth.test`, `users.list`, `conversations.{list,create,open,history,replies,archive}`, `chat.postMessage`, `GET /_localslack/state/conversations`, htmx UI (channels, threads, DMs, quick-switcher, acting-as), conformance setup/captures + `gap_diffs` ratchet, restart-persistence smoke test, Playwright e2e, CI. Arch review: random per-process runtime ids, single `server.Build`, endpoint-bound cursors. Deferred: Bolt (M2), Socket Mode (M3), rate limits (M5), OpenAPI (M6); see plan "Arch review follow-ups" for remaining items (`data` JSON columns, `Service.Reload`, `:memory:` DB path).

## Milestone 0.5 — Testing infrastructure + hardening
v2 lint config + ast-grep rules, pre-commit (build/test/ruff/tidy), `make setup`/`verify`, `scripts/with-server.sh` isolated live server, server middleware + injected slog, CLI double-start/stop-wait, parallel xdg tests, conformance v2 (matchers, known_gap, record.py), smoke harness with strict-xfail M1 tests. Deferred: Bolt (M2), Socket Mode (M3), rate limits (M5), OpenAPI (M6).

## Milestone 0.1 — Scaffolding
`localslack start` / `start -d` / `stop`, XDG PID file, health endpoint, unit tests, agent docs.

## Milestone 0 — Quality harness
golangci-lint v2 config, pre-commit hooks, Makefile targets.
