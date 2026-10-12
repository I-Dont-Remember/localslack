"""Reset suite: POST /_localslack/reset and `localslack reset`.

Reset is global, so these tests must own their server. Run `make smoke-reset` (it sets
LOCALSLACK_RESET_SUITE=1 and starts a dedicated server); `make smoke` skips this file.
"""

import json
import os
import subprocess
import threading
import time

import httpx
import pytest
from slack_sdk import WebClient
from slack_sdk.errors import SlackApiError

from bolt_http import BoltServer
from localslack_admin import AdminError

pytestmark = pytest.mark.skipif(
    os.environ.get("LOCALSLACK_RESET_SUITE") != "1",
    reason="resets global state; run via `make smoke-reset` against a dedicated server",
)

MANIFEST = {
    "display_information": {"name": "reset-suite-app"},
    "features": {"bot_user": {"display_name": "resetbot"}},
    "oauth_config": {"scopes": {"bot": ["chat:write"]}},
}


def user_ids(client) -> list[str]:
    return sorted(u["id"] for u in client.users_list()["members"])


def channel_ids(client) -> list[str]:
    return sorted(c["id"] for c in client.conversations_list()["channels"])


def general(client) -> str:
    return next(c["id"] for c in client.conversations_list()["channels"] if c["name"] == "general")


def register_or_skip(admin):
    try:
        return admin.register_app(MANIFEST, verify=False)
    except AdminError as e:
        if e.status == 404:
            pytest.skip("apps API not available on this build")
        raise


def test_reset_reply_shape(admin):
    got = admin.reset(seed="shape")

    assert got["mode"] == "partial"
    assert got["seed"] == "shape"
    assert got["generation"] >= 1
    assert set(got["counts"]) == {"users", "channels", "messages"}
    assert got["counts"]["users"] > 0 and got["took_ms"] >= 0


def test_same_seed_reproduces_ids(admin, client):
    admin.reset(seed="repro")
    first = (user_ids(client), channel_ids(client))
    admin.reset(seed="something-else")
    assert (user_ids(client), channel_ids(client)) != first

    admin.reset(seed="repro")

    assert (user_ids(client), channel_ids(client)) == first


def test_reset_removes_posted_messages(admin, client):
    admin.reset(seed="wipe")
    channel = general(client)
    client.chat_postMessage(channel=channel, text="before the reset")

    admin.reset(seed="wipe")

    texts = [m["text"] for m in client.conversations_history(channel=general(client))["messages"]]
    assert "before the reset" not in texts


def test_ts_stays_monotonic_across_reset(admin, client):
    admin.reset(seed="ts")
    before = client.chat_postMessage(channel=general(client), text="pre")["ts"]

    admin.reset(seed="ts")
    after = client.chat_postMessage(channel=general(client), text="post")["ts"]

    assert float(after) > float(before)


def test_reset_clears_call_log_and_keeps_seq_monotonic(admin, client):
    client.auth_test()
    before = admin.calls()
    last_seq = before["items"][-1]["seq"]

    admin.reset()
    after_reset = admin.calls()
    client.auth_test()
    after = admin.calls()

    assert after_reset["items"] == []
    assert after_reset["generation"] > before["generation"]
    assert after["items"][0]["seq"] > last_seq


def test_reset_wakes_a_parked_call_poll(admin):
    base = admin.calls()["generation"]
    result: dict = {}

    def poll():
        started = time.monotonic()
        result["page"] = admin.calls(min=1, timeout_ms=20000, since=10**9)
        result["took"] = time.monotonic() - started

    t = threading.Thread(target=poll)
    t.start()
    time.sleep(0.3)  # let the poll park
    admin.reset()
    t.join(timeout=10)

    assert not t.is_alive(), "poll stayed parked after reset"
    assert result["took"] < 8
    assert result["page"]["generation"] > base


