# Roadmap (priority order)

Owner-level view of *what matters most*. The live inventory is `docs/COMPATIBILITY.md` /
`internal/capabilities/capabilities.toml`; process is `workflow.md`. Work top-down unless the owner says
otherwise. Claim before starting (see `capabilities.md`).

**Sizes:** S = one method, follow `endpoint_guide.md`. M = a few files, one agent. L = needs a written plan
in `plans/` (and an `arch-reviewer` pass) *before* code; do not start an L item from this file alone.

**Positioning:** a test environment for Slack integrations, not a Slack clone. We know `emulate` is broader
and moves fast; we are not matching its method count. LocalSlack explores the gaps it leaves (the test
loop) and is a deliberate exercise in building such a service. Where we need breadth, `emulate` is the
reference (`docs/ALTERNATIVES.md`). Items tagged `(gap)` close a
competitor gap; re-check ALTERNATIVES when re-ranking.

## Tier 1: the test loop (what emulate lacks)
1. **Events API over HTTP (L, M2) `(gap)` DONE (on `test-loop`):** subscription config, `url_verification`, signed delivery
   (`X-Slack-Signature`), 3s ack/retry, `message`/`app_mention`/`reaction_added` events fired by both
   API and UI actions. Turns LocalSlack from a response stub into a workspace an app *reacts to*.
   Plan must also cover test controls: `POST /_localslack/events` to inject an event,
   duplicate/retry delivery with `X-Slack-Retry-Num`, bad/stale signature toggle, delivery trace in the
   inspection API. Also a real Bolt delivery smoke test. Needs the enabling work in items 5-6 for a
   realistic Bolt app. Note: emulate has signed payloads but no CLI subscriber registration; upstream
   issues #293/#294/#263 are closing parts of this, so keep our edge in the controls.
2. **Socket Mode (L, M3) `(gap)` DONE (on `test-loop`; gap: interactivity and slash-command envelopes):** `apps.connections.open`, `xapp-` tokens, WebSocket envelope + ack.
   Reuses M2 delivery. The default local-dev mode for Bolt; `emulate` lacks it.
3. **Call log + reset (M) `(gap)` DONE (on `test-loop`; reset has partial and `--all` modes):** middleware recording every API call;
   `GET /_localslack/calls?method=&channel=`, `POST /_localslack/reset`. Cheap and the core CI loop;
   independent of items 1-2, can run in parallel. (Snapshot/diff is item 10.)
4. **Deterministic fault injection (M) `(gap)` DONE (on `test-loop`):** `POST /_localslack/faults` {method, nth/every,
   status or Slack error, `Retry-After`, once}. Covers 429, 5xx, timeouts, `ratelimited`. Replaces
   time-based rate limiting as the first rate-limit feature (design doc: Rate-limiting).

## Tier 2: enabling work and fidelity
5. **Bolt-core methods (S each, independent) DONE (on `test-loop`):** `conversations.info`, `users.info`, `chat.update`,
   `chat.delete`, `chat.postEphemeral`, `reactions.add`, `conversations.members`, `api.test`. Nearly
   every Bolt/SDK app calls these; today they 404. Table stakes that `emulate` already covers: do them as
   the loop (items 1-2) needs them, not as a headline.
6. **Blocks/attachments round-trip (M) DONE (on `test-loop`; UI does not render blocks yet):** store and return `blocks`, `attachments`, `metadata`;
   accept blocks-only `chat.postMessage` (deviation `no-blocks`; red test exists; `emulate` passes it).
   Render basic blocks in the UI later (tier 3).
7. **Access edge cases (M) DONE (on `test-loop`):** private channels (`is_private`, group membership), archived, bot not a
   member, `not_in_channel` + membership checks. Cases happy-path fixtures never cover.
8. **Error/header fidelity (S-M, each) DONE (on `test-loop`):** `x-slack-req-id` and other headers, `unknown_method` as 200,
   split `invalid_name_*` errors. Each flips a red test in `tests/smoke/known_deviations_test.py`;
   cheap, well-scoped agent work. (`emulate` shares these deviations.)
9. **Scenario seeding (L; plan with item 10) DONE (on `test-loop`; modes a, b, c shipped):** three modes. (a) Explicit fixture first: exact user/
   channel IDs, memberships, messages (JSON/TOML; validate duplicate/malformed IDs; synthetic profiles
   only). (b) Generated scale ("50 channels, 400 users, busy threads") on the seed generator; fast, and
   exposes `users-list-unpaginated`. (c) Behavioral = item 4. Via CLI flag / config / inspection API.
   `emulate` has random per-run IDs; reproducibility is our edge.
10. **State snapshot/diff (L) DONE (on `test-loop`; in-memory snapshots):** with item 9, one feature family with reset.
11. **First real recordings (owner):** `make evidence-gaps`, core methods first. Biggest trust win per hour.

### Remaining gaps from tiers 1-2 (known, not bugs)
- Message objects lack the `team` field; multipart/form-data bodies are not parsed; `chat.postMessage` by channel *name* and DMs by user id (`channel=U...`) are not supported.
- Interactivity and slash commands are not sent over Socket Mode (nor HTTP); Tier 3 below.
- Snapshots live in memory (lost on restart); blocks are stored but not rendered in the UI.
- Item 11 (first real recordings) is still open: event, socket and tombstone payload shapes are `llm` evidence.

## Tier 3: common but secondary
- Interactivity `(gap)` (L): `views.open/update/publish`, `block_actions`, slash commands, `response_url`.
  `emulate` lacks interaction/slash simulation. Needs blocks (6) and events (1); do right after M3. Upstream issue #289 is building this, so check before starting.
- Channel membership flows: `conversations.join/leave/invite/kick`, `setTopic/setPurpose`,
  `users.conversations`, `team.info`, `reactions.remove/get`, `pins.*`, `chat.getPermalink`.
- Scopes (M4): `missing_scope` checks driven by the manifest's `scopes`, permissive default (`emulate`
  has opt-in strict scopes; match it cheaply). Skip mock OAuth flows and exhaustive token-error
  emulation: auth failures stop developers immediately.
- Opt-in realistic rate-limit tiers (M, M5): per-method tiers from the manifest, `429` + `Retry-After`,
  env var, off by default. After fault injection.
- Files (M-L): `files.getUploadURLExternal` flow, multipart bodies.
- Real `users.profile.*`; `users.lookupByEmail`.
- UI: inspector first (call log, event deliveries, state); then render blocks, reactions, edit/delete,
  ephemeral. No chat-client polish for its own sake.
- OpenAPI conformance validator (M6): extra evidence signal, not urgent.
- Claude Skill for LocalSlack users (S, later): a concise skill that knows the inspection/seed/reset
  APIs and internals. Write it once those APIs have stabilized.

## Tier 4: low payoff (only on demand)
`admin.*`, Enterprise Grid/multi-workspace, `usergroups.*`, `bookmarks.*`, `dnd.*`, `search.*`,
`workflows.*`, `calls.*`, `stars.*`, `emoji.list` and similar long-tail methods. Add with
`verify = "unlikely"` where recording is not realistic. Not worth doing before tiers 1-2.

## Rules of thumb
- Prefer many small independent S items to one big change. Do not mix an L item with anything else.
- A fidelity fix with an existing red test beats a new endpoint nobody has asked for.
- New method: fixture + SDK call + manifest entry are all required (`endpoint_guide.md`).
- Update this file when you finish or re-rank something; the owner decides the ranking.
