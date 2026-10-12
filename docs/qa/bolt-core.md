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

<!-- stream B2 (write methods and reactions) owns everything between these markers -->
<!-- B2:start -->
## Write methods and reactions

Covers `chat.update`, `chat.delete`, `chat.postEphemeral`, `reactions.add` and the UI react button. Status: implemented (stream B2).

### B2 setup

```bash
make run                      # LocalSlack on http://127.0.0.1:3232
export L=http://127.0.0.1:3232
export BOT=xoxb-test          # acts as the bot
HUMAN=$(curl -s -H "Authorization: Bearer $BOT" $L/api/users.list | jq -r '[.members[]|select(.is_bot|not)][0].id')
export HUMAN_TOK=xoxp-$HUMAN  # acts as a human
CH=$(curl -s -H "Authorization: Bearer $BOT" -d limit=1000 $L/api/conversations.list | jq -r '.channels[]|select(.name=="general").id')
```

Use `#general` (everyone is a member): a channel the bot just created has only the bot as a member, so the human token and `chat.postEphemeral` to `$HUMAN` would hit membership errors.

Open `http://127.0.0.1:3232/ui/c/$CH` in a browser (the UI refreshes every 2s).

### B2 steps

1. **Update.** Post, then edit as the same bot:
   ```bash
   TS=$(curl -s -H "Authorization: Bearer $BOT" -d channel=$CH -d text=v1 $L/api/chat.postMessage | jq -r .ts)
   curl -s -H "Authorization: Bearer $BOT" -d channel=$CH -d ts=$TS -d text=v2 $L/api/chat.update | jq
   curl -s -H "Authorization: Bearer $BOT" -d channel=$CH $L/api/conversations.history | jq '.messages[0]'
   ```
   SDK equivalent: `client.chat_update(channel=ch, ts=ts, text="v2")`. Also try `blocks='[{"type":"divider"}]'` alone (text is kept).
2. **Update errors.** Same call with `-H "Authorization: Bearer $HUMAN_TOK"` (not the author), a made-up `ts=1.000001`, a made-up channel, and no `text`/`blocks`.
3. **Delete a reply.** Post a root and two thread replies (`thread_ts=$ROOT`), `chat.delete` the second reply, read `conversations.history`.
4. **Delete a root with replies.** `chat.delete` the root, then `conversations.history` and `conversations.replies`. Delete the remaining replies and read history again.
5. **Delete errors.** As `$HUMAN_TOK` on a bot message; on an unknown ts; the same ts twice.
6. **Ephemeral.** `curl -s -H "Authorization: Bearer $BOT" -d channel=$CH -d user=$HUMAN -d text=psst $L/api/chat.postEphemeral | jq`; then `-d user=UNOTAMEMBER`. Check history and the UI.
7. **Reactions via API.** `curl -s -H "Authorization: Bearer $BOT" -d channel=$CH -d timestamp=$TS -d name=eyes $L/api/reactions.add | jq`; repeat it; add `eyes` as `$HUMAN_TOK`; try `-d name=:eyes:`; read `conversations.history` and `conversations.replies`.
8. **Reactions in the UI.** In the browser, click the smiley next to a message, pick an emoji. Then click an existing reaction chip (adds your reaction; repeating is a no-op). Switch "Acting as" and react again to see the count grow. Open a thread and react to a reply.
9. **Events.** With a registered app subscribed to `message_changed`, `message_deleted` and `reaction_added` (see events-http.md), repeat steps 1, 4 and 7. `reaction_added` should carry `item_user` = the reacted-to message's author.

### B2 expected

- Step 1: `ok:true`, `channel`, `ts` (same as the original), `text:"v2"`; history shows `text:"v2"` and `edited:{user,ts}` with `edited.ts` later than `ts`.
- Step 2: `cant_update_message`, `message_not_found`, `channel_not_found`, `no_text`, all HTTP 200 with `ok:false`.
- Step 3: root shows `reply_count:1` and `latest_reply` = the first reply.
- Step 4: the root becomes `{subtype:"tombstone", text:"This message was deleted."}` and keeps `reply_count`; replies stay readable. Once the last reply is deleted the tombstone disappears. (Known deviation `tombstone-shape`: Slack's tombstone has different fields.)
- Step 5: `cant_delete_message`, `message_not_found`, `message_not_found`.
- Step 6: `{ok:true, message_ts}`; nothing in history or the UI; `user_not_in_channel` for the non-member. The message is only remembered in memory (`Service.Ephemerals()`, for the future inspector); deviation `ephemeral-not-delivered`.
- Step 7: `{ok:true}`; repeat gives `already_reacted`; bad name gives `invalid_name`; history shows `reactions:[{name:"eyes",users:[bot,human],count:2}]` in first-added order.
- Step 8: chips such as `👀 1` appear within 2s; a repeat click does not error or double count; thread replies show chips too.
- Step 9: `message_changed` (with `previous_message`), `message_deleted`, `reaction_added` deliveries.

### B2 feedback prompts

- Does the tombstone behaviour match what your app sees in real Slack, and what fields does your code read from it?
- Is "chat.update with only blocks keeps the old text" what you expect, or should text be cleared?
- Is the emoji set in the react picker (thumbs up, heart, smile, tada, eyes) enough? Should the picker stay open across the 2s refresh?
- Do you need ephemeral messages visible somewhere (UI, `/_localslack/*`), or is the in-memory ring plus the inspector enough?
- Which reaction methods should come next (`reactions.remove`, `reactions.get`, `reactions.list`)?
<!-- B2:end -->
