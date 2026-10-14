# CLAUDE.md

Layering (M1): `store` (SQL only) -> `service` (rules, typed domain errors) -> `api` / `web` (translation only). `api` and `web` never import each other or `store`.

- `cli`: CLI command utilities.
- `config`: env-driven settings (`Load(getenv)`).
- `server`: `Build` (the single wiring point), router (`Deps`), `Run`, middleware, `guard` (Host/Origin/body limits), routing.
- `admin`: `/_localslack/*` control API, one file per feature (apps, events, calls, faults, reset, snapshots, socket, members). `calllog`, `faults`: leaf seams (Recorder, Interceptor) for api; depguard keeps them off store/service.
- `events`: Engine (HTTP delivery: signing, retries, trace, per-app config) behind the Dispatcher/Transport seams. `socket`: Socket Mode Hub (WebSocket transport for events; deny api/web/admin/store). `control`: Reset/Snapshot/Diff orchestrator over service+dispatcher. `scenario`: Fixture types, validate, generate, diff (leaf). `appmanifest`: manifest types (leaf). `respond`: shared JSON/error writers for `/api` and `/_localslack` (leaf). `server.Components` (build.go) wires the optional seams once; add a field, not shared lines. depguard in `.golangci.yml` enforces the layering.
- `storetest`: `NewApp(t)` isolated app (temp DB, pinned seed) for tests.
- `capabilities`: embedded `capabilities.toml` manifest (methods, behaviors, deviations) + tests that keep it in sync with `api.MethodNames()`.
- `xdg`, `constants`: paths and shared constants.
- `store` (SQL only, writer+reader pools), `ids`, `seed` (deterministic demo data).
- `service`: rules (auth, names, ts, threads, DMs); owns `Conversation`/`Message`; `Team`/`User` are aliases of store rows.
- `api`: Slack Web API (/api/{method}), DTO mappers, error-code table. `web`: htmx UI (/ui, cookie "acting as"). Neither imports `store` or each other.

Per-dir conventions: `admin/CLAUDE.md` (/_localslack response shape, route-contract test), `api/CLAUDE.md` (Slack-shaped responses).
