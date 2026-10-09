"""E2E fixtures. Run via `make e2e` (scripts/with-server.sh sets LOCALSLACK_URL)."""

import os
import uuid

import pytest
from slack_sdk import WebClient


@pytest.fixture(scope="session")
def base_url() -> str:
    """Overrides pytest-base-url so `page.goto("/...")` hits the isolated server."""
    return os.environ.get("LOCALSLACK_URL", "http://localhost:3232")


@pytest.fixture
def client(base_url: str) -> WebClient:
    return WebClient(token="xoxb-test", base_url=f"{base_url}/api/")


@pytest.fixture
def unique() -> str:
    return uuid.uuid4().hex[:8]


def channel_id(page) -> str:
    """The conversation id from the current /ui/c/<id>[...] URL."""
    return page.url.split("/ui/c/")[1].split("/")[0]
