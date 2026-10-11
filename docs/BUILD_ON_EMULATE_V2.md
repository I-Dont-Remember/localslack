# Build on emulate? v2: if we're willing to start over

Draft, 2026-10-10. Not committed. v1 (`BUILD_ON_EMULATE.md`) assumed we keep the Go code and treated rewriting as a cost. v2 drops that: we may discard the project, and we are not worried about emulate's pre-1.0 version number. Same question: is building on emulate the better path to the test-loop product?

## What changes from v1

- "Abandons Go / throws away the repo" is no longer a con by itself.
- "0.x churn" is downgraded to a process risk (pin exact versions, gate upgrades with our own suite), not a blocker. Evidence it is manageable: 13 releases in ~6 months; but behavior does change in minors (0.12.0 switched event callbacks from GitHub-style to Slack-style headers/envelopes), so pin and test.
- A new option becomes the lead candidate: a TypeScript wrapper that uses `@emulators/core` + `@emulators/slack` as libraries (E below).

## What we would be discarding, and what survives

| | Size | Fate |
|---|---|---|
| Go server, store (SQLite), service layer, seed generator, CLI, htmx UI | ~4,100 lines production, ~3,600 test | Discarded or ported (UI ~530 lines) |
| `tests/smoke` (Python, black-box HTTP), `tests/e2e` | ~1,230 lines | **Survives unchanged.** Already ran against emulate over HTTP (29 pass, 10 shared xfails, 4 deviations it fixes) |
| Conformance fixtures (18 JSON), redaction/recording tools (Python) | | Survives; the Go-side matcher runner needs a check or port |
| Capability manifest (`capabilities.toml`), `COMPATIBILITY.md` generator, evidence grading | ~830 lines Go | Concept survives; tooling is Go (keep as a standalone tool or port) |
| Docs, roadmap, design decisions | | Survive; the roadmap gets shorter |

So the real loss is the server we've been building, not the test assets. The black-box suite becomes the **upgrade gate** for every emulate bump.

## Options

### E. Wrapper (recommended): our TypeScript package depending on `@emulators/core` and `@emulators/slack`
Published packages (both Apache-2.0, 0.12.1). `createServer(slackPlugin, options)` returns `{ app, store, webhooks, tokenMap }`, so we get the Slack app, its state and its event dispatcher as objects. We wrap `app.fetch` and add our own layers.

**Spike (done, `tmp/emulate-eval/spike/spike.mjs`, output in `results/spike.txt`).** With about 50 lines against the published packages:
- A fetch-level wrapper recorded every call (`/api/conversations.list:200`, `/api/chat.postMessage:200`), the basis for a call log, fault injection and rate limiting.
- `webhooks.register({url, events, owner: 'slack'})` made a `chat.postMessage` deliver a signed `event_callback` (signature header present; envelope has `team_id`, `event_id`, `event_time`) to a local receiver. This is the subscriber registration the CLI lacks. `getDeliveries()` returned the delivery trace.
- `store.snapshot()` then mutate then `store.restore(...)` returned history from 2 messages to 1, so snapshot/restore works on the real Slack state.
- In library mode an unknown token returns `not_authed`; the "any token maps to the first user" behavior I saw is CLI-layer fallback, not core.

**What we build on top (all in our code, no upstream changes):**
- `_localslack/` routes: calls, reset, snapshot/restore, state, deliveries, subscriptions (HTTP wrapper over `webhooks.register`).
- Fault injection and rate-limit tiers in the wrapper's middleware (match method, nth/every, `Retry-After`).
- Fidelity shims in the wrapper: `x-slack-req-id`, replace GitHub-style `x-ratelimit-*` headers, `unknown_method` for unrouted paths, `invalid_auth`.
- Events: `url_verification` on register, retry redelivery with `X-Slack-Retry-Num` (replay from `getDeliveries()` payloads), bad/stale signature toggle (wrap the header factory), event injection.
- Socket Mode: add `ws`; `apps.connections.open` returns a ws URL; forward events by registering an internal loopback subscription that pushes envelopes to open sockets and handles acks. No upstream change needed. (Not spiked.)
- Persistence across restarts: snapshot the store to a file on exit/interval (replaces SQLite).
- Scenario seeding: `seedFromConfig` already takes YAML; add a generator (port our seed package) and explicit-ID fixtures.
- UI: emulate's inspector for free; port our chat UI later if wanted.

