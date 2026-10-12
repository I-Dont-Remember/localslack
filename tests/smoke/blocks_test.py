"""Blocks, attachments and message metadata round-trip through the real SDK."""

import pytest
from slack_sdk import WebClient
from slack_sdk.models.blocks import DividerBlock, SectionBlock
from slack_sdk.models.blocks.basic_components import MarkdownTextObject

from conftest import error_of

META = {"event_type": "task_created", "event_payload": {"id": "T1", "owner": "kpq"}}
ATTACHMENTS = [{"color": "good", "text": "attached", "fallback": "attached"}]


@pytest.fixture
def channel(client: WebClient, unique: str) -> str:
    return client.conversations_create(name=f"blk-{unique}")["channel"]["id"]


def kit() -> list:
    return [
        SectionBlock(text=MarkdownTextObject(text="hello *blocks*")),
        DividerBlock(),
    ]


def kit_dicts() -> list[dict]:
    return [b.to_dict() for b in kit()]


def test_blocks_only_message_is_accepted(client, channel):
    resp = client.chat_postMessage(channel=channel, blocks=kit())
    assert resp["ok"]
    assert resp["message"]["blocks"] == kit_dicts()


def test_blocks_round_trip_through_history(client, channel):
    client.chat_postMessage(channel=channel, text="fallback", blocks=kit())
    message = client.conversations_history(channel=channel)["messages"][0]
    assert message["text"] == "fallback"
    assert message["blocks"] == kit_dicts()


def test_attachments_only_and_round_trip(client, channel):
    client.chat_postMessage(channel=channel, attachments=ATTACHMENTS)
    message = client.conversations_history(channel=channel)["messages"][0]
    assert message["attachments"] == ATTACHMENTS


def test_blocks_survive_in_thread_replies(client, channel):
    root = client.chat_postMessage(channel=channel, text="root")["ts"]
    client.chat_postMessage(channel=channel, thread_ts=root, blocks=kit(), attachments=ATTACHMENTS)
    reply = client.conversations_replies(channel=channel, ts=root)["messages"][1]
    assert (reply["blocks"], reply["attachments"]) == (kit_dicts(), ATTACHMENTS)


def test_metadata_only_with_include_all_metadata(client, channel):
    client.chat_postMessage(channel=channel, text="with meta", metadata=META)
    hidden = client.conversations_history(channel=channel)["messages"][0]
    shown = client.conversations_history(channel=channel, include_all_metadata=True)["messages"][0]
    assert "metadata" not in hidden
    assert shown["metadata"] == META


@pytest.mark.parametrize(
    ("kwargs", "error"),
    [
        ({"blocks": [{"text": "no type"}]}, "invalid_blocks"),
        ({"blocks": [{"type": "divider"}] * 51}, "invalid_blocks"),
        ({"text": "x", "attachments": {"not": "a list"}}, "invalid_attachments"),
        ({"text": "x", "metadata": {"event_type": "t"}}, "invalid_metadata_format"),
        ({"text": "x" * 50_000}, "msg_too_long"),
        ({"blocks": []}, "no_text"),
    ],
)
def test_invalid_content_errors(client, channel, kwargs, error):
    assert error_of(lambda: client.chat_postMessage(channel=channel, **kwargs)) == (200, error)
