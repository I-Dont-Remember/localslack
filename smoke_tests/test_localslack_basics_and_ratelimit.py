#!/usr/bin/env python3
import os
import sys
import time
from typing import Any, Dict, List, Callable

from slack_sdk import WebClient
from slack_sdk.errors import SlackApiError


SLACK_API_BASE_URL = os.getenv("SLACK_API_BASE_URL", "http://localhost:4000/api")
SLACK_BOT_TOKEN = os.getenv("SLACK_BOT_TOKEN", "xoxb-localslack-test-token")

# how many attempts before we give up on seeing a rate limit
MAX_RATE_ATTEMPTS = int(os.getenv("LOCALSLACK_RATE_MAX_ATTEMPTS", "50"))
# cap how long we'll actually sleep, even if Retry-After is large
MAX_RETRY_AFTER_SLEEP = int(os.getenv("LOCALSLACK_RATE_MAX_SLEEP", "5"))


def fail(msg: str) -> None:
    print(f"[FAIL] {msg}")
    sys.exit(1)


def ok(msg: str) -> None:
    print(f"[ OK ] {msg}")


def new_client() -> WebClient:
    """
    Configure the Slack Python SDK client to talk to the mock server instead of
    https://slack.com/api.
    """
    return WebClient(
        token=SLACK_BOT_TOKEN,
        base_url=SLACK_API_BASE_URL,
    )


def require_ok(resp: Dict[str, Any], context: str) -> None:
    if not resp.get("ok"):
        fail(f"{context}: ok=false, error={resp.get('error')}")


# -------------------------
# Basic happy-path tests
# -------------------------


def test_auth_test(client: WebClient) -> Dict[str, Any]:
    resp = client.auth_test()
    require_ok(resp, "auth.test")

    for field in ("user_id", "team_id"):
        if field not in resp:
            fail(f"auth.test: missing field '{field}'")

    ok("auth.test")
    return resp


def test_users_list(client: WebClient) -> List[Dict[str, Any]]:
    resp = client.users_list()
    require_ok(resp, "users.list")

    members = resp.get("members", [])
    if not isinstance(members, list):
        fail("users.list: 'members' is not a list")

    if not members:
        print("[WARN] users.list: no members returned")
    else:
        first = members[0]
        for field in ("id", "name"):
            if field not in first:
                fail(f"users.list: first member missing '{field}'")

    ok(f"users.list (count={len(members)})")
    return members


def test_conversations_list(client: WebClient) -> List[Dict[str, Any]]:
    resp = client.conversations_list()
    require_ok(resp, "conversations.list")

    chans = resp.get("channels", [])
    if not isinstance(chans, list):
        fail("conversations.list: 'channels' is not a list")

    if not chans:
        print("[WARN] conversations.list: no channels returned")
    else:
        first = chans[0]
        for field in ("id", "name"):
            if field not in first:
                fail(f"conversations.list: first channel missing '{field}'")

    ok(f"conversations.list (count={len(chans)})")
    return chans


def test_conversations_create(client: WebClient, name: str) -> Dict[str, Any]:
    resp = client.conversations_create(name=name)
    require_ok(resp, "conversations.create")

    chan = resp.get("channel") or {}
    for field in ("id", "name"):
        if field not in chan:
            fail(f"conversations.create: channel missing '{field}'")

    ok(f"conversations.create ({chan['id']} -> {chan['name']})")
    return chan


def test_chat_post_message(client: WebClient, channel_id: str) -> Dict[str, Any]:
    resp = client.chat_postMessage(channel=channel_id, text="Hello from LocalSlack test")
    require_ok(resp, "chat.postMessage")

    for field in ("channel", "ts", "message"):
        if field not in resp:
            fail(f"chat.postMessage: missing '{field}' in response")

    msg = resp["message"]
    if "text" not in msg:
        fail("chat.postMessage: message missing 'text'")

    ok(f"chat.postMessage (channel={resp['channel']}, ts={resp['ts']})")
    return resp


def test_chat_update(client: WebClient, channel_id: str, ts: str) -> Dict[str, Any]:
    resp = client.chat_update(
        channel=channel_id,
        ts=ts,
        text="Hello from LocalSlack test (edited)",
    )
    require_ok(resp, "chat.update")

    for field in ("channel", "ts", "text"):
        if field not in resp:
            fail(f"chat.update: missing '{field}' in response")

    ok(f"chat.update (channel={resp['channel']}, ts={resp['ts']})")
    return resp


# -------------------------
# Rate limit hammer tests
# -------------------------