**Known hard part: determinism.** IDs (`randomBytes`) and `ts` (wall clock plus a process-wide counter) are generated inside module-private functions. Seeded entities can have pinned IDs, but IDs for entities created through the API stay random. Options: patch `crypto.randomBytes` and `Date.now` at process start (hacky; ESM named imports need care, unproven), vendor a patched copy (Apache-2.0), or send upstream a small PR for injectable ID/clock. This is the one item to spike before committing to E.

**Cost:** Node runtime. Single-binary packaging (Node SEA or `bun build --compile`) is plausible but unverified. Effort estimate: call log, reset, faults, snapshot are small; events controls and Socket Mode are moderate; determinism is a spike. All smaller than building the same on a Go core, because breadth and event plumbing are inherited.

### F. Fork/vendor emulate's Slack + core (Apache-2.0)
Copy `@emulators/slack` and `@emulators/core` into our repo and change them directly.
- **For:** total control (injectable IDs and clock, SQLite or any storage, remove the multi-service pieces).
- **Against:** we own every merge; upstream ships Slack changes weekly; we lose the free breadth, which was the point.
- Use only if determinism cannot be done in E.

### G. Contribute upstream only (no product of our own)
Subscriber registration, call log, reset, faults, Socket Mode as PRs to emulate.
- **For:** least code, helps the ecosystem, maintainer responds quickly.
- **Against:** their design decides what lands; fault injection and determinism may not fit; Socket Mode and call logs may never be wanted. Also no LocalSlack as a thing.
- Compatible with E: put generalizable pieces upstream later, keep opinionated ones.

### H. Stay on Go (status quo, for comparison)
Keep building the Go core, as in v1's recommendation.
- **For:** single binary, SQLite persistence, deterministic IDs by design, full control, the learning goal.
- **Against:** every new Slack method is ours to build and verify; events and Socket Mode are L items; the fast-moving upstream will keep widening the breadth gap.

## Comparison

| | E wrapper | F fork | G upstream | H Go |
|---|---|---|---|---|
| Method breadth | inherited (~75, growing) | frozen at fork unless merged | inherited | ours, small |
| Time to events + Socket Mode + call log + faults | shortest | short | unknown | longest |
| Deterministic IDs | needs spike | easy | PR needed | built in |
| Ownership / control | medium | high | low | high |
| Single binary, no Node | unverified | unverified | no | yes |
| Upstream drift risk | pin + our suite as gate | merge burden | none | n/a (we trail instead) |
| Learning value (building a service) | lower | medium | lower | highest |

## Risks specific to starting over

- **Inherited fidelity gaps.** Upstream shares our known deviations (no `bot_id`, public-channel posting without `chat:write.public`, orphan thread replies accepted, bogus cursors ignored, O(n) thread-parent lookup). The wrapper can paper over response shapes but not state-model quirks, which need upstream fixes or a fork.
- **Their model, their choices.** Messages are one flat collection with denormalized counters; membership lives on the channel row. Features that need a different model (per-user read cursors, richer threads) fight the base.
- **Duplicated effort risk.** Open upstream issues (#293 payload fidelity, #294 retry redelivery, #263 `app_mention`, #289 interactivity) overlap our events work. Mitigation: build only what the wrapper must own (controls, faults, determinism, call log) and take the rest from upstream as it ships.
- **Evidence grading continues to apply.** Our manifest grades behavior (recorded/docs/llm), regardless of who wrote the handler. Recordings from real Slack remain the way to move methods up.
- **Learning goal.** You said you value building a service like this. E gives that up for the test-loop layer; H keeps it. This is a values call, not a technical one.

## Recommendation

If the learning goal is genuinely negotiable, do E, behind one more spike. Order of proof, each independently go/no-go:

1. Determinism: seeded RNG and clock without forking (patch approach or minimal upstream PR). Fail means F, or stay on H.
2. Run `tests/smoke` and `tests/e2e` against the wrapper (proves the upgrade gate and the swap).
3. Bolt over HTTP end to end: register subscriber, post, Bolt handler replies, signature verifies.
4. Socket Mode: `ws` server plus Bolt's socket-mode adapter connects, receives, acks.
5. Single-binary packaging (SEA or bun) and cold start time.

Passing 1 to 4 means the Go server can be retired and most of the roadmap's tier 1 collapses into wrapper features. If 1 fails or the learning goal wins, keep H, and use emulate as the oracle as v1 recommends.

Not verified here: determinism patching, Socket Mode, packaging, whether `tests/conformance` Go runners can target another server, and how upstream behaves across the 0.13+ line.
