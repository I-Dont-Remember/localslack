"""Smoke tests: full-stack SDK compatibility against a real running binary."""

import httpx
import pytest
from slack_sdk.errors import SlackApiError


def test_health(base_url):
    resp = httpx.get(f"{base_url}/_localslack/health")
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "ok"
    assert isinstance(body["events_dropped"], int)


def test_unknown_method_returns_unknown_method(client):
    with pytest.raises(SlackApiError) as exc_info:
        client.api_call("does.notExist")
    assert exc_info.value.response["error"] == "unknown_method"
