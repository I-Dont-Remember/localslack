"""Socket Mode: a real Bolt SocketModeHandler gets events from LocalSlack over a WebSocket.

No sleeps: waits long-poll the delivery and call logs; connection changes poll the control API
with a bounded deadline (the client reconnects on its own thread).
"""

from __future__ import annotations

import time

import pytest
from slack_bolt import App
from slack_bolt.adapter.socket_mode import SocketModeHandler
from slack_sdk import WebClient

CONNECT_DEADLINE_S = 15.0


def wait_connections(admin, app_id: str, want: int, *, not_ids: frozenset[str] = frozenset()):
    """Poll until `want` connections of app_id exist, none of them in `not_ids`."""
    deadline = time.monotonic() + CONNECT_DEADLINE_S
    while time.monotonic() < deadline:
        mine = [c for c in admin.socket_connections() if c["app_id"] == app_id]
        if len(mine) == want and not any(c["id"] in not_ids for c in mine):
            return mine
        time.sleep(0.05)
    raise AssertionError(
        f"app {app_id}: wanted {want} connections, have {admin.socket_connections()}"
    )


@pytest.fixture
def bolt(admin, base_url, unique):
    """A socket-mode app registered with LocalSlack and connected with a real Bolt handler."""
    bot_token = "xo" + f"xb-sock-{unique}"
    app_token = "xa" + f"pp-1-sock-{unique}"
    manifest = {
        "display_information": {"name": f"Sock {unique}"},
        "features": {"bot_user": {"display_name": f"sock-{unique}"}},
        "settings": {
            "socket_mode_enabled": True,
            "event_subscriptions": {"bot_events": ["app_mention", "message.channels"]},
        },
    }
    record = admin.register_app(
        manifest, signing_secret=f"sock-secret-{unique}", bot_token=bot_token, app_token=app_token
    )
    client = WebClient(token=bot_token, base_url=f"{base_url}/api/")
    app = App(client=client, token_verification_enabled=False, signing_secret="unused")
    handler = SocketModeHandler(app, app_token, web_client=client)
    handler.connect()  # non-blocking: the client runs on its own threads
    first = wait_connections(admin, record["id"], 1)[0]
    yield type("BoltEnv", (), {"app": app, "record": record, "first": first, "handler": handler})
    handler.close()
    admin.delete_app(record["id"])


@pytest.fixture
def human(base_url):
    probe = WebClient(token="xo" + "xb-probe", base_url=f"{base_url}/api/")
    uid = next(u["id"] for u in probe.users_list()["members"] if not u["is_bot"])
    client = WebClient(token=f"xoxp-{uid}", base_url=f"{base_url}/api/")
    channels = client.conversations_list(types="public_channel")["channels"]
    general = next(c["id"] for c in channels if c["name"] == "general")
    return type("Human", (), {"client": client, "id": uid, "general": general})


def mention(human, bolt, text: str):
    return human.client.chat_postMessage(
        channel=human.general, text=f"<@{bolt.record['bot_user_id']}> {text}"
    )


def test_mention_over_socket_gets_a_threaded_reply(admin, bolt, human):
    @bolt.app.event("app_mention")
    def on_mention(event, say):
        say(text="pong", thread_ts=event["ts"])

    app_id = bolt.record["id"]
    cursor = admin.head_seq()

    sent = mention(human, bolt, "ping")

    got = admin.wait_deliveries(1, app_id=app_id, event_type="app_mention", state="delivered")
    assert got[0]["transport"] == "socket"
    assert got[0]["attempts"][0]["status"] == 200
    admin.wait_calls(1, since=cursor, method="chat.postMessage", app_id=app_id)
    thread = human.client.conversations_replies(channel=human.general, ts=sent["ts"])["messages"]
    reply = next(m for m in thread if m.get("text") == "pong")
    assert reply["user"] == bolt.record["bot_user_id"] and reply["thread_ts"] == sent["ts"]


def test_forced_refresh_disconnect_reconnects_and_next_event_is_delivered(admin, bolt, human):
    @bolt.app.event("app_mention")
    def on_mention(event, say):
        say(text="pong again", thread_ts=event["ts"])

    app_id = bolt.record["id"]
    old = bolt.first["id"]

    admin.socket_disconnect(connection_id=old, reason="refresh_requested")

    fresh = wait_connections(admin, app_id, 1, not_ids=frozenset({old}))[0]
    assert fresh["id"] != old, "the client opened a new connection after refresh_requested"
    sent = mention(human, bolt, "after refresh")
    got = admin.wait_deliveries(1, app_id=app_id, event_type="app_mention", state="delivered")
    assert got[0]["transport"] == "socket"
    thread = human.client.conversations_replies(channel=human.general, ts=sent["ts"])["messages"]
    deadline = time.monotonic() + CONNECT_DEADLINE_S
    while not any(m.get("text") == "pong again" for m in thread) and time.monotonic() < deadline:
        time.sleep(0.05)
        thread = human.client.conversations_replies(channel=human.general, ts=sent["ts"])[
            "messages"
        ]
    assert any(m.get("text") == "pong again" for m in thread)


def test_event_with_no_connection_fails_the_attempt_then_retries(admin, bolt, human):
    app_id = bolt.record["id"]
    admin.put_delivery_config(app_id, retry_delays_ms=[5000], timeout_ms=500)
    bolt.handler.close()  # the real client goes away; the server notices the closed socket
    wait_connections(admin, app_id, 0)

    mention(human, bolt, "nobody home")

    got = admin.wait_deliveries(1, app_id=app_id, event_type="app_mention", state="retrying")
    assert got[0]["transport"] == "socket"
    assert got[0]["attempts"][0]["error"] == "no_socket_connection"
