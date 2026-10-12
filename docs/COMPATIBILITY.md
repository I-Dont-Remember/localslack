# Compatibility

> Generated from `internal/capabilities/capabilities.toml`, the conformance fixtures and the smoke tests by `make docs`. Do not edit by hand.

What LocalSlack supports, how well each piece is verified against real Slack, and where it knowingly differs. Anything not listed as implemented returns `404 not_implemented`.

**Scope:** LocalSlack deliberately covers a small part of Slack's API: the common surface Bolt and SDK apps use. Many methods (admin.*, Enterprise Grid, paid-plan features, destructive operations) may never be recorded from real Slack; this page says honestly what is verified and what is a best guess.

**Methods:** 8 implemented, 1 partial, 9 in_progress, 7 planned. **Evidence of shipped methods:** 0 recorded, 0 docs, 9 llm, 0 none.

## How to read the evidence column

| Level | Meaning |
|---|---|
| `recorded` | A sanitized response captured from real Slack is committed as a conformance fixture. |
| `docs` | Checked against a cited source (Slack docs or SDK source). |
| `llm` | Written from model knowledge and never checked. Treat as a best guess. |
| `none` | No conformance fixture exists. |

"Red test" in the deviations table means an executable strict-xfail test in `tests/smoke/known_deviations_test.py` encodes the real Slack behavior. Tier and scopes are transcribed from Slack's docs and are unverified until a recording says otherwise. "SDK-called" is a text scan: a Slack SDK client call to the method appears in `tests/smoke` or `tests/e2e`. It is a hint, not proof the behavior is verified.

## Methods

| Method | Status | Evidence | Tier | Scopes | SDK-called | Deviations |
|---|---|---|---|---|---|---|
| `auth.test` | implemented | llm | 3 | — | yes | `any-token-accepted` |
| `chat.postMessage` | partial | llm | — | `chat:write`, `chat:write.public` | yes | `name-targets-unresolved`, `no-membership-check`, `partial-message-fields` |
| `conversations.archive` | implemented | llm | 2 | `channels:manage`, `groups:write`, `im:write`, `mpim:write` | yes | `collapsed-errors` |
| `conversations.create` | implemented | llm | 2 | `channels:manage`, `groups:write`, `im:write`, `mpim:write` | yes | `collapsed-errors`, `no-private-channels` |
| `conversations.history` | implemented | llm | 3 | `channels:history`, `groups:history`, `im:history`, `mpim:history` | yes | `no-membership-check`, `partial-message-fields`, `limit-clamping`, `cursor-format` |
| `conversations.list` | implemented | llm | 2 | `channels:read`, `groups:read`, `im:read`, `mpim:read` | yes | `no-private-channels`, `partial-conversation-fields`, `limit-clamping` |
| `conversations.open` | implemented | llm (errors only) | 3 | `groups:write`, `im:write`, `mpim:write` | yes | `partial-conversation-fields` |
| `conversations.replies` | implemented | llm | 3 | `channels:history`, `groups:history`, `im:history`, `mpim:history` | yes | `no-membership-check`, `partial-message-fields`, `limit-clamping`, `cursor-format` |
| `users.list` | implemented | llm | 2 | `users:read` | yes | `users-list-unpaginated`, `partial-user-fields` |

### Planned and in progress

