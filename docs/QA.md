# QA guide: hands-on walkthrough

A guided pass over everything the test-loop work added, about 45 minutes. Do the numbered path in order; each step says what to do and what you should see. Per-feature pages (the table at the end) hold the full recipes, edge cases and contract details: follow the links when you want to go deeper. API shapes live in [TESTING_API.md](TESTING_API.md).

Please collect your answers in [Feedback I want](#feedback-i-want) as you go.

## 0. Prerequisites (5 min)

- Tools: `go`, `uv`, `make`, `curl`, `jq`, a browser. (`make setup` installs the pinned toolchain, pre-commit hooks and the Python env.)
- Free ports: 3232 (LocalSlack), 3000 and 3001 (the demo Bolt apps).

```bash
make setup
make demo-check && make demo-check-socket     # pre-flight, ~30 s each
```

Expected: both end with `demo-check OK: mention delivered, bot replied "pong" in the thread`. If either fails, stop and tell me; nothing below will work.

Shell helpers used throughout (paste once per terminal):

```bash
export LS=http://127.0.0.1:3232 L=http://127.0.0.1:3232/_localslack TOK=xoxb-test
api() { curl -s -H "Authorization: Bearer $TOK" "$@"; }
```

## 1. `make demo`: two Bolt apps over HTTP (8 min)

Terminal 1: `make demo` (Ctrl-C stops it; state is thrown away on exit).

1. It prints an app table with **Demo Bolt** and **Echo Bolt**, both `delivery ready`, `verified true`, plus the UI URL.
2. Open <http://127.0.0.1:3232/ui>, pick a human under "Acting as", open `#general`.
3. Post `@demo-bolt ping`. Expected: within a second or two Demo Bolt replies `pong` in a thread on your message; Echo Bolt adds a robot-face reaction to that pong, and Demo Bolt then posts `saw :robot_face:`.
4. Post `hello everyone`. Expected: `Hello @you! :wave:` from Demo Bolt. Post `echo it works`: Echo Bolt echoes.
5. React to a message with the smiley button. Expected: a `saw :<emoji>:` message.
6. See what LocalSlack delivered:

   ```bash
   curl -s "$L/deliveries?min=1&timeout_ms=3000" | jq '.items[] | {event_type, app: .app_id, state, status: [.attempts[].status]}'
   ```

   Expected: `app_mention`, `message`, `reaction_added` rows, `state: "delivered"` and status `200` (a row still in flight may briefly read `pending`).

Details and failure modes (bad signatures, redeliver, pause): [Events over HTTP](qa/events-http.md). Methods the apps use: [Bolt core](qa/bolt-core.md), [Blocks](qa/blocks.md).

## 2. The inspector (6 min)

Keep `make demo` running. Open <http://127.0.0.1:3232/ui/inspector> (or the **Inspector** link in the chat sidebar).

1. **Calls**: the Bolt apps' `chat.postMessage`, `auth.test` etc., newest first. Click a row's method to expand params and response.
2. **Deliveries**: the events from step 1, with attempts. Expand one, pick `invalid` next to **Redeliver**, click it. Expected: a duplicate delivery appears with signature mode `invalid`, and Demo Bolt does *not* reply (Bolt rejects the signature); the delivery goes `retrying`/`failed`.
3. **Apps**: both apps `ready`; **reveal credentials** shows LocalSlack-generated tokens.
4. **Controls**: click **Pause**, mention the bot (nothing happens; the delivery sits `pending`), click **Resume** (the reply arrives).

Full checklist: [Inspector UI](qa/inspector.md).

## 3. Fault injection (5 min)

Still on the HTTP demo.

```bash
curl -s -X POST $L/faults -d '{"method":"chat.postMessage","action":{"status":429,"retry_after":2},"times":1}' | jq -c .
general=$(api $LS/api/conversations.list | jq -r '.channels[]|select(.name=="general")|.id')
curl -si -H "Authorization: Bearer $TOK" -d channel=$general -d text=x $LS/api/chat.postMessage | head -4
curl -si -H "Authorization: Bearer $TOK" -d channel=$general -d text=x $LS/api/chat.postMessage | head -1
curl -s -X DELETE $L/faults -o /dev/null -w '%{http_code}\n'
```

Expected: the first post is `HTTP/1.1 429 Too Many Requests` with `Retry-After: 2` and body `{"error":"ratelimited","ok":false}`; the second is `200`; the final `DELETE` prints `204`. In the inspector, the faulted call has a fault badge; you can also add the same fault from the Faults form. Now add a fault `chat.postMessage` / error `fatal_error` / times `1` in the form, mention the bot, and watch Demo Bolt's reply fail while the call row carries the badge.

More: [Fault injection](qa/faults.md).

## 4. `make demo-socket`: the same apps over Socket Mode (6 min)

Ctrl-C the HTTP demo, then `make demo-socket`.

1. The script prints `socket <app id> connected <connection id>` twice; the app table has no "events go to" URL.
2. Repeat the `@demo-bolt ping` / `hello everyone` checks from section 1. Expected: identical behavior. Deliveries show `transport: "socket"`:

   ```bash
   curl -s "$L/deliveries?min=1&timeout_ms=3000" | jq '.items[] | {event_type, transport, state}'
   ```
3. Inspector, **Socket Mode connections** panel: two rows (id, app, connected, last ack, pending). Click **Disconnect** on one and confirm. Expected: a `refresh_requested` notice; the row vanishes and a new one with a different id appears within a few seconds (Bolt reconnects). Mention the bot again: it still answers.
4. From the CLI: `curl -i -X POST $L/socket/disconnect -d '{"reason":"refresh_requested"}'` returns `204`; `curl -s $L/socket/connections | jq '.items|length'` returns to `2` shortly after.

More: [Socket Mode](qa/socket-mode.md).

## 5. Reset: partial vs `--all` (4 min)

Stay on the socket demo (or HTTP; same result). In a second terminal build the CLI once with `make build`, then:

```bash
bin/localslack reset                   # partial
bin/localslack apps list
```

Expected: JSON `"mode":"partial"`. The inspector's Calls and Deliveries lists are empty, users/channels/messages are re-seeded (the chat UI shows a fresh workspace), but both apps are still listed and still connected. Mention the bot in `#general` again: `pong` still comes back.

```bash
bin/localslack reset --all
bin/localslack apps list
```

Expected: `"mode":"all"`; `apps list` prints only the header; the inspector's Apps and Socket panels are empty. Mention the bot: silence. (The inspector has both as **Reset** and **Reset --all** buttons.) Ctrl-C the demo.

More (call log paging, long-poll, seeds, `reset` errors): [Call log and reset](qa/calls-reset.md).

## 6. Your own app: loading a real manifest, fixing a blocked URL (7 min)

Real manifests carry a public `request_url`; LocalSlack refuses to deliver to non-loopback hosts unless told otherwise. Start a clean server (terminal 1) and register such a manifest (terminal 2):

```bash
# terminal 1
LOCALSLACK_DB_PATH=tmp/qa-walk.db bin/localslack start

# terminal 2
mkdir -p tmp && cat > tmp/qa-remote.json <<'EOF'
{"display_information":{"name":"Remote App"},"features":{"bot_user":{"display_name":"remote-app"}},
 "oauth_config":{"scopes":{"bot":["chat:write","app_mentions:read"]}},
 "settings":{"event_subscriptions":{"request_url":"https://my-app.example.com/slack/events","bot_events":["app_mention"]}}}
EOF
bin/localslack apps add tmp/qa-remote.json
```

Expected: `delivery=blocked` with a warning that the host `my-app.example.com` is not loopback/private, and the exact fix: `localslack apps set-url <id> http://localhost:3000`. The inspector's Apps row says `blocked` too.

```bash
bin/localslack apps set-url "Remote App" http://localhost:3000     # name or id
bin/localslack apps list                                            # ready, events go to .../slack/events
bin/localslack apps env "Remote App"                                # SLACK_API_URL, SLACK_BOT_TOKEN, SLACK_SIGNING_SECRET, SLACK_APP_TOKEN
```

Expected: `delivery=ready`, the manifest path (`/slack/events`) is kept with the host swapped, and `apps env` prints `export ...` lines you can `eval` in your own app's shell. Two other ways to the same fix: set the default for every app with `LOCALSLACK_DELIVER_TO=http://localhost:3000` (or `start --deliver-to`) when starting the server, or edit **deliver_to** on the app's inspector row. `--app-manifest file.json` on `start` registers a manifest at boot.

Point your own app at it if you have one (`eval "$(bin/localslack apps env 'Remote App')"`, then run it); otherwise just check the output. Then Ctrl-C terminal 1. Details: [Events over HTTP](qa/events-http.md), [Inspector UI](qa/inspector.md).

## 7. Scenarios, snapshots, diff (6 min)

Server from section 6 still running (restart it if you stopped it).

```bash
bin/localslack reset --scenario examples/scenarios/acme.json
api -X POST $LS/api/auth.test | jq -c '{user_id, bot_id}'
curl -s -X POST $L/snapshots -d '{"name":"before"}' | jq -c .name
api -d channel=C0GENERAL01 -d text="hello diff" $LS/api/chat.postMessage | jq -c '[.ok,.error]'
curl -s "$L/diff?from=before" | jq '.messages.added | length'
curl -s -X POST $L/snapshots/before/restore | jq -c .name
curl -s "$L/diff?from=before" | jq '.messages.added | length'
```

Expected: the reset reports 7 users, 8 channels, 15 messages and `auth.test` returns the fixture's ids `U0ACMEBOT01` / `B0ACMEBOT01`; the snapshot prints `"before"`; the post is `[true,null]`; the diff shows `1` added message, and after the restore `0`. Then in the inspector's **Snapshots and diff** panel save a snapshot `ui`, post in the chat UI, Diff `ui` (summary `messages: +1`), Restore, Diff again (all zeros).

Also try `bin/localslack reset --scenario examples/scenarios/standup.yaml` (hand-written, no ids) and the generated 400-user recipe (`examples/scenarios/scale.json`). Details: [Scenarios, snapshots, diff](qa/scenarios.md).

## 8. Access edge cases (3 min)

Fresh state: `bin/localslack reset`. Then:

```bash
HU=$(api $LS/api/users.list | jq -r '[.members[]|select(.is_bot|not)][0].id'); H="xoxp-$HU"
PRIV=$(curl -s -H "Authorization: Bearer $H" -d name=qa-priv -d is_private=true $LS/api/conversations.create | jq -r .channel.id)
api -d channel=$PRIV $LS/api/conversations.history | jq -c '[.ok,.error]'
PUB=$(curl -s -H "Authorization: Bearer $H" -d name=qa-pub $LS/api/conversations.create | jq -r .channel.id)
api -d channel=$PUB $LS/api/conversations.history | jq -c '[.ok,.error]'
api -d channel=$PUB -d text=hi $LS/api/chat.postMessage | jq -c '[.ok,.error]'
```

Expected: `[false,"channel_not_found"]` (the bot cannot see a private channel it is not in), `[false,"not_in_channel"]` (public, not joined), `[true,null]` (posting works because default scopes include `chat:write.public`). More: [Access edge cases](qa/access.md).

## 9. Header fidelity (2 min)

```bash
api -i -X POST $LS/api/auth.test | head -8
api -i -X POST $LS/api/no.such.method | head -6
```

Expected: both `HTTP/1.1 200 OK` with `X-Slack-Req-Id` (same value as `X-Request-Id`) and `X-Oauth-Scopes`; the unknown method answers `{"error":"unknown_method","ok":false}` with `200` plus `X-Localslack-Not-Implemented: 1`, and shows the not-implemented badge in the inspector. Rate-limit responses carry `Retry-After` (section 3).

Stop the server (Ctrl-C). Delete `tmp/qa-walk.db` if you like.

## Feedback I want

Write short answers under each item (or "fine"). Items are copied from each page's own prompts, so the pages stay the reference.

### Overall

- [ ] Did every step above behave as written? Where did the guide lie or confuse?
  - Answer:
- [ ] What took longest to get working? What would have made it shorter?
  - Answer:
- [ ] Would you use this for your own app's tests tomorrow? What blocks you?
  - Answer:

### Events over HTTP ([page](qa/events-http.md))

- [ ] Did the delivery trace answer "why did my app not get this event?" without server logs? What was missing from an attempt record?
  - Answer:
- [ ] Are the defaults right: events to loopback only, bot auto-joined to every public channel that exists at registration (later channels need `conversations.join`/`invite`), permissive url_verification?
  - Answer:
- [ ] Is the delivery-config shape (`signature`, `stale_seconds`, `timeout_ms`, `retry_delays_ms`, `paused`) enough, or per-event-type controls?
  - Answer:
- [ ] Which Slack event type would you reach for next that we do not emit (see the `event-subset` deviation)?
  - Answer:
- [ ] Was `localslack apps env` enough to point your own app at LocalSlack, or do you need more variables?
  - Answer:

### Bolt core, reads ([page](qa/bolt-core.md))

- [ ] Do the `conversations.info` / `users.info` objects have the fields your app reads? Name any missing.
  - Answer:
- [ ] Is `include_num_members` defaulting off (like Slack) surprising?
  - Answer:
- [ ] Should a missing `channel`/`user` param say `invalid_arguments` rather than `*_not_found`?
  - Answer:
- [ ] Does `conversations.members` ordering (by id, not join order) matter?
  - Answer:

### Bolt core, writes and reactions ([page](qa/bolt-core.md))

- [ ] Does the tombstone behaviour match what your app sees in real Slack, and what fields does your code read from it?
  - Answer:
- [ ] Is "chat.update with only blocks keeps the old text" what you expect, or should text be cleared?
  - Answer:
- [ ] Is the emoji set in the react picker enough? Should the picker stay open across the 2s refresh?
  - Answer:
- [ ] Do you need ephemeral messages visible somewhere (UI, `/_localslack/*`), or is the in-memory ring plus the inspector enough?
  - Answer:
- [ ] Which reaction methods next (`reactions.remove`, `reactions.get`, `reactions.list`)?
  - Answer:

### Blocks, attachments, metadata ([page](qa/blocks.md))

- [ ] Does any real-app payload you use get rejected that Slack accepts (or the reverse)?
  - Answer:
- [ ] Is it acceptable that block element schemas are not validated?
  - Answer:
- [ ] Do you want blocks rendered in the web UI, and how faithfully?
  - Answer:

### Call log and reset ([page](qa/calls-reset.md))

- [ ] Is a bare `reset` (back to the startup seed) what you expect, or should it always randomize?
  - Answer:
- [ ] Is `generation` clear enough for a test to detect "someone reset the server under me"?
  - Answer:
- [ ] Should `GET /calls` have a way to ask for "just the newest seq"?
  - Answer:
- [ ] Do the redacted keys cover what your app sends (tokens in JSON bodies, `client_secret`, signing secrets)? What else should be hidden?
  - Answer:
- [ ] Is 5s the right default long-poll wait when `min` is given without `timeout_ms`?
  - Answer:
- [ ] Would you want `reset` to keep a named subset (the `keep` list was dropped on purpose: apps persist, `--all` blanks)?
  - Answer:

### Fault injection ([page](qa/faults.md))

- [ ] Did `seen`/`fired` match what you expected for nth/every/times combos (esp. `nth` + `every`, shadowed rules)?
  - Answer:
- [ ] Is it useful that `spent` rules stay in the list, or should they disappear?
  - Answer:
- [ ] Does the SDK retry flow (`RateLimitErrorRetryHandler`, `ServerErrorRetryHandler`) behave like real Slack? Other failure shapes needed (`Retry-After` on 200, `X-Slack-*` headers, connection drops)?
  - Answer:
- [ ] Missing match fields (thread_ts, text substring)? Probabilistic faults wanted, or is determinism enough?
  - Answer:

### Socket Mode ([page](qa/socket-mode.md))

- [ ] Does the `hello`/`events_api`/`disconnect` flow match what your client expects? Which real-Slack behavior do you rely on that is missing (slash commands and interactivity envelopes are not sent)?
  - Answer:
- [ ] Is `retry_reason: "http_timeout"` on socket retries confusing?
  - Answer:
- [ ] Is a `no_socket_connection` retry schedule shared with HTTP what you want, or hold events until a client connects?
  - Answer:
- [ ] Which control is missing from `socket/disconnect` for your tests (delay before closing, close code)?
  - Answer:

### Access edge cases ([page](qa/access.md))

- [ ] Is bot history on an unjoined public channel being `not_in_channel` (while posting works via `chat:write.public`) the right default?
  - Answer:
  - Answer:
- [ ] Should the UI "acting as bot" be exempt from read membership so you can browse any public channel?
  - Answer:
- [ ] Any Slack behavior you have seen differ (e.g. `chat.delete` in an archived channel, `reactions.add` for a non-member bot)?
  - Answer:

### Scenarios, snapshots, diff ([page](qa/scenarios.md))

- [ ] Is the fixture format pleasant to hand-write? Which fields did you miss (emails, topics, custom profile fields, pinned messages)?
  - Answer:
- [ ] Do the validation messages tell you what to change? Which one confused you?
  - Answer:
- [ ] Does diff output answer "what did my app do?" Per-field text view, or filtering by channel?
  - Answer:
- [ ] Should restore also reset the call log/deliveries, or is keeping them right?
  - Answer:
- [ ] Snapshots are lost on restart: acceptable, or do you need them on disk?
  - Answer:
- [ ] Is `users.list` returning 401 users in one page a problem for your app?
  - Answer:

### Inspector UI ([page](qa/inspector.md))

- [ ] Can you tell at a glance whether your app's calls succeeded, and why not? What column or badge is missing?
  - Answer:
- [ ] Is the blocked/ready state and its fix obvious enough, or should the page show the `localslack apps set-url` command?
  - Answer:
- [ ] Is 100 rows / 1 s the right window? Pause/freeze on the Calls list while reading?
  - Answer:
- [ ] Per-app delivery-config (signature/timeout) in the UI, or is global enough?
  - Answer:
- [ ] Is the confirmation on Reset enough, or should `--all` require typing something?
  - Answer:
- [ ] What would you want in the Deliveries expand view beyond the payload and attempts table?
  - Answer:

## Per-feature pages

| Area | Covers |
|---|---|
| [Events over HTTP](qa/events-http.md) | Apps, signed event delivery, retries, redeliver, delivery controls |
| [Bolt core methods](qa/bolt-core.md) | conversations.info, users.info, conversations.members, api.test, chat.update/delete/postEphemeral, reactions.add |
| [Call log and reset](qa/calls-reset.md) | GET/DELETE /_localslack/calls, POST /_localslack/reset, `localslack reset` |
| [Fault injection](qa/faults.md) | Rate limits, server errors, delays and nth-call faults |
| [Blocks, attachments, metadata](qa/blocks.md) | Rich message content round-trips |
| [Socket Mode](qa/socket-mode.md) | apps.connections.open, WebSocket envelopes, acks, disconnects (`make demo-socket`) |
| [Access edge cases](qa/access.md) | Membership, private channels, archived channels |
| [Scenarios, snapshots, diff](qa/scenarios.md) | Fixtures, generated data, snapshot/restore, diff |
| [Inspector UI](qa/inspector.md) | /ui/inspector live panels |

Known gaps (not bugs): message `team` field, multipart bodies, posting by channel name, DMs by user id, interactivity and slash commands over Socket Mode. See `agent-docs/ROADMAP.md`.
