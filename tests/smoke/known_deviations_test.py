"""Red tests for real Slack behavior LocalSlack does not match yet.

Each test is a strict xfail tagged with an id from `[deviations.*]` in
internal/capabilities/capabilities.toml (a Go test checks the ids). They document, executably,
what apps would see. When you fix a gap the test XPASSes and fails the run: remove the marker
and the deviation entry, then `make docs`.

The expected Slack behavior in these tests comes from memory of Slack's docs (evidence level
`llm`), not from recordings; if a recording contradicts one, fix the test and the manifest.
"""

import httpx
from slack_sdk import WebClient

from conftest import deviation, error_of, humans


def new_channel(client: WebClient, name: str) -> str:
    return client.conversations_create(name=name)["channel"]["id"]


@deviation("no-blocks")
def test_blocks_only_message_is_accepted(client, unique):
    channel = new_channel(client, f"blk-{unique}")
    blocks = [{"type": "section", "text": {"type": "mrkdwn", "text": "hello *blocks*"}}]
    assert client.chat_postMessage(channel=channel, blocks=blocks)["ok"]


@deviation("no-blocks")
def test_blocks_round_trip_through_history(client, unique):
    channel = new_channel(client, f"blk2-{unique}")
    blocks = [{"type": "section", "text": {"type": "mrkdwn", "text": "hello *blocks*"}}]
    client.chat_postMessage(channel=channel, text="fallback", blocks=blocks)
    message = client.conversations_history(channel=channel)["messages"][0]
    assert message.get("blocks") == blocks


@deviation("name-targets-unresolved")
def test_post_by_channel_name(client):
    assert client.chat_postMessage(channel="#general", text="by name")["ok"]


@deviation("name-targets-unresolved")
def test_post_to_user_id_opens_a_dm(client):
    user = humans(client)[0]
    assert client.chat_postMessage(channel=user, text="hello via user id")["ok"]


@deviation("no-membership-check")
def test_non_member_cannot_read_a_dm(api_url, client, unique):
    a, b = humans(client)[:2]
    dm = WebClient(token=f"xoxp-{a}", base_url=api_url).conversations_open(users=b)["channel"]["id"]
    client.chat_postMessage(channel=dm, text=f"secret {unique}")  # the bot is not in this DM
    status, error = error_of(lambda: client.conversations_history(channel=dm))
    assert (status, error) == (200, "channel_not_found")


@deviation("unknown-method-404")
def test_unknown_method_is_200_unknown_method(client):
    assert error_of(lambda: client.api_call("does.notExist")) == (200, "unknown_method")


@deviation("response-headers-missing")
def test_responses_carry_slack_headers(api_url):
    resp = httpx.post(api_url + "auth.test", headers={"Authorization": "Bearer xoxb-test"})
    assert "x-slack-req-id" in resp.headers
    assert "x-oauth-scopes" in resp.headers
    assert resp.headers["content-type"].lower().replace(" ", "") == "application/json;charset=utf-8"


@deviation("multipart-ignored")
def test_multipart_form_post_is_understood(api_url, unique):
    data = {"token": "xoxb-test", "name": f"mp-{unique}"}
    resp = httpx.post(
        api_url + "conversations.create", files={k: (None, v) for k, v in data.items()}
    )
    assert resp.json()["ok"] is True


@deviation("users-list-unpaginated")
def test_users_list_honors_limit(client):
    page = client.users_list(limit=1)
    assert len(page["members"]) == 1
    assert page["response_metadata"]["next_cursor"]


@deviation("message-length-unbounded")
def test_overlong_message_is_rejected(client, unique):
    channel = new_channel(client, f"long-{unique}")
    status, error = error_of(lambda: client.chat_postMessage(channel=channel, text="x" * 50_000))
    assert (status, error) == (200, "msg_too_long")


@deviation("no-private-channels")
def test_create_private_channel(client, unique):
    channel = client.conversations_create(name=f"priv-{unique}", is_private=True)["channel"]
    assert channel["is_private"] is True


@deviation("token-in-json-body")
def test_token_in_json_body_is_not_accepted(api_url):
    resp = httpx.post(api_url + "auth.test", json={"token": "xoxb-test"})
    assert resp.json().get("ok") is False


@deviation("partial-message-fields")
def test_messages_carry_bot_fields(client, unique):
    channel = new_channel(client, f"bot-{unique}")
    client.chat_postMessage(channel=channel, text="from the bot")
    message = client.conversations_history(channel=channel)["messages"][0]
    assert message.get("bot_id")
    assert message.get("subtype") in (None, "bot_message")


@deviation("collapsed-errors")
def test_empty_channel_name_has_its_own_error(client):
    status, error = error_of(lambda: client.conversations_create(name=""))
    assert (status, error) == (200, "invalid_name_required")
