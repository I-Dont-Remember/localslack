"""P3: real Bolt for Python (HTTP receiver) against the wrapper. Run via scripts/with-server.sh."""
import os
import socket
import threading
import time

import httpx
from slack_bolt import App
from slack_sdk import WebClient

URL = os.environ["LOCALSLACK_URL"]
SECRET = "localslack-spike-secret"
api = WebClient(token="xoxb-test", base_url=f"{URL}/api/")
bot = api.auth_test()["user_id"]
alice = next(u["id"] for u in api.users_list()["members"] if u["name"] == "alice")
alice_api = WebClient(token=f"xoxp-{alice}", base_url=f"{URL}/api/")
general = next(c["id"] for c in api.conversations_list()["channels"] if c["name"] == "general")

seen: list[dict] = []
app = App(client=api, signing_secret=SECRET)  # default: ack first, run listeners in a thread pool


@app.middleware
def capture(req, next):
    b = req.body
    seen.append({
        "type": b.get("type"), "event_id": b.get("event_id"),
        "retry_num": (req.headers.get("x-slack-retry-num") or [None])[0],
        "retry_reason": (req.headers.get("x-slack-retry-reason") or [None])[0],
    })
    next()


@app.event("message")
def on_message(event, say):
    if event.get("user") == bot or event.get("bot_id"):  # emulate sets no bot_id on bot posts
        return
    say(channel=event["channel"], text=f"pong: {event['text']}")


def free_port() -> int:
    s = socket.socket(); s.bind(("127.0.0.1", 0)); p = s.getsockname()[1]; s.close(); return p


port = free_port()
threading.Thread(target=lambda: app.start(port=port, path="/slack/events"), daemon=True).start()
time.sleep(0.5)
sink = f"http://127.0.0.1:{port}/slack/events"


def wait_for(pred, timeout=5.0):
    end = time.time() + timeout
    while time.time() < end:
        if (v := pred()):
            return v
        time.sleep(0.05)
    return None


def check(name, ok, detail=""):
    print(f"{'PASS' if ok else 'FAIL'}  {name} {detail}")
    if not ok:
        raise SystemExit(1)


# 1. url_verification handshake: bad target rejected, Bolt target accepted.
r = httpx.post(f"{URL}/_localslack/subscriptions", json={"url": f"http://127.0.0.1:{free_port()}/nope"})
check("registration to a dead URL is rejected", r.status_code == 400 and r.json()["error"] == "url_verification_failed", r.text)
r = httpx.post(f"{URL}/_localslack/subscriptions", json={"url": sink, "events": ["message"]})
check("registration to Bolt succeeds after url_verification", r.status_code == 200 and r.json()["ok"], r.text)
# (Bolt answers url_verification before user middleware, so `seen` never lists it; success above proves the echo.)

# 2. user posts -> Bolt handler fires -> reply in history.
posted = alice_api.chat_postMessage(channel=general, text="hello bolt")
reply = wait_for(lambda: next((m for m in api.conversations_history(channel=general)["messages"]
                               if m["text"] == "pong: hello bolt"), None))
check("Bolt handler replied via chat.postMessage", reply is not None and reply["user"] == bot, str(reply))
deliveries = httpx.get(f"{URL}/_localslack/deliveries").json()
first = next(d for d in deliveries if d["event_type"] == "message")
check("delivery trace shows 200", first["status_code"] == 200 and first["success"], str(first))

# 3. bad signature toggle -> Bolt 401 -> delivery failure visible.
httpx.post(f"{URL}/_localslack/config", json={"bad_signature": True})
n_before = len(seen)
alice_api.chat_postMessage(channel=general, text="unsigned")
time.sleep(0.5)
bad = [d for d in httpx.get(f"{URL}/_localslack/deliveries").json() if d["status_code"] == 401]
check("bad signature: Bolt returns 401, recorded in deliveries", len(bad) >= 1 and not bad[0]["success"], str(bad[:1]))
check("bad signature: handler never ran", len(seen) == n_before and not any(
    m["text"] == "pong: unsigned" for m in api.conversations_history(channel=general)["messages"]))
httpx.post(f"{URL}/_localslack/config", json={"bad_signature": False})

# 4. retry redelivery.
event_id = first["event_id"]
r = httpx.post(f"{URL}/_localslack/events/{event_id}/retry", json={"reason": "http_timeout"})
check("retry endpoint accepted", r.status_code == 200 and r.json()["status_code"] == 200, r.text)
got = wait_for(lambda: next((s for s in seen if s["retry_num"] == "1"), None))
check("Bolt saw X-Slack-Retry-Num/Reason with the same event_id",
      got is not None and got["event_id"] == event_id and got["retry_reason"] == "http_timeout", str(got))
dupes = [m for m in api.conversations_history(channel=general)["messages"] if m["text"] == "pong: hello bolt"]
print(f"INFO  dedupe is the app's job: handler replied {len(dupes)}x to the same event_id")
print("ALL P3 CHECKS PASSED")
