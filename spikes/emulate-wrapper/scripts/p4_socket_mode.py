"""P4: real Bolt for Python in Socket Mode against the wrapper (only base_url differs). Run via with-server.sh."""
import os
import time

import httpx
from slack_bolt import App
from slack_bolt.adapter.socket_mode import SocketModeHandler
from slack_sdk import WebClient

URL = os.environ["LOCALSLACK_URL"]
web = WebClient(token="xoxb-test", base_url=f"{URL}/api/")
bot = web.auth_test()["user_id"]
alice = next(u["id"] for u in web.users_list()["members"] if u["name"] == "alice")
alice_api = WebClient(token=f"xoxp-{alice}", base_url=f"{URL}/api/")
general = next(c["id"] for c in web.conversations_list()["channels"] if c["name"] == "general")

app = App(client=web, signing_secret="unused-in-socket-mode")
got: list[dict] = []


@app.event("message")
def on_message(event, say):
    got.append(event)
    if event.get("user") != bot:
        say(channel=event["channel"], text=f"sock-pong: {event['text']}")


handler = SocketModeHandler(app, "xapp-1-test")  # web client = app.client => base_url is the wrapper
handler.connect()


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


sock = lambda: httpx.get(f"{URL}/_localslack/socket").json()  # noqa: E731
check("Bolt connected over ws", wait_for(lambda: sock()["connections"] == 1) is not None, str(sock()))
alice_api.chat_postMessage(channel=general, text="hello socket")
reply = wait_for(lambda: next((m for m in web.conversations_history(channel=general)["messages"]
                               if m["text"] == "sock-pong: hello socket"), None))
check("message event -> Bolt handler -> reply in history", reply is not None, str(reply))
check("client acks recorded, none pending", wait_for(lambda: sock()["acks"] >= 2 and not sock()["pending"]) is not None, str(sock()))

# Direct client-side check of apps.connections.open token typing.
bad = httpx.post(f"{URL}/api/apps.connections.open", headers={"Authorization": "Bearer xoxb-test"}).json()
check("apps.connections.open rejects non-xapp token", bad == {"ok": False, "error": "not_allowed_token_type"}, str(bad))
handler.close()
print("ALL P4 CHECKS PASSED")
