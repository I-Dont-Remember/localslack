# QA: Scenarios, snapshots, diff

Covers fixtures (`start --scenario`, `localslack reset --scenario`, `POST /_localslack/reset` with `scenario`/`generate`), `GET /_localslack/state`, snapshots, restore, diff and the inspector's Snapshots panel. Contract: [TESTING_API.md](../TESTING_API.md) (State, snapshots, diff; Reset). About 15 minutes. Examples live in `examples/scenarios/` (`acme.json` pinned ids, `standup.yaml` hand-written without ids, `scale.json` generator recipe).

## Setup

```bash
make run                      # or: make dev. Server on :3232
export LS=http://127.0.0.1:3232
export TOK=xoxb-test          # any xoxb- token is the default bot
api() { curl -s -H "Authorization: Bearer $TOK" "$@"; }
```

## Steps

### 1. Start from a fixture

```bash
rm -f tmp/qa-scenario.db
LOCALSLACK_DB_PATH=tmp/qa-scenario.db go run ./cmd/localslack start --addr 127.0.0.1:3299 --scenario examples/scenarios/acme.json
```

In a second shell: `curl -s http://127.0.0.1:3299/api/users.list -H "Authorization: Bearer x" -X POST | jq '[.members[].id]'`.

Expected: user ids exactly `U0ANN000001`, `U0BOB000001`, ... (the ids in the file). Stop it and start again with `--scenario examples/scenarios/standup.yaml` on the same db: the log warns `scenario ignored: database already has data` and shows the fix `localslack reset --scenario ...`; data is unchanged.

### 2. Reset to a fixture (exact ids)

```bash
go run ./cmd/localslack reset --scenario examples/scenarios/acme.json
api -X POST $LS/api/auth.test | jq '{user_id, bot_id}'
api -X POST -d channel=C0GENERAL01 $LS/api/conversations.history | jq '.messages[] | {ts, text, reply_count, reactions: [.reactions[]?.name]}'
api -X POST -d channel=C0GENERAL01 -d ts=1767312000.000100 $LS/api/conversations.replies | jq '.messages | length'
```

