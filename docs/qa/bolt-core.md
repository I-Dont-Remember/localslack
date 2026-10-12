# QA: Bolt core methods

Status: not yet implemented (streams B1/B2).

conversations.info, users.info, conversations.members, api.test, chat.update/delete/postEphemeral, reactions.add. Owned by streams B1/B2; fill this in with the stream.

<!-- BEGIN stream B1: Read methods -->
## Read methods

`conversations.info`, `users.info`, `conversations.members`, `api.test` (stream B1).

### Setup

```bash
make run                      # server on :3232 (separate terminal)
export L=http://127.0.0.1:3232/api T="Authorization: Bearer xoxb-test"
```

### Steps

```bash
# 1. conversations.info (+ member count)
GEN=$(curl -s -H "$T" $L/conversations.list | jq -r '.channels[]|select(.name=="general").id')
curl -s -H "$T" "$L/conversations.info?channel=$GEN" | jq .channel
curl -s -H "$T" "$L/conversations.info?channel=$GEN&include_num_members=true" | jq .channel.num_members
curl -s -H "$T" "$L/conversations.info?channel=C00000000" | jq .

# 2. users.info (human, then the bot)
U=$(curl -s -H "$T" $L/users.list | jq -r '[.members[]|select(.is_bot==false)][0].id')
curl -s -H "$T" "$L/users.info?user=$U" | jq .user
B=$(curl -s -H "$T" $L/auth.test | jq -r .user_id)
curl -s -H "$T" "$L/users.info?user=$B" | jq '.user.profile.bot_id'
curl -s -H "$T" "$L/users.info?user=U00000000" | jq .

# 3. conversations.members, paged two at a time
C=""; while :; do
  R=$(curl -s -H "$T" "$L/conversations.members?channel=$GEN&limit=2&cursor=$C")
  echo "$R" | jq -c .members; C=$(echo "$R" | jq -r .response_metadata.next_cursor)
  [ -z "$C" ] && break
done

# 4. api.test (no token needed)
curl -s "$L/api.test?foo=bar" | jq .
curl -s "$L/api.test?error=my_error&foo=bar" | jq .
```

Or with `slack_sdk`: `client.conversations_info(channel=..., include_num_members=True)`,
`client.users_info(user=...)`, `client.conversations_members(channel=..., limit=2)` (iterate pages),
`client.api_test(error="x")` (raises `SlackApiError`).

### Expected

1. `#general` object with `is_channel:true`, `is_private:false`; `num_members` appears only with
   `include_num_members=true`; unknown id -> `{"ok":false,"error":"channel_not_found"}`.
2. Human user object (`is_bot:false`); bot has `is_bot:true` and `profile.bot_id` equal to `auth.test`'s
   `bot_id` (starts with `B`); unknown id -> `user_not_found`.
3. Every member id appears exactly once across pages, ids ascending, last page has empty `next_cursor`;
   a made-up cursor -> `invalid_cursor`.
4. `ok:true` with `args` echoing your params (never `token`); with `error=my_error`:
   `ok:false`, `error:"my_error"`, `args` still present.

### Feedback prompts

- Do the `conversations.info` / `users.info` objects have the fields your app reads? Name any missing.
- Is `include_num_members` defaulting off (like Slack) surprising for your tests?
- Should a missing `channel`/`user` param say `invalid_arguments` rather than `*_not_found`?
- Does `conversations.members` ordering (by id, not join order) matter to your app?
<!-- END stream B1: Read methods -->

## Setup

## Expected

## Feedback prompts
