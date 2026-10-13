"""Scenario seeding, snapshots, restore and diff against a live server.

Resets global state, so it runs in the reset suite (`make smoke-reset`), never in `make smoke`.
"""

import json
import os
import subprocess
from pathlib import Path

import pytest

from localslack_admin import AdminError

pytestmark = pytest.mark.skipif(
    os.environ.get("LOCALSLACK_RESET_SUITE") != "1",
    reason="resets global state; run via `make smoke-reset` against a dedicated server",
)

EXAMPLES = Path(__file__).resolve().parents[2] / "examples" / "scenarios"


@pytest.fixture(autouse=True)
def _leave_a_normal_workspace(admin):
    yield
    admin.reset(seed="scenarios-suite-done")


def acme() -> dict:
    return json.loads((EXAMPLES / "acme.json").read_text())


def test_fixture_populates_exact_ids(admin, client):
    admin.reset(scenario=acme())

    ids = {u["id"] for u in client.users_list()["members"]}
    assert {"U0ANN000001", "U0BOB000001", "U0ACMEBOT01"} <= ids
    assert client.users_info(user="U0ANN000001")["user"]["real_name"] == "Ann Example"
    me = client.auth_test()
    assert (me["user_id"], me["bot_id"]) == ("U0ACMEBOT01", "B0ACMEBOT01")

    history = client.conversations_history(channel="C0GENERAL01")["messages"]
    root = next(m for m in history if m["ts"] == "1767312000.000100")
    assert root["reply_count"] == 3
    assert root["reactions"][0]["name"] == "wave"
    replies = client.conversations_replies(channel="C0GENERAL01", ts=root["ts"])["messages"]
    assert [m["ts"] for m in replies][0] == root["ts"] and len(replies) == 4
    bot_msg = next(m for m in replies if m.get("bot_id"))
    assert bot_msg["bot_id"] == "B0ACMEBOT01" and bot_msg["blocks"][0]["type"] == "section"
    edited = next(m for m in history if m["ts"] == "1767312240.000500")
    assert edited["edited"]["user"] == "U0DAVE00001"


def test_fixture_by_server_side_path(admin, client):
    got = admin.reset(scenario=str(EXAMPLES / "standup.yaml"))

    assert got["counts"] == {"users": 3, "channels": 2, "messages": 3}
    names = {u["name"] for u in client.users_list()["members"]}
    assert {"ann", "bob", "helper"} <= names


def test_generated_scale(admin, client):
    got = admin.reset(generate=json.loads((EXAMPLES / "scale.json").read_text())["generate"])

    assert got["counts"] == {"users": 401, "channels": 50, "messages": 2000}
    assert got["took_ms"] < 10_000, "target is well under 2s on a quiet machine"
    assert len(client.users_list()["members"]) == 401
    state = admin.state()
    assert sum(c.get("is_private", False) for c in state["channels"]) == 5


def test_invalid_scenario_lists_every_problem(admin):
    bad = {
        "users": [{"id": "X1", "name": "a"}],
        "messages": [{"channel": "C9", "ts": "1", "user": "U9", "reply_count": 2}],
    }

    with pytest.raises(AdminError) as err:
        admin.reset(scenario=bad)

    assert err.value.status == 422 and err.value.code == "invalid_scenario"
    pointers = {d["pointer"] for d in err.value.details}
    assert {"/scenario/messages/0/reply_count"} <= pointers


def test_snapshot_diff_restore_after_sdk_activity(admin, client, calls_since):
    admin.reset(seed="snap-suite")
    general = next(
        c["id"] for c in client.conversations_list()["channels"] if c["name"] == "general"
    )
    admin.create_snapshot("before")

    posted = client.chat_postMessage(channel=general, text="hello snapshot")
    client.reactions_add(channel=general, timestamp=posted["ts"], name="eyes")
    client.chat_update(channel=general, ts=posted["ts"], text="hello snapshot v2")
    created = client.conversations_create(name="snap-room")["channel"]["id"]

    diff = admin.diff("before")
    assert [m["ts"] for m in diff["messages"]["added"]] == [posted["ts"]]
    assert [c["id"] for c in diff["conversations"]["added"]] == [created]
    assert [r["name"] for r in diff["reactions"]["added"]] == ["eyes"]
    assert diff["users"] == {"added": [], "removed": [], "changed": []}

    mid = admin.diff("before", "current")
    assert mid == diff

    restored = admin.restore_snapshot("before")
    assert restored["name"] == "before"
    assert admin.state() == admin.snapshot("before")
    after = admin.diff("before")
    assert all(not s["added"] and not s["removed"] and not s["changed"] for s in after.values())
    assert len(calls_since(method="chat.postMessage")) == 1, "call log survives a restore"
    assert [s["name"] for s in admin.snapshots()] == ["before"]


def test_snapshot_is_reusable_as_a_scenario(admin):
    admin.reset(scenario=acme())
    admin.create_snapshot("acme-snap")
    snap = admin.snapshot("acme-snap")

    admin.reset(seed="elsewhere")
    admin.reset(scenario=snap)

    assert {u["id"] for u in admin.state()["users"]} == {u["id"] for u in snap["users"]}


def test_reset_clears_snapshots(admin):
    admin.create_snapshot("gone-soon")

    admin.reset(seed="clear")

    with pytest.raises(AdminError) as err:
        admin.snapshot("gone-soon")
    assert err.value.status == 404


def test_cli_reset_with_scenario(admin):
    binary = os.environ["LOCALSLACK_BIN"]

    done = subprocess.run(
        [binary, "reset", "--scenario", str(EXAMPLES / "acme.json")],
        capture_output=True,
        text=True,
        timeout=30,
    )

    assert done.returncode == 0, done.stderr
    assert json.loads(done.stdout)["counts"]["users"] == 6
