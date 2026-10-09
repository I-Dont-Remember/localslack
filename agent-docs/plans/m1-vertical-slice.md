# M1 vertical slice (branch `milestone/1-vertical-slice`)

Playable end to end: SDK + htmx UI over SQLite. Ask user before stacking on unmerged 0.5 branch.
Every endpoint: handler test + conformance fixture + smoke write-then-read-back test. Flip M0.5 xfails to pass.

## 1a Storage (`internal/store`, `internal/ids`)
- [ ] modernc sqlite + embedded goose migrations; WAL + busy_timeout; 1 writer conn + reader pool; `LOCALSLACK_DB_PATH` (default `xdg.AppDataDir()/localslack.db`)
- [ ] tables teams/users/conversations(channel|im|mpim)/members/messages: typed cols + JSON `data`; messages PK (channel_id, ts), thread_ts, reply_count, latest_reply
- [ ] ids: Slack-style T/U/C/D ids; monotonic unique `ts` generator
- [ ] `storetest.New(t)` helper (temp-file DB)

## 1b Seed (`internal/seed`)
- [ ] `LOCALSLACK_SEED` (random+logged if unset), `LOCALSLACK_NO_SEED=1`; 1 team, 1-10 users + `localslack-bot`, 2-50 channels (`#general` always), one channel 3-25 msgs incl. a thread; only when DB empty; same seed => same data (test)

## 1c API (`internal/api`, `/api/{method}`, GET+POST)
- [ ] param parsing (form/JSON/query); token middleware (`xoxb-` -> bot, `xoxp-<userID>` -> user, missing -> `not_authed`)
- [ ] auth.test, users.list
- [ ] conversations.list (types, exclude_archived), conversations.create (name_taken, invalid_name)
- [ ] conversations.history (newest first), conversations.replies, conversations.open (users -> im/mpim)
- [ ] chat.postMessage (thread_ts bumps parent reply_count; channel_not_found)
- [ ] cursor pagination (opaque base64 offset, `response_metadata.next_cursor`)
- [ ] `GET /_localslack/state/conversations`

## 1d UI (`internal/web`, html/template + vendored htmx via go:embed)
- [ ] `/` -> `/ui/c/{general}`; `/ui/c/{id}`, `/ui/c/{id}/t/{ts}`
- [ ] sidebar (channels, DMs); quick-switcher `GET /ui/switch?q=`; channel view polling 2s; composer; thread pane; "acting as" user cookie
- [ ] UI uses store/service funcs directly (never HTTP to self); httptest HTML assertions

## 1e E2E (`tests/smoke/e2e_test.py`, pytest-playwright, `make e2e` via with-server.sh, in `verify`)
- [ ] seeded channels visible; UI post -> SDK history; SDK post -> UI; thread reply; DM via switcher; screenshots on fail to `tmp/`

## 1f Finish
- [ ] `make verify` green; `arch-reviewer`; STATUS/CHANGELOG/README "Try it"
