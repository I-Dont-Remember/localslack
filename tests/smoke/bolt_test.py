"""A real Bolt for Python app against LocalSlack.

Covers what works without the Events API: the app boots (token verification calls auth.test)
and a handler's `say()` posts through LocalSlack. Real event delivery over HTTP and Socket Mode
is covered by the M2/M3 smoke tests once LocalSlack can deliver events.
"""

import json
import time

from slack_bolt import App, BoltRequest
from slack_sdk import WebClient
from slack_sdk.signature import SignatureVerifier

SECRET = "local-signing-secret"


def make_app(api_url: str) -> App:
    client = WebClient(token="xoxb-test", base_url=api_url)
    return App(client=client, signing_secret=SECRET, process_before_response=True)


def signed_event(event: dict, team_id: str, bot_user: str) -> BoltRequest:
    """An Events API callback signed the way Slack signs deliveries."""
    body = json.dumps(
        {
            "token": "unused",
            "team_id": team_id,
            "api_app_id": "A0LOCAL0000",
            "type": "event_callback",
            "event_id": "Ev0LOCAL0000",
            "event_time": int(time.time()),
            "event": event,
            "authorizations": [
                {
                    "enterprise_id": None,
                    "team_id": team_id,
                    "user_id": bot_user,
                    "is_bot": True,
                    "is_enterprise_install": False,
                }
            ],
        }
    )
    ts = str(int(time.time()))
    signature = SignatureVerifier(SECRET).generate_signature(timestamp=ts, body=body)
    headers = {
        "content-type": ["application/json"],
        "x-slack-signature": [signature],
        "x-slack-request-timestamp": [ts],
    }
    return BoltRequest(body=body, headers=headers)


def test_bolt_app_boots_against_localslack(api_url):
    app = make_app(api_url)  # token verification => auth.test must succeed
    auth = app.client.auth_test()
    assert auth["ok"]
    assert auth["user_id"].startswith("U")
    assert auth["team_id"].startswith("T")


def test_bolt_handler_replies_through_localslack(api_url, unique):
    app = make_app(api_url)
    auth = app.client.auth_test()
    channel = app.client.conversations_create(name=f"bolt-{unique}")["channel"]["id"]
    human = next(u["id"] for u in app.client.users_list()["members"] if not u["is_bot"])

    @app.event("app_mention")
    def reply(event, say):
        say(f"pong: {event['text']}")

    ts = f"{int(time.time())}.000100"
    request = signed_event(
        {
            "type": "app_mention",
            "user": human,
            "text": f"<@{auth['user_id']}> ping",
            "ts": ts,
            "channel": channel,
            "event_ts": ts,
        },
        team_id=auth["team_id"],
        bot_user=auth["user_id"],
    )
    response = app.dispatch(request)

    assert response.status == 200
    history = app.client.conversations_history(channel=channel)["messages"]
    assert [m["text"] for m in history] == [f"pong: <@{auth['user_id']}> ping"]
    assert history[0]["user"] == auth["user_id"]


def test_bolt_rejects_a_badly_signed_request(api_url):
    app = make_app(api_url)
    request = signed_event({"type": "app_mention", "channel": "C1", "text": "x"}, "T1", "U1")
    request.headers["x-slack-signature"] = ["v0=deadbeef"]
    assert app.dispatch(request).status == 401
