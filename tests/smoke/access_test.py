"""Access rules through the real SDK: membership, private channels, scope gating, archived.

Actors: the default bot (default scopes include chat:write.public), a human user token
(xoxp-<id>), and a registered app whose manifest lacks chat:write.public.
"""

import pytest
from slack_sdk import WebClient

from conftest import error_of, humans

NOT_FOUND = (200, "channel_not_found")
NOT_IN = (200, "not_in_channel")


@pytest.fixture
def human(client: WebClient, base_url: str) -> WebClient:
    return WebClient(token=f"xoxp-{humans(client)[0]}", base_url=f"{base_url}/api/")


@pytest.fixture
def priv(human: WebClient, unique: str) -> str:
    """A private channel only the human is in."""
    return human.conversations_create(name=f"priv-{unique}", is_private=True)["channel"]["id"]


@pytest.fixture
def pub(human: WebClient, unique: str) -> str:
    """A public channel only the human is in (the bot has not joined)."""
    return human.conversations_create(name=f"pub-{unique}")["channel"]["id"]


@pytest.fixture
def strict_bot(admin, base_url: str, unique: str):
    """A registered app that has chat:write but not chat:write.public."""
    token = "xo" + f"xb-strict-{unique}"
    manifest = {
        "display_information": {"name": f"Strict {unique}"},
        "features": {"bot_user": {"display_name": f"strict-{unique}"}},
        "oauth_config": {"scopes": {"bot": ["chat:write"]}},
    }
    record = admin.register_app(manifest, bot_token=token, verify=False)
    yield WebClient(token=token, base_url=f"{base_url}/api/")
    admin.delete_app(record["id"])


def test_create_private_channel_shape(human, priv):
    ch = human.conversations_info(channel=priv)["channel"]
    assert (ch["is_private"], ch["is_group"], ch["is_channel"], ch["is_member"]) == (
        True,
        True,
        False,
        True,
    )


def test_private_channel_listing_is_per_member(client, human, priv):
    mine = human.conversations_list(types="private_channel", limit=1000)["channels"]
    assert priv in [c["id"] for c in mine]
    assert all(c["is_private"] for c in mine)
    theirs = client.conversations_list(types="private_channel", limit=1000)["channels"]
    assert priv not in [c["id"] for c in theirs]
    public = human.conversations_list(types="public_channel", limit=1000)["channels"]
    assert priv not in [c["id"] for c in public]


@pytest.mark.parametrize(
    "call",
    [
        lambda c, ch: c.conversations_history(channel=ch),
        lambda c, ch: c.conversations_info(channel=ch),
        lambda c, ch: c.conversations_members(channel=ch),
        lambda c, ch: c.chat_postMessage(channel=ch, text="x"),
        lambda c, ch: c.conversations_replies(channel=ch, ts="1.000001"),
    ],
    ids=["history", "info", "members", "post", "replies"],
)
def test_non_member_cannot_touch_private_channel(client, priv, call):
    assert error_of(lambda: call(client, priv)) == NOT_FOUND


def test_member_uses_private_channel(human, priv):
    ts = human.chat_postMessage(channel=priv, text="inside")["ts"]
    assert [m["ts"] for m in human.conversations_history(channel=priv)["messages"]] == [ts]


def test_non_member_cannot_read_a_dm(api_url, client, unique):
    a, b = humans(client)[:2]
    alice = WebClient(token=f"xoxp-{a}", base_url=api_url)
    dm = alice.conversations_open(users=b)["channel"]["id"]
    alice.chat_postMessage(channel=dm, text=f"secret {unique}")  # the bot is not in this DM
    assert error_of(lambda: client.chat_postMessage(channel=dm, text="x")) == NOT_FOUND
    assert error_of(lambda: client.conversations_history(channel=dm)) == NOT_FOUND


def test_bot_not_in_public_channel_cannot_read_but_can_post(client, pub):
    assert error_of(lambda: client.conversations_history(channel=pub)) == NOT_IN
    assert client.chat_postMessage(channel=pub, text="via chat:write.public")["ok"]
    assert client.conversations_info(channel=pub)["channel"]["is_member"] is False


def test_app_without_write_public_cannot_post_to_unjoined_channel(strict_bot, pub):
    assert error_of(lambda: strict_bot.chat_postMessage(channel=pub, text="x")) == NOT_IN


def test_app_without_write_public_posts_where_it_is_a_member(strict_bot, unique):
    ch = strict_bot.conversations_create(name=f"own-{unique}")["channel"]["id"]
    assert strict_bot.chat_postMessage(channel=ch, text="mine")["ok"]
    assert len(strict_bot.conversations_history(channel=ch)["messages"]) == 1


def test_user_token_non_member_reads_public_but_cannot_post(client, human, unique):
    ch = client.conversations_create(name=f"botpub-{unique}")["channel"]["id"]
    client.chat_postMessage(channel=ch, text="hello")
    assert len(human.conversations_history(channel=ch)["messages"]) == 1
    assert error_of(lambda: human.chat_postMessage(channel=ch, text="x")) == NOT_IN


def test_react_requires_membership(client, human, unique):
    ch = client.conversations_create(name=f"react-{unique}")["channel"]["id"]
    ts = client.chat_postMessage(channel=ch, text="react to me")["ts"]
    assert error_of(lambda: human.reactions_add(channel=ch, timestamp=ts, name="tada")) == NOT_IN


def test_ephemeral_to_non_member_is_user_not_in_channel(client, pub, human):
    me = humans(client)[1]  # not in the human-only channel
    assert error_of(lambda: client.chat_postEphemeral(channel=pub, user=me, text="x")) == (
        200,
        "user_not_in_channel",
    )


def test_archived_channel_rejects_writes(client, unique):
    ch = client.conversations_create(name=f"arch-{unique}")["channel"]["id"]
    ts = client.chat_postMessage(channel=ch, text="before")["ts"]
    client.conversations_archive(channel=ch)
    archived = (200, "is_archived")
    assert error_of(lambda: client.chat_postMessage(channel=ch, text="x")) == archived
    assert error_of(lambda: client.chat_update(channel=ch, ts=ts, text="y")) == archived
    assert error_of(lambda: client.reactions_add(channel=ch, timestamp=ts, name="tada")) == archived
    assert len(client.conversations_history(channel=ch)["messages"]) == 1  # still readable