| Method | Status | Priority | Tier | Scopes | Plan |
|---|---|---|---|---|---|
| `api.test` | in_progress | bolt-core | — | — | agent-docs/plans/test-loop.md |
| `chat.delete` | in_progress | bolt-core | 3 | `chat:write` | agent-docs/plans/test-loop.md |
| `chat.postEphemeral` | in_progress | bolt-core | 4 | `chat:write` | agent-docs/plans/test-loop.md |
| `chat.update` | in_progress | bolt-core | 3 | `chat:write` | agent-docs/plans/test-loop.md |
| `conversations.info` | in_progress | bolt-core | 3 | `channels:read`, `groups:read`, `im:read`, `mpim:read` | agent-docs/plans/test-loop.md |
| `conversations.members` | in_progress | bolt-core | 4 | `channels:read`, `groups:read`, `im:read`, `mpim:read` | agent-docs/plans/test-loop.md |
| `reactions.add` | in_progress | bolt-core | 3 | `reactions:write` | agent-docs/plans/test-loop.md |
| `users.info` | in_progress | bolt-core | 4 | `users:read` | agent-docs/plans/test-loop.md |
| `conversations.invite` | planned | common | 3 | `channels:manage`, `groups:write`, `im:write`, `mpim:write` | — |
| `conversations.join` | planned | common | 3 | `channels:join` | — |
| `conversations.leave` | planned | common | 3 | `channels:manage`, `groups:write`, `im:write`, `mpim:write` | — |
| `team.info` | planned | common | 3 | `team:read` | — |
| `users.conversations` | planned | common | 3 | `channels:read`, `groups:read`, `im:read`, `mpim:read` | — |
| `views.open` | planned | interactivity | 4 | — | — |
| `views.publish` | planned | interactivity | 4 | — | — |
| `apps.connections.open` | in_progress | socket-mode | 1 | — | agent-docs/plans/test-loop.md |

## Behaviors and quirks

| Behavior | Status | Milestone | Evidence | Summary | Deviations |
|---|---|---|---|---|---|
| `api-errors-http-200` | implemented | — | llm | API errors are HTTP 200 with ok:false and an error code, as Slack does. | `unknown-method-404`, `collapsed-errors` |
| `app-manifest` | planned | — | — | Register multiple apps from Slack app manifests (JSON/YAML) via flag or /_localslack/apps. | — |
| `block-kit` | implemented | — | llm | chat.postMessage stores blocks, attachments and message metadata (JSON strings or JSON values), validates their shape (not Block Kit element schemas), and history/replies return them (metadata only with include_all_metadata). The UI renders text only. | — |
| `call-log` | planned | — | — | Every API call recorded (params, actor, response, timing, fault id) and queryable via /_localslack/calls. | — |
| `event-delivery-controls` | planned | — | — | Test controls for event delivery: inject, redeliver, bad/stale signatures, pause, delivery trace. | — |
| `events-api-http` | planned | M2 | — | Events API delivery over HTTP with url_verification and X-Slack-Signature signing. | — |
| `fault-injection` | planned | — | — | Deterministic per-method faults (429 + Retry-After, 5xx, Slack errors, delays) via /_localslack/faults. | — |
| `form-and-json-bodies` | implemented | — | docs | Request params are read from the query string, urlencoded form bodies and JSON bodies; arrays become comma lists. | `token-in-json-body` |
| `get-methods-accept-post` | implemented | — | docs | Any method accepts POST (and GET) regardless of the HTTP verb the docs list; the Python SDK always POSTs. | — |
| `inspection-api` | planned | — | — | LocalSlack-only /_localslack/ endpoints to snapshot, reset and assert on state and API calls, beyond what Slack's own APIs expose. | — |
| `membership-enforcement` | planned | — | — | not_in_channel / channel_not_found for non-members reading or posting in channels and DMs. | `no-membership-check` |
| `message-ts` | implemented | — | llm | Message ts values are unique and strictly increasing, in Slack's seconds.micros format, across restarts. | — |
| `multipart-bodies` | planned | — | — | multipart/form-data requests (file uploads). | `multipart-ignored` |
| `oauth-scopes` | planned | M4 | — | Per-method scope checks (missing_scope) with a permissive default; full OAuth flow emulation is intentionally low priority. | `any-token-accepted` |
| `opaque-cursor-pagination` | implemented | — | llm | Cursor pagination with response_metadata.next_cursor; cursors are opaque and bound to their endpoint. | `cursor-format`, `limit-clamping` |
| `private-channels` | planned | — | — | Private channels and groups. | `no-private-channels` |
| `rate-limiting` | planned | M5 | — | Per-method rate-limit tiers returning HTTP 429 with Retry-After (opt-in via env var). | — |
| `response-headers` | planned | — | — | Slack response headers (x-slack-req-id, x-oauth-scopes, x-accepted-oauth-scopes, retry-after) and the charset on Content-Type. | `response-headers-missing` |
| `scenario-seeding` | planned | — | — | Reproducible workspace scenarios (e.g. 50 channels, 400 users, busy threads) from a flag, config or the inspection API, so tests need no setup boilerplate. | — |
| `slack-style-ids` | implemented | — | llm | Teams, users, channels, DMs and group DMs get Slack-shaped IDs (T/U/C/D/G + 10 chars). | — |
| `socket-mode` | planned | M3 | — | Socket Mode: xapp- tokens, apps.connections.open and a WebSocket event/ack channel. | — |
| `state-snapshot` | planned | — | — | Snapshot, restore and diff workspace state; snapshot format doubles as scenario fixture. | — |