def hammer_for_rate_limit(
    method_label: str,
    call_fn: Callable[[], Dict[str, Any]],
) -> None:
    """
    Call `call_fn` repeatedly until:
      - we see a SlackApiError with HTTP 429 and error='ratelimited', OR
      - we hit MAX_RATE_ATTEMPTS and fail.

    Then we:
      - verify Retry-After header is present and positive
      - sleep up to MAX_RETRY_AFTER_SLEEP seconds
      - ensure a subsequent call succeeds (ok=true).
    """
    print(f"--- Hammering {method_label} for rate limiting ---")

    for i in range(1, MAX_RATE_ATTEMPTS + 1):
        try:
            resp = call_fn()
            # We *expect* some calls to succeed before limit hits.
            require_ok(resp, f"{method_label} attempt {i}")
        except SlackApiError as e:
            resp = e.response
            if resp is None:
                fail(f"{method_label}: SlackApiError with no response: {e}")

            status = getattr(resp, "status_code", None)
            data = getattr(resp, "data", {}) or {}

            if status == 429:
                error_code = data.get("error")
                if error_code != "ratelimited":
                    fail(
                        f"{method_label}: got 429 but error='{error_code}', "
                        "expected 'ratelimited'"
                    )

                retry_after_raw = resp.headers.get("Retry-After", "0")
                try:
                    retry_after = int(retry_after_raw)
                except ValueError:
                    fail(
                        f"{method_label}: Retry-After header not an int: "
                        f"{retry_after_raw!r}"
                    )

                if retry_after <= 0:
                    fail(
                        f"{method_label}: Retry-After not positive: "
                        f"{retry_after_raw!r}"
                    )

                ok(
                    f"{method_label}: hit rate limit on attempt {i} "
                    f"(Retry-After={retry_after}s)"
                )

                sleep_for = min(retry_after, MAX_RETRY_AFTER_SLEEP)
                print(f"{method_label}: sleeping {sleep_for}s before retry...")
                time.sleep(sleep_for)

                # After sleeping, we expect to be allowed again
                resp2 = call_fn()
                require_ok(resp2, f"{method_label} after waiting Retry-After")
                ok(f"{method_label}: recovered after waiting Retry-After")
                return
            else:
                fail(
                    f"{method_label}: unexpected SlackApiError status "
                    f"{status}, data={data}"
                )

    fail(
        f"{method_label}: never hit rate limit after {MAX_RATE_ATTEMPTS} "
        "attempts – check your mock config"
    )


def test_rate_limit_chat_postMessage(client: WebClient, channel_id: str) -> None:
    def call_fn() -> Dict[str, Any]:
        return client.chat_postMessage(
            channel=channel_id,
            text="Spam from LocalSlack rate test",
        )

    hammer_for_rate_limit("chat.postMessage", call_fn)


def test_rate_limit_conversations_list(client: WebClient) -> None:
    def call_fn() -> Dict[str, Any]:
        # limit doesn't really matter here, we just want calls to increment
        return client.conversations_list(limit=10)

    hammer_for_rate_limit("conversations.list", call_fn)


# -------------------------
# Main
# -------------------------


def main() -> None:
    print("=== LocalSlack basic + rate limit SDK test ===")
    print(f"Using SLACK_API_BASE_URL={SLACK_API_BASE_URL}")
    print(f"Using SLACK_BOT_TOKEN={SLACK_BOT_TOKEN!r}")
    client = new_client()

    try:
        # Basic flow first to ensure everything works
        auth = test_auth_test(client)
        team_id = auth["team_id"]

        _ = test_users_list(client)
        _ = test_conversations_list(client)

        ts_suffix = int(time.time())
        channel_name = f"localslack-test-{team_id.lower()}-{ts_suffix}"
        created_channel = test_conversations_create(client, channel_name)
        channel_id = created_channel["id"]

        msg_resp = test_chat_post_message(client, channel_id)
        msg_ts = msg_resp["ts"]
        _ = test_chat_update(client, channel_id, msg_ts)

        # Now hammer for rate limits on two endpoints
        test_rate_limit_chat_postMessage(client, channel_id)
        test_rate_limit_conversations_list(client)

    except SlackApiError as e:
        fail(f"SlackApiError from SDK: {e.response if e.response is not None else e}")
    except Exception as e:
        fail(f"Unhandled exception: {e}")

    print("=== All tests (basic + rate limits) passed ===")
    sys.exit(0)


if __name__ == "__main__":
    main()

