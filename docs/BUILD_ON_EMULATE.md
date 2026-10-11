# Build on emulate?

Draft, 2026-10-10. Not committed. Question: instead of building our own Slack API surface, use [vercel-labs/emulate](https://github.com/vercel-labs/emulate) as the base and layer LocalSlack's test-loop features on top. Evidence: `docs/ALTERNATIVES.md` (hands-on, v0.12.1).

## Why ask

- emulate has ~75 Slack method routes and ships Slack work weekly (releases every 1-2 weeks; open issues/PRs for events fidelity, retries, `app_mention`, interactivity, `users.conversations`, `search.*`, usergroups). Chasing that breadth alone is a losing race.
- What it lacks is what we care about: no CLI way to register an event subscriber, no call log, reset, faults or rate limits, random IDs per run, no Socket Mode, non-Slack error shapes/headers.
- Both facts argue for layering: take the breadth, add the loop.

## What a base would give us

~75 routes (chat, conversations, users, reactions, files, pins, bookmarks, views, OAuth v2, incoming webhooks), blocks/metadata round-trip, private channels, opt-in `strict_scopes` with real `missing_scope`, signed event envelopes (dispatcher exists), YAML seeding, ordered unique `ts` under concurrency.

Facts that shape the options: license is Apache-2.0; `@emulators/slack` and `@emulators/core` are published separately (0.12.1, pre-1.0); the built-in Slack `Emulator` object exposes only `url`, `reset()`, `close()` while the underlying `WebhookDispatcher` has `register`/`dispatch`/`getDeliveries`.

## Options

### A. Go sidecar: proxy in front of `npx emulate`
LocalSlack (Go) supervises emulate as a child and proxies `/api/*`.
- **Adds:** call log, fault injection, 429/`Retry-After`, rate tiers, Slack-shaped unknown-method/error/header rewriting, reset (restart child), stable IDs only by rewriting responses (fragile).
- **Hard:** events and Socket Mode. The proxy cannot see emulate's internal dispatcher, and the CLI has no subscriber registration, so we would re-derive events from observed writes. That duplicates the work we wanted to skip.
- **Cost:** needs Node at runtime; two processes; our UI/state would not match emulate's store.
- **Effort:** S-M for call log/faults, L for events.

### B. TypeScript layer over `@emulators/*`
Embed `@emulators/slack` + `@emulators/core` in our own Node process; register subscribers on the dispatcher (what the introspection-org fork does), add `_localslack` routes, a Socket Mode bridge, a call-log middleware.
- **Adds:** everything, with full access to state and dispatch. Cheapest path to events + Socket Mode.
- **Cost:** abandons Go; pre-1.0 internal APIs may break each release; loses the learning goal of building the service ourselves; most of our existing code (store, UI, capability manifest) would not carry over.
- **Effort:** M overall, but it is a rewrite of what exists.

### C. Contribute upstream
Send subscriber registration, call log, reset, faults, Socket Mode to emulate.
- **For:** least code; maintainer is responsive (two Slack issues fixed within a day); contributors are already landing Slack events work.
- **Against:** their roadmap and design (in-process, multi-service) decide what lands; no ownership; no learning from building the service; faults/determinism may not fit their model.
- **Effort:** per feature S-M plus review latency.

### D. Hybrid fallback: our core, their breadth
LocalSlack stays the primary server. For methods we have not implemented, forward to an emulate instance (`LOCALSLACK_FALLBACK_URL`), mark the capability `evidence = "upstream"`.
- **Adds:** breadth without chasing it; our loop features cover native methods.
- **Catch:** two stores. IDs, channels and messages created in one are unknown to the other, so forwarded calls act on a different workspace. Workable only for stateless or read-light methods unless we sync state, which is the hard part.
- **Effort:** S for the proxy, unbounded for consistency. Cheapest experiment of the four.

## Cross-cutting risks

- **0.x churn:** A, B and D depend on pre-1.0 behavior and payload shapes.
- **Fidelity evidence:** our capability manifest grades methods by recording; inherited behavior is someone else's guess. Upstream shares our deviations (any token accepted, public-channel posting without `chat:write.public`, no `bot_id`), so "inherited" does not mean "correct".
- **Determinism:** emulate's non-seeded IDs are random per run; reproducible scenarios need control over ID generation, which only B and C provide.
- **Runtime:** A, B, D require Node in CI; our pitch of a dependency-free binary goes away.

## Recommendation

Not now. Keep building the Go core for the gaps (events + Socket Mode, call log + reset, faults, seeded scenarios) and as a deliberate project in building such a service, not to match breadth. Meanwhile:

1. Use emulate as a differential oracle: run `tests/smoke` against it periodically (done once: 29 pass, 10 shared xfails, 4 deviations it already fixes).
2. Revisit if upstream ships CLI subscriber registration plus Socket Mode (our differentiators shrink), or if a real user needs breadth we lack.
3. First experiment if revisited: D with read-only/stateless methods (e.g. `team.info`, `pins.list`), because it tests the value without a rewrite.
