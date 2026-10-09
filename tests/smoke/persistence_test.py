"""Data survives a server restart on the same DB (needs scripts/with-server.sh env)."""

import os
import subprocess
import time
import urllib.request

import pytest


def restart_server() -> None:
    binary = os.environ.get("LOCALSLACK_BIN")
    if not binary:
        pytest.skip("run via scripts/with-server.sh (LOCALSLACK_BIN not set)")
    subprocess.run([binary, "stop"], check=True)
    subprocess.run([binary, "start", "-d"], check=True)  # same env => same DB and address
    health = os.environ["LOCALSLACK_URL"] + "/_localslack/health"
    for _ in range(50):
        try:
            urllib.request.urlopen(health, timeout=1).close()
            return
        except OSError:
            time.sleep(0.2)
    raise AssertionError("server did not come back after restart")


def test_data_persists_across_restart(client, unique):
    channel = client.conversations_create(name=f"keep-{unique}")["channel"]["id"]
    before = client.chat_postMessage(channel=channel, text="before")["ts"]

    restart_server()

    history = client.conversations_history(channel=channel)["messages"]
    assert [(m["ts"], m["text"]) for m in history] == [(before, "before")]

    # New ids and timestamps after a restart must not collide with stored ones.
    again = client.conversations_create(name=f"keep2-{unique}")["channel"]["id"]
    assert again != channel
    after = client.chat_postMessage(channel=channel, text="after")["ts"]
    assert after > before
