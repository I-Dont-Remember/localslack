# Plan: test-loop (ROADMAP tier 1 + tier 2 items 5-10)

Branch: `test-loop` (integration). Streams branch from it as `test-loop/w<N>-<stream>`. Checkboxes: tick in the commit that completes the task.


## Context
Owner chose depth over breadth: emulate/mocks cover method count; LocalSlack wins on the test loop (events, socket mode, call log, reset, faults, seeding, snapshot/diff). Few basic endpoints, then deep test-partner features. Owner wants parallel subagent execution and a hands-on QA experience (demo app, inspector UI, QA guide), not just code review.

## Decided with owner (don't re-ask)
- App registration: real Slack app manifest (JSON + YAML) via `--app-manifest`/env AND `POST /_localslack/apps`.
- **Multiple apps** per workspace, like a real workspace. Each app gets its own bot user (`U…`) + `bot_id` (`B…`), bot token, app token, signing secret, subscriptions.
- Membership: strict, scope-gated. Bot gets `not_in_channel` on public channels it hasn't joined unless manifest has `chat:write.public` (default scopes include it). UI actor auto-joins public channel on post.
- Unknown method → 200 `unknown_method` + `X-LocalSlack-Not-Implemented: 1` header + call-log/inspector badge.
- Deps: `github.com/coder/websocket`, `go.yaml.in/yaml/v3` (network fetch OK).
- QA deliverables: `make demo` (+ `make demo-socket`), `/ui/inspector`, `docs/QA.md` + `docs/qa/*.md`.
- Execution: integration branch `test-loop`; parallel subagents in worktrees per wave; orchestrator merges + `make verify`; **pause for owner review at each gate**.
- Out of scope: item 11 (owner recordings), Tier 3+.

## Design facts that shape the plan
1. Bolt self-filters by `bot_id` from `auth.test`. Messages carry no `bot_id` today (`fabricated-bot-id` deviation) → store real `bot_id` on users/messages in Wave 0, or demo bot loops on its own replies.
2. Membership enforcement breaks current flows (xoxp-created channel lacks bot; SDK-created channel lacks UI actor) → audit smoke/e2e in stream X.
3. `api.serve` (`internal/api/api.go`) is the shared hook point for call log, faults, headers → split into stages in Wave 0 so streams don't collide.
4. Unused `messages.data` JSON column holds blocks/attachments/metadata/edited.
5. Snapshot export format == scenario fixture format → seed/reset/snapshot/restore/diff share one path.
6. Events must be emitted from `service` (UI posts call `svc.PostMessage` directly, `internal/web/handlers.go`).

## Multi-app model
- `apps` table, many rows: `id, name, manifest JSON, signing_secret, bot_token, app_token, verification_token, request_url, bot_events JSON, scopes JSON, socket_mode, verified, bot_user_id, bot_id`.
- Register creates a bot user (name = `features.bot_user.display_name`); app persists across reset (bot users re-created by `control.Reset` after wipe, same IDs).
- Credentials deterministic from app id, overridable in body/flag. Short non-real-looking token shapes (push protection); Python tests keep `fake_tokens.py`.
- `service.Authenticate`: registered bot/app tokens → that app's bot user + app ctx; unregistered `xoxb-*` → default seeded bot (back-compat); `xoxp-<U>` unchanged; `xapp-*` only for `authApp` methods.
- Dispatcher fans out per app: subscription match + that app's bot membership (or `app_mention` of that bot). Each app gets its own envelope (`api_app_id`, `authorizations` = its bot). Apps see each other's bot messages (with `bot_id`) — enables multi-app demo.
- `--app-manifest` repeatable. Deviation `single-app` dropped.

## Pre-flight (owner runs manually before Wave 0)
- `! go mod download github.com/coder/websocket@latest go.yaml.in/yaml/v3@latest` — warm module cache (sandbox network may block proxy.golang.org); does not edit go.mod.
- `! pre-commit install-hooks` — warm `~/.cache/pre-commit` (not sandbox-writable); commits still run unsandboxed → approve `git commit` prompts, or allowlist it via `/permissions`.
- Confirm `make verify` green on `main` (sanity check that Playwright/uv envs are set up).
- Demos (`make demo`, `make demo-socket`) are run by the owner in their own terminal, not by me — sandboxed processes may not be reachable from the owner's browser.

## Waves

