# LocalSlack testing API (`/_localslack/*`)

Contract for the test-loop controls. LocalSlack-only; never on real Slack. Unauthenticated, loopback-guarded like the rest of the server. Status: **draft contract** (agent-docs/plans/test-loop.md); each stream owns its section.

## Conventions
- **Real HTTP semantics (unlike Slack's `/api/*`, which keeps `200 + ok:false` for SDK fidelity).** Success = 2xx with the resource as the body, no `ok` wrapper. Failure = 4xx/5xx with `{"error":{"code":"<snake_case>","message":"<human>","details":[...]?}}`.
- Status codes: 200 read/update, 201 create (+`Location`), 204 delete/clear/no body, 400 malformed request, 404 unknown id/name, 409 conflict (duplicate, reset in progress), 422 well-formed but invalid (e.g. bad manifest, `details:[{"message","pointer"}]`), 5xx server fault.
- Lists: `{"items":[...],"next_since":<seq>,"generation":<n>}`; oldest first; long-poll with `since=<seq>` (exclusive), `min=<n>`, `timeout_ms=<=30000`. Default `limit` 100, cap 1000.
- `seq` is monotonic across DELETE/reset; `next_since` echoes input `since` when empty; parked long-polls wake on reset (new `generation`).

## Apps (stream A)
Multiple apps per workspace; each owns a bot user, `bot_id`, tokens, signing secret.
- `POST /_localslack/apps` -> 201 app. Body: raw Slack manifest (`application/json`, `application/yaml`) or `{"manifest":{..}|"<yaml>","deliver_to":"http://localhost:3000","signing_secret","bot_token","app_token","app_id","verify":true}`. Honored: `display_information.name`, `features.bot_user.display_name`, `oauth_config.scopes.bot`, `settings.event_subscriptions.{request_url,bot_events}`, `settings.socket_mode_enabled`; rest stored as-is.
  - App: `{"id","name","bot_user_id","bot_id","credentials":{"signing_secret","bot_token","app_token","verification_token"},"event_subscriptions":{"manifest_request_url","effective_request_url","bot_events":[]},"deliver_to","delivery":"ready|blocked","blocked_reason","socket_mode_enabled","scopes":{"bot":[]},"verified","warnings":[]}`
  - 422 `invalid_manifest` with `details` pointers.
- `GET /_localslack/apps`, `GET|DELETE /_localslack/apps/{id}`, `PATCH /_localslack/apps/{id}` (`{"deliver_to":...}`), `POST /_localslack/apps/{id}/verify` (re-sends `url_verification`).
- Startup: `--app-manifest <file>` (repeatable, env `LOCALSLACK_APP_MANIFEST`). Upsert keyed by explicit `app_id`, else deterministic from manifest name; safe on every restart; never clobbers a stored `deliver_to`.
- CLI: `localslack apps add <manifest> [--deliver-to URL]`, `apps list`, `apps set-url <app> <URL>`, `apps env <app>` (prints credentials as env vars for the app under test).

### Where events go (request URL resolution)
1. Per-app `deliver_to` (stored with the app; survives restart and partial reset), else
2. global `LOCALSLACK_DELIVER_TO` / `--deliver-to` (default for every app), else
3. manifest `request_url` **only if loopback/private**; a remote manifest URL leaves the app `delivery: "blocked"`.
- `deliver_to` is host-only (`http://localhost:3000`): it replaces scheme/host/port and keeps the manifest URL's path. A value with a path is used verbatim.
- Blocked apps: warning logged with a copy-paste `localslack apps set-url ...` fix; inspector shows the same with an editable field. Nothing is ever sent to a remote host unless `--allow-remote-request-urls`.
- Redirects are never followed. Socket Mode apps have no request URL.

## Events (stream A)
- `GET /_localslack/deliveries?event_type=&state=&app_id=&since=&min=&timeout_ms=` -> list of `{"id","seq","event_id","event_type","app_id","transport":"http|socket","state":"pending|retrying|delivered|failed","injected","payload":{..},"attempts":[{"n","at","status","latency_ms","error","retry_reason","signature_mode","timestamp","signature","response_body"}]}`
- `GET /_localslack/deliveries/{id}`; `DELETE /_localslack/deliveries` (204).
- `POST /_localslack/events` `{"event":{..},"app_id"?,"signature":"valid|invalid|stale|missing"?,"envelope":{..}?}` -> 201 `{"event_id","deliveries":[{"app_id","delivery_id"}]}`. No `app_id` = one `event_id`, one delivery per subscribed app.
- `POST /_localslack/events/{event_id}/redeliver` `{"retry_num":1,"retry_reason":"http_timeout","signature":"valid"}` -> 201; aged-out event -> 404 `event_not_found`.
- `GET|PUT /_localslack/delivery-config` (global) and `/apps/{id}/delivery-config` (override): `{"signature":"valid|invalid|stale|missing","stale_seconds":600,"timeout_ms":3000,"retry_delays_ms":[0,60000,300000],"paused":false}`. `paused` queues events.
- Delivery: signed `POST`; `X-Slack-Request-Timestamp`, `X-Slack-Signature: v0=hex(HMAC_SHA256(secret,"v0:"+ts+":"+body))`, retries add `X-Slack-Retry-Num`, `X-Slack-Retry-Reason`; 2xx = ack; `X-Slack-No-Retry: 1` stops retries. Your service just receives HTTP like it would from Slack: no polling needed.

## Socket Mode (stream S)
- Slack method `apps.connections.open` (xapp- token) -> `{"ok":true,"url":"ws://host/_localslack/socket?ticket=..&app_id=A.."}` (Slack-shaped); ticket one-time, 30s; WS upgrade checks Origin.
- WS: `hello`; `{"envelope_id","payload","type":"events_api","accepts_response_payload":false,"retry_attempt","retry_reason"}`; ack `{"envelope_id"}`.
- `GET /_localslack/socket/connections` -> list `{"id","app_id","connected_at","last_ack_at","pending"}`.
- `POST /_localslack/socket/disconnect` `{"connection_id"?,"reason":"refresh_requested|warning|link_disabled","drop":false}` -> 204.

## Call log (stream C)
- `GET /_localslack/calls?method=&channel=&app_id=&ok=&error=&since=&limit=&min=&timeout_ms=` -> list of `{"seq","time","request_id","method","http_method","status","ok","error","params":{..},"actor":{"user_id","app_id","token_kind":"bot|user|app|none"},"response":{..},"response_truncated","duration_ms","fault_id","not_implemented"}`.
- Records `/api/*` calls (incl. unknown-method fallback); not `/_localslack/*` or UI actions. Redacted: `Authorization`, `Cookie`, `token`, `client_secret`, `signing_secret`.
- `DELETE /_localslack/calls` (204).

## Reset (stream C; scenario/generate from G)
Two modes, no knobs. Reset is global: one test session per server.
- `POST /_localslack/reset` (partial, default) -> 200 `{"mode":"partial","generation","seed","counts":{"users","channels","messages"},"took_ms"}`. Wipes and reseeds workspace data (users, channels, messages, reactions) and clears call log, deliveries, faults, snapshots; delivery-config back to defaults. **Keeps** apps (credentials, bot users/ids, `deliver_to`) and socket connections.
- `POST /_localslack/reset?all=true` also removes apps: truly blank.
- Optional body: `{"seed"?,"scenario"?:{..}|"path","generate"?:{..}}` (scenario XOR generate, else 400 `invalid_reset`). Concurrent reset -> 409 `reset_in_progress`.
- CLI: `localslack reset [--all] [--seed s] [--scenario f]`.

## Faults (stream D)
- `POST /_localslack/faults` `{"method":"chat.postMessage"|"*","match":{"channel","app_id"}?,"nth"?,"every"?,"times"?,"once"?,"action":{"status":429,"error":"ratelimited","retry_after":2,"delay_ms":0,"body":{..}}}` -> 201 `{"id":"f_1",..,"seen":0,"fired":0}`.
- `GET /_localslack/faults`; `DELETE /_localslack/faults/{id}`; `DELETE /_localslack/faults` (204).
- First firing rule in creation order wins; 429 sets `Retry-After`; `delay_ms` alone delays then runs the real call; faulted calls never run the method and are logged with `fault_id`. Checked after parse and non-failing token identification, before auth enforcement (bad token is still faulted).

## State, snapshots, diff (stream G)
- Fixture v1 (snapshot format and `--scenario` format; JSON canonical, TOML/YAML loaders only): `{"version":1,"team":{..},"users":[..],"channels":[..],"dms":[..],"messages":[..],"generate":{"seed","users","channels","messages_per_channel","thread_ratio","private_channels"}}`. Includes ts, `bot_id`, reactions, edited, blocks/attachments/metadata; app bot users excluded.
- `GET /_localslack/state`; `POST /_localslack/snapshots` `{"name"}` -> 201; `GET /_localslack/snapshots` (list of `{"name","created","counts"}`); `GET /_localslack/snapshots/{name}` (fixture); `POST /_localslack/snapshots/{name}/restore` (never emits events); duplicate name -> 409.
- `GET /_localslack/diff?from=<name|current>&to=<name|current>` -> `{"users|conversations|members|messages|reactions":{"added":[],"removed":[],"changed":[{"id"|"channel+ts","fields":{"f":{"from","to"}}}]}}`.

## Misc
`GET /_localslack/health` -> 200 `{"status":"ok"}`; `GET /_localslack/state/conversations` (existing). Default bot (no app) reports a fixed default scope list incl. `chat:write`, `chat:write.public`.
