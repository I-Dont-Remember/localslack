# QA: Call log and reset

Covers `GET/DELETE /_localslack/calls`, `POST /_localslack/reset` and `localslack reset [--all] [--seed s]`. Contract: [TESTING_API.md](../TESTING_API.md) (Call log, Reset). About 10 minutes.

## Setup

```bash
make run                      # or: make dev. Server on :3232
export LS=http://127.0.0.1:3232
export TOK=xoxb-test          # any xoxb- token is the default bot
general=$(curl -s -H "Authorization: Bearer $TOK" $LS/api/conversations.list | jq -r '.channels[] | select(.name=="general") | .id')
curl -s -X DELETE $LS/_localslack/calls     # start from an empty log
```

## Steps

### 1. Calls are recorded

```bash
curl -s -H "Authorization: Bearer $TOK" -d "channel=$general" -d "text=hello qa" $LS/api/chat.postMessage | jq .ok
curl -s $LS/_localslack/calls | jq '{next_since, generation, n: (.items|length), first: .items[0] | {seq, method, ok, params, actor, duration_ms}}'
```

Expected: `ok: true`; the log holds one `chat.postMessage` with `params.channel`/`params.text`, `actor.token_kind: "bot"` and a `user_id`, `response` with the message `ts`. The `Authorization` header is never stored.

### 2. Failures, unknown methods, redaction

```bash
curl -s -H "Authorization: Bearer $TOK" -d channel=CNOSUCH -d text=x $LS/api/chat.postMessage >/dev/null
curl -s -H "Authorization: Bearer $TOK" -d x=1 $LS/api/totally.unknown >/dev/null
curl -s -d "token=xoxb-this-must-not-be-stored" $LS/api/auth.test >/dev/null
curl -s $LS/_localslack/calls | jq -c '.items[] | {seq, method, ok, error, not_implemented, token: .params.token}'
```

Expected: `channel_not_found` with `ok:false`; `totally.unknown` has `not_implemented:true` and still shows the bot actor; `auth.test` shows `token:"[redacted]"`. `grep -c xoxb-this-must-not` over `curl -s $LS/_localslack/calls` prints `0`.

### 3. Filters and paging

```bash
curl -s "$LS/_localslack/calls?method=chat.postMessage" | jq '.items|length'          # 2
curl -s "$LS/_localslack/calls?channel=$general" | jq '.items|length'                 # 1
curl -s "$LS/_localslack/calls?ok=false" | jq -c '[.items[].error]'
curl -s "$LS/_localslack/calls?error=channel_not_found" | jq '.items|length'
curl -s "$LS/_localslack/calls?limit=1" | jq '{next_since, n: (.items|length)}'
curl -s "$LS/_localslack/calls?since=2" | jq '[.items[].seq]'                          # seqs > 2 only
curl -s "$LS/_localslack/calls?ok=maybe" | jq .                                        # 400 invalid_query
```

### 4. Long-poll

Find the newest seq, then park a request waiting for anything newer (terminal A) and make a call (terminal B):

```bash
head=$(curl -s $LS/_localslack/calls | jq .next_since)
curl -s "$LS/_localslack/calls?since=$head&min=1&timeout_ms=20000" | jq -c '.items[] | {seq, method}'   # terminal A
curl -s -H "Authorization: Bearer $TOK" $LS/api/auth.test >/dev/null                                      # terminal B
```

Expected: terminal A returns the moment B's call lands. With `timeout_ms=1000` and no traffic it returns `items: []` after about a second.

### 5. Delete keeps the sequence

```bash
curl -s $LS/_localslack/calls | jq '[.next_since, .generation]'
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE $LS/_localslack/calls     # 204
curl -s -H "Authorization: Bearer $TOK" $LS/api/auth.test >/dev/null
curl -s $LS/_localslack/calls | jq '{seq: .items[0].seq, generation}'
```

Expected: `seq` is larger than the old `next_since` (never restarts at 1); `generation` went up by one.

### 6. Reset (partial)

```bash
curl -s -H "Authorization: Bearer $TOK" -d name=qa-scratch $LS/api/conversations.create | jq .ok
curl -s -X POST "$LS/_localslack/reset" -d '{"seed":"qa-1"}' -H 'Content-Type: application/json' | jq .
curl -s -H "Authorization: Bearer $TOK" $LS/api/conversations.list | jq -r '[.channels[].name]|index("qa-scratch")'   # null
curl -s $LS/_localslack/calls | jq '.items|length'
```