### Wave 0 — foundations (orchestrator, serial, ~11 atomic commits)
1. Plans: master `agent-docs/plans/test-loop.md`; sub-plans for L items `events-http.md`, `socket-mode.md`, `scenarios-snapshots.md`. Contract for all `/_localslack/*` shapes in `docs/TESTING_API.md`. Claims on `main` (8 bolt-core methods, `apps.connections.open` → `in_progress`); new behaviors `call-log`, `fault-injection`, `app-manifest`, `event-delivery-controls`, `state-snapshot`. Update ROADMAP/STATUS. `arch-reviewer` pass on plans.
2. Registry → `map[string]spec{fn, auth}` (`authToken|authNone|authApp`); per-file registration funcs (`a.messageMethods()` etc.) so streams never edit `api.go`. No tier in spec (manifest owns it).
3. Split `serve`: resolve → parse (also returns raw JSON map) → `Interceptor` (faults) → auth → call → respond, wrapped by `Recorder`. `write`/`fail` → `api/respond.go`.
4. `internal/admin`: `Mount` + one file per feature (stubs); move health/state; caller-free snapshot; fix N+1. depguard: admin/control/events/calllog/faults ⟂ store.
5. Migration `00002_test_loop.sql`: `apps`, `users.bot_id`, `conversations.is_private`, `reactions`. Reserved ranges for later: A=10, B=20, E=30, G=40, X=50, S=60.
6. Service-owned types: `User.BotID`; `Message{BotID, Subtype, Blocks, Attachments, Metadata, Edited, Reactions}`; `Conversation.IsPrivate`. `PostMessage(ctx, author, PostOpts{Channel, ThreadTS, Content})`. Seed sets `bot_id`; `auth.test` returns stored one. DTO split into `addContent/addReactions/addEdited/addBot` helpers.
7. Event seam `service/events.go`: `MessagePosted`, `MessageChanged`, `MessageDeleted`, `ReactionAdded`; `Service.Subscribe(fn)` fired only after commit, non-blocking.
8. Pre-allocate all new domain errors + mappings in `service/errors.go`, `api/errors.go`, `web.fail` (message_not_found, cant_update/delete_message, already_reacted, invalid_name (reaction), not_in_channel, user_not_in_channel, invalid_blocks/attachments/metadata, msg_too_long, not_allowed_token_type, invalid_name_{required,specials,maxlength,punctuation}).
9. Config + CLI in one commit: `AppManifest` (repeatable), `EventTimeout` (3s), `EventRetryDelays` (`0s,1m,5m`), `Scenario`, `CallLogSize`, `DeliveryLogSize`; `startDetached` passes through; `localslack reset` stub. `with-server.sh` exports fast retries (`0s,100ms,200ms`, 500ms timeout).
10. Skeleton pkgs wired in `server/build.go` + `storetest.App`: `calllog`, `faults`, `events` (`Dispatcher`, `Transport`), `appmanifest`, `control` (Reset/Snapshot orchestrator shared by admin + web), `scenario` interface.
11. Python helper `tests/smoke/localslack_admin.py` (`Admin` client for every endpoint, long-poll waits) + fixtures `admin`, `calls_since`, `faults`; `docs/QA.md` index + `docs/qa/<stream>.md` stubs.

**Gate 0 (owner):** review `docs/TESTING_API.md` contract — cheapest time to change API shapes.

### Hot-spot ownership
| Hot spot | Rule |
|---|---|
| `api.go` registry, errors, config, migrations | Frozen after Wave 0; streams use own files/reserved ranges; ask orchestrator for additions |
| `capabilities.toml` | Streams flip only their own entries; `COMPATIBILITY.md` regenerated via `make docs` at merge |
| `layout.html` | W1: only B2 (react button). W2: only I (nav link) + X (lock icon) |
| `conftest.py` | Frozen after 0.11; streams add own `*_test.py` |
| QA docs / TESTING_API.md | One file/section per stream |

