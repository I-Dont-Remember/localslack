# Roadmap (priority order)

Owner-level view of *what matters most*. The live inventory is `docs/COMPATIBILITY.md` /
`internal/capabilities/capabilities.toml`; process is `workflow.md`. Work top-down unless the owner says
otherwise. Claim before starting (see `capabilities.md`).

**Sizes:** S = one method, follow `endpoint_guide.md`. M = a few files, one agent. L = needs a written plan
in `plans/` (and an `arch-reviewer` pass) *before* code; do not start an L item from this file alone.

**Positioning:** a test environment for Slack integrations, not a Slack clone. We don't compete with
`emulate` on method breadth; we compete on the test loop. Items tagged `(gap)` close a competitor gap
(`docs/ALTERNATIVES.md`; re-check it when re-ranking).

## Tier 1: makes real apps work (biggest payoff)
1. **Bolt-core methods (S each, independent, do in any order):** `conversations.info`,
   `users.info`, `chat.update`, `chat.delete`, `chat.postEphemeral`, `reactions.add`,
   `conversations.members`, `api.test`. Nearly every Bolt/SDK app calls these; today they 404.
   Table stakes, not a differentiator.
2. **Blocks/attachments round-trip (M):** store and return `blocks`, `attachments`, `metadata`;
   accept blocks-only `chat.postMessage` (deviations `no-blocks`; red test exists). Most real bot
   messages use blocks. Render basic blocks in the UI later (tier 3).
3. **Events API over HTTP (L, M2):** subscription config, `url_verification`, signed delivery
   (`X-Slack-Signature`), 3s ack/retry, `message`/`app_mention`/`reaction_added` events fired by both
   API and UI actions. Turns LocalSlack from a response stub into a workspace an app *reacts to*.
   Plan must also cover test controls `(gap)`: `POST /_localslack/events` to inject an event,
   duplicate/retry delivery with `X-Slack-Retry-Num`, bad/stale signature toggle, delivery trace in the
   inspection API. Also a real Bolt delivery smoke test.
4. **Socket Mode (L, M3) `(gap)`:** `apps.connections.open`, `xapp-` tokens, WebSocket envelope + ack.
   Reuses M2 delivery. The default local-dev mode for Bolt; `emulate` lacks it.

## Tier 2: trust and fidelity (what separates us from a hand-written mock)
5. **Call log + reset (M) `(gap)`:** middleware recording every API call;
   `GET /_localslack/calls?method=&channel=`, `POST /_localslack/reset`. Cheap and the core CI loop;
   can run in parallel with tier 1. (Snapshot/diff is item 10.)
6. **Deterministic fault injection (M) `(gap)`:** `POST /_localslack/faults` {method, nth/every,
   status or Slack error, `Retry-After`, once}. Covers 429, 5xx, timeouts, `ratelimited`. Replaces
   time-based rate limiting as the first rate-limit feature (design doc: Rate-limiting).
7. **Access edge cases (M):** private channels (`is_private`, group membership), archived, bot not a
   member, `not_in_channel` + membership checks. Cases happy-path fixtures never cover.
8. **Error/header fidelity (S-M, each):** `x-slack-req-id` and other headers, `unknown_method` as 200,
   split `invalid_name_*` errors. Each flips a red test in `tests/smoke/known_deviations_test.py`;
   cheap, well-scoped agent work.
9. **Scenario seeding (L; plan with item 10):** three modes. (a) Explicit fixture first: exact user/
   channel IDs, memberships, messages (JSON/TOML; validate duplicate/malformed IDs; synthetic profiles
   only). (b) Generated scale ("50 channels, 400 users, busy threads") on the seed generator; fast, and
   exposes `users-list-unpaginated`. (c) Behavioral = item 6. Via CLI flag / config / inspection API.
10. **State snapshot/diff (L):** with item 9, one feature family with reset.
11. **First real recordings (owner):** `make evidence-gaps`, core methods first. Biggest trust win per hour.

## Tier 3: common but secondary
- Interactivity `(gap)` (L): `views.open/update/publish`, `block_actions`, slash commands, `response_url`.
  `emulate` lacks interaction/slash simulation. Needs blocks (2) and events (3); do right after M3.
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
