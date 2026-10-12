"""Browser tests: /ui/inspector shows SDK calls, faults, apps, deliveries and resets."""

import httpx
import pytest
from playwright.sync_api import Page, expect
from slack_sdk.errors import SlackApiError

POLL = 6_000  # ms; the inspector polls every second


@pytest.fixture
def admin(base_url: str):
    """Admin API client that removes the faults and apps a test made, even on failure."""
    c = httpx.Client(base_url=f"{base_url}/_localslack")
    apps: list[str] = []
    c.created_apps = apps  # type: ignore[attr-defined]
    yield c
    c.delete("/faults")
    for app_id in apps:
        c.delete(f"/apps/{app_id}")
    c.close()


def register(admin: httpx.Client, name: str, request_url: str, **extra) -> dict:
    manifest = {
        "display_information": {"name": name},
        "features": {"bot_user": {"display_name": name}},
        "settings": {
            "event_subscriptions": {
                "request_url": request_url,
                "bot_events": ["message.channels"],
            }
        },
    }
    resp = admin.post("/apps", json={"manifest": manifest, "verify": False, **extra})
    assert resp.status_code == 201, resp.text
    app = resp.json()
    admin.created_apps.append(app["id"])  # type: ignore[attr-defined]
    return app


def open_inspector(page: Page) -> None:
    page.goto("/ui/inspector")
    expect(page.get_by_test_id("inspector-calls")).to_be_visible()


def test_nav_link_reaches_inspector(page: Page):
    page.goto("/")
    page.get_by_test_id("nav-inspector").click()
    expect(page).to_have_url(f"{page.url.split('/ui')[0].rstrip('/')}/ui/inspector")
    expect(page.get_by_test_id("inspector-sockets")).to_contain_text(
        "available when Socket Mode"
    )


def test_sdk_call_appears_with_params_and_response(page: Page, client, unique):
    open_inspector(page)
    channel = client.conversations_list(limit=1)["channels"][0]["id"]
    client.chat_postMessage(channel=channel, text=f"inspect me {unique}")

    row = page.get_by_test_id("call-row").filter(has_text="chat.postMessage").first
    expect(row).to_be_visible(timeout=POLL)
    row.get_by_test_id("call-method").click()  # expand
    expect(row).to_contain_text(f"inspect me {unique}", timeout=POLL)


def test_unknown_method_gets_badge(page: Page, base_url: str):
    open_inspector(page)
    httpx.post(f"{base_url}/api/no.such.method", data={"token": "xoxb-test"})
    expect(page.get_by_test_id("badge-not-implemented").first).to_be_visible(
        timeout=POLL
    )


def test_add_fault_from_form_marks_call(page: Page, client, admin, unique):
    open_inspector(page)
    form = page.get_by_test_id("fault-form")
    form.get_by_placeholder("method or *").fill("chat.postMessage")
    form.get_by_placeholder("error code").fill("fatal_error")
    form.get_by_placeholder("times").fill("1")
    page.get_by_test_id("fault-add").click()
    row = page.get_by_test_id("fault-row")
    expect(row).to_contain_text("chat.postMessage", timeout=POLL)

    channel = client.conversations_list(limit=1)["channels"][0]["id"]
    with pytest.raises(SlackApiError) as err:
        client.chat_postMessage(channel=channel, text=f"faulted {unique}")
    assert err.value.response["error"] == "fatal_error"

    expect(page.get_by_test_id("badge-fault").first).to_be_visible(timeout=POLL)
    expect(row.get_by_test_id("fault-fired")).to_have_text("1", timeout=POLL)

    row.get_by_test_id("fault-delete").click()
    expect(page.get_by_test_id("fault-row")).to_have_count(0, timeout=POLL)


def test_registered_apps_show_ready_and_blocked(page: Page, admin, unique):
    blocked = register(admin, f"Remote {unique}", "https://example.com/slack/events")
    ready = register(admin, f"Local {unique}", "http://localhost:9/slack/events")
    open_inspector(page)

    blocked_row = page.locator(f'[data-app-id="{blocked["id"]}"]')
    ready_row = page.locator(f'[data-app-id="{ready["id"]}"]')
    expect(blocked_row.get_by_test_id("app-state")).to_have_text(
        "blocked", timeout=POLL
    )
    expect(blocked_row).to_contain_text("not loopback/private")
    expect(ready_row.get_by_test_id("app-state")).to_have_text("ready")

    blocked_row.get_by_test_id("deliver-to").fill("http://localhost:3000")
    blocked_row.get_by_test_id("deliver-to-save").click()
    expect(blocked_row.get_by_test_id("app-state")).to_have_text("ready", timeout=POLL)
    expect(blocked_row.get_by_test_id("app-url")).to_have_text(
        "http://localhost:3000/slack/events"
    )

    blocked_row.get_by_test_id("reveal-credentials").click()
    expect(blocked_row.get_by_test_id("credentials")).to_contain_text(
        blocked["credentials"]["bot_token"]
    )


def test_injected_event_shows_delivery(page: Page, admin, unique):
    app = register(admin, f"Hook {unique}", "http://127.0.0.1:9/slack/events")
    open_inspector(page)
    resp = admin.post(
        "/events",
        json={"app_id": app["id"], "event": {"type": "message", "text": unique}},
    )
    assert resp.status_code == 201, resp.text
    event_id = resp.json()["deliveries"][0]["event_id"]

    row = page.locator(f'[data-testid="delivery-row"][data-event-id="{event_id}"]')
    expect(row).to_be_visible(timeout=POLL)
    expect(row.get_by_test_id("delivery-state")).to_have_text("failed", timeout=15_000)
    row.locator("summary").click()
    expect(row.get_by_test_id("attempt").first).to_be_visible()
    expect(row).to_contain_text(unique)


def test_delivery_config_toggles(page: Page, admin):
    open_inspector(page)
    try:
        page.get_by_test_id("sig-stale").click()
        expect(page.get_by_test_id("sig-stale")).to_have_class("on", timeout=POLL)
        page.get_by_test_id("toggle-pause").click()
        expect(page.get_by_test_id("paused-state")).to_contain_text(
            "paused", timeout=POLL
        )
        assert admin.get("/delivery-config").json()["paused"] is True
    finally:
        admin.put("/delivery-config", json={})
    expect(page.get_by_test_id("paused-state")).to_contain_text("running", timeout=POLL)
    expect(page.get_by_test_id("events-dropped")).to_have_text("0")


def test_reset_from_page_clears_calls(page: Page, client):
    client.auth_test()
    open_inspector(page)
    expect(page.get_by_test_id("call-row").first).to_be_visible(timeout=POLL)

    page.once("dialog", lambda d: d.accept())
    page.get_by_test_id("reset-partial").click()

    expect(page.get_by_test_id("reset-result")).to_contain_text("partial", timeout=POLL)
    expect(page.get_by_test_id("call-row")).to_have_count(0, timeout=POLL)
