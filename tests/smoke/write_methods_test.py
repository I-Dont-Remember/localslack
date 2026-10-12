"""Write methods (update, delete, ephemeral, reactions) through the real SDK."""

import pytest
from slack_sdk import WebClient

from conftest import error_of


@pytest.fixture
def human_client(client: WebClient, base_url: str) -> WebClient:
    human = next(u["id"] for u in client.users_list()["members"] if not u["is_bot"])
    return WebClient(token=f"xoxp-{human}", base_url=f"{base_url}/api/")


@pytest.fixture
def chan(client: WebClient, unique: str) -> str:
    return client.conversations_create(name=f"wr-{unique}")["channel"]["id"]


def test_update_edits_text_and_marks_edited(client, chan):
    ts = client.chat_postMessage(channel=chan, text="v1")["ts"]
    resp = client.chat_update(channel=chan, ts=ts, text="v2")
    assert (resp["ts"], resp["text"]) == (ts, "v2")

    msg = client.conversations_history(channel=chan)["messages"][0]
    assert msg["text"] == "v2"
    assert msg["edited"]["user"] == msg["user"]
    assert msg["edited"]["ts"] > ts


def test_update_with_blocks(client, chan):
    ts = client.chat_postMessage(channel=chan, text="v1")["ts"]
    blocks = [{"type": "section", "text": {"type": "mrkdwn", "text": "*v2*"}}]
    client.chat_update(channel=chan, ts=ts, text="v2", blocks=blocks)
    assert client.conversations_history(channel=chan)["messages"][0]["blocks"] == blocks


def assert_errors(calls: dict) -> None:
    """Each call must fail with the Slack error code that is its key."""
    got = {want: error_of(call)[1] for want, call in calls.items()}
    assert got == {want: want for want in calls}


def test_update_errors(client, human_client, chan):
    ts = client.chat_postMessage(channel=chan, text="mine")["ts"]
    assert_errors(
        {
            "cant_update_message": lambda: human_client.chat_update(channel=chan, ts=ts, text="x"),
            "message_not_found": lambda: client.chat_update(channel=chan, ts="1.000001", text="x"),
            "channel_not_found": lambda: client.chat_update(channel="C00000000", ts=ts, text="x"),
        }
    )
