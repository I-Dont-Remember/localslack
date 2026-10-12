"""The call log records what the SDK sends (GET/DELETE /_localslack/calls)."""

import httpx
import pytest
from slack_sdk.errors import SlackApiError

from conftest import humans


def head_seq(admin) -> int:
    """Seq of the newest call (0 when empty). `calls(limit=1)` is the oldest page, not the head."""
    since = 0
    while page := admin.calls(since=since, limit=1000)["items"]:
        since = page[-1]["seq"]
    return since


@pytest.fixture
def calls_since(admin):
    """Calls recorded since this test began (the shared fixture's cursor is the oldest call)."""
    cursor = head_seq(admin)

    def fetch(min: int | None = None, timeout_ms: int = 5000, **filters):
        if min:
            return admin.wait_calls(min, timeout_ms, since=cursor, **filters)
        return admin.calls(since=cursor, **filters)["items"]

    return fetch


def general_id(client) -> str:
    return next(c["id"] for c in client.conversations_list()["channels"] if c["name"] == "general")


def test_sdk_post_is_recorded_with_params_actor_and_response(client, calls_since):
    channel = general_id(client)

    resp = client.chat_postMessage(channel=channel, text="call log hello")

    (call,) = calls_since(min=1, method="chat.postMessage")
    assert call["params"]["channel"] == channel
    assert call["params"]["text"] == "call log hello"
    assert call["ok"] is True
    assert call["error"] == ""
    assert call["status"] == 200
    assert call["http_method"] == "POST"
    assert call["actor"]["token_kind"] == "bot"
    assert call["actor"]["user_id"]
    assert call["response"]["ts"] == resp["ts"]
    assert call["response_truncated"] is False
    assert call["duration_ms"] >= 0
    assert call["request_id"]
    assert call["fault_id"] == ""
    assert call["not_implemented"] is False


def test_failed_call_is_recorded_with_error_code(client, calls_since):
    with pytest.raises(SlackApiError):
        client.chat_postMessage(channel="CNOSUCHCHAN", text="x")

    (call,) = calls_since(min=1, method="chat.postMessage")
    assert call["ok"] is False
    assert call["error"] == "channel_not_found"


def test_unknown_method_is_flagged_and_keeps_its_actor(client, calls_since):
    with pytest.raises(SlackApiError):
        client.api_call("totally.unknown", params={"x": "1"})

    (call,) = calls_since(min=1, method="totally.unknown")
    assert call["not_implemented"] is True
    assert call["params"]["x"] == "1"
    assert call["actor"]["token_kind"] == "bot"


def test_filters_by_channel_ok_and_error(client, admin, calls_since):
    channel = general_id(client)
    client.chat_postMessage(channel=channel, text="filter me")
    with pytest.raises(SlackApiError):
        client.chat_postMessage(channel="CNOSUCHCHAN", text="x")
    calls_since(min=2, method="chat.postMessage")

    by_channel = calls_since(channel=channel)
    failed = calls_since(ok=False)
    by_error = calls_since(error="channel_not_found")

    assert [c["params"]["channel"] for c in by_channel] == [channel]
    assert [c["error"] for c in failed] == ["channel_not_found"]
    assert len(by_error) == 1


def test_secret_params_are_redacted(base_url, calls_since):
    secret = "xoxb-" + "redaction-canary"

    httpx.post(f"{base_url}/api/auth.test", data={"token": secret})

    (call,) = calls_since(min=1, method="auth.test")
    assert call["params"]["token"] == "[redacted]"
    assert secret not in str(call)


def test_json_body_params_keep_their_types(base_url, client, calls_since):
    channel = general_id(client)

    httpx.post(
        f"{base_url}/api/chat.postMessage",
        json={"channel": channel, "text": "typed", "unfurl_links": False},
        headers={"Authorization": "Bearer " + client.token},
    )

    (call,) = calls_since(min=1, method="chat.postMessage")
    assert call["params"]["unfurl_links"] is False


def test_since_and_next_since_page_through_calls(client, admin, calls_since):
    for _ in range(3):
        client.auth_test()
    calls_since(min=3, method="auth.test")
    first = admin.calls(method="auth.test", limit=2)

    rest = admin.calls(method="auth.test", since=first["next_since"])

    assert len(first["items"]) == 2
    assert rest["items"], "the remaining calls follow next_since"
    assert rest["items"][0]["seq"] > first["items"][-1]["seq"]


def test_long_poll_returns_when_a_matching_call_arrives(client, admin):
    cursor = head_seq(admin)

    client.users_list()  # in flight before the wait: a long poll must still see it
    got = admin.wait_calls(1, timeout_ms=5000, method="users.list", since=cursor)

    assert got[0]["method"] == "users.list"


def test_users_and_ui_actions_do_not_flood_the_log(base_url, calls_since):
    httpx.get(f"{base_url}/_localslack/health")
    httpx.get(f"{base_url}/ui/", follow_redirects=True)

    assert calls_since() == []


def test_user_token_actor_is_a_user(base_url, client, calls_since):
    user = humans(client)[0]

    httpx.post(f"{base_url}/api/auth.test", headers={"Authorization": f"Bearer xoxp-{user}"})

    (call,) = calls_since(min=1, method="auth.test")
    assert call["actor"] == {"user_id": user, "app_id": "", "token_kind": "user"}


def test_delete_clears_log_and_seq_keeps_growing(client, admin):
    client.auth_test()
    before = admin.calls()

    admin.delete_calls()
    client.auth_test()
    after = admin.calls()

    assert len(after["items"]) == 1
    assert after["items"][0]["seq"] > before["items"][-1]["seq"]
    assert after["generation"] > before["generation"]
