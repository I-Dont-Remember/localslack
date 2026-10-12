# LocalSlack testing API (`/_localslack/*`)

Contract for the test-loop controls. LocalSlack-only; never on real Slack. All JSON. Unauthenticated, loopback-guarded like the rest of the server. Status: **draft contract** (agent-docs/plans/test-loop.md); each stream owns its section.

Conventions: success `{"ok":true,...}`; failure `{"ok":false,"error":"<code>"}` (HTTP 400 for malformed control requests). Long-poll lists accept `since=<seq>` (exclusive), `min=<n>`, `timeout_ms=<=30000` and return `next_since`. Lists are oldest-first.

## Apps (stream A)
Multiple apps per workspace; each owns a bot user, `bot_id`, tokens, signing secret.
- `POST /_localslack/apps`: body is a raw Slack manifest (`application/json`, `application/yaml`) or `{"manifest":{..}|"<yaml>","signing_secret","bot_token","app_token","app_id","verify":true}`. Honored: `display_information.name`, `features.bot_user.display_name`, `oauth_config.scopes.bot`, `settings.event_subscriptions.{request_url,bot_events}`, `settings.socket_mode_enabled`; rest stored.
  - 200 `{"ok":true,"app":{"id","name","bot_user_id","bot_id","credentials":{"signing_secret","bot_token","app_token","verification_token"},"event_subscriptions":{"request_url","bot_events":[]},"socket_mode_enabled","scopes":{"bot":[]},"verified","request_url_verification":{"ok","status","latency_ms","error"}},"warnings":[]}`
  - 400 `{"ok":false,"error":"invalid_manifest","errors":[{"message","pointer":"/settings/..."}]}`
- `GET /_localslack/apps`, `GET|DELETE /_localslack/apps/{id}`, `POST /_localslack/apps/{id}/verify` (re-sends `url_verification`).
- Startup: `--app-manifest <file>` (repeatable, env `LOCALSLACK_APP_MANIFEST`), registered with `verify:false`.

## Events (stream A)
- `GET /_localslack/deliveries?event_type=&state=&app_id=&since=&min=&timeout_ms=` → `{"ok","deliveries":[{"id","seq","event_id","event_type","app_id","transport":"http|socket","state":"pending|retrying|delivered|failed","injected","payload":{..},"attempts":[{"n","at","status","latency_ms","error","retry_reason","signature_mode","timestamp","signature","response_body"}]}],"next_since"}`
- `GET /_localslack/deliveries/{id}`; `DELETE /_localslack/deliveries`.
- `POST /_localslack/events` `{"event":{..inner event..},"app_id"?,"signature":"valid|invalid|stale|missing"?,"envelope":{..overrides}?}` → `{"ok","event_id","delivery_ids":[]}`.
- `POST /_localslack/events/{event_id}/redeliver` `{"retry_num":1,"retry_reason":"http_timeout","signature":"valid"}` → duplicate delivery, same `event_id`.
- `GET|PUT /_localslack/delivery-config` `{"signature":"valid|invalid|stale|missing","stale_seconds":600,"timeout_ms":3000,"retry_delays_ms":[0,60000,300000],"paused":false}`.
- Delivery: signed `POST` to `request_url`; headers `X-Slack-Request-Timestamp`, `X-Slack-Signature: v0=hex(HMAC_SHA256(secret,"v0:"+ts+":"+body))`, on retries `X-Slack-Retry-Num`, `X-Slack-Retry-Reason`; 2xx = ack; `X-Slack-No-Retry: 1` stops retries.

## Socket Mode (stream S)
- Slack method `apps.connections.open` (xapp- token) → `{"ok","url":"ws://host/_localslack/socket?ticket=..&app_id=A.."}`; ticket one-time, 30s.
- WS: `hello`; `{"envelope_id","payload","type":"events_api","accepts_response_payload":false,"retry_attempt","retry_reason"}`; ack `{"envelope_id"}`.
- `GET /_localslack/socket/connections` → `[{"id","app_id","connected_at","last_ack_at","pending"}]`.
- `POST /_localslack/socket/disconnect` `{"connection_id"?,"reason":"refresh_requested|warning|link_disabled","drop":false}`.

