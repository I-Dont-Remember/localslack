"""Bolt-core read methods through the real SDK: conversations.info/members, users.info, api.test."""

from slack_sdk.errors import SlackApiError

from conftest import error_of, humans


def general_id(client) -> str:
    channels = client.conversations_list(types="public_channel", limit=1000)["channels"]
    return next(c["id"] for c in channels if c["name"] == "general")


def test_conversations_info_reads_back_a_created_channel(client, unique):
    name = f"info-{unique}"
    created = client.conversations_create(name=name)["channel"]
    info = client.conversations_info(channel=created["id"], include_num_members=True)["channel"]
    assert (info["id"], info["name"], info["is_channel"], info["is_private"]) == (
        created["id"],
        name,
        True,
        False,
    )
    assert info["num_members"] == 1


def test_conversations_info_hides_num_members_by_default(client):
    assert "num_members" not in client.conversations_info(channel=general_id(client))["channel"]


def test_conversations_info_unknown_channel(client):
    assert error_of(lambda: client.conversations_info(channel="C00000000")) == (
        200,
        "channel_not_found",
    )


def test_users_info_matches_users_list(client):
    user_id = humans(client)[0]
    listed = next(u for u in client.users_list()["members"] if u["id"] == user_id)
    assert client.users_info(user=user_id)["user"] == listed


def test_users_info_bot_matches_auth_test(client):
    auth = client.auth_test()
    user = client.users_info(user=auth["user_id"])["user"]
    assert user["is_bot"] is True
    assert user["profile"]["bot_id"] == auth["bot_id"]


def test_users_info_unknown_user(client):
    assert error_of(lambda: client.users_info(user="U00000000")) == (200, "user_not_found")


def test_conversations_members_iterator_walks_every_page(client):
    channel = general_id(client)
    everyone = client.conversations_members(channel=channel, limit=1000)["members"]
    walked, pages = [], 0
    for page in client.conversations_members(channel=channel, limit=2):
        pages += 1
        walked += page["members"]
    assert pages > 1
    assert walked == everyone


def test_conversations_members_unknown_channel(client):
    assert error_of(lambda: client.conversations_members(channel="C00000000")) == (
        200,
        "channel_not_found",
    )


def test_api_test_echoes_args(client):
    assert client.api_test(foo="bar")["args"] == {"foo": "bar"}


def test_api_test_error_param_fails_with_args(client):
    try:
        client.api_test(error="my_error", foo="bar")
    except SlackApiError as e:
        assert e.response["error"] == "my_error"
        assert e.response["args"] == {"error": "my_error", "foo": "bar"}
    else:
        raise AssertionError("expected SlackApiError")
