# LocalSlack testing API (`/_localslack/*`)

Contract for the test-loop controls. LocalSlack-only; never on real Slack. Unauthenticated, loopback-guarded like the rest of the server. Status: **draft contract** (agent-docs/plans/test-loop.md); each stream owns its section.

## Conventions
- **Real HTTP semantics (unlike Slack's `/api/*`, which keeps `200 + ok:false` for SDK fidelity).** Success = 2xx with the resource as the body, no `ok` wrapper. Failure = 4xx/5xx with `{"error":{"code":"<snake_case>","message":"<human>","details":[...]?}}`. This includes server-level failures under `/_localslack/`: unknown path 404 `not_found`, wrong verb 405 `method_not_allowed`, guard 403 `invalid_host` / `forbidden_origin`, body over the cap (1 MiB) 413 `payload_too_large`, panic 500 `internal_error`. `/api/*` and the UI keep Slack's `{"ok":false,"error":code}` for the same failures.
- Status codes: 200 read/update, 201 create (+`Location`), 204 delete/clear/no body, 400 malformed request, 404 unknown id/name, 409 conflict (duplicate, reset in progress), 422 well-formed but invalid (e.g. bad manifest, `details:[{"message","pointer"}]`), 5xx server fault.
- Lists: `{"items":[...],"next_since":<seq>,"generation":<n>}`; oldest first; long-poll with `since=<seq>` (exclusive), `min=<n>`, `timeout_ms=<=30000`. Default `limit` 100, cap 1000.
- `seq` is monotonic across DELETE/reset; `next_since` echoes input `since` when empty; parked long-polls wake on reset (new `generation`).

## Apps (stream A)
Multiple apps per workspace; each owns a bot user, `bot_id`, tokens, signing secret.
- `POST /_localslack/apps` -> 201 app. Without `app_id` the id is derived from the display name, so two apps with equal names collide: 409 `app_exists` (message names the id; send a distinct `app_id` or DELETE the existing app). Body: raw Slack manifest (`application/json`, `application/yaml`) or `{"manifest":{..}|"<yaml>","deliver_to":"http://localhost:3000","signing_secret","bot_token","app_token","app_id","verify":true}`. Honored: `display_information.name`, `features.bot_user.display_name`, `oauth_config.scopes.bot`, `settings.event_subscriptions.{request_url,bot_events}`, `settings.socket_mode_enabled`; rest stored as-is.
  - App: `{"id","name","bot_user_id","bot_id","credentials":{"signing_secret","bot_token","app_token","verification_token"},"event_subscriptions":{"manifest_request_url","effective_request_url","bot_events":[]},"deliver_to","delivery":"ready|blocked","blocked_reason","socket_mode_enabled","scopes":{"bot":[]},"verified","warnings":[]}`
  - 422 `invalid_manifest` with `details` pointers. 409 `already_exists` when `bot_token`/`app_token` already belong to another app (tokens are unique across apps).
  - Tokens: deleting an app revokes its bot/app tokens for good (`invalid_auth`, even in permissive mode) until `reset --all` or restart. `--strict-tokens`: any `xoxb-` token that is not a registered app's is `invalid_auth` (default: it acts as the built-in bot).
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
- **`event_id` is per app per event**, like real Slack: each app's envelope carries its own. A redelivery reuses the original `event_id` (a duplicate) with a new `delivery_id`.
- `POST /_localslack/events` `{"event":{..},"app_id"?,"signature":"valid|invalid|stale|missing"?,"envelope":{..}?}` -> 201 `{"deliveries":[{"app_id","event_id","delivery_id"}]}`. No `app_id` = one delivery (own `event_id`) per subscribed app.
- Redeliver: `POST /_localslack/events/{event_id}/redeliver` or `POST /_localslack/deliveries/{delivery_id}/redeliver`, body `{"app_id"?,"retry_num":1,"retry_reason":"http_timeout","signature":"valid"}` -> 201 `{"deliveries":[{..}]}`. Because event ids are unique per app delivery, `event_id` alone is unambiguous; `app_id`, if given, must be that event's app. Aged-out/unknown/mismatched event -> 404 `event_not_found`; unknown delivery -> 404 `delivery_not_found`.
- Delivery config document: `{"signature":"valid|invalid|stale|missing","stale_seconds":600,"timeout_ms":3000,"retry_delays_ms":[0,60000,300000],"paused":false}`. `paused` queues events.
  - Global `/delivery-config`: `GET`; `PUT` = full replace (omitted fields return to the server's defaults); `PATCH` = merge (omitted fields keep their value). Invalid -> 422 `invalid_delivery_config`.
  - Per app `/apps/{id}/delivery-config`: stored as a sparse **override**; fields not overridden follow the global config live. `GET` -> the effective config plus `"override":{..only the overridden fields..}`. `PUT` replaces the override with exactly the given fields, `PATCH` adds to it, `DELETE` (204) drops it.
  - `reset` (partial or `all`) drops all per-app overrides and restores the global defaults; deleting an app drops its override.
- Delivery: signed `POST`; `X-Slack-Request-Timestamp`, `X-Slack-Signature: v0=hex(HMAC_SHA256(secret,"v0:"+ts+":"+body))`, retries add `X-Slack-Retry-Num`, `X-Slack-Retry-Reason`; 2xx = ack; `X-Slack-No-Retry: 1` stops retries. Ordering: first attempts to one app are made one at a time in publish order (a slow app delays its later events, never another app's); retries run on their own timers, so a retry may arrive after later events. Your service just receives HTTP like it would from Slack: no polling needed.

## Socket Mode (stream S)
- Slack method `apps.connections.open` (xapp- token) -> `{"ok":true,"url":"ws://host/_localslack/socket?ticket=..&app_id=A.."}` (Slack-shaped); ticket one-time, 30s; WS upgrade checks Origin.
- WS: `hello`; `{"envelope_id","payload","type":"events_api","accepts_response_payload":false,"retry_attempt","retry_reason"}`; ack `{"envelope_id"}`.
- `GET /_localslack/socket/connections` -> list `{"id","app_id","connected_at","last_ack_at","pending"}`.
- `POST /_localslack/socket/disconnect` `{"connection_id"?,"reason":"refresh_requested|warning|link_disabled","drop":false}` -> 204.

## Call log (stream C)
- `GET /_localslack/calls?method=&channel=&app_id=&ok=&error=&since=&limit=&min=&timeout_ms=` -> list of `{"seq","time","request_id","method","http_method","status","ok","error","params":{..},"actor":{"user_id","app_id","token_kind":"bot|user|app|none"},"response":{..},"response_truncated","duration_ms","fault_id","not_implemented"}`.
- Records `/api/*` calls (incl. unknown-method fallback); not `/_localslack/*` or UI actions. Redacted: `Authorization`, `Cookie`, `token`, `client_secret`, `signing_secret`.
- Bad query values (`ok=maybe`, negative `since`, ...) -> 400 `invalid_query`. `min` without `timeout_ms` waits up to 5s. A response over 64KB is dropped: `response` null, `response_truncated` true. Unknown methods keep their params and actor.
- `DELETE /_localslack/calls` (204): empties the log; `seq` keeps growing and `generation` increments (parked long-polls wake and return the new generation).

## Reset (stream C; scenario/generate from G)
Two modes, no knobs. Reset is global: one test session per server.
- `POST /_localslack/reset` (partial, default) -> 200 `{"mode":"partial","generation","seed","counts":{"users","channels","messages"},"took_ms"}`. Wipes and reseeds workspace data (users, channels, messages, reactions) and clears call log, deliveries, faults, snapshots; delivery-config back to defaults. **Keeps** apps (credentials, bot users/ids, `deliver_to`) and socket connections.
- `POST /_localslack/reset?all=true` also removes apps: truly blank.
- Optional body: `{"seed"?,"scenario"?:{..}|"path","generate"?:{..}}` (scenario XOR generate, else 400 `invalid_reset`). Concurrent reset -> 409 `reset_in_progress`.
- Bare reset reseeds with the server's configured seed (random if none); the reply echoes the seed used. The team id, apps, `seq` counters and ts high-water mark survive, so new ts always exceed old ones. Until the scenario stream lands, `scenario`/`generate` -> 422 `not_supported_yet`. Unknown body fields -> 400 `invalid_reset`; bad `all` -> 400 `invalid_query`; a seed whose ids collide with an app bot user -> 422 `invalid_reset` (nothing changed).
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
`GET /_localslack/health` -> 200 `{"status":"ok"}`; `GET /_localslack/state/conversations` (existing). Default bot (no app) reports the fixed default scope list `chat:write`, `chat:write.public` (`service.DefaultBotScopes`, also given to apps whose manifest lists none).
