# Spike: LocalSlack as a wrapper over emulate

**Throwaway branch only** (`spike/emulate-wrapper`). Do not merge. Do not edit the Go code, existing tests, `docs/`, or other plans. Everything new goes under `spikes/emulate-wrapper/` plus this file's checkboxes/Handoff.

You run unattended: do not stop to ask questions. Where this plan is silent, pick the simplest option, write the choice in `RESULTS.md`, and continue. (The repo's "clarify requirements first" rule is waived for this spike.)

## Question

Could LocalSlack's test-loop features be built as a TypeScript layer over the published `@emulators/core` and `@emulators/slack` packages (vercel-labs/emulate, Apache-2.0, 0.12.1) instead of the Go server? Context and reasoning: `docs/BUILD_ON_EMULATE_V2.md` (and v1) if present on this branch. This spike produces evidence for a go/no-go decision; it is not a product.

## Deliverable

`spikes/emulate-wrapper/RESULTS.md`: for each proof P1-P5 below: **PASS / PARTIAL / FAIL**, the exact commands to reproduce, observed output (trimmed), what you changed or patched, and surprises. End with an overall recommendation (E wrapper / fork / stay on Go), a rough effort estimate for the remaining test-loop features, and a list of changes worth sending upstream. Be honest about anything unverified. Commit often; tick boxes below as you go.

Time box: one working session. If a proof is blocked, record why, mark it, and move on. P1 gates the rest only for the recommendation, not for doing the other proofs.

## Known facts (already verified, do not redo)

- `createServer(slackPlugin, { baseUrl, tokens })` from `@emulators/core` returns `{ app, store, webhooks, tokenMap }`. Import `slackPlugin` and `seedFromConfig` from `@emulators/slack`. Pin exact versions (`0.12.1`), no `^`.
- `@emulators/core` ships its own minimal `Hono` router: it has `use`, `get`, `post` etc. but **no `.all()`**. Wrap at the fetch level: `inner.app.fetch(request)`. `core` also exports `serve`, `Store`, `WebhookDispatcher`, `serializeTokenMap`/`restoreTokenMap`, `filePersistence`.
- `webhooks.register({ url, events: ['*'], active: true, owner: 'slack' })` makes writes deliver a signed `event_callback` (envelope: `type, team_id, event_id, event_time, event`) to `url`. Signing uses `seedFromConfig(store, baseUrl, { signing_secret })`. `webhooks.getDeliveries()` returns the trace. `webhooks.setHeaderFactory(fn)` controls headers.
- `store.snapshot()` / `store.restore(snap)` / `store.reset()` work on real Slack state (history 2 messages -> restore -> 1).
- In library mode an unknown token returns `not_authed`. (The CLI's "any token maps to the first user" is CLI-layer fallback; `ServerOptions` also has `fallbackUser`.)
- IDs come from `randomBytes(5).toString('hex')` (module-private `generateSlackId`); `ts` is `Math.floor(Date.now()/1000)` plus a process-wide counter (module-private `generateTs`); event ids use `randomUUID`; rows get ISO `created_at`/`updated_at` from `new Date()`. Seeded entities may carry pinned IDs via seed config.
- Throwaway spike code from the earlier session lived in gitignored `tmp/` and is **not** in this repo; the ~50-line recipe is in Appendix A.
- Baseline against emulate's CLI (sandboxed run): of `tests/smoke`, 29 passed, 10 known-deviation xfails held, 4 known-deviation tests passed unexpectedly (strict xfail -> reported FAILED): `test_blocks_only_message_is_accepted`, `test_blocks_round_trip_through_history`, `test_users_list_honors_limit`, `test_token_in_json_body_is_not_accepted`. Against the Go server: 43 passed, 14 xfailed.

## Environment notes (learned the hard way)

- A server started in one tool call may not survive to the next. Start the server, run the checks, and stop it inside a single command (a small `with-server.sh` for the wrapper, modeled on `scripts/with-server.sh`).
- Python SDK clients may route through a proxy: set `NO_PROXY=127.0.0.1,localhost`.
- If `~/.npm` is read-only, set `npm_config_cache` to a writable dir.
- `tests/smoke` is a uv project: `uv run --project tests/smoke pytest ...`; it reads `LOCALSLACK_URL` (default `http://localhost:3232`).

## Tasks

### P0. Skeleton
- [x] `spikes/emulate-wrapper/` with `package.json` (type module, exact pins for `@emulators/core`, `@emulators/slack`; add `ws` when needed), `src/server.mjs` (or `.ts`; plain ESM is fine).
- [x] Wrapper object with `fetch(request)`: records each call (method, path, status, trimmed body) to an in-memory log; serves `GET /_localslack/health`, `GET /_localslack/calls`, `POST /_localslack/reset`, then delegates to `inner.app.fetch`.
- [x] Serve over real HTTP on a configurable port/addr (use core's `serve` or a small `node:http` bridge). Seed from a fixed seed so runs are repeatable. A real Slack token for tests: `xoxb-` anything (add a tiny auth shim mapping unknown `xoxb-*` to the bot user and `xoxp-<userID>` to that user, mirroring the Go server, if cheap; otherwise register tokens explicitly and say so).
- [x] `spikes/emulate-wrapper/scripts/with-server.sh <cmd...>`: start on a free port, export `LOCALSLACK_URL`, run, clean up.

### P1. Determinism (the gating risk)
Goal: same seed -> byte-identical IDs, `ts`, and event ids for entities created through the API, across fresh processes.
- [x] Inventory every nondeterminism source in the two packages' `dist/index.js`: `randomBytes`, `randomUUID`, `Math.random`, `Date.now`, `new Date`, timers. Record in `RESULTS.md`.
- [x] Try, in order: (a) seeded PRNG replacing `crypto.randomBytes`/`randomUUID` before the packages load (watch ESM named-import binding; `module.syncBuiltinESMExports()` after patching the CJS `crypto` object) plus a controllable clock for `Date.now`/`new Date`; (b) `patch-package` style patch of the dist files; (c) report what only a source change upstream can fix.
- [x] Test: script that, for a given seed, creates a public channel, a private channel, opens a DM, posts 3 messages (one threaded), adds a reaction, and prints all IDs/`ts`/event ids/deliveries; run in two separate processes with the same seed (must be identical), then a different seed (must differ). Save outputs.
- Pass: identical for same seed including `created_at`, `event_id`, delivery ids. Partial: IDs and `ts` identical, some metadata not. Fail: cannot control IDs without forking.

### P2. Swap-in: Python smoke suite against the wrapper
- [x] Run `tests/smoke` (unmodified) against the wrapper via `LOCALSLACK_URL`. Do not edit those tests.
- [x] Table: each test -> pass / xfail held / unexpected failure / expected strict-xpass, with the reason. Compare with the Go baseline (43 passed, 14 xfailed).
- [x] Add wrapper shims (in `spikes/`) for cheap mismatches, in this order of value: `GET /_localslack/health`; unrouted paths return Slack-shaped `{ok:false,error:"unknown_method"}` (the Go server returns `404 not_implemented`, which the smoke tests assert; note which behavior the tests expect and pick the one that keeps the suite green, recording the difference); `invalid_auth` for bad tokens; `x-slack-req-id`; drop GitHub-style `x-ratelimit-*`. Re-run after each.
- [x] Note which tests can never pass without upstream state-model changes (e.g. `bot_id`, membership) and which pass only because emulate is better than the Go server.
- Pass: only expected differences remain and every one is explained. `tests/e2e` (browser UI of the Go server) is out of scope; say so.

### P3. Bolt over HTTP, end to end
- [x] `POST /_localslack/subscriptions` {url, events}: registers via `webhooks.register`, performs the `url_verification` challenge handshake (POST `{type:"url_verification",challenge}`, require the challenge echoed) and rejects the registration if it fails.
- [x] A real Bolt app (Python in the smoke uv project is simplest; JS is fine) with signing secret, an HTTP receiver, a `message` handler that replies via `chat.postMessage` through the wrapper. Post as a user -> Bolt handler fires -> reply appears in `conversations.history`.
- [x] Signature actually verified by Bolt; with a wrapper toggle for a bad signature (wrap the header factory), Bolt rejects (401) and the delivery shows failure in `getDeliveries()` / `GET /_localslack/deliveries`.
- [x] Retry redelivery: `POST /_localslack/events/:event_id/retry` resends the recorded payload with the same `event_id`, `X-Slack-Retry-Num`, `X-Slack-Retry-Reason`, re-signed; verify the Bolt app sees the headers (dedupe is the app's job).
- Out of scope: `app_mention` (upstream gap), interactivity.

### P4. Socket Mode
- [x] `apps.connections.open` (xapp- token) returns `{ok:true, url:"ws://.../link"}`; `ws` server sends `hello`, then events as `{envelope_id, type:"events_api", payload, accepts_response_payload:false}`; client ack `{envelope_id}` recorded; unacked envelopes retried or logged (your call; document).
- [x] Feed it by registering an internal loopback subscription (no upstream change), so the same write path drives HTTP and Socket Mode.
- [x] A real Bolt Socket Mode app (SDK `SocketModeClient`/adapter with the web client pointed at the wrapper's `base_url`) connects, receives a posted message, and replies.
- Pass: Bolt connects and round-trips with no modifications to Bolt or the SDK beyond the base URL.

### P5. Packaging and startup
- [x] Try a single executable: Node SEA, and/or `bun build --compile` if Bun is installable. Record binary size, whether it runs with no `node_modules`, and cold start to first `200` on `/_localslack/health`.
- [x] Record plain `node` cold start as the baseline.
- Partial is fine: write what blocked it.

### P6. Wrap-up
- [x] `RESULTS.md` complete (see Deliverable), including the effort estimate and upstream-PR candidates.
- [x] If you found that a state-model quirk blocks a roadmap item (e.g. `bot_id` missing from bot posts, orphan thread replies accepted), list it with a minimal repro request.

## Out of scope

Rewriting the UI, porting the capability manifest tooling, anything under `internal/`, `cmd/`, `tests/` (read-only inputs), opening PRs upstream, hitting real Slack.

## Handoff
Done. See `spikes/emulate-wrapper/RESULTS.md`. Node SEA (P5) was not attempted; `stale_signature`, snapshot/restore endpoints and concurrency determinism are unverified (listed in RESULTS).

## Appendix A: the earlier 50-line spike (recipe)

```js
import http from 'node:http';
import { createServer } from '@emulators/core';
import { slackPlugin, seedFromConfig } from '@emulators/slack';

const inner = createServer(slackPlugin, {
  baseUrl: 'http://localhost:4010',
  tokens: { 'xoxb-spike': { login: 'admin', id: 1, scopes: ['chat:write', 'channels:read', 'channels:history'] } },
});
seedFromConfig(inner.store, 'http://localhost:4010', { signing_secret: 'sek', channels: [{ name: 'general' }] });

const calls = [];
const outer = {
  async fetch(req) {
    const url = new URL(req.url);
    if (url.pathname === '/_localslack/calls') return Response.json(calls);
    const body = req.method === 'POST' ? (await req.clone().text()).slice(0, 80) : '';
    const res = await inner.app.fetch(req);
    calls.push({ method: url.pathname, status: res.status, body });
    return res;
  },
};

inner.webhooks.register({ url: 'http://127.0.0.1:5057/events', events: ['*'], active: true, owner: 'slack' });
// POST /api/chat.postMessage via outer.fetch -> receiver gets a signed event_callback
// inner.store.snapshot(); mutate; inner.store.restore(snapshot) -> state rolled back
```