def test_partial_reset_keeps_apps_and_all_removes_them(admin):
    app = register_or_skip(admin)
    token = app["credentials"]["bot_token"]
    app_client = httpx.Client(base_url=admin.base_url, headers={"Authorization": f"Bearer {token}"})

    admin.reset(seed="keep-apps")

    assert app["id"] in [a["id"] for a in admin.apps()]
    kept = app_client.post("/api/auth.test").json()
    assert kept["ok"] and kept["user_id"] == app["bot_user_id"]

    admin.reset(all=True, seed="keep-apps")

    assert admin.apps() == []
    gone = app_client.post("/api/auth.test").json()
    assert gone.get("user_id") != app["bot_user_id"]


def test_event_is_delivered_to_a_kept_app_after_reset(admin, client, unique):
    server = BoltServer()  # no Bolt app: handshake only; any 404 still records the delivery attempt
    manifest = {
        **MANIFEST,
        "display_information": {"name": f"rejoin-{unique}"},
        "settings": {
            "event_subscriptions": {"request_url": server.url, "bot_events": ["message.channels"]}
        },
    }
    try:
        app = admin.register_app(manifest)
        admin.reset(seed=f"rejoin-{unique}")
        human = next(u["id"] for u in client.users_list()["members"] if not u["is_bot"])
        poster = WebClient(token=f"xoxp-{human}", base_url=client.base_url)

        poster.chat_postMessage(channel=general(client), text="after reset")

        got = admin.wait_deliveries(1, app_id=app["id"], event_type="message")
        assert got[0]["app_id"] == app["id"]
    finally:
        admin.delete_app(app["id"])
        server.close()


def test_reset_clears_faults(admin, client):
    try:
        admin.add_fault("auth.test", {"status": 500, "error": "boom"})
    except AdminError as e:
        if e.status == 404:
            pytest.skip("faults API not available on this build")
        raise

    admin.reset()

    assert admin.faults() == []
    client.auth_test()  # no longer faulted


def test_posting_while_resetting_never_errors(admin, client):
    admin.reset(seed="storm")
    stop = threading.Event()
    failures: list[str] = []

    def poster():
        while not stop.is_set():
            try:
                client.chat_postMessage(channel=general(client), text="load")
            except SlackApiError as e:
                if e.response["error"] != "channel_not_found":  # raced a wipe: legal
                    failures.append(e.response["error"])
            except Exception as e:  # noqa: BLE001 - any transport error is a failure
                failures.append(repr(e))

    threads = [threading.Thread(target=poster) for _ in range(3)]
    for t in threads:
        t.start()
    for _ in range(5):
        admin.reset(seed="storm")
    stop.set()
    for t in threads:
        t.join(timeout=10)

    assert failures == []
    assert client.chat_postMessage(channel=general(client), text="alive")["ok"]


def test_concurrent_resets_reject_the_loser_with_409(admin):
    outcomes: list[object] = []

    def go():
        try:
            outcomes.append(admin.reset(seed="race"))
        except AdminError as e:
            outcomes.append(e)

    threads = [threading.Thread(target=go) for _ in range(8)]
    for t in threads:
        t.start()
    for t in threads:
        t.join(timeout=20)

    losers = [o for o in outcomes if isinstance(o, AdminError)]
    assert len(outcomes) == 8
    assert any(not isinstance(o, AdminError) for o in outcomes), "someone must win"
    assert all(e.status == 409 and e.code == "reset_in_progress" for e in losers)


def cli(*args: str) -> subprocess.CompletedProcess:
    binary = os.environ["LOCALSLACK_BIN"]
    return subprocess.run([binary, "reset", *args], capture_output=True, text=True, timeout=30)


def test_cli_reset_prints_the_reply(admin):
    done = cli("--seed", "from-cli")

    assert done.returncode == 0, done.stderr
    reply = json.loads(done.stdout)
    assert reply["mode"] == "partial" and reply["seed"] == "from-cli"


def test_cli_reset_all(admin):
    done = cli("--all", "--seed", "from-cli")

    assert done.returncode == 0, done.stderr
    assert json.loads(done.stdout)["mode"] == "all"


def test_cli_reports_an_unreachable_server_clearly():
    binary = os.environ["LOCALSLACK_BIN"]

    done = subprocess.run(
        [binary, "reset", "--addr", "127.0.0.1:1"], capture_output=True, text=True, timeout=30
    )

    assert done.returncode != 0
    assert "is it running" in done.stderr
