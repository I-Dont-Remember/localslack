# Roadmap (priority order)

Owner-level view of *what matters most*. The live inventory is `docs/COMPATIBILITY.md` /
`internal/capabilities/capabilities.toml`; process is `workflow.md`. Work top-down unless the owner says
otherwise. Claim before starting (see `capabilities.md`).

**Sizes:** S = one method, follow `endpoint_guide.md`. M = a few files, one agent. L = needs a written plan
in `plans/` (and an `arch-reviewer` pass) *before* code; do not start an L item from this file alone.

## Tier 1: makes real apps work (biggest payoff)
1. **Bolt-core methods (S each, independent, do in any order):** `conversations.info`,
   `users.info`, `chat.update`, `chat.delete`, `chat.postEphemeral`, `reactions.add`,
   `conversations.members`, `api.test`. Nearly every Bolt/SDK app calls these; today they 404.
2. **Blocks/attachments round-trip (M):** store and return `blocks`, `attachments`, `metadata`;
   accept blocks-only `chat.postMessage` (deviations `no-blocks`; red test exists). Most real bot
   messages use blocks. Render basic blocks in the UI later (tier 3).
3. **Events API over HTTP (L, M2):** subscription config, `url_verification`, signed delivery
   (`X-Slack-Signature`), 3s ack/retry, `message`/`app_mention`/`reaction_added` events fired by both
   API and UI actions. This is what turns LocalSlack from a response stub into a workspace an app
   *reacts to*. Needs a design plan; also a real Bolt delivery smoke test.
4. **Socket Mode (L, M3):** `apps.connections.open`, `xapp-` tokens, WebSocket envelope + ack. Reuses
   M2 delivery. The default local-dev mode for Bolt, so high value right after M2.

## Tier 2: trust and fidelity (what separates us from a hand-written mock)
5. **Test-inspection API + reset (L):** `/_localslack/` endpoints to snapshot/reset state, seed
   scenarios, and assert calls made (DESIGN_DOC appendix). The main reason to use us in CI.
6. **Rate limiting (M, M5):** per-method tiers (already in the manifest), `429` + `Retry-After`, opt-in
   via env. Lets users test their backoff code, which mocks never do.
7. **Scopes and token errors (L, M4):** `missing_scope`, `invalid_auth`, `token_revoked`, mock OAuth
   with a god-mode token. Reads `scopes` from the embedded manifest. Needs scopes verified first.
8. **Error/header fidelity (S-M, each):** `not_in_channel` + membership checks, `x-slack-req-id` and
   other headers, `unknown_method` as 200, split `invalid_name_*` errors. Each flips a red test in
   `tests/smoke/known_deviations_test.py`; fixing deviations is cheap, well-scoped agent work.
9. **First real recordings (owner):** `make evidence-gaps`, core methods first. Biggest trust win per hour.

## Tier 3: common but secondary
- Channel membership flows: `conversations.join/leave/invite/kick`, `setTopic/setPurpose`,
  `users.conversations`, `team.info`, `reactions.remove/get`, `pins.*`, `chat.getPermalink`.
- Interactivity (L): `views.open/update/publish`, `block_actions`, slash commands, `response_url`.
  Needs blocks (2) and events (3) first.
- Files (M-L): `files.getUploadURLExternal` flow, multipart bodies.
- Private channels (`is_private`, group membership); real `users.profile.*`; `users.lookupByEmail`.
- UI polish: render blocks, reactions, edit/delete, ephemeral messages.
- OpenAPI conformance validator (M6): extra evidence signal, not urgent.

## Tier 4: low payoff (only on demand)
`admin.*`, Enterprise Grid, `usergroups.*`, `bookmarks.*`, `dnd.*`, `search.*`, `workflows.*`, `calls.*`,
`stars.*`, `emoji.list` and similar long-tail methods. Add with `verify = "unlikely"` where recording is
not realistic. Not worth doing before tiers 1-2.

## Rules of thumb
- Prefer many small independent S items to one big change. Do not mix an L item with anything else.
- A fidelity fix with an existing red test beats a new endpoint nobody has asked for.
- New method: fixture + SDK call + manifest entry are all required (`endpoint_guide.md`).
- Update this file when you finish or re-rank something; the owner decides the ranking.