## Call log (stream C)
- `GET /_localslack/calls?method=&channel=&app_id=&ok=&error=&since=&limit=&min=&timeout_ms=` → `{"ok","calls":[{"seq","time","request_id","method","http_method","status","ok","error","params":{..},"actor":{"user_id","app_id","token_kind":"bot|user|app|none"},"response":{..<=64KB},"duration_ms","fault_id","not_implemented"}],"next_since"}`. Tokens and `client_secret` never stored.
- `DELETE /_localslack/calls`.

## Reset (stream C; scenario/generate from G)
- `POST /_localslack/reset` `{"seed"?,"scenario"?:{..}|"path","generate"?:{..},"keep"?:["faults","calls","deliveries","snapshots"]}` → `{"ok","seed","counts":{"users","channels","messages"},"took_ms"}`. Apps persist; clears calls/deliveries/faults unless kept.
- CLI: `localslack reset [--seed s] [--scenario f]`.

## Faults (stream D)
- `POST /_localslack/faults` `{"method":"chat.postMessage"|"*","match":{"channel","app_id"}?,"nth"?,"every"?,"times"?,"once"?,"action":{"status":429,"error":"ratelimited","retry_after":2,"delay_ms":0,"body":{..}}}` → `{"ok","fault":{"id":"f_1",..,"seen":0,"fired":0}}`.
- `GET /_localslack/faults`; `DELETE /_localslack/faults/{id}`; `DELETE /_localslack/faults`.
- First firing rule in creation order wins; 429 sets `Retry-After`; `delay_ms` alone delays then runs the real call; faulted calls never run the method and are logged with `fault_id`. Checked after parse and non-failing token identification, before auth enforcement (so `match.app_id` works; a bad token is still faulted).

## State, snapshots, diff (stream G)
- Fixture v1 (also the snapshot format and `--scenario` format; JSON/TOML/YAML): `{"version":1,"team":{..},"users":[..],"channels":[..],"dms":[..],"messages":[..],"generate":{"seed","users","channels","messages_per_channel","thread_ratio","private_channels"}}`.
- `GET /_localslack/state`; `POST|GET /_localslack/snapshots` (`{"name"}`); `GET /_localslack/snapshots/{name}`; `POST /_localslack/snapshots/{name}/restore`.
- `GET /_localslack/diff?from=<name>&to=current` → `{"ok","changes":{"users|conversations|members|messages|reactions":{"added":[],"removed":[],"changed":[{"id"|"channel+ts","fields":{"f":{"from","to"}}}]}}}`.

## Unchanged
`GET /_localslack/health`, `GET /_localslack/state/conversations` (moves to `internal/admin`).

## Cross-cutting rules (arch-review amendments)
- Errors: unknown id/name -> 404 `{"ok":false,"error":"not_found"}`; duplicates (app_id, credentials, snapshot name) -> 409 `already_exists`; concurrent reset -> 409 `reset_in_progress`; bad reset combo -> 400 `invalid_reset` (`seed`/`scenario`/`generate` precedence: scenario XOR generate; seed alone ok).
- `--app-manifest` is an upsert keyed by explicit `app_id`, else deterministic from manifest name; credentials updated in place; safe on every restart.
- `POST /events` without `app_id`: one `event_id`, one delivery per subscribed app (`delivery_ids` per app). `redeliver` after the event ages out -> 404 `event_not_found`. `paused` holds (queues) events.
- Delivery config: global default plus per-app override (`PUT /apps/{id}/delivery-config`).
- Long-poll: `seq` monotonic across DELETE/reset; `next_since` echoes input `since` when empty; responses carry `generation`; parked pollers are woken on reset. Default `limit` 100, cap 1000; truncated responses carry `response_truncated`.
- Call log: `/_localslack/*` and UI actions not logged; unknown-method fallback is logged; redact `Authorization`, `Cookie`, `token`, `client_secret`, `signing_secret`.
- Reset: `keep` allow-list may include `apps`, `delivery-config`, `sockets`, `faults`, `calls`, `deliveries`, `snapshots`; response includes `generation`. Apps persist by default.
- Fixture: JSON canonical (TOML/YAML are loaders only); schema includes ts, `bot_id`, reactions, edited, blocks/attachments/metadata so snapshots round-trip; app bot users excluded from export.
- Diff accepts `from`/`to` as snapshot names or `current`.
- Safety: `request_url` must be loopback/private unless `--allow-remote-request-urls`; never follow redirects; WebSocket upgrade checks Origin plus one-time ticket.
- Default bot (no app) reports a fixed default scope list incl. `chat:write`, `chat:write.public`.
