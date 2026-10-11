# Spike results: LocalSlack as a wrapper over `@emulators/core` + `@emulators/slack` 0.12.1

Branch: `claude/emulate-wrapper-spike-0zt597` (the plan names `spike/emulate-wrapper`; the session designated this branch). Node v22.22, Bun 1.4.2, Linux. Nothing outside `spikes/emulate-wrapper/` and the plan's checkboxes was touched.

| Proof | Result |
|---|---|
| P0 Skeleton | **PASS** |
| P1 Determinism | **PASS** (Node, Bun binary: same IDs; in-process scenario verified, see caveats) |
| P2 Smoke suite swap-in | **PASS** (only explained differences; 2 real gaps, 1 unmeasurable) |
| P3 Bolt over HTTP | **PASS** |
| P4 Socket Mode | **PASS** |
| P5 Packaging | **PARTIAL** (Bun single binary works; Node SEA not attempted) |

**Recommendation: E (wrapper).** Details at the end.

## Layout

```
spikes/emulate-wrapper/
  src/wrapper.mjs        fetch-level wrapper (call log, _localslack routes, shims, subscriptions, retry, Socket Mode)
  src/determinism.mjs    seeded PRNG + virtual clock, patched in before the packages load
  src/server.mjs         node:http bridge + ws upgrade        src/server-bun.mjs  entry for the Bun build
  src/crypto-shim.mjs, src/determinism-boot.mjs, scripts/build-bun.mjs   Bun-only pieces
  src/p1-determinism.mjs P1 scenario                           p1-out/  saved outputs
  scripts/with-server.sh start on a free port, export LOCALSLACK_URL, run, clean up (LOCALSLACK_CMD overrides the server command)
  scripts/p3_bolt_http.py, p4_socket_mode.py, p4-unacked.mjs, reset-check.sh, cold-start.sh   proof drivers
  p2-out/smoke-unknown-go.txt   per-test outcomes of tests/smoke
```
Setup: `cd spikes/emulate-wrapper && npm install` (set `npm_config_cache` to a writable dir if `~/.npm` is read-only). Python proofs use the existing smoke uv project: `export UV_CACHE_DIR=...; uv run --project tests/smoke ...`. `with-server.sh` unsets `HTTP(S)_PROXY` because slack_sdk's async client (aiohttp) ignores `NO_PROXY` and sent loopback calls through the sandbox proxy (405).

## P0. Skeleton: PASS

