# CLAUDE.md

Layering (M1): `store` (SQL only) -> `service` (rules, typed domain errors) -> `api` / `web` (translation only). `api` and `web` never import each other or `store`.

- `cli`: CLI command utilities.
- `config`: env-driven settings (`Load(getenv)`).
- `server`: router (`Deps`), `Run`, middleware.
- `storetest`: `NewApp(t)` isolated app (temp DB, pinned seed) for tests.
- `xdg`, `constants`: paths and shared constants.
- Planned (see `agent-docs/plans/m1-vertical-slice.md`): `store`, `ids`, `seed`, `service`, `api`, `web`.