## Known deviations from real Slack

| Deviation | Severity | What differs | Affects | Red test |
|---|---|---|---|---|
| `any-token-accepted` | medium | Any xoxb- token maps to the bot and xoxp-<userID> to that user; there is no token validation, revocation or scope checking. | `auth.test`, `oauth-scopes` | — |
| `multipart-ignored` | medium | multipart/form-data bodies are silently ignored, so their params are dropped (usually surfacing as not_authed or no_text). | `multipart-bodies` | yes |
| `name-targets-unresolved` | medium | The channel param must be an ID: "#general" and user IDs as DM targets return channel_not_found (Slack resolves both). | `chat.postMessage` | yes |
| `no-membership-check` | medium | Any caller can read or post in any channel or DM (Slack returns not_in_channel or channel_not_found); only im/mpim listings are filtered by membership. | `chat.postMessage`, `conversations.history`, `conversations.replies`, `membership-enforcement` | yes |
| `no-private-channels` | medium | Only public channels exist; types=private_channel returns nothing and is_private is never true for channels. | `conversations.create`, `conversations.list`, `private-channels` | yes |
| `partial-message-fields` | medium | Messages omit bot_profile, team, app_id, client_msg_id, edited, reactions, subtype and reply_users. | `chat.postMessage`, `conversations.history`, `conversations.replies` | yes |
| `response-headers-missing` | medium | Responses lack x-slack-req-id, x-oauth-scopes, x-accepted-oauth-scopes and retry-after, and Content-Type has no charset. | `response-headers` | yes |
| `collapsed-errors` | low | Distinct Slack errors are merged: invalid_name_required/specials/maxlength become invalid_name; unparseable bodies become invalid_arguments. | `api-errors-http-200`, `conversations.archive`, `conversations.create` | yes |
| `cursor-format` | low | Cursors are unpadded base64url of an internal key; Slack's are padded base64. Clients should treat both as opaque. | `conversations.history`, `conversations.replies`, `opaque-cursor-pagination` | — |
| `limit-clamping` | low | Invalid or out-of-range limit values are silently clamped (default 100, max 1000) instead of erroring; Slack's defaults and caps vary by method. | `conversations.history`, `conversations.list`, `conversations.replies`, `opaque-cursor-pagination` | — |
| `partial-conversation-fields` | low | Conversations omit topic, purpose, previous_names, is_shared/is_ext_shared/is_org_shared and updated; IMs include num_members. | `conversations.list`, `conversations.open` | — |
| `partial-user-fields` | low | Users carry only id, team_id, name, real_name, deleted, is_bot and a minimal profile. | `users.list` | — |
| `token-in-json-body` | low | A token field inside a JSON body is accepted; Slack only accepts it in the Authorization header or form/query params. | `form-and-json-bodies` | yes |
| `unknown-method-404` | low | Unimplemented methods return HTTP 404 {error: not_implemented} (a deliberate signal); Slack returns HTTP 200 unknown_method. | `api-errors-http-200` | yes |
| `users-list-unpaginated` | low | users.list ignores limit and cursor and always returns every user with an empty next_cursor. | `users.list` | yes |

## Clients verified

Versions pinned by `tests/smoke/uv.lock`, i.e. what the smoke suite really runs.

| Client | Version |
|---|---|
| slack-sdk | 3.41.0 |
| slack-bolt | 1.30.0 |
| aiohttp | 3.14.4 |