### Wave 1 — 6 parallel worktree agents
- **A Events HTTP (L)** — `internal/appmanifest`, `internal/events/{dispatcher,http,sign,payload,trace,controls}.go`, `service/apps.go`, `store/apps.go`, `admin/{apps,events}.go`, `examples/demo-bolt/` (HTTP), `scripts/demo.sh`.
  - Apps API: `POST/GET /_localslack/apps`, `GET/DELETE /_localslack/apps/{id}`, `POST …/{id}/verify`. Body = raw manifest (JSON/YAML) or `{manifest, signing_secret, bot_token, app_token, app_id, verify}`. Bad manifest → 400 `invalid_manifest` with JSON-pointer errors; unsupported events → `warnings[]`.
  - `url_verification` signed; failure → `verified:false`, still deliver (permissive; `strict_verification` opt-in).
  - Dispatcher: queue + workers; mapping `MessagePosted`→`message.{channels,groups,im,mpim}` + `app_mention`; `MessageChanged/Deleted`→subtypes; `ReactionAdded`→`reaction_added`. `event_callback` envelope w/ `event_id`, `authorizations`. v0 HMAC signing, `X-Slack-Request-Timestamp`, timeout, retries w/ `X-Slack-Retry-Num/Reason`, honors `X-Slack-No-Retry`. Injected clock; generation counter for reset.
  - Trace ring buffer; `GET /_localslack/deliveries?event_type=&state=&app_id=&since=&min=&timeout_ms=` (long-poll), `GET …/{id}`, `DELETE`.
  - Controls: `POST /_localslack/events` (inject, `signature: valid|invalid|stale|missing`, target `app_id`), `POST /_localslack/events/{event_id}/redeliver` (duplicate w/ retry headers), `GET/PUT /_localslack/delivery-config` (signature mode, timeout, retry delays, `paused`).
  - Tests: Go w/ `httptest` fake app (sig, verification, retries, timeout, no-retry, membership, mapping table, UI post emits, two apps both receive). Smoke `events_http_test.py`: real Bolt in thread, mention → reply; bad sig → 401 + retries; redeliver duplicate.
  - Demo app `examples/demo-bolt/app.py` (uses `tests/smoke` uv env, `DEMO_MODE=http|socket`): mention→"pong" in thread; "hello"→greeting; `reaction_added`→"saw :x:". Second tiny app `examples/demo-bolt/echo.py` to show multi-app interaction. `make demo` starts LocalSlack (seed `demo`) + both apps, registers, prints URLs, cleans up on Ctrl-C.
- **B1 Read methods** — `conversations.info`, `users.info`, `conversations.members` (cursor), `api.test` (`authNone`, `?error=`). Handler test, fixture, SDK smoke each.
- **B2 Write methods** — `chat.update` (author-only, `edited`, emits Changed), `chat.delete` (reply_count fix; root w/ replies → tombstone + deviation), `chat.postEphemeral` (in-memory ring, no history/events), `reactions.add` (history gains `reactions[]`, emits `ReactionAdded`). UI react button.
- **C Call log + reset** — `internal/calllog`, `admin/{calls,reset}.go`, `control/reset.go`, `service/reset.go`, `store/wipe.go`, `cli/reset.go`.
  - Record: seq, time, request_id, method, status, ok, error, params (raw JSON; token/secret redacted), actor `{user_id, app_id, token_kind}`, response (≤64KB), duration, `fault_id`, `not_implemented`.
  - `GET /_localslack/calls?method=&channel=&app_id=&ok=&error=&since=&limit=&min=&timeout_ms=`; `DELETE`.
  - `POST /_localslack/reset {seed?, scenario?, generate?, keep?[]}` → pause dispatcher, wipe+repopulate in one tx (keep apps; re-create app bot users), `ts.advanceTo`, reseed runtime id gen, clear calls/deliveries/faults unless kept, resume. CLI `localslack reset`.
  - Tests: reset during concurrent posts (race), ts monotonic, redaction, filters, long-poll, ring wrap.
- **D Faults** — `internal/faults`, `admin/faults.go`. Rule `{method|"*", match{channel,app_id}?, nth?, every?, times?, once?, action{status, error, retry_after, delay_ms, body}}`; first match in creation order wins; 429 sets `Retry-After`; `delay_ms` alone = slow then real call. CRUD endpoints. Smoke: SDK `RateLimitErrorRetryHandler`, `ServerErrorRetryHandler`, timeout, nth=3.
- **E Blocks/attachments/metadata** — `service/content.go`, `addContent`. Accept JSON strings or decoded values; validate; blocks-only accepted; stored in `messages.data`; metadata only with `include_all_metadata`. Flips `no-blocks` red tests.

**Gate 1 (owner QA via `docs/qa/*.md`, curl/jq):** `make demo` mention/hello/react; two apps interacting; invalid-signature mode → failures + retries; redeliver duplicate; 429 fault → delayed reply + `fault_id` in call log; reset; update/delete/reactions/blocks via SDK. Then `arch-reviewer`.

