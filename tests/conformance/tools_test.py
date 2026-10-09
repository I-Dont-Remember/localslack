"""Tests for the fixture recording and redaction tools (record.py, redact_check.py).

The end-to-end test records against a live LocalSlack, so run via `make tools-test`
(scripts/with-server.sh sets LOCALSLACK_URL); it is skipped otherwise.
"""

import json
import os
from pathlib import Path

import pytest
import record
import redact_check

FIXTURES = Path(__file__).parent / "fixtures"


@pytest.mark.parametrize(
    ("key", "value", "want"),
    [
        ("ts", "1700000000.000100", "<ts>"),
        ("channel", "C0123456789", "<id:C>"),
        ("permalink", "https://acme.slack.com/archives/C0123456789/p1", "<string>"),
        ("contact", "alice@example.com", "<string>"),
        (
            "note",
            "this is a long free text value that could contain anything private",
            "<string>",
        ),
        ("name", "general", "<string>"),
        ("created", 1700000000, "<any>"),
        ("ok", True, True),
        ("short", "hi", "hi"),
    ],
)
def test_sanitize_scalar(key, value, want):
    assert record.sanitize(value, key) == want


def test_sanitize_nested():
    got = record.sanitize(
        {"channel": {"id": "C0123456789", "created": 1700000000, "n": 3}}
    )
    assert got == {"channel": {"id": "<id:C>", "created": "<any>", "n": 3}}


@pytest.mark.parametrize(
    "doc",
    [
        {"request": {"token": "xoxb-test"}},
        {"request": {"token": "xoxp-nope"}},
        {"body": {"id": "<id:U>", "ts": "<ts>", "user": "<string>"}},
        {"request": {"params": {"channel": "C00000000", "thread_ts": "{{root}}"}}},
    ],
)
def test_redact_check_accepts_clean(doc):
    assert redact_check.find_problems(doc) == []


@pytest.mark.parametrize(
    ("doc", "needle"),
    [
        ({"t": "xox" + "b-1234567890123-abcdefghijklmnop"}, "token"),
        ({"t": "xa" + "pp-1-A0123456789-123456-abcdef"}, "token"),
        ({"e": "alice@example.com"}, "email"),
        ({"u": "https://acme.slack.com/archives/C1"}, "slack url"),
        ({"i": "U012ABCDEFG"}, "id"),
        ({"i": "F012ABCDEFG"}, "id"),
        ({"i": "<@U012ABCDEFG> hi"}, "id"),
        ({"s": "0123456789abcdef0123456789abcdef"}, "hex"),
        ({"x": "word " * 30}, "long"),
    ],
)
def test_redact_check_flags_leaks(doc, needle):
    problems = redact_check.find_problems(doc)
    assert problems, f"expected a problem for {doc}"
    assert any(needle in p for p in problems), problems


def test_repo_fixtures_are_clean():
    assert redact_check.scan_dir(FIXTURES) == {}


def test_record_from_fixture_against_localslack(tmp_path):
    base = os.environ.get("LOCALSLACK_URL")
    if not base:
        pytest.skip("run via `make tools-test` (needs a live LocalSlack)")
    src = FIXTURES / "conversations_replies.json"
    argv = [
        "--from", str(src), "--base-url", base,
        "--workspace-type", "localslack-test", "--out-dir", str(tmp_path),
    ]  # fmt: skip
    env = {"SLACK_TOKEN": "xoxb-test"}

    assert record.main(argv, env) == 0
    data = json.loads((tmp_path / "conversations_replies__recorded.json").read_text())

    assert data["source"] == "local", (
        "non-Slack hosts must never produce recorded evidence"
    )
    assert "recorded" not in data
    assert data["state"] == "known_gap"
    # The saved request keeps the fixture's own params; the unique suffix only went on the wire.
    assert data["request"]["token"] == "xoxb-test"
    assert data["request"]["params"]["channel"] == "{{chan}}"
    assert data["setup"][0]["params"]["name"] == "fx-replies"
    assert data["response"]["status"] == 200
    assert len(data["response"]["body"]["messages"]) == 2
    assert redact_check.find_problems(data) == []

    assert record.main(argv, env) != 0, "must refuse to overwrite an existing recording"


@pytest.mark.parametrize(
    "path", ["@evil.example/x", "//evil/x", "/api/../x", "/other", "/api/a?b=1"]
)
def test_unsafe_paths_are_refused(path):
    with pytest.raises(ValueError):
        record.safe_path(path)


def test_is_slack_host():
    assert record.is_slack("https://slack.com")
    assert record.is_slack("https://acme.slack.com")
    assert not record.is_slack("https://slack.com.evil.example")
    assert not record.is_slack("http://127.0.0.1:3232")
