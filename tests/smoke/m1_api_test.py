"""M1 vertical-slice API tests (write, then read back through a different endpoint).

Strict xfail per test until its endpoint ships: must FAIL now (red) with an API/assertion
error only (not a crash or typo); delete the `@m1` marker per test as endpoints land (green).
XPASS fails the run, forcing marker removal.
"""

import pytest
from slack_sdk.errors import SlackApiError

m1 = pytest.mark.xfail(
    strict=True, raises=(SlackApiError, AssertionError), reason="M1: not implemented yet"
)


@m1
def test_auth_test(client):
    resp = client.auth_test()
    assert resp["ok"]
    assert resp["user_id"].startswith("U")
    assert resp["team_id"].startswith("T")


@m1
def test_users_list_has_members(client):
    members = client.users_list()["members"]
    assert members
    assert all(m["id"].startswith("U") for m in members)


@m1
def test_create_channel_persists(client, unique):
    name = f"smoke-{unique}"
    created = client.conversations_create(name=name)["channel"]
    listed = client.conversations_list(types="public_channel", limit=1000)["channels"]
    assert created["id"] in {c["id"] for c in listed}
    assert name in {c["name"] for c in listed}


@m1
def test_create_duplicate_channel_is_name_taken(client, unique):
    name = f"dup-{unique}"
    client.conversations_create(name=name)
    with pytest.raises(SlackApiError) as exc_info:
        client.conversations_create(name=name)
    assert exc_info.value.response["error"] == "name_taken"


@m1
def test_post_message_persists(client, unique):
    channel = client.conversations_create(name=f"msg-{unique}")["channel"]["id"]
    posted = client.chat_postMessage(channel=channel, text=f"hello {unique}")
    history = client.conversations_history(channel=channel)["messages"]
    assert [(m["ts"], m["text"]) for m in history] == [(posted["ts"], f"hello {unique}")]


@m1
def test_post_to_missing_channel(client):
    with pytest.raises(SlackApiError) as exc_info:
        client.chat_postMessage(channel="C00000000", text="x")
    assert exc_info.value.response["error"] == "channel_not_found"


@m1
def test_thread_reply_persists(client, unique):
    channel = client.conversations_create(name=f"thr-{unique}")["channel"]["id"]
    parent = client.chat_postMessage(channel=channel, text="parent")["ts"]
    reply = client.chat_postMessage(channel=channel, text="reply", thread_ts=parent)["ts"]

    replies = client.conversations_replies(channel=channel, ts=parent)["messages"]
    assert [m["ts"] for m in replies] == [parent, reply]
    top_level = client.conversations_history(channel=channel)["messages"]
    assert [(m["ts"], m.get("reply_count")) for m in top_level] == [(parent, 1)]


@m1
def test_open_dm_and_message_persists(client, unique):
    users = [u["id"] for u in client.users_list()["members"]]
    dm = client.conversations_open(users=users[0])["channel"]["id"]
    assert dm.startswith("D")
    client.chat_postMessage(channel=dm, text=f"dm {unique}")
    assert client.conversations_history(channel=dm)["messages"][0]["text"] == f"dm {unique}"
    ims = client.conversations_list(types="im")["channels"]
    assert dm in {c["id"] for c in ims}
