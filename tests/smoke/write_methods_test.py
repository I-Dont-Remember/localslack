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


def test_delete_removes_message(client, chan):
    ts = client.chat_postMessage(channel=chan, text="bye")["ts"]
    client.chat_delete(channel=chan, ts=ts)
    assert client.conversations_history(channel=chan)["messages"] == []


def test_delete_reply_updates_thread_counters(client, chan):
    root = client.chat_postMessage(channel=chan, text="root")["ts"]
    first = client.chat_postMessage(channel=chan, text="one", thread_ts=root)["ts"]
    second = client.chat_postMessage(channel=chan, text="two", thread_ts=root)["ts"]
    client.chat_delete(channel=chan, ts=second)
    head = client.conversations_history(channel=chan)["messages"][0]
    assert (head["reply_count"], head["latest_reply"]) == (1, first)


def test_delete_root_with_replies_leaves_tombstone(client, chan):
    root = client.chat_postMessage(channel=chan, text="root")["ts"]
    reply = client.chat_postMessage(channel=chan, text="reply", thread_ts=root)["ts"]
    client.chat_delete(channel=chan, ts=root)
    head = client.conversations_history(channel=chan)["messages"][0]
    assert (head["subtype"], head["text"]) == ("tombstone", "This message was deleted.")
    thread = client.conversations_replies(channel=chan, ts=root)["messages"]
    assert [m["ts"] for m in thread] == [root, reply]


def test_delete_errors(client, human_client, chan):
    ts = client.chat_postMessage(channel=chan, text="mine")["ts"]
    assert_errors(
        {
            "cant_delete_message": lambda: human_client.chat_delete(channel=chan, ts=ts),
            "message_not_found": lambda: client.chat_delete(channel=chan, ts="1.000001"),
            "channel_not_found": lambda: client.chat_delete(channel="C00000000", ts=ts),
        }
    )


def test_post_ephemeral_is_not_in_history(client, chan, unique):
    me = client.auth_test()["user_id"]
    resp = client.chat_postEphemeral(channel=chan, user=me, text=f"psst {unique}")
    assert resp["message_ts"]
    assert client.conversations_history(channel=chan)["messages"] == []


def test_reaction_shows_in_history_and_replies(client, human_client, chan):
    root = client.chat_postMessage(channel=chan, text="root")["ts"]
    reply = client.chat_postMessage(channel=chan, text="reply", thread_ts=root)["ts"]
    me = client.auth_test()["user_id"]
    client.reactions_add(channel=chan, timestamp=root, name="eyes")
    human_client.reactions_add(channel=chan, timestamp=root, name="eyes")
    client.reactions_add(channel=chan, timestamp=reply, name="tada")

    head = client.conversations_history(channel=chan)["messages"][0]
    assert [(r["name"], r["count"]) for r in head["reactions"]] == [("eyes", 2)]
    assert head["reactions"][0]["users"][0] == me
    thread = client.conversations_replies(channel=chan, ts=root)["messages"]
    assert [m.get("reactions", [{}])[0].get("name") for m in thread] == ["eyes", "tada"]


def test_reaction_errors(client, chan):
    ts = client.chat_postMessage(channel=chan, text="react to me")["ts"]
    client.reactions_add(channel=chan, timestamp=ts, name="eyes")
    assert_errors(
        {
            "already_reacted": lambda: client.reactions_add(
                channel=chan, timestamp=ts, name="eyes"
            ),
            "invalid_name": lambda: client.reactions_add(channel=chan, timestamp=ts, name=":eyes:"),
            "message_not_found": lambda: client.reactions_add(
                channel=chan, timestamp="1.000001", name="eyes"
            ),
            "channel_not_found": lambda: client.reactions_add(
                channel="C00000000", timestamp=ts, name="eyes"
            ),
        }
    )


def test_post_ephemeral_errors(client, chan):
    me = client.auth_test()["user_id"]
    assert_errors(
        {
            "user_not_in_channel": lambda: client.chat_postEphemeral(
                channel=chan, user="UNOTAMEMBER", text="x"
            ),
            "channel_not_found": lambda: client.chat_postEphemeral(
                channel="C00000000", user=me, text="x"
            ),
        }
    )