Expected: the reset prints counts (6 users, 8 channels incl. DMs, 15 messages). `auth.test` says `U0ACMEBOT01` / `B0ACMEBOT01` (the file's only bot is the default bot). History shows the `wave` reaction on the welcome message with `reply_count: 3`, a bot message with `blocks`, and `dave`'s message with `edited`. Replies returns 4.

### 3. Same through the API, YAML without ids, validation errors

```bash
curl -s -X POST $LS/_localslack/reset -d "{\"scenario\":\"$PWD/examples/scenarios/standup.yaml\"}" | jq .counts
curl -s $LS/_localslack/state | jq '{users: [.users[] | {id, name}], channels: [.channels[].name]}'
curl -s -X POST $LS/_localslack/reset -d '{"scenario":{"users":[{"id":"X1","name":"Ann"}],"messages":[{"channel":"C9","ts":"1","user":"U9","reply_count":2}]}}' | jq .
```

Expected: counts `3/2/3`; ids in the state are server-drawn (stable for the same seed) and the file's names resolve everywhere. The bad one answers `422 invalid_scenario` with `details` listing every problem with JSON pointers (`/scenario/users/0/id`, `/scenario/messages/0/reply_count`, unknown channel and user ...). Nothing changed: `state` is still the standup workspace.

### 4. Generate at scale

```bash
curl -s -X POST $LS/_localslack/reset -d "$(jq -c '{generate: .generate}' examples/scenarios/scale.json)" | jq '{counts, took_ms}'
api -X POST $LS/api/users.list | jq '.members | length'
```

Expected: 401 users (400 + the bot), 50 channels (5 private), 2000 messages; `took_ms` well under 2000 on a normal laptop. Repeat with the same body: identical ids (compare `curl -s $LS/_localslack/state | md5sum`). Note: `users.list` returns all 401 in one page (no pagination yet).

### 5. Snapshot, poke, diff, restore

```bash
curl -s -X POST $LS/_localslack/reset -d '{"seed":"qa-snap"}' >/dev/null
curl -s -X POST $LS/_localslack/snapshots -d '{"name":"before"}' | jq .
general=$(api $LS/api/conversations.list | jq -r '.channels[] | select(.name=="general") | .id')
ts=$(api -d "channel=$general" -d "text=hello snapshot" $LS/api/chat.postMessage | jq -r .ts)
api -d "channel=$general" -d "timestamp=$ts" -d name=eyes $LS/api/reactions.add | jq .ok
api -d "channel=$general" -d "ts=$ts" -d "text=hello snapshot v2" $LS/api/chat.update | jq .ok
api -d name=qa-room $LS/api/conversations.create | jq .channel.id
curl -s "$LS/_localslack/diff?from=before&to=current" | jq '{messages: .messages.added, reactions: .reactions.added, conversations: .conversations.added, changed: .messages.changed}'
```

Expected: the snapshot reply has `counts`. The diff lists exactly one added message (text `hello snapshot v2`, an `edited` marker), one added `eyes` reaction, one added conversation `qa-room` (plus its members under `members.added`); users untouched. Then:

```bash
curl -s -X POST $LS/_localslack/snapshots/before/restore | jq .
curl -s "$LS/_localslack/diff?from=before" | jq 'map_values(.added + .removed + .changed | length)'
curl -s "$LS/_localslack/calls?method=chat.postMessage" | jq '.items | length'
curl -s $LS/_localslack/snapshots | jq '.items[].name'
```

Expected: restore replies `{name, counts, took_ms}`; the diff is all zeros; the call log still has the earlier `chat.postMessage` (restore keeps logs); the snapshot is still listed. A restore never delivers events to registered apps.

### 6. Snapshots are scenarios; limits

```bash
curl -s $LS/_localslack/snapshots/before > tmp/before.json
go run ./cmd/localslack reset --scenario tmp/before.json     # snapshot file reused as a scenario (the reset clears snapshots)
curl -s $LS/_localslack/snapshots | jq '.items | length'
curl -s -X POST $LS/_localslack/snapshots -d '{"name":"bad name"}' | jq .error.code
curl -s -X POST $LS/_localslack/snapshots -d '{"name":"current"}' | jq .error.code
curl -s $LS/_localslack/snapshots/nope | jq .error.code
```

Expected: the reset prints counts; snapshots are now `0`; the errors are `invalid_snapshot_name`, `invalid_snapshot_name`, `not_found`. A duplicate name is `409 snapshot_exists`.

### 7. Apps survive and rejoin

```bash
go run ./cmd/localslack apps add examples/demo-bolt/manifest.http.json --deliver-to http://127.0.0.1:1   # any manifest works
go run ./cmd/localslack reset --scenario examples/scenarios/acme.json
curl -s $LS/_localslack/apps | jq '.items[] | {id, bot_user_id}'
curl -s $LS/_localslack/state | jq '[.channels[] | select(.name=="general") | .members | length]'
```

Expected: the app and its credentials are unchanged after the reset; its bot user is a member of every public channel (it is not listed in `state` users, and a fixture that tries to define the bot's id is rejected with a collision error).

### 8. Inspector

Open `http://127.0.0.1:3232/ui/inspector`, find **Snapshots and diff**. Save a snapshot named `ui`, post a message in the chat UI, enter `ui` under Diff and press Diff (summary shows `messages: +1`), then press Restore on the row and Diff again (all zeros).

## Expected (summary)

Reset/start with a fixture gives exactly the ids in the file; generated workspaces are deterministic and fast; snapshots/restore/diff round-trip SDK activity; failures name every problem with a JSON pointer and change nothing.

## Feedback prompts

- Is the fixture format pleasant to hand-write? Which fields did you miss (emails, topics, custom profile fields, pinned messages)?
- Do the validation messages tell you what to change? Which one confused you?
- Does diff output answer "what did my app do?" Would you want a per-field text view, or filtering by channel?
- Should restore also reset the call log/deliveries, or is keeping them right for debugging?
- Snapshots are lost on restart: is that acceptable, or do you need them on disk?
- Is `users.list` returning 401 users in one page a problem for your app?