Expected: reply `{"mode":"partial","generation":N,"seed":"qa-1","counts":{...},"took_ms":..}`; `qa-scratch` is gone; the call log is empty (only the `conversations.list` call just made) and its `generation` advanced; the team id in `auth.test` is unchanged.

Reproducibility:

```bash
ids() { curl -s -H "Authorization: Bearer $TOK" $LS/api/users.list | jq -c '[.members[].id]'; }
curl -s -X POST $LS/_localslack/reset -d '{"seed":"a"}' >/dev/null; A=$(ids)
curl -s -X POST $LS/_localslack/reset -d '{"seed":"b"}' >/dev/null
curl -s -X POST $LS/_localslack/reset -d '{"seed":"a"}' >/dev/null; [ "$A" = "$(ids)" ] && echo same || echo DIFFERENT
```

Expected: `same`. A bare `POST /_localslack/reset` returns to the server's configured seed (`LOCALSLACK_SEED`), or a fresh random one, reported in `seed`.

### 7. Timestamps never go backwards

```bash
post() { curl -s -H "Authorization: Bearer $TOK" -d "channel=$general" -d text=ts $LS/api/chat.postMessage | jq -r .ts; }
curl -s -X POST $LS/_localslack/reset -d '{"seed":"t"}' >/dev/null
general=$(curl -s -H "Authorization: Bearer $TOK" $LS/api/conversations.list | jq -r '.channels[]|select(.name=="general")|.id')
before=$(post); curl -s -X POST $LS/_localslack/reset -d '{"seed":"t"}' >/dev/null; after=$(post); echo "$before < $after"
```

Expected: the second ts is greater.

### 8. CLI

```bash
go run ./cmd/localslack reset --seed qa-2          # or ./bin/localslack after `make build`
go run ./cmd/localslack reset --all
go run ./cmd/localslack reset --addr 127.0.0.1:1   # no server there
go run ./cmd/localslack reset --scenario x.json    # not available yet
```

Expected: the first two print the JSON reply (`mode` `partial` then `all`). The third exits non-zero with `reach server at ... (is it running?)`. The last exits non-zero with `reset failed: not_supported_yet: ... (HTTP 422)`.

### 9. Errors

```bash
curl -s -X POST "$LS/_localslack/reset?all=maybe" | jq .          # 400 invalid_query
curl -s -X POST $LS/_localslack/reset -d '{' | jq .               # 400 invalid_json
curl -s -X POST $LS/_localslack/reset -d '{"keep":["apps"]}' | jq . # 400 invalid_reset (no keep list)
curl -s -X POST $LS/_localslack/reset -d '{"generate":{"users":3}}' | jq .   # 422 not_supported_yet
```

All errors have the shape `{"error":{"code","message"}}` with a real HTTP status.

### 10. UI "acting as" survives a reset

Open <http://127.0.0.1:3232/ui/>, pick another user in the switcher, then run `curl -s -X POST $LS/_localslack/reset -d '{"seed":"new-people"}'` and reload the page.

Expected: the page still renders (as the first human of the new workspace); no error page for the vanished user.

### 11. Apps (needs stream A merged)

Register an app (`POST /_localslack/apps`), then `reset` and check `GET /_localslack/apps`: the app, its tokens and its bot user id are unchanged and its bot token still works with `auth.test`. `reset?all=true` (or `localslack reset --all`) leaves `GET /_localslack/apps` empty and the bot token no longer maps to the app.

## Expected (summary)

- Every `/api/*` call, including failures and unknown methods, is logged with redacted params; `/_localslack/*` and `/ui/*` are not.
- `seq` only grows (across DELETE and reset); `generation` bumps on each clear/reset and wakes parked long-polls.
- Partial reset: new seeded users/channels/messages, same team id and apps, empty call log, faults and delivery state cleared, no events emitted to apps. `--all` also drops apps.
- Two resets at once: the second gets `409 reset_in_progress`.

## Feedback prompts

- Is a bare `reset` (back to the startup seed) what you expect, or should it always randomize?
- Is `generation` clear enough for a test to detect "someone reset the server under me"?
- Should `GET /calls` have a way to ask for "just the newest seq" (today you page forward to find the head)?
- Do the redacted keys cover what your app sends (tokens in JSON bodies, `client_secret`, signing secrets)? What else should be hidden?
- Is 5s the right default wait when `min` is given without `timeout_ms`?
- Would you want `reset` to also keep a named subset (we dropped the `keep` list on purpose: apps persist, `--all` blanks)?
