"""Common Slack SDK interactions that must not throw errors against LocalSlack.

Covers what real scripts and Bolt apps do beyond one-shot calls: the SDK's pagination
iterators, the full error contract, message payload variants and retry handlers.
"""

import time

import pytest
from slack_sdk import WebClient
from slack_sdk.errors import SlackApiError
from slack_sdk.http_retry.builtin_handlers import (
    RateLimitErrorRetryHandler,
    ServerErrorRetryHandler,
)

from conftest import error_of, humans


def walk(response, key: str, field: str) -> tuple[list[str], int]:
    """Consume an SDK pagination iterator, returning (values of `field`, number of pages).

    The SDK's iterator mutates and re-yields ONE response object, so each page must be read
    inside the loop: `list(response)` would show the last page N times.
    """
    values, pages = [], 0
    for page in response:
        pages += 1
        values += [item[field] for item in page[key]]
    return values, pages


def new_channel(client: WebClient, name: str) -> str:
    return client.conversations_create(name=name)["channel"]["id"]


def test_conversations_list_iterator_walks_every_page(client, unique):
    for i in range(3):
        new_channel(client, f"it-{unique}-{i}")
    walked, pages = walk(
        client.conversations_list(limit=2, types="public_channel"), "channels", "id"
    )
    everything = [c["id"] for c in client.conversations_list(limit=1000)["channels"]]
    assert pages >= 2
    assert walked == everything
    assert len(set(walked)) == len(walked)


def test_history_iterator_returns_every_message_newest_first(client, unique):
    channel = new_channel(client, f"hist-{unique}")
    posted = [client.chat_postMessage(channel=channel, text=f"m{i}")["ts"] for i in range(5)]
    walked, pages = walk(client.conversations_history(channel=channel, limit=2), "messages", "ts")
    assert pages == 3
    assert walked == list(reversed(posted))


def test_replies_iterator_returns_root_once_then_replies(client, unique):
    channel = new_channel(client, f"rep-{unique}")
    root = client.chat_postMessage(channel=channel, text="root")["ts"]
    replies = [
        client.chat_postMessage(channel=channel, text=f"r{i}", thread_ts=root)["ts"]
        for i in range(4)
    ]
    walked, pages = walk(
        client.conversations_replies(channel=channel, ts=root, limit=2), "messages", "ts"
    )
    assert pages == 3
    assert walked == [root, *replies]


def test_users_list_iterator_yields_members(client):
    members, _ = walk(client.users_list(limit=2), "members", "id")
    assert members
    assert len(set(members)) == len(members)


def test_seed_has_at_least_two_humans(client):
    """Guards the tests that need two distinct human users."""
    assert len(humans(client)) >= 2


def general_id(client: WebClient) -> str:
    channels = client.conversations_list(limit=1000)["channels"]
    return next(c["id"] for c in channels if c["name"] == "general")


ERROR_CASES = {
    "invalid_name": lambda c, u: c.conversations_create(name="Bad Name!"),
    "name_taken": lambda c, u: (
        c.conversations_create(name=f"dup-{u}"),
        c.conversations_create(name=f"dup-{u}"),
    ),
    "channel_not_found": lambda c, u: c.chat_postMessage(channel="C00000000", text="x"),
    "no_text": lambda c, u: c.chat_postMessage(channel=general_id(c), text=""),
    "thread_not_found": lambda c, u: c.chat_postMessage(
        channel=general_id(c), text="x", thread_ts="1.000001"
    ),
    "user_not_found": lambda c, u: c.conversations_open(users="UNOPE"),
    # The SDK refuses conversations_open() with no arguments client-side, so go through api_call.
    "users_list_not_supplied": lambda c, u: c.api_call("conversations.open"),
    "invalid_cursor": lambda c, u: c.conversations_list(cursor="bogus"),
    "invalid_types": lambda c, u: c.conversations_list(types="nope"),
    "cant_archive_general": lambda c, u: c.conversations_archive(channel=general_id(c)),
    "already_archived": lambda c, u: _archive_twice(c, u),
    "is_archived": lambda c, u: _post_to_archived(c, u),
}


def _archive_twice(c: WebClient, u: str):
    channel = new_channel(c, f"arc2-{u}")
    c.conversations_archive(channel=channel)
    c.conversations_archive(channel=channel)


def _post_to_archived(c: WebClient, u: str):
    channel = new_channel(c, f"arc-{u}")
    c.conversations_archive(channel=channel)
    c.chat_postMessage(channel=channel, text="late")


@pytest.mark.parametrize("code", sorted(ERROR_CASES))
def test_api_errors_are_http_200_with_a_slack_error_code(client, unique, code):
    status, error = error_of(lambda: ERROR_CASES[code](client, unique))
    assert (status, error) == (200, code)


def test_auth_errors(api_url):
    assert error_of(lambda: WebClient(token="nope", base_url=api_url).auth_test()) == (
        200,
        "invalid_auth",
    )
    assert error_of(lambda: WebClient(base_url=api_url).auth_test()) == (200, "not_authed")


def test_unimplemented_method_is_a_clear_sdk_error(client):
    """Deliberate deviation (see unknown-method-404): a 404 with not_implemented."""
    assert error_of(lambda: client.api_call("does.notExist")) == (404, "not_implemented")


@pytest.mark.parametrize(
    "text",
    [
        "héllo wörld 🎉 日本語",
        "*bold* _italic_ ~strike~ `code` <https://example.com|a link> <@U123> <#C123>",
        "x" * 5000,
        "line one\nline two\n\n    indented",
    ],
    ids=["unicode", "mrkdwn", "long", "multiline"],
)
def test_message_text_round_trips_unchanged(client, unique, text):
    channel = new_channel(client, f"txt-{unique}")
    posted = client.chat_postMessage(channel=channel, text=text)
    assert posted["message"]["text"] == text
    history = client.conversations_history(channel=channel)["messages"]
    assert [(m["ts"], m["text"]) for m in history] == [(posted["ts"], text)]


def test_post_message_accepts_common_optional_arguments(client, unique):
    channel = new_channel(client, f"opt-{unique}")
    posted = client.chat_postMessage(
        channel=channel,
        text="with options",
        username="Deploy Bot",
        icon_emoji=":robot_face:",
        mrkdwn=True,
        unfurl_links=False,
        unfurl_media=False,
        link_names=True,
    )
    assert posted["ok"]
    assert client.conversations_history(channel=channel)["messages"][0]["text"] == "with options"


def test_retry_handlers_do_not_get_in_the_way(api_url):
    client = WebClient(
        token="xoxb-test",
        base_url=api_url,
        retry_handlers=[
            RateLimitErrorRetryHandler(max_retry_count=2),
            ServerErrorRetryHandler(max_retry_count=2),
        ],
    )
    started = time.monotonic()
    assert client.auth_test()["ok"]
    with pytest.raises(SlackApiError):
        client.api_call("does.notExist")  # 404 is not retried
    assert time.monotonic() - started < 3