### Wave 2 — 5 parallel worktree agents
- **S Socket Mode (L)** — `internal/events/socket`, `api/methods_socket.go`, `admin/socket.go`. `apps.connections.open` (`xapp-`, one-time 30s ticket URL); WS `hello`, `events_api` envelopes, ack by `envelope_id`, no-ack → retry via shared engine (`SocketTransport` implements `events.Transport`); round-robin multi-conn per app; none → `no_socket_connection` + retry. Controls: `GET /_localslack/socket/connections`, `POST /_localslack/socket/disconnect {connection_id?, reason, drop}`. Smoke: Bolt `SocketModeHandler` mention→reply; forced refresh → reconnect → still delivered. `make demo-socket`.
- **X Access edge cases** — `service/access.go` single `canRead/canPost` rule set; private channels (`is_private`, `types=private_channel`, non-member → `channel_not_found`); bot non-member public → `not_in_channel` (history; post unless `chat:write.public`); archived → `is_archived` on post/update/react; UI auto-join + lock icon. Audit/fix smoke+e2e. Flip `no-membership-check`, `no-private-channels`.
- **F Error/header fidelity** (one commit each) — `x-slack-req-id`, content-type charset; `x-oauth-scopes` (app's scopes) / `x-accepted-oauth-scopes` (manifest); `unknown_method` 200 + LocalSlack header; split `invalid_name_*`; `msg_too_long`; `users.list` pagination if time. Each flips its red test.
- **G Scenarios + snapshot/diff (L)** — `internal/scenario` (Fixture v1 JSON/TOML/YAML; Validate w/ JSON pointers: ID prefixes, dups, refs, ts format, thread roots, synthetic emails only; Populate; Generate `{seed, users, channels, messages_per_channel, thread_ratio, private_channels}`; Diff). Refactor `seed` to emit a Fixture (keep determinism test). Entry: `start --scenario`, `reset {scenario}`, `localslack reset --scenario`. Snapshots in memory: `POST/GET /_localslack/snapshots`, `GET …/{name}` (fixture JSON, reusable as scenario), `POST …/{name}/restore`, `GET /_localslack/state`, `GET /_localslack/diff?from=&to=current` (users/conversations/members/messages/reactions added/removed/changed, deterministic order). Perf target: 400 users × 50 channels < 2s. `examples/scenarios/acme.json`.
- **I Inspector UI** — `web/inspector.go`, `templates/inspector.html`, nav link. htmx 1s poll panels: calls (expand params/response, filters, fault/not-implemented badges), deliveries (attempts, sig mode, latency, retry reason, payload, redeliver), faults (add/delete, seen/fired), apps (verified, subscriptions, credentials), socket connections (disconnect), controls (reset, snapshot, diff, delivery-config toggles). web imports control/calllog/events/faults, never admin/api. Playwright `tests/e2e/inspector_test.py`.

**Gate 2 (owner QA):** `make demo-socket` + inspector disconnect/reconnect; inspector live while poking UI; reset/snapshot/diff from browser; `--scenario examples/scenarios/acme.json` exact IDs; generated scale speed; private channel invisibility; unknown method header.

### Wave 3 — polish (orchestrator + 1 agent)
`docs/QA.md` final walkthrough (per feature: steps, expected result, "what to give feedback on" prompts); README "Try it"; Playwright inspector e2e; CHANGELOG/STATUS/ROADMAP; COMPATIBILITY regen; final `arch-reviewer`; `make verify`; `make review`. Owner merges `test-loop` → `main`.

## Subagent execution protocol
- Each stream: `Agent` with `isolation: "worktree"`, branch `test-loop/w<N>-<stream>` from latest `test-loop`; prompt = sub-plan section + owned files + "red-green TDD, atomic commits passing `make check && make test`, update own capabilities entries + `docs/qa/<stream>.md`, don't touch frozen files".
- Orchestrator merges streams one at a time, `make docs && make verify` after each, fixes integration breakage, then gate.
- L sub-plans get `arch-reviewer` before their wave starts.

## Critical files
`internal/api/api.go`, `internal/api/params.go`, `internal/service/{service,messages,channels,errors,types}.go`, `internal/server/{build,server}.go`, `internal/storetest/app.go`, `internal/store/migrations/`, `internal/seed/seed.go`, `internal/config/config.go`, `internal/cli/start.go`, `internal/web/{web,handlers}.go`, `internal/web/templates/layout.html`, `internal/capabilities/capabilities.toml`, `tests/smoke/{conftest,bolt_test,known_deviations_test}.py`, `scripts/with-server.sh`, `Makefile`.

## Verification
- Each merge: `make docs && make verify` (Go unit, conformance, tools, smoke incl. real Bolt HTTP + socket delivery, Playwright e2e).
- Timing tests use long-poll `min/timeout_ms` + injected clock, never sleeps; reset tests run last; fault fixture clears on teardown.
- Owner hands-on at every gate: `make demo` / `make demo-socket`, `/ui/inspector`, `docs/qa/*.md` scenarios with expected results + feedback prompts.

## Risks
- Event/socket/tombstone payload shapes are `llm` evidence until recorded (item 11).
- Membership enforcement may ripple through existing tests (X audits).
- Multi-app adds token→app routing + per-app bot users; covered in Wave 0.6/0.10 + A.
- Wave 0 is large but is the main conflict-avoidance lever; don't start Wave 1 early.
- Commits need unsandboxed pre-commit; no literal tokens in repo.

## Arch-review amendments (supersede conflicting text above)
**Extra/changed Wave 0 steps**
- 0.2a **Principal**: `service.Principal{User, App *App, TokenKind, Scopes}`; `Identify(token)` (non-failing) + `Authenticate`; in-memory app cache (token->app) invalidated on register/delete/reset; migrate api/web; `state.go` uses an internal service call, not a fake token. Unregistered `xoxb-*` -> default bot (no app: no events, scope gating skipped); add `--strict-tokens` + per-app revoke so `invalid_auth`/`token_revoked` are testable.
- 0.3 pipeline order: resolve -> parse -> Identify -> Interceptor(faults) -> enforce auth -> call, all inside Recorder (incl. unknown-method fallback). Faulted call with bad token is still faulted and logged.
- 0.4 depguard as explicit step: deny store from admin/control/events/calllog/faults/scenario/appmanifest/seed; deny api->admin/events/control, web->admin, events->api/web, admin->api/web. `calllog`, `faults`, `scenario` are leaf pkgs (no service import); service consumes `scenario.Fixture` (`svc.Populate`, `svc.Export`). `seed` emits a Fixture, stops importing store.
- 0.5 migration: goose `WithAllowOutofOrder(true)`; additive DDL only; Wave 0 ships only `apps`, `users.bot_id`, `conversations.is_private`; `reactions` table belongs to B2 (range 20).
- 0.6 pre-split hot files: `service/{post,edit,react,access}.go`, `store/{messages,reactions,apps,wipe}.go`, `api/methods_*`, `dto_{content,reactions}.go`; stub one-line `emit`/`checkAccess` hooks in `PostMessage`.
- 0.7 event seam: ordered publish (afterCommit hook in `Store.Write` or mutex spanning commit+enqueue), O(1) non-blocking; events self-contained (full message, channel kind/private/name, target bot user ids; Changed has prev+new; Deleted has message); bounded queue, drop+log+counter; `Subscribe` returns unsubscribe; seed/populate/restore/reset never emit (tested); collector-based tests.
- 0.8 errors: export constants only; each stream adds mappings in its own `errors_<stream>.go` (map merge) instead of a frozen table.
- 0.10 define interfaces + noop impls: `Dispatcher{Pause,Reset,Resume}`, CallLog, Faults. `Deps` = optional components list to avoid shared-line edits in `build.go`/`storetest`.
- 0.11 test hygiene: fixtures restore global control state (faults, delivery-config, paused) on teardown; reset/snapshot suites run against a dedicated second server instance.
- Add `Store.Read(ctx, fn)` (single read tx) for consistent export/diff/state.

**Reset invariants**: ts `advanceTo` forward-only, inside reset write tx, >= max fixture ts; ids generator behind mutex/`Reseed` in place; team id stable across reset (keep team row) or apps re-stamped; app bot users inserted after populate, populate rejects id collisions with them, snapshot export excludes them; dispatcher reset = stop enqueue, cancel in-flight + timers, trace writes check generation; call/delivery `seq` monotonic across DELETE/reset, parked long-polls woken with `generation` change; reset scope documented (delivery-config -> defaults, sockets persist, UI cookie falls back if user gone); one write tx, batched prepared inserts; concurrent reset -> 409. Reset is global: one test session per server. Documented decisions: apps persist across reset (`keep`/`drop` incl. `apps`); snapshots volatile (lost on restart); single-team coupling is a known deviation (DESIGN_DOC multi-workspace is the eventual answer).

**Extra tests**: concurrent-post event ordering; no events on seed/restore/reset; late delivery after reset not recorded; two-app Principal resolution; export->wipe->import->export identity; populate perf as benchmark + loose bound.

## Progress
- [ ] Wave 0 (11 steps)  - [ ] Gate 0
- [ ] Wave 1: A B1 B2 C D E  - [ ] Gate 1
- [ ] Wave 2: S X F G I  - [ ] Gate 2
- [ ] Wave 3
