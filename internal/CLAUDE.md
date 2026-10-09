# CLAUDE.md

Layering (M1): `store` (SQL only) -> `service` (rules, typed domain errors) -> `api` / `web` (translation only). `api` and `web` never import each other or `store`.

- `cli`: CLI command utilities.
- `config`: env-driven settings (`Load(getenv)`).
- `server`: router (`Deps`), `Run`, middleware.
- `storetest`: `NewApp(t)` isolated app (temp DB, pinned seed) for tests.
- `xdg`, `constants`: paths and shared constants.
- `store` (SQL only, writer+reader pools), `ids`, `seed` (deterministic demo data).
- `service`: rules (auth, names, ts, threads, DMs); re-exports store row types for api/web.
- Planned (see `agent-docs/plans/m1-vertical-slice.md`): `api`, `web`.
