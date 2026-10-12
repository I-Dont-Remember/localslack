"""Events API over HTTP: a real Bolt app receives signed events from LocalSlack and replies.

No sleeps: waits long-poll the delivery log and the call log.
"""

from __future__ import annotations

import pytest
from slack_bolt import App
from slack_sdk import WebClient

from bolt_http import BoltServer


@pytest.fixture
def bolt(admin, base_url, unique):
    """Register a Bolt app (real HTTP server + signature check) and return its pieces."""
    server = BoltServer()
    secret = f"smoke-secret-{unique}"
    bot_token = "xo" + f"xb-smoke-{unique}"
    manifest = {
        "display_information": {"name": f"Smoke {unique}"},
        "features": {"bot_user": {"display_name": f"smoke-{unique}"}},
        "settings": {
            "event_subscriptions": {
                "request_url": server.url,
                "bot_events": ["app_mention", "message.channels", "reaction_added"],
            }
        },
    }
    record = admin.register_app(manifest, signing_secret=secret, bot_token=bot_token)
    api = f"{base_url}/api/"
    app = App(
        token=bot_token, signing_secret=secret, client=WebClient(token=bot_token, base_url=api)
    )
    server.app = app
    yield type("BoltEnv", (), {"app": app, "server": server, "record": record})
    admin.delete_app(record["id"])
    server.close()


@pytest.fixture
def human(base_url):
    """A WebClient acting as a human user, plus #general's id."""
    probe = WebClient(token="xo" + "xb-probe", base_url=f"{base_url}/api/")
    uid = next(u["id"] for u in probe.users_list()["members"] if not u["is_bot"])
    client = WebClient(token=f"xoxp-{uid}", base_url=f"{base_url}/api/")
    channels = client.conversations_list(types="public_channel")["channels"]
    general = next(c["id"] for c in channels if c["name"] == "general")
    return type("Human", (), {"client": client, "id": uid, "general": general})


def test_mention_gets_a_threaded_reply(admin, bolt, human):
    @bolt.app.event("app_mention")
    def on_mention(event, say):
        say(text="pong", thread_ts=event["ts"])

    app_id = bolt.record["id"]
    assert bolt.record["delivery"] == "ready" and bolt.record["verified"] is True
    cursor = admin.head_seq()

    sent = human.client.chat_postMessage(
        channel=human.general, text=f"<@{bolt.record['bot_user_id']}> ping"
    )

    admin.wait_deliveries(1, app_id=app_id, event_type="app_mention", state="delivered")
    admin.wait_calls(1, since=cursor, method="chat.postMessage", app_id=app_id)
    thread = human.client.conversations_replies(channel=human.general, ts=sent["ts"])["messages"]
    reply = next(m for m in thread if m.get("text") == "pong")
    assert reply["user"] == bolt.record["bot_user_id"]
    assert reply["thread_ts"] == sent["ts"]
    assert reply["bot_id"] == bolt.record["bot_id"]


def test_hello_message_reaches_a_bot_that_is_in_the_channel(admin, bolt, human):
    @bolt.app.message("hello")
    def on_hello(say):
        say(text="hi there")

    app_id = bolt.record["id"]
    cursor = admin.head_seq()
    human.client.chat_postMessage(channel=human.general, text="hello everyone")

    admin.wait_deliveries(1, app_id=app_id, event_type="message", state="delivered")
    calls = admin.wait_calls(1, since=cursor, method="chat.postMessage", app_id=app_id)
    assert calls[0]["ok"] is True


def test_reaction_added_is_delivered(admin, bolt, human):
    seen = []

    @bolt.app.event("reaction_added")
    def on_reaction(event):
        seen.append(event["reaction"])

    sent = human.client.chat_postMessage(channel=human.general, text="react to me")
    human.client.reactions_add(channel=human.general, timestamp=sent["ts"], name="eyes")

    got = admin.wait_deliveries(
        1, app_id=bolt.record["id"], event_type="reaction_added", state="delivered"
    )
    assert got[0]["payload"]["event"]["reaction"] == "eyes"
    assert seen == ["eyes"]


def test_bad_signature_is_rejected_by_bolt_with_retries(admin, bolt, human):
    app_id = bolt.record["id"]
    admin.put_delivery_config(app_id, signature="invalid")

    human.client.chat_postMessage(
        channel=human.general, text=f"<@{bolt.record['bot_user_id']}> nope"
    )

    failed = admin.wait_deliveries(1, app_id=app_id, event_type="app_mention", state="failed")
    attempts = failed[0]["attempts"]
    assert len(attempts) >= 2, "failed deliveries are retried"
    assert {a["status"] for a in attempts} == {401}
    assert {a["signature_mode"] for a in attempts} == {"invalid"}
    assert attempts[1]["retry_reason"] == "http_error"


def test_redeliver_sends_a_duplicate_with_retry_headers(admin, bolt, human):
    @bolt.app.event("app_mention")
    def on_mention(say):
        say(text="seen")

    app_id = bolt.record["id"]
    human.client.chat_postMessage(
        channel=human.general, text=f"<@{bolt.record['bot_user_id']}> once"
    )
    first = admin.wait_deliveries(1, app_id=app_id, event_type="app_mention", state="delivered")[0]

    admin.redeliver(first["event_id"], retry_num=1, retry_reason="http_timeout")

    both = admin.wait_deliveries(2, app_id=app_id, event_type="app_mention", state="delivered")
    assert {d["event_id"] for d in both} == {first["event_id"]}
    # Unrelated message events may 404 in Bolt (no listener) and be retried, so match by event id.
    mine = [r["headers"] for r in bolt.server.requests if first["event_id"] in r["body"]]
    retried = [h for h in mine if "X-Slack-Retry-Num" in h]
    assert len(mine) == 2 and len(retried) == 1, "only the redelivery carries retry headers"
    assert retried[0]["X-Slack-Retry-Num"] == "1"
    assert retried[0]["X-Slack-Retry-Reason"] == "http_timeout"


def test_injected_event_with_stale_signature_is_rejected(admin, bolt):
    app_id = bolt.record["id"]
    res = admin.inject_event(
        {"type": "reaction_added", "reaction": "tada"}, app_id=app_id, signature="stale"
    )
    assert res["deliveries"][0]["app_id"] == app_id
    failed = admin.wait_deliveries(1, app_id=app_id, state="failed")
    assert failed[0]["injected"] is True
    assert failed[0]["attempts"][0]["status"] == 401
