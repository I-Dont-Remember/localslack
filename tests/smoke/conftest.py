"""Shared fixtures. Run via `make smoke` (scripts/with-server.sh sets LOCALSLACK_URL)."""

import os
import uuid

import pytest
from slack_sdk import WebClient
from slack_sdk.errors import SlackApiError

from localslack_admin import Admin, AdminError


@pytest.fixture(scope="session")
def base_url() -> str:
    return os.environ.get("LOCALSLACK_URL", "http://localhost:3232")


@pytest.fixture
def client(base_url: str) -> WebClient:
    return WebClient(token="xoxb-test", base_url=f"{base_url}/api/")


@pytest.fixture
def unique() -> str:
    """Short unique suffix so tests don't collide on names within one server."""
    return uuid.uuid4().hex[:8]


def deviation(deviation_id: str):
    """Mark a test that encodes real Slack behavior LocalSlack does not match yet.

    `deviation_id` must exist under [deviations.*] in internal/capabilities/capabilities.toml
    (a Go test checks this). The test is a strict xfail: when the gap is fixed it XPASSes and the
    run fails until you delete this marker and the manifest entry.
    """
    return pytest.mark.xfail(
        strict=True,
        raises=(SlackApiError, AssertionError),
        reason=f"known deviation: {deviation_id}",
    )


def error_of(call) -> tuple[int, str]:
    """Run an SDK call that must fail; return (HTTP status, Slack error code)."""
    try:
        call()
    except SlackApiError as e:
        return e.response.status_code, e.response["error"]
    raise AssertionError("expected the call to fail with SlackApiError")


@pytest.fixture
def api_url(base_url: str) -> str:
    return f"{base_url}/api/"


def humans(client: WebClient) -> list[str]:
    return [u["id"] for u in client.users_list()["members"] if not u["is_bot"]]


@pytest.fixture
def admin(base_url: str):
    """Client for the /_localslack testing API."""
    a = Admin(base_url)
    yield a
    a.close()


@pytest.fixture
def calls_since(admin: Admin):
    """Call log scoped to this test: a function returning calls recorded since the fixture ran.

    `calls_since(min=1, timeout_ms=2000, method=...)` long-polls for `min` matching calls.
    """
    cursor = admin.head_seq()

    def fetch(min: int | None = None, timeout_ms: int = 5000, **filters):
        if min:
            return admin.wait_calls(min, timeout_ms, since=cursor, **filters)
        return admin.calls(since=cursor, **filters)["items"]

    return fetch


@pytest.fixture
def faults(admin: Admin):
    """Fault rules for one test; teardown clears them and restores the delivery config."""
    try:
        saved = admin.delivery_config()
    except AdminError:
        saved = None  # endpoint not implemented yet: nothing to restore
    yield admin
    for cleanup in (
        admin.delete_faults,
        lambda: saved and admin.put_delivery_config(**saved),
    ):
        try:
            cleanup()
        except AdminError:
            pass