`node src/server.mjs` (env `PORT`, `HOST`, `LOCALSLACK_SEED`, `UNKNOWN_METHOD=go|slack`). Wrapper `fetch(request)` records `{seq, method, path, status, body[:120]}` (tokens masked) and serves `GET /_localslack/health|calls|deliveries|config|socket|subscriptions`, `POST /_localslack/reset|config|clock|snapshot|restore|subscriptions|events/:id/retry`, then delegates to `inner.app.fetch`. Fixed seed: users alice/bob/carol/localslack-bot (+ upstream's `admin`), channels `general`, `random`, signing secret `localslack-spike-secret`.

Auth shim (cheap, done): unknown `xoxb-*` maps to the bot user, `xoxp-<userID>` to that user, other tokens `invalid_auth`. **Surprise:** the seed's `bots:` entries create a `bots` row but no bot *user*, so the wrapper seeds a normal user named `localslack-bot` and flips `is_bot` after seeding (it is then a member of the seeded channels). Upstream also has no `bot_id` on posted messages (see P2/P6).

```
$ scripts/with-server.sh bash -c 'curl -s $LOCALSLACK_URL/_localslack/health'
{"ok":true}
```

## P1. Determinism: PASS (with caveats)

### Inventory (both `dist/index.js`)

| Source | Where | Effect |
|---|---|---|
| `randomBytes(5)` | slack `generateSlackId` (users/channels/bots/files... also used by `seedFromConfig`) | all entity IDs |
| `Date.now()/1e3` + module-private `tsCounter` | slack `generateTs` | message `ts` |
| `randomUUID()` | slack `buildSlackEventEnvelope` | `event_id` |
| `Math.floor(Date.now()/1e3)` | `event_time`, `created`, `last_set`, signature timestamp, ~40 other sites | |
| `new Date().toISOString()` | core `Collection.insert/update` (`created_at`/`updated_at`), `WebhookDispatcher` (`delivered_at`) | row metadata |
| `Date.now()` | `WebhookDispatcher` duration, rate-limit window | delivery `duration` |
| `Math.random()` | slack `generateTriggerId` (views) | `trigger_id` |
| `randomBytes(20)` | slack OAuth `code`, `xoxb-`/`xoxp-` tokens | OAuth flow only |
| `randomUUID()` | core `filePersistence` temp file names | harmless |
| `setTimeout`/`performance.now` | core request tracing, timeouts | harmless |

### Approach (a) worked on Node
`src/determinism.mjs`, installed before `import('@emulators/*')`: replaces `crypto.randomBytes`/`randomUUID` with a counter-mode SHA-256 stream keyed by the seed, `Math.random`, and `globalThis.Date` with a subclass whose `now()`/no-arg constructor read a virtual clock (frozen at 2026-01-01T12:00:00Z, `POST /_localslack/clock {advance_ms}` moves it), then `module.syncBuiltinESMExports()`. The packages use the global `Date` and look up `randomBytes` through a named builtin import, which is why patching the CJS `crypto` object plus the sync call is enough on Node.

```
$ node src/p1-determinism.mjs seed-a > a1.json; node src/p1-determinism.mjs seed-a > a2.json; node src/p1-determinism.mjs seed-b > b.json
$ cmp a1.json a2.json && echo IDENTICAL          -> IDENTICAL
$ cmp a1.json b.json  || echo DIFFERENT          -> DIFFERENT (user/channel/DM ids, event ids, signatures differ)
```
The scenario (public channel, private channel, DM, 3 messages with one threaded, a reaction) prints seeded user IDs, channel IDs, `ts` (`1767268800.000001`..`3`), `created_at`/`updated_at`, every received `event_id`, `event_time`, `X-Slack-Signature` and the `getDeliveries()` rows (id, `delivered_at`, status, duration). All of it is byte-identical for the same seed (`p1-out/run-a1.json`, `run-a2.json`) and differs for another (`run-b.json`). That is the PASS bar (includes `created_at`, `event_id`, delivery ids).

### Things that needed more than a PRNG
- **Reset.** `tsCounter` is module-private and survives `store.reset()`, so after a reset `ts` would continue at `.000004`. Fix on Node: rebuild the server on `POST /_localslack/reset` with a *cache-busted dynamic import* of `@emulators/slack` (`?gen=N`), giving a fresh module instance, and reseed the PRNG. `scripts/reset-check.sh` (create channel + post, reset, repeat) → identical channel ID and `ts`.
- **Signatures vs the virtual clock.** A frozen 2026 clock makes Bolt reject deliveries (timestamp older than 5 minutes → 401). The wrapper therefore re-signs with wall-clock time by default (`realClockSignatures`); P1 turns it off so signatures are byte-stable too. Alternatives: start the virtual clock at wall time (not reproducible across runs).
- **Bun is different (P5).** Bun's named builtin imports do not observe patching of the `crypto` default export (checked with a 10-line probe: `bun run` prints random bytes, `node` prints the patched ones), so the Bun build rewrites `from "crypto"` inside the two packages to `src/crypto-shim.mjs` via a `Bun.build` `onLoad` plugin (builtins bypass `onResolve`) and exposes a `tsCounter` reset hook by rewriting `var tsCounter = 0;`. With that, the compiled binary produces the same IDs as Node for the same seed (`C3583E2ED5` for the first channel in both) and passes `reset-check.sh`.

### Caveats / unverified
- Concurrent in-flight requests share one PRNG stream and one `tsCounter`; determinism holds for a serialized call sequence, not for parallel clients.
- Frozen clock side effects: all messages share one `ts` second (counter keeps them unique), delivery `duration` is 0, time-based features (scheduled messages, OAuth code TTL, rate-limit window) only move when the clock is advanced. Not exercised.
- The P1 scenario ran in-process (`wrapper.fetch`), not over the HTTP bridge; the HTTP path was checked via `reset-check.sh` and the smoke suite.
- OAuth tokens/codes also come from the stream; not exercised.

## P2. Python smoke suite against the wrapper: PASS

```
$ spikes/emulate-wrapper/scripts/with-server.sh uv run --project tests/smoke pytest tests/smoke -q -rA
9 failed, 40 passed, 1 skipped, 7 xfailed     (57 tests; Go baseline: 43 passed, 14 xfailed)
```
(Re-run after each shim; full per-test list in `p2-out/smoke-unknown-go.txt`. `tests/e2e` is out of scope.) Without shims, the first run was 14 failed / 34 passed / 8 xfailed, dominated by (a) loopback going through the proxy and (b) the async client, see below.

Accounting against the Go baseline:

| Go baseline | Here | Count | Why |
|---|---|---|---|
| passed (43) | passed | 40 | |
| passed | skipped | 1 | `persistence_test.py` needs `LOCALSLACK_BIN` to restart a server; no persistence implemented in the spike (`filePersistence` exists, untried) |
| passed | **FAILED (real gap)** | 1 | `invalid_name`: emulate returns `invalid_name_specials` (which is what real Slack returns; the test encodes Go's simplification) |
| passed | **FAILED (real gap)** | 1 | `test_replies_iterator_returns_root_once_then_replies`: `conversations.replies` ignores `limit`/`cursor` (returns all 4 messages, no `next_cursor`) |
| xfailed (14) | xfail held | 7 | see below |
| xfailed | strict-XPASS → "FAILED" | 7 | emulate or a shim is better than the Go server; each is a candidate to delete from the deviations manifest |

Held xfails (emulate shares the Go server's deviation): `empty_channel_name` (`invalid_name_specials`, not `invalid_name_required`), `messages_carry_bot_fields` (no `bot_id` on bot posts; `user` only), `multipart_form_post` (multipart ignored → treated as missing name), `non_member_cannot_read_a_dm` (no membership check; `conversations.history` returns `ok` with `[]`), `overlong_message` (emulate truncates at 40 000 chars with `warning: message_truncated`, no `msg_too_long`; arguably closer to Slack), `post_by_channel_name` (`#general` → `channel_not_found`), `unknown_method_is_200_unknown_method` (held only because I picked the Go behavior, below).

Strict-XPASS (7): `blocks_only_message` and `blocks_round_trip` (emulate supports blocks), `post_to_user_id_opens_a_dm`, `users_list_honors_limit`, `token_in_json_body` (these three are upstream-native), `responses_carry_slack_headers` (shim), `create_private_channel` (shim). Compared with the CLI baseline in the plan (4 unexpected passes), the library mode + shims adds `post_to_user_id` (the CLI run apparently differs), headers and private channel.

### Shims added (order of value), all in `src/wrapper.mjs`
1. `GET /_localslack/health` (needed by `test_health`; `with-server.sh` polls it).
2. **Unrouted `/api/*` paths.** The two requirements conflict: `smoke_test.py::test_unknown_method_returns_not_implemented` and `test_unimplemented_method_is_a_clear_sdk_error` expect Go's `404 {"error":"not_implemented"}`, while the deviation test expects `200 unknown_method`. Default `UNKNOWN_METHOD=go` keeps those green (40 passed). `UNKNOWN_METHOD=slack` gives 38 passed / 11 failed / 7 xfailed (two tests fail, one strict-XPASSes).
3. `invalid_auth` for non-`xox*` tokens and unknown `xoxp-` users (Slack returns 200 `invalid_auth`; upstream library mode returns `not_authed`).
4. `x-slack-req-id`, `x-oauth-scopes`, `application/json; charset=utf-8`; GitHub-style `x-ratelimit-*` dropped.
5. **Request normalization (found by failures, not on the plan):** (i) slack_sdk's async client puts method args in the *query string* with an empty POST body and uses `GET` for read methods; emulate registers `POST` only for most methods (`conversations.history`, `users.list`... are POST-only) and ignores query params, so everything async failed until the wrapper folded query + GET into a urlencoded POST; (ii) the Python SDK serializes bools as `1`/`0`/`True`, and emulate's `conversations.create` only accepts `"true"` → `is_private=1` silently created a *public* channel (upstream bug candidate). The wrapper normalizes a fixed allow-list of boolean param names.
6. State-aware validation emulate lacks: `invalid_types`, `invalid_cursor` (conversations.list; cursor is a channel id), `thread_not_found` on `chat.postMessage` (orphan `thread_ts` accepted upstream). These needed store access, i.e. a pure HTTP proxy in front of emulate could not do them; a wrapper that owns the process can.

## P3. Bolt over HTTP end to end: PASS

`scripts/p3_bolt_http.py` (Bolt for Python, built-in HTTP server, no modifications):

```
$ spikes/emulate-wrapper/scripts/with-server.sh uv run --project tests/smoke python spikes/emulate-wrapper/scripts/p3_bolt_http.py
PASS  registration to a dead URL is rejected {"ok":false,"error":"url_verification_failed"}
PASS  registration to Bolt succeeds after url_verification {"ok":true,"subscription":{...,"id":1}}
PASS  Bolt handler replied via chat.postMessage {'user': 'UFEBACA82A', 'text': 'pong: hello bolt', ...}
PASS  delivery trace shows 200 {... 'event_type': 'message', 'status_code': 200, 'success': True ...}
PASS  bad signature: Bolt returns 401, recorded in deliveries [{... 'status_code': 401, 'success': False}]
PASS  bad signature: handler never ran
PASS  retry endpoint accepted {"ok":true,"event_id":"Ev7a27...","status_code":200,"retry_num":1,"retry_reason":"http_timeout"}
PASS  Bolt saw X-Slack-Retry-Num/Reason with the same event_id {... 'retry_num': '1', 'retry_reason': 'http_timeout'}
INFO  dedupe is the app's job: handler replied 2x to the same event_id
```
- `POST /_localslack/subscriptions {url, events}` runs `url_verification` (signed POST, requires the challenge echoed as JSON or plain text), then `webhooks.register`. Bolt answers the challenge itself.
- Signature really verified by Bolt: with `POST /_localslack/config {"bad_signature":true}` Bolt returns 401, `getDeliveries()` shows `status_code 401, success false` (also at `GET /_localslack/deliveries`). `stale_signature` (timestamp − 10 min) is implemented but **not exercised**.
- Retry re-signs the recorded payload (same `event_id`) with `X-Slack-Retry-Num` / `X-Slack-Retry-Reason`; records live in the wrapper (`retries`), not in upstream's delivery list.
- Surprises: (1) the bot's own `chat.postMessage` produces a `message` event with `user` = bot and **no `bot_id`**, so a Bolt handler needs a `user == bot` guard or it loops; (2) `chat.postMessage` **awaits** `webhooks.dispatch` before responding, so a slow receiver delays the poster (10 s timeout per delivery, sequential per subscription); not measured beyond Bolt (fast ack), probably wants fire-and-forget; (3) in the P1 scenario `conversations.create` delivered no `channel_created` event (events seen: `im_created`, `im_open`, `message`, `reaction_added`); (4) `app_mention` and interactivity are out of scope as planned.

## P4. Socket Mode: PASS

`scripts/p4_socket_mode.py`: Bolt for Python `SocketModeHandler(app, "xapp-1-test")`, web client at the wrapper's `base_url`; no changes to Bolt or the SDK.

```
PASS  Bolt connected over ws {'connections': 1, ...}
PASS  message event -> Bolt handler -> reply in history {'user': 'UFEBACA82A', 'text': 'sock-pong: hello socket', ...}
PASS  client acks recorded, none pending {'acks': 2, 'pending': []}
PASS  apps.connections.open rejects non-xapp token {'ok': False, 'error': 'not_allowed_token_type'}
```
Design: `apps.connections.open` (xapp- token) returns `ws://host/link?ticket=...`; `ws` server (`noServer` upgrade on `/link`) sends `hello`, then `{envelope_id, type:"events_api", payload, accepts_response_payload:false}`. The first connection lazily registers an *internal loopback subscription* (`webhooks.register` with url `/_localslack/socket-sink`), so HTTP and Socket Mode share the same dispatch path; no upstream change. Acks are recorded (`GET /_localslack/socket`). **Unacked policy (my choice):** resend after 3 s with `retry_attempt` 1, 2 and `retry_reason: "timeout"` (same `envelope_id`), then mark `unacked` (`scripts/p4-unacked.mjs` shows the two resends; the final `unacked` flag was not observed within the script's 7.5 s window). Not done: `disconnect` envelopes/`refresh_requested`, multiple-app routing, slash commands/interactive envelopes.

## P5. Packaging and startup: PARTIAL

`bun scripts/build-bun.mjs` → `out/localslack-bun`, 81.7 MB (Bun runtime dominates), runs from an empty directory with no `node_modules`. Blockers found and solved: fonts are `readFileSync`'d from `dist/fonts` at import time (embedded as assets at `<bunfs>/fonts` with `--asset-naming='[dir]/[name].[ext]'`); `import(resolvedPath)` cannot be bundled (static imports in `server-bun.mjs`, hence the tsCounter reset hook instead of module busting); crypto patching (P1). The compiled binary passes the smoke suite with the same result as Node (9 failed [7 strict-XPASS + 2 gaps]/40 passed), P3, P4 and `reset-check.sh`.

| | cold start to first `200` on `/_localslack/health` (3 runs, includes shell polling at 10 ms) |
|---|---|
| `node src/server.mjs` | 195, 208, 214 ms |
| `out/localslack-bun` | 59-88 ms |

**Not attempted: Node SEA** (needs a bundle step such as esbuild to CJS plus the same font/crypto handling; Bun already answers "can it be one binary"). Also untried: `bun build` for other OS/arch, binary behavior under `ws` backpressure, Windows.

## P6. State-model quirks and requests (upstream behavior, verified in this spike unless noted)

| Quirk | Minimal repro | Blocks |
|---|---|---|
| `bot_id` missing on bot-authored messages and `message` events | token of a bot user → `chat.postMessage` → `conversations.history`: message has `user` only (held xfail `messages_carry_bot_fields`) | bot-loop guards in apps; `bot_message` semantics |
| `seedFromConfig` `bots:` creates no bot user | seed `bots:[{name:'b'}]` → `users.list` has no bot; `auth.test` for a bot token has no user to resolve | seeding a bot identity (worked around) |
| `conversations.create` ignores `is_private=1`/`True` | `curl -d name=x -d is_private=1 .../conversations.create` → `is_private:false` | Python SDK users creating private channels |
| Orphan `thread_ts` accepted | `chat.postMessage thread_ts=1.000001` → `ok:true` | (shimmed) |
| `conversations.replies` ignores `limit`/`cursor` | 1 root + 3 replies, `limit=2` → 4 messages, no `next_cursor` | pagination tests |
| No membership checks | non-member `conversations.history` on a DM → `ok`, `[]` | DM privacy tests |
| Bogus `cursor`/`types` ignored | `conversations.list cursor=bogus` → `ok` | (shimmed) |
| `by-name` post targets unresolved | `channel=#general` → `channel_not_found` | |
| Query-string and `GET` method args not read | `GET /api/conversations.history?channel=...` → 404 | slack_sdk async client, any GET caller (shimmed) |
| Writes await webhook dispatch | slow receiver delays `chat.postMessage` | |
| No `channel_created`/membership events observed | register `*`, create channel → no delivery | events roadmap |

## Overall recommendation: **E (TypeScript wrapper over the published packages)**

Evidence: determinism, the gating risk, is solvable with no fork (PRNG + clock patch on Node; small build-time rewrite on Bun), a real Bolt app round-trips over both HTTP and Socket Mode with only a base URL change, the unmodified smoke suite runs with 2 genuine gaps left (both emulate-side and upstreamable), and a single ~82 MB binary starts in under 100 ms. Seven Go-baseline xfails pass here, i.e. the base layer is already better than our server for blocks, DM-by-user-id, pagination limits and JSON-token handling.

Where I would **fork instead of wrap**: only if we need things the module-private state blocks and upstream refuses: `bot_id`/membership/threads (state-model changes), fire-and-forget dispatch, injectable ID/clock. The cache-busting import and source-rewrite tricks are the smell: they work today but depend on the exact shape of `dist/index.js` (`var tsCounter = 0;`, `from "crypto"`), so every upgrade must be gated by this suite plus a determinism test. Pin exact versions.

**Stay on Go** only if single-binary-without-a-JS-runtime and SQLite-grade persistence are hard requirements: the Bun binary is 82 MB and bundles a JS runtime.

### Rough effort for the remaining test-loop features (wrapper, one engineer, using this spike as a base)
| Item | Estimate |
|---|---|
| Call log (filters, body capture, per-test scoping), reset/snapshot/restore endpoints | 1-2 d (mostly here; snapshot/restore only smoke-verified by earlier work) |
| Fault injection + rate-limit tiers (`Retry-After`) | 2-3 d |
| Events controls: event injection, stale-signature test, delivery retries with backoff, `app_mention`, `channel_*`/membership events | 3-5 d (several need upstream events or our own emission) |
| Socket Mode hardening (disconnect/refresh, multi-client, slash commands/interactivity) | 3-4 d |
| Persistence across restart (`filePersistence`/snapshot to file) | 1-2 d |
| Deterministic mode as a first-class, tested feature (concurrency policy, OAuth, upgrade guard test) | 2-3 d |
| Fidelity shims to remove the 2 remaining P2 gaps and decide `unknown_method` | 1-2 d |
| Port seed generator, capability manifest tooling decision, CI | 3-5 d |

### Upstream candidates
1. `conversations.create`: accept `is_private` as `1`/`True`/`true` (use the existing `booleanField`).
2. Read method args from the query string and accept `GET` for read methods (slack_sdk async + many clients).
3. Set `bot_id` (and `bot_message` semantics) on bot posts; make `seedFromConfig` `bots:` create the bot user.
4. `conversations.replies` honor `limit`/`cursor`; validate `thread_ts`, `cursor`, `types`.
5. Injectable ID generator and clock (`createServer({ ids, clock })`) and a resettable `ts` counter; this removes every hack in P1.
6. Dispatch webhooks without blocking the originating request (or make it an option); expose retry metadata (`X-Slack-Retry-*`) and a documented subscriber registration API.
7. `unknown_method` for unrouted `/api/*` paths and `invalid_auth` for bad tokens; drop `x-ratelimit-*` in Slack mode; add `x-slack-req-id`.
8. `channel_created` and other conversation events.

### Honest list of what is not verified
Concurrency determinism; time-dependent features under the virtual clock; `stale_signature`; snapshot/restore through the wrapper endpoints; OAuth under the seeded PRNG; Windows/macOS binaries; Node SEA; persistence; the `UNKNOWN_METHOD=slack` mode beyond the smoke counts; `tests/e2e` and the Go conformance fixtures (not run). `npm audit` warnings were not reviewed.
