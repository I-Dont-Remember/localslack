"""Smoke tests: full-stack SDK compatibility against a real running binary."""

import httpx
import pytest
from slack_sdk.errors import SlackApiError


def test_health(base_url):
    resp = httpx.get(f"{base_url}/_localslack/health")
    assert resp.status_code == 200
    assert resp.json() == {"ok": True}


def test_unknown_method_returns_not_implemented(client):
    with pytest.raises(SlackApiError) as exc_info:
        client.api_call("does.notExist")
    assert exc_info.value.response["error"] == "not_implemented"
