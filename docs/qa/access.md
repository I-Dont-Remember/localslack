# QA: Access edge cases

Strict, scope-gated membership. One rule set (`internal/service/access.go`) covers history,
replies, info, members, post, update, delete, react and ephemeral. Membership comes from creating a
channel, registration-time join of existing public channels (apps), seeded data, the web UI actor
joining public channels when it posts or reacts, and the real methods `conversations.join`,
`.leave`, `.invite`, `.kick`. **A channel created after startup does not auto-add your app's bot** (as in
Slack): join or invite it, or give it `chat:write.public` to post without joining.

```python
client.conversations_join(channel=channel_id)                       # public channels
human.conversations_invite(channel=channel_id, users=bot_user_id)   # private channels (a member invites)
```
`conversations.join` on a channel you are in succeeds with `warning: already_in_channel`; on a private
channel or DM you are not in it is `channel_not_found`; on an archived one `is_archived`. `.invite` fails the
whole call (and changes nothing) if any user is refused unless `force=true`; failures list `errors:[{user,ok,error}]`.
`.leave`/`.kick` refuse `#general`. No `member_joined_channel`/`member_left_channel` events are emitted (deviation
`membership-events`).

| Actor / channel | read (history) | post | info / members |
|---|---|---|---|
| member, any channel | ok | ok | ok |
| bot not in public channel | `not_in_channel` | ok if app has `chat:write.public` (default scopes do) else `not_in_channel` | ok |
| user token (`xoxp-`) not in public channel | ok | `not_in_channel` | ok |
| non-member, private channel or DM | `channel_not_found` | `channel_not_found` | `channel_not_found` |
| archived channel | ok | `is_archived` (also update, react, ephemeral) | ok |

Reactions need membership for everyone (`not_in_channel`; `channel_not_found` on private).

## Setup

```bash
make run                     # server on :3232
export L=http://localhost:3232/api T=xoxb-test
HU=$(curl -s -H "Authorization: Bearer $T" $L/users.list | jq -r '[.members[]|select(.is_bot|not)][0].id')
H="xoxp-$HU"                 # the same human the web UI acts as by default
```

## Steps

1. Private channel by the human; shape and listing:
   ```bash
   PRIV=$(curl -s -H "Authorization: Bearer $H" -d name=qa-priv -d is_private=true $L/conversations.create | tee /dev/stderr | jq -r .channel.id)
   # expect is_private:true, is_group:true, is_channel:false, is_member:true
   curl -s -H "Authorization: Bearer $H" -d types=private_channel $L/conversations.list | jq '[.channels[].name]'   # includes qa-priv
   curl -s -H "Authorization: Bearer $T" -d types=private_channel $L/conversations.list | jq '[.channels[].name]'   # bot: no qa-priv
   ```
2. Bot is not a member of the private channel: every call answers `channel_not_found`:
   ```bash
   for m in conversations.history conversations.info conversations.members; do
     curl -s -H "Authorization: Bearer $T" -d channel=$PRIV $L/$m | jq -c '[.ok,.error]'
   done
   curl -s -H "Authorization: Bearer $T" -d channel=$PRIV -d text=hi $L/chat.postMessage | jq -c '[.ok,.error]'
   ```
3. Public channel the bot has not joined:
   ```bash
   PUB=$(curl -s -H "Authorization: Bearer $H" -d name=qa-pub $L/conversations.create | jq -r .channel.id)
   curl -s -H "Authorization: Bearer $T" -d channel=$PUB $L/conversations.history | jq -c '[.ok,.error]'            # not_in_channel
   curl -s -H "Authorization: Bearer $T" -d channel=$PUB -d text=hi $L/chat.postMessage | jq -c '[.ok,.error]'      # ok (chat:write.public)
   curl -s -H "Authorization: Bearer $T" -d channel=$PUB $L/conversations.info | jq .channel.is_member             # false: posting did not join
   ```
4. A user token that is not in a public channel reads but cannot post:
   ```bash
   BOTPUB=$(curl -s -H "Authorization: Bearer $T" -d name=qa-botpub $L/conversations.create | jq -r .channel.id)
   curl -s -H "Authorization: Bearer $H" -d channel=$BOTPUB $L/conversations.history | jq .ok                       # true
   curl -s -H "Authorization: Bearer $H" -d channel=$BOTPUB -d text=hi $L/chat.postMessage | jq -c '[.ok,.error]'   # not_in_channel
   ```
5. App without `chat:write.public` (scopes gate the rule):
   ```bash
   curl -s -X POST localhost:3232/_localslack/apps -H 'Content-Type: application/json' -d '{
     "display_information":{"name":"qa-strict"},"features":{"bot_user":{"display_name":"qa-strict"}},
     "oauth_config":{"scopes":{"bot":["chat:write"]}}}' | jq '{id,bot_token}'
   # with that bot_token: post to $PUB -> not_in_channel; create a channel with it, then post -> ok
   curl -s -X DELETE localhost:3232/_localslack/apps/qa-strict      # clean up (id as returned)
   ```
6. Archived:
   ```bash
   ARCH=$(curl -s -H "Authorization: Bearer $T" -d name=qa-arch $L/conversations.create | jq -r .channel.id)
   curl -s -H "Authorization: Bearer $T" -d channel=$ARCH $L/conversations.archive | jq .ok
   curl -s -H "Authorization: Bearer $T" -d channel=$ARCH -d text=hi $L/chat.postMessage | jq -c '[.ok,.error]'   # is_archived
   ```
7. Ephemeral to a non-member: `chat.postEphemeral` with `user=$HU` in `$BOTPUB` -> `user_not_in_channel`.
8. Web UI (http://localhost:3232/ui, acting as the first human):
   - The channel `qa-priv` shows in the sidebar with a lock icon in place of `#`; `qa-botpub` shows `#`.
   - Open `qa-botpub` (the human is not a member), type a message, Send: it posts (UI auto-joins). Then `conversations.members` for it lists both the bot and the human.
   - Create a private channel with the bot token (`-d is_private=true`): it must NOT appear in the sidebar, and `/ui/c/<id>` is 404.
   - Switch the "acting as" selector to the bot and open a public channel it has not joined: expect a 403 page (not_in_channel), since the UI follows the same rules.

## Expected

Codes as in the table and comments above; all failures are HTTP 200 `ok:false`. Reset
(`localslack reset`) re-seeds channels with everyone, including registered app bots, as members.

## Feedback prompts

- Is bot history on an unjoined public channel being `not_in_channel` (while posting works via `chat:write.public`) the right default for your test apps?
- Should the UI "acting as bot" be exempt from read membership so you can browse any public channel?
- Any Slack behavior you have seen differ (e.g. `chat.delete` in an archived channel, `reactions.add` for a non-member bot)?
