"""Shared fixtures. Run via `make smoke` (scripts/with-server.sh sets LOCALSLACK_URL)."""

import os
import uuid

import pytest
from slack_sdk import WebClient
from slack_sdk.errors import SlackApiError


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
