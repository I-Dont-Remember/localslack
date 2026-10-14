# QA: Inspector UI

`/ui/inspector` is a live view of what your app and the Slack SDK are doing to LocalSlack: API calls, event deliveries, apps, faults and delivery controls. Panels refresh every second, newest first, last 100 rows. Expanded rows stay open across refreshes; a panel pauses refreshing while you type in one of its fields.

## Setup

```bash
make run                      # server on :3232
export L=http://localhost:3232/api T=xoxb-test A=http://localhost:3232/_localslack
```

Open <http://localhost:3232/> and click **Inspector** at the top of the sidebar. (`make demo` also works: the demo apps show up under Apps.)

## Steps

### 1. Calls
1. In a terminal: `curl -s -H "Authorization: Bearer $T" -d channel=general -d text=hi $L/chat.postMessage` (use a real channel id from `conversations.list` if you prefer).
2. Click the new `chat.postMessage` row. Params and the response JSON expand; it stays open on the next refresh.
3. `curl -s -d token=$T $L/no.such.method` and confirm a red **not implemented** badge.
4. Filters: type `chat.update` in the method box (rows narrow live), pick "errors" in the result select, enter a channel id.

### 2. Faults
1. In the Faults form enter method `chat.postMessage`, error code `fatal_error`, times `1`, click **Add fault**.
2. Repeat the curl from step 1. The response is `{"ok":false,"error":"fatal_error"}`.
3. Calls shows that row with a yellow **fault f_N** badge; Faults shows seen 1 / fired 1 and a **spent** tag. Click **Delete**.
4. Also try status `429` + retry_after `2` (SDK clients should back off), and `delay_ms` alone (call is slow but succeeds).

### 3. Apps
1. Register one blocked and one ready app:
   ```bash
   mk() { curl -s -d "{\"verify\":false,\"manifest\":{\"display_information\":{\"name\":\"$1\"},\"features\":{\"bot_user\":{\"display_name\":\"$1\"}},\"settings\":{\"event_subscriptions\":{\"request_url\":\"$2\",\"bot_events\":[\"message.channels\"]}}}}" $A/apps | jq -r .id; }
   mk Remote https://example.com/slack/events; mk Local http://127.0.0.1:9/slack/events
   ```
2. Apps lists both: **Remote** is red `blocked` with the reason (host is not loopback/private), **Local** is green `ready` with its effective request URL.
3. In Remote's `deliver_to` field type `http://localhost:3000`, **Save**. It turns `ready` and the request URL becomes `http://localhost:3000/slack/events` (host replaced, manifest path kept). Saving an empty value clears the override; `not a url` shows an error.
4. Click **credentials (click to reveal)** to see tokens and the signing secret.

### 4. Deliveries and controls
1. Inject: `curl -s -d '{"app_id":"<Local id>","event":{"type":"message","text":"hello"}}' $A/events`.
2. Deliveries shows the event with its state moving `pending` / `retrying` to `failed` (nothing listens on port 9). Expand it: attempts table (status, latency, retry reason, signature mode) and payload.
3. Click the event id link: the list filters to that event. Clear the box to undo.
4. Pick `invalid` next to **Redeliver** and click it: a new delivery for the same event id appears (a duplicate, as Slack's retries are), with signature mode `invalid`.
5. Controls: click **stale** (signature mode highlights), **Pause** (label becomes Resume; new events stay `pending`), **Resume**. Set a timeout.
6. "Events dropped" should read 0; red and non-zero means events were lost to a full queue.

### 5. Reset
1. **Reset** asks for confirmation. After OK: a green summary (mode, seed, counts), the Calls list empties, deliveries and faults clear, delivery-config returns to defaults. Apps stay.
2. **Reset --all** asks again; after OK the Apps list is empty.

### 6. Socket Mode connections
With a socket app connected (`make demo-socket`), the panel lists each connection: id, app, connected time, last ack, pending envelopes. **Disconnect** (after confirmation) sends `refresh_requested`; the row disappears and the app's Bolt client reconnects with a new id within a few seconds. With nothing connected: "No Socket Mode connections."

## Expected
- Every action above is visible within about a second without reloading.
- Nothing in the page shows real Slack tokens; credentials are LocalSlack-generated.
- The chat UI itself is unchanged apart from the **Inspector** link under the title.

## Feedback prompts
- Can you tell at a glance whether your app's calls succeeded, and why not? What column or badge is missing?
- Is the blocked/ready state and its fix obvious enough, or should the page show the `localslack apps set-url` command?
- Is 100 rows / 1 s the right window? Would you want pause/freeze on the Calls list while reading?
- Do you need per-app delivery-config (signature/timeout) in the UI, or is global enough?
- Is the confirmation on Reset enough, or should `--all` require typing something?
- What would you want in the Deliveries expand view that the payload and attempts table do not give you?
