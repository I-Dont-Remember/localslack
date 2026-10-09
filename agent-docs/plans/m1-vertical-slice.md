# M1 vertical slice (branch `milestone/1-vertical-slice`)

Playable end to end: SDK + htmx UI over SQLite. Ask user before stacking on unmerged 0.5 branch.
Per endpoint: handler test (real temp-file SQLite via `storetest`, no mocked stores) + conformance fixture + smoke write-then-read-back. Remove that test's `@m1` xfail marker in `tests/smoke/m1_api_test.py` when it goes green.

## Architecture decisions (from arch review 2026-10-09)
- Layering: `store` (SQL only) -> `service` (rules; typed domain errors e.g. `ErrNameTaken`) -> `api` / `web` (translation only: Slack error codes / HTML). `api` and `web` never import each other or `store` (enforce with `depguard` in `.golangci.yml`). Service owns: reply_count/latest_reply bump, name validation, DM open-or-create, ts generation, token/acting-as resolution.
- `internal/localslack/` -> delete or use for service; update `internal/CLAUDE.md` package map.
- DI: `internal/config` (env struct + test: ADDR, DB_PATH, SEED, NO_SEED); `server.NewRouter(Deps{Log, Svc})`; `Run` opens store, migrates, seeds, serves, closes on shutdown; `start.go` stays thin. `api`/`web` expose `Mount(chi.Router)`. `/_localslack/health` works with nil service. Unknown methods stay 404 `not_implemented`.
- Error convention: real Slack API errors are HTTP 200 + `ok:false`; only `not_implemented` is 404 (deliberate). Document in `endpoint_guide.md`.
- ts: unique+monotonic across restarts/concurrent writers. Process-wide generator seeded from `max(ts)` in DB at open, advanced inside the writer txn; test with `-race` + parallel postMessage. All writes go via the single writer conn.
- SQLite: per-connection pragmas (busy_timeout, foreign_keys, WAL) via DSN `_pragma=` on BOTH pools; test write-while-read. State only in tables so `reset` = DELETE + reseed.
- Lint policy: `gochecknoglobals` exclusion for `//go:embed`/goose FS; templates parsed at startup returning errors (no `template.Must`/panic); `QueryContext`/`ExecContext` only.

## Commit order (arch-reviewer mid-run after 3 exists)
1. [ ] config + DI skeleton + `storetest.NewApp(t)` (temp DB, pinned seed, router)
2. [ ] store + ids + seed (determinism test) + depguard
3. [ ] service layer (domain errors, ts, thread bump) -> **run arch-reviewer here**
4. [ ] token middleware + auth.test + users.list (+ negative auth: missing token, invalid_auth, unknown xoxp) -> flip `auth_test.json` to passing
5. [ ] conversations.list/create/history/replies/open (+ cursor pagination test)
6. [ ] chat.postMessage (thread_ts bump, channel_not_found)
7. [ ] `GET /_localslack/state/conversations`
8. [ ] web UI (below)
9. [ ] e2e (below)
10. [ ] finish (below)

## Details
### Storage (`internal/store`, `internal/ids`)
- modernc sqlite + embedded goose migrations; `LOCALSLACK_DB_PATH` (default `xdg.AppDataDir()/localslack.db`)
- tables teams/users/conversations(channel|im|mpim)/members/messages: typed cols + JSON `data`; messages PK (channel_id, ts), thread_ts, reply_count, latest_reply
- ids: Slack-style T/U/C/D ids
### Seed (`internal/seed`)
- `LOCALSLACK_SEED` (random+logged if unset), `LOCALSLACK_NO_SEED=1`; 1 team, 1-10 users + `localslack-bot`, 2-50 channels (`#general` always), one channel 3-25 msgs incl. a thread; only when DB empty; same seed => same data (test)
- `scripts/with-server.sh` exports a fixed `LOCALSLACK_SEED`; fail if binary stale/SKIP_BUILD misuse
### API (`internal/api`, `/api/{method}`, GET+POST)
- param parsing (form/JSON/query); token middleware (`xoxb-` -> bot, `xoxp-<userID>` -> user, missing -> `not_authed`, unknown -> `invalid_auth`)
- methods: auth.test, users.list, conversations.list (types, exclude_archived), create (name_taken, invalid_name), history (newest first), replies, open (users -> im/mpim), chat.postMessage; opaque cursor pagination
### Conformance upgrades needed for M1
- one app per fixture (`storetest.NewApp`), pinned seed; optional fixture `setup` requests with `{{captures}}`; document in RECORDING.md
- `known_gap` ratchet: record expected diff set (`gap_diffs`); fail if it changes (so partly-right -> broken is caught)
### Smoke upgrades
- pick non-bot, non-self user explicitly for DMs; pagination test; `persist across restart` test (restart server on same DB via with-server variant)
### UI (`internal/web`, html/template + vendored htmx via go:embed)
- `/` -> `/ui/c/{general}`; `/ui/c/{id}`, `/ui/c/{id}/t/{ts}`; sidebar (channels, DMs); quick-switcher `GET /ui/switch?q=`; channel view polling 2s; composer; thread pane; "acting as" user cookie
- calls service directly (never HTTP to self); httptest HTML assertions
### E2E
- add `playwright`+`pytest-playwright` to `tests/smoke/pyproject.toml`; `playwright install chromium` in `make setup`; `tests/e2e/` dir; `make e2e` via with-server.sh; add to `test-all`/`verify`
- tests: seeded channels visible; UI post -> SDK history; SDK post -> UI; thread reply; DM via switcher; screenshots on fail to `tmp/`
### Finish
- [ ] minimal GitHub Actions workflow running `make setup verify`
- [ ] `make verify` green; `arch-reviewer`; STATUS/CHANGELOG/README "Try it"
