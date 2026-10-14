# QA: Events over HTTP

Apps, signed event delivery, retries, redeliver and delivery controls. Contract: `docs/TESTING_API.md` (Apps, Events). Run it with `make demo`; every `curl` below hits the testing API on `http://127.0.0.1:3232`.

## Setup

1. `make demo` (needs `uv`; Ctrl-C stops everything). It builds, starts LocalSlack with seed `demo`, registers two Bolt apps (Demo Bolt on :3000, Echo Bolt on :3001) and prints the UI URL and the app table. Both should show `delivery ready`, `verified true`.
2. Open the printed UI URL, pick any human in the "acting as" selector and open `#general`.
3. Handy in a second terminal:
   ```sh
   L=http://127.0.0.1:3232/_localslack
   curl -s $L/apps | jq '.items[] | {id,name,bot_user_id,delivery,verified}'
   ```
4. Scripted version of step 1-3 without a browser: `make demo-check` (prints `demo-check OK`).

## Steps

### 1. Mention, greeting, reaction (UI)
- Post `@demo-bolt ping` (type the bot's handle exactly; it renders as `<@U...>`).
- Post `hello everyone`.
- React to any message with an emoji (UI react button, if present; otherwise `curl` `reactions.add` as a user token).
- Post `echo it works`.

### 2. See the deliveries
```sh
curl -s "$L/deliveries?min=1&timeout_ms=3000" | jq '.items[] | {id,event_type,app_id,state,attempts:[.attempts[]|{n,status,latency_ms,signature_mode}]}'
```

### 3. Two apps, one workspace
Mention the demo bot again, then look at `#general`: Demo Bolt answers `pong` in a thread, Echo Bolt adds a `:robot_face:` reaction to that pong, and Demo Bolt then posts `saw :robot_face:`.
```sh
curl -s "$L/deliveries?event_type=reaction_added" | jq '.items[] | {app_id,event_id}'
```
Each app has its own `event_id` (like real Slack, one per app delivery), `api_app_id` and `authorizations[0].user_id` in `payload`.

### 4. Bad signature mode
```sh
DEMO=$(curl -s $L/apps | jq -r '.items[] | select(.name=="Demo Bolt") | .id')
curl -s -X PATCH $L/apps/$DEMO/delivery-config -H 'content-type: application/json' -d '{"signature":"invalid"}'   # effective config + sparse "override"
```
Mention the demo bot in the UI. No reply arrives. Then:
```sh
curl -s "$L/deliveries?app_id=$DEMO&state=failed&min=1&timeout_ms=10000" | jq '.items[0].attempts[] | {n,status,retry_reason,error}'
```
Restore: `curl -X DELETE $L/apps/$DEMO/delivery-config`. (`PUT` replaces the app's whole override, `PATCH` adds to it; fields not overridden follow the global config live.)

### 5. Redeliver a duplicate
```sh
EV=$(curl -s "$L/deliveries?event_type=app_mention&state=delivered" | jq -r '.items[0].event_id')
curl -s -X POST $L/events/$EV/redeliver -H 'content-type: application/json' -d '{"retry_num":1,"retry_reason":"http_timeout"}'
```
Event ids are per app delivery, so `$EV` alone is enough. You can also redeliver by delivery id: `curl -s -X POST $L/deliveries/<delivery id>/redeliver -d '{}'`.
Demo Bolt replies `pong` a second time (it does not de-duplicate).

### 6. Pause and release
```sh
curl -s -X PATCH $L/delivery-config -H 'content-type: application/json' -d '{"paused":true}'
```
Mention the bot: nothing happens, the delivery sits in `pending`. Then `PATCH` `{"paused":false}`: the reply appears. (`PUT` would replace the whole document and reset the fields you omit to their defaults.)

### 7. Inject an event
```sh
curl -s -X POST $L/events -H 'content-type: application/json' \
  -d '{"event":{"type":"reaction_added","user":"U1","reaction":"tada","item":{"type":"message","channel":"C1","ts":"1.0"}},"signature":"stale"}'
```
`stale` signs correctly but backdates the timestamp; Bolt rejects it (401).

### 8. Where events go
```sh
localslack apps list --addr 127.0.0.1:3232
localslack apps set-url "Demo Bolt" http://127.0.0.1:9 --addr 127.0.0.1:3232   # nothing listens there
```
Mention the bot: the delivery fails with `connection_failed`. Fix it with the port `make demo` printed for Demo Bolt (default `http://127.0.0.1:3000`). To see the safety rule, register a manifest whose `request_url` is `https://example.com/slack/events`: the app shows `delivery: blocked`, with a `localslack apps set-url ...` fix in `warnings`; nothing is ever sent.

### 9. Credentials for your own app
`localslack apps env "Demo Bolt" --addr 127.0.0.1:3232` prints `export SLACK_BOT_TOKEN=...` lines for the app under test.

## Expected
- Step 1: `pong` in a thread under your mention; `Hello <@you>! :wave:` after `hello`; `saw :emoji:` after a reaction; `it works` after `echo it works`.
- Step 3: the pong / robot_face / saw chain, and no infinite loop.
- Step 4: 4 attempts (initial + 3 retries), status 401, `signature_mode: invalid`, `retry_reason: http_error` on attempts 2-4; state `failed`.
- Step 5: second delivery with the same `event_id`; Bolt saw `X-Slack-Retry-Num: 1`.
- Step 6: `pending` while paused, delivered after unpausing, in order.
- Step 7: delivery `failed` with status 401 and `injected: true`.
- Step 8: `connection_failed` retries on a dead URL; a blocked app never produces a request.

## Feedback prompts
- Did the delivery trace answer "why did my app not get this event?" without reading server logs? What was missing from an attempt record?
- Are the defaults right: events to loopback only, bot auto-joined to every public channel that exists at registration (later channels need `conversations.join`/`invite`), permissive url_verification?
- Is the delivery-config shape (`signature`, `stale_seconds`, `timeout_ms`, `retry_delays_ms`, `paused`) enough, or would you want per-event-type controls?
- Which Slack event type would you reach for next that we do not emit (see the `event-subset` deviation)?
- Was `localslack apps env` enough to point your own app at LocalSlack, or do you need more variables?
