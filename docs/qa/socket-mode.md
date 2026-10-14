# QA: Socket Mode

Events over a WebSocket instead of an HTTP request URL: `apps.connections.open`, `hello`, `events_api` envelopes, acks, retries, disconnects. Contract: `docs/TESTING_API.md` (Socket Mode). Everything below uses the same demo apps as the HTTP page; `curl` hits `http://127.0.0.1:3232`.

## Setup

1. `make demo-socket` (needs `uv`; Ctrl-C stops everything). Same two Bolt apps (Demo Bolt, Echo Bolt), but they run `SocketModeHandler` and connect out to LocalSlack. The script waits for both sockets and prints them (`socket <app id> connected <connection id>`); the app table shows no "events go to" URL.
   - Want fast retries for step 5? Start with `LOCALSLACK_EVENT_RETRY_DELAYS=0s,2s,5s LOCALSLACK_EVENT_TIMEOUT=2s make demo-socket`.
2. Open the printed UI URL, pick a human, open `#general`.
3. In a second terminal:
   ```sh
   L=http://127.0.0.1:3232/_localslack
   curl -s $L/socket/connections | jq
   ```
4. Scripted version without a browser: `make demo-check-socket` (prints `demo-check OK`).

## Steps

### 1. Mention, greeting, reaction
Post `@demo-bolt ping`, `hello everyone`, `echo it works`. Same behavior as `make demo` (pong in a thread, greeting, echo).

### 2. Look at the connections and the trace
```sh
curl -s $L/socket/connections | jq '.items[] | {id,app_id,connected_at,last_ack_at,pending}'
curl -s "$L/deliveries?min=1&timeout_ms=3000" | jq '.items[] | {event_type,app_id,transport,state,attempts:[.attempts[]|{n,status,latency_ms,error}]}'
```

### 3. Forced reconnect (what Slack does every few hours)
```sh
curl -i -X POST $L/socket/disconnect -H 'content-type: application/json' -d '{"reason":"refresh_requested"}'
curl -s $L/socket/connections | jq '.items[] | {id,app_id}'   # repeat until both apps are back
```
Expect `204`; the connection ids change (the apps opened new sockets through `apps.connections.open`). Mention the bot again: it still answers.
Other reasons: `warning` (Slack's "about to disconnect" notice), `link_disabled`. Target one connection with `{"connection_id":"<id>","reason":"warning"}`.

### 4. Dropped network
```sh
curl -s -X POST $L/socket/disconnect -H 'content-type: application/json' -d '{"drop":true}'
```
No `disconnect` message is sent; the socket just closes. Bolt notices and reconnects on its own (may take several seconds).

### 5. No connection, then recovery
Right after step 4 (before the apps are back), mention the bot.
```sh
curl -s "$L/deliveries?event_type=app_mention&min=1&timeout_ms=5000" | jq '.items[0] | {state, attempts: [.attempts[]|{n,error,retry_reason}]}'
```
Expect the first attempt `error: "no_socket_connection"`, `state: "retrying"`; once the app is back a retry is sent (`retry_attempt` > 0 in the envelope, `retry_reason` on the attempt after the first) and the reply appears. With the default schedule (`0s,1m,5m`) that can take up to a minute; use the fast settings from Setup to watch it quickly.

### 6. No ack
Hard to do by hand with Bolt (it always acks). With the contract instead: `curl -s -X PATCH $L/delivery-config -H 'content-type: application/json' -d '{"timeout_ms":1}'` then mention the bot. The ack cannot arrive in 1 ms, so attempts fail with `error: "timeout"` and are retried; the bot may answer more than once (a duplicate). Restore with `curl -s -X PUT $L/delivery-config -d '{}'`.

### 7. Reset and removed apps
`curl -s -X POST $L/reset -d '{}'`: connections stay (`socket/connections` unchanged), the bots keep answering. `curl -s -X POST "$L/reset?all=true" -d '{}'` removes the apps, and their sockets close.

## Expected
- Mentions and messages arrive over the socket; the trace shows `transport: "socket"` and an attempt `status: 200` per ack.
- `disconnect` with a reason returns 204, the app reconnects and later events still arrive.
- With no connection every attempt fails with `no_socket_connection` and follows the normal retry schedule.
- Events to one app arrive in the order they were posted.
- Two connections of one app share events round-robin (run two copies of the same app to see it; the Go and smoke tests cover this).

## Feedback prompts
- Does the `hello`/`events_api`/`disconnect` flow match what your Socket Mode client expects? Which real-Slack behavior do you rely on that is missing (slash commands and interactivity envelopes are not sent)?
- Is `retry_reason: "http_timeout"` on socket retries confusing? Real Slack's socket values are unverified here.
- Is a `no_socket_connection` retry schedule shared with HTTP what you want, or would you prefer to hold events until a client connects?
- Which control is missing from `socket/disconnect` for your tests (delay before closing, close code)?
