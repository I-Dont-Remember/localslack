"""Fault injection (/_localslack/faults) exercised through the real Slack SDK."""

import time

import pytest
from slack_sdk import WebClient
from slack_sdk.errors import SlackApiError
from slack_sdk.http_retry.builtin_handlers import (
    RateLimitErrorRetryHandler,
    ServerErrorRetryHandler,
)

from conftest import error_of
from localslack_admin import AdminError


def _client(base_url: str, **kwargs) -> WebClient:
    return WebClient(token="xoxb-test", base_url=f"{base_url}/api/", **kwargs)


def test_rate_limit_handler_retries_after_retry_after(faults, base_url):
    rule = faults.add_fault("auth.test", {"status": 429, "retry_after": 1}, once=True)
    client = _client(base_url, retry_handlers=[RateLimitErrorRetryHandler(max_retry_count=1)])

    start = time.monotonic()
    resp = client.auth_test()

    assert resp["ok"]
    assert time.monotonic() - start >= 1.0, "SDK must wait Retry-After before retrying"
    (got,) = [f for f in faults.faults() if f["id"] == rule["id"]]
    assert (got["seen"], got["fired"], got["spent"]) == (1, 1, True)


def test_rate_limit_without_handler_surfaces_429(faults, base_url):
    faults.add_fault("auth.test", {"status": 429, "retry_after": 3}, once=True)
    client = _client(base_url, retry_handlers=[])

    with pytest.raises(SlackApiError) as exc_info:
        client.auth_test()

    resp = exc_info.value.response
    assert resp.status_code == 429
    assert resp.headers["Retry-After"] == "3"
    assert resp["error"] == "ratelimited"


def test_server_error_handler_retries_on_500(faults, base_url):
    rule = faults.add_fault("auth.test", {"status": 500}, times=2)
    client = _client(base_url, retry_handlers=[ServerErrorRetryHandler(max_retry_count=3)])

    resp = client.auth_test()

    assert resp["ok"]
    (got,) = [f for f in faults.faults() if f["id"] == rule["id"]]
    assert (got["seen"], got["fired"]) == (2, 2)


def test_500_without_handler_is_internal_error(faults, base_url):
    faults.add_fault("auth.test", {"status": 500}, once=True)

    status, code = error_of(_client(base_url, retry_handlers=[]).auth_test)

    assert (status, code) == (500, "internal_error")


def test_slack_error_with_200(faults, base_url):
    faults.add_fault("auth.test", {"error": "fatal_error"}, once=True)

    status, code = error_of(_client(base_url, retry_handlers=[]).auth_test)

    assert (status, code) == (200, "fatal_error")


def test_delay_makes_sdk_time_out(faults, base_url):
    faults.add_fault("auth.test", {"delay_ms": 1500}, once=True)
    client = _client(base_url, timeout=1, retry_handlers=[])

    with pytest.raises(Exception) as exc_info:  # urllib surfaces socket timeouts variously
        client.auth_test()

    assert "timed out" in str(exc_info.value).lower()
    assert client.auth_test()["ok"], "once-rule is spent; next call is normal"


def test_delay_alone_runs_the_real_method(faults, base_url):
    faults.add_fault("auth.test", {"delay_ms": 200}, once=True)

    start = time.monotonic()
    resp = _client(base_url, retry_handlers=[]).auth_test()

    assert resp["ok"] and resp["user_id"]
    assert time.monotonic() - start >= 0.2


def test_nth_3_affects_only_third_call(faults, base_url):
    faults.add_fault("auth.test", {"status": 500}, nth=3)
    client = _client(base_url, retry_handlers=[])

    outcomes = []
    for _ in range(5):
        try:
            client.auth_test()
            outcomes.append(200)
        except SlackApiError as e:
            outcomes.append(e.response.status_code)

    assert outcomes == [200, 200, 500, 200, 200]


def test_faulted_call_never_runs_the_method(faults, client, unique):
    channel = client.conversations_create(name=f"fault-{unique}")["channel"]["id"]
    faults.add_fault("chat.postMessage", {"error": "fatal_error"}, once=True)

    with pytest.raises(SlackApiError):
        client.chat_postMessage(channel=channel, text="never stored")

    texts = [m["text"] for m in client.conversations_history(channel=channel)["messages"]]
    assert "never stored" not in texts


def test_match_channel_only_faults_that_channel(faults, client, unique):
    hit = client.conversations_create(name=f"hit-{unique}")["channel"]["id"]
    miss = client.conversations_create(name=f"miss-{unique}")["channel"]["id"]
    faults.add_fault("chat.postMessage", {"error": "fatal_error"}, match={"channel": hit})

    assert client.chat_postMessage(channel=miss, text="ok")["ok"]
    with pytest.raises(SlackApiError):
        client.chat_postMessage(channel=hit, text="blocked")


def test_bad_token_is_still_faulted(faults, base_url):
    faults.add_fault("auth.test", {"status": 503}, once=True)
    client = WebClient(token="not-a-token", base_url=f"{base_url}/api/", retry_handlers=[])

    status, _ = error_of(client.auth_test)

    assert status == 503


def test_validation_errors_are_422_with_pointers(faults):
    with pytest.raises(AdminError) as exc_info:
        faults.add_fault("auth.test", {"status": 302})

    assert exc_info.value.status == 422
    assert [d["pointer"] for d in exc_info.value.details] == ["/action/status"]


def test_delete_fault_stops_it(faults, base_url):
    rule = faults.add_fault("auth.test", {"status": 500})
    client = _client(base_url, retry_handlers=[])
    assert error_of(client.auth_test)[0] == 500

    faults.delete_fault(rule["id"])

    assert client.auth_test()["ok"]
    with pytest.raises(AdminError) as exc_info:
        faults.delete_fault(rule["id"])
    assert exc_info.value.status == 404


def test_fault_id_in_call_log(faults, base_url):
    try:
        cursor = faults.head_seq()
    except AdminError as e:
        if e.status == 404:
            pytest.skip("/calls not implemented yet (stream C)")
        raise
    rule = faults.add_fault("auth.test", {"status": 500}, once=True)
    client = _client(base_url, retry_handlers=[])

    error_of(client.auth_test)
    client.auth_test()

    calls = faults.wait_calls(2, since=cursor, method="auth.test")
    assert [c["fault_id"] for c in calls] == [rule["id"], ""]
    assert calls[0]["status"] == 500
