# QA: Blocks, attachments, metadata

Rich message content is stored and returned unchanged. The web UI renders only `text`
(blocks and attachments are not drawn; UI rendering is Tier 3), so verify through the API.

## Setup

```bash
make run                     # server on :3232
export L=http://localhost:3232/api T=xoxb-test
CH=$(curl -s -H "Authorization: Bearer $T" -d name=qa-blocks $L/conversations.create | jq -r .channel.id)
```

## Steps

1. Blocks-only post (no `text`):
   ```bash
   curl -s -H "Authorization: Bearer $T" -d channel=$CH \
     --data-urlencode 'blocks=[{"type":"section","text":{"type":"mrkdwn","text":"hello *blocks*"}}]' \
     $L/chat.postMessage | jq .
   ```
2. JSON body with real JSON values, plus attachments and metadata:
   ```bash
   curl -s -H "Authorization: Bearer $T" -H 'Content-Type: application/json' $L/chat.postMessage -d "{
     \"channel\":\"$CH\",\"text\":\"fallback\",
     \"blocks\":[{\"type\":\"divider\"}],
     \"attachments\":[{\"color\":\"good\",\"text\":\"attached\"}],
     \"metadata\":{\"event_type\":\"task_created\",\"event_payload\":{\"id\":\"T1\"}}}" | jq .ok
   ```
3. Read back; metadata is hidden by default:
   ```bash
   curl -s -H "Authorization: Bearer $T" -d channel=$CH $L/conversations.history | jq '.messages[0]'
   curl -s -H "Authorization: Bearer $T" -d channel=$CH -d include_all_metadata=true $L/conversations.history | jq '.messages[0].metadata'
   ```
4. Thread replies carry the same fields: post with `thread_ts=<ts>`, read `conversations.replies`.
5. Errors (each is HTTP 200, `ok:false`):
   ```bash
   curl -s -H "Authorization: Bearer $T" -d channel=$CH --data-urlencode 'blocks=[{"text":"no type"}]' $L/chat.postMessage      # invalid_blocks
   curl -s -H "Authorization: Bearer $T" -d channel=$CH -d text=x --data-urlencode 'attachments={"a":1}' $L/chat.postMessage    # invalid_attachments
   curl -s -H "Authorization: Bearer $T" -d channel=$CH -d text=x --data-urlencode 'metadata={"event_type":"t"}' $L/chat.postMessage  # invalid_metadata_format
   curl -s -H "Authorization: Bearer $T" -d channel=$CH -d "text=$(head -c 41000 /dev/zero | tr '\0' a)" $L/chat.postMessage     # msg_too_long
   curl -s -H "Authorization: Bearer $T" -d channel=$CH $L/chat.postMessage                                                       # no_text
   ```
6. slack_sdk:
   ```python
   from slack_sdk import WebClient
   from slack_sdk.models.blocks import SectionBlock, DividerBlock
   from slack_sdk.models.blocks.basic_components import MarkdownTextObject
   c = WebClient(token="xoxb-test", base_url="http://localhost:3232/api/")
   c.chat_postMessage(channel=CH, blocks=[SectionBlock(text=MarkdownTextObject(text="*hi*")), DividerBlock()])
   c.conversations_history(channel=CH, include_all_metadata=True)["messages"][0]["blocks"]
   ```

## Expected

- Steps 1-2: `ok: true`; `message.blocks` echoes what you sent.
- Step 3: `blocks` and `attachments` always present; `metadata` only with `include_all_metadata=true`.
- Step 5: `invalid_blocks` (not an array of objects with a string `type`, or more than 50 blocks),
  `invalid_attachments` (not an array of objects), `invalid_metadata_format` (needs string
  `event_type` and object `event_payload`), `msg_too_long` (text over 40000 characters),
  `no_text` (no text, no non-empty blocks, no non-empty attachments).
- Only the shape is validated, not Block Kit element schemas: `{"type":"bogus"}` is accepted.
- Open `/ui` on that channel: only `text` shows (empty for blocks-only posts).

## Feedback prompts

- Does any real-app payload you use get rejected that Slack accepts (or the reverse)?
- Is it acceptable that block element schemas are not validated?
- Do you want blocks rendered in the web UI, and how faithfully?
