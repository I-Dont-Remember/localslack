"""Shared fixtures. Run via `make smoke` (scripts/with-server.sh sets LOCALSLACK_URL)."""

import os
import uuid

import pytest
from slack_sdk import WebClient


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
