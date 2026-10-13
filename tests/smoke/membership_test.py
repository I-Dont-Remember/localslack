"""conversations.join/leave/invite/kick through the real SDK: membership drives access."""

import pytest
from slack_sdk import WebClient

from conftest import error_of, humans

NOT_IN = (200, "not_in_channel")


@pytest.fixture
def human(client: WebClient, base_url: str) -> WebClient:
    return WebClient(token=f"xoxp-{humans(client)[0]}", base_url=f"{base_url}/api/")


@pytest.fixture
def bot_id(client: WebClient) -> str:
    return client.auth_test()["user_id"]


def members(c: WebClient, channel: str) -> list[str]:
    return c.conversations_members(channel=channel, limit=1000)["members"]


def test_bot_joins_public_channel_then_reads_it(client, human, unique):
    ch = human.conversations_create(name=f"join-{unique}")["channel"]["id"]
    human.chat_postMessage(channel=ch, text="before")
    assert error_of(lambda: client.conversations_history(channel=ch)) == NOT_IN

    joined = client.conversations_join(channel=ch)

    assert joined["channel"]["is_member"] is True
    assert [m["text"] for m in client.conversations_history(channel=ch)["messages"]] == ["before"]
    assert "warning" not in joined


def test_join_twice_warns(client, human, unique):
    ch = human.conversations_create(name=f"join2-{unique}")["channel"]["id"]
    client.conversations_join(channel=ch)

    again = client.conversations_join(channel=ch)

    assert again["warning"] == "already_in_channel"
    assert again["response_metadata"]["warnings"] == ["already_in_channel"]


def test_join_private_channel_is_hidden(client, human, unique):
    ch = human.conversations_create(name=f"priv-{unique}", is_private=True)["channel"]["id"]
    assert error_of(lambda: client.conversations_join(channel=ch)) == (200, "channel_not_found")


def test_leave_revokes_read_access(client, human, unique):
    ch = human.conversations_create(name=f"leave-{unique}")["channel"]["id"]
    client.conversations_join(channel=ch)
    client.conversations_leave(channel=ch)
    assert error_of(lambda: client.conversations_history(channel=ch)) == NOT_IN
    assert error_of(lambda: client.conversations_leave(channel=ch)) == NOT_IN


def test_invite_bot_into_private_channel(client, human, bot_id, unique):
    ch = human.conversations_create(name=f"inv-{unique}", is_private=True)["channel"]["id"]
    post = error_of(lambda: client.chat_postMessage(channel=ch, text="x"))
    assert post == (200, "channel_not_found")

    human.conversations_invite(channel=ch, users=bot_id)

    client.chat_postMessage(channel=ch, text="now I can")
    assert bot_id in members(human, ch)
    code = error_of(lambda: human.conversations_invite(channel=ch, users=bot_id))
    assert code == (200, "already_in_channel")


def test_invite_errors_carry_per_user_detail(human, unique):
    ch = human.conversations_create(name=f"inv2-{unique}")["channel"]["id"]
    with pytest.raises(Exception) as exc:
        human.conversations_invite(channel=ch, users="UNOPE")
    resp = exc.value.response
    assert resp["error"] == "user_not_found"
    assert resp["errors"] == [{"user": "UNOPE", "ok": False, "error": "user_not_found"}]


def test_kick_removes_the_member(client, human, bot_id, unique):
    ch = human.conversations_create(name=f"kick-{unique}")["channel"]["id"]
    human.conversations_invite(channel=ch, users=bot_id)

    human.conversations_kick(channel=ch, user=bot_id)

    assert bot_id not in members(human, ch)
    assert error_of(lambda: client.conversations_history(channel=ch)) == NOT_IN
