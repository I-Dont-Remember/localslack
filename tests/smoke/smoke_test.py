"""Smoke tests: full-stack SDK compatibility against real running binary."""

import os

import httpx
import pytest
from slack_sdk import WebClient
from slack_sdk.errors import SlackApiError

BASE_URL = os.environ.get("LOCALSLACK_URL", "http://localhost:3232")


def test_health():
    resp = httpx.get(f"{BASE_URL}/_localslack/health")
    assert resp.status_code == 200
    assert resp.json() == {"ok": True}


def test_unknown_endpoint_returns_not_implemented():
    client = WebClient(token="xoxb-test", base_url=f"{BASE_URL}/")
    with pytest.raises(SlackApiError) as exc_info:
        client.chat_postMessage(channel="#general", text="hello")
    assert exc_info.value.response["error"] == "not_implemented"
