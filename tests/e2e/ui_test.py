"""Browser tests: the htmx UI and the Slack SDK see the same data."""

from conftest import channel_id
from playwright.sync_api import Page, expect

POLL = 6_000  # ms; the UI polls every 2s


def open_general(page: Page) -> str:
    page.goto("/")
    expect(page.get_by_test_id("channel-title")).to_have_text("#general")
    return channel_id(page)


def test_seeded_channels_visible(page: Page):
    open_general(page)
    channels = page.get_by_test_id("channel-link")
    expect(channels.first).to_be_visible()
    assert channels.count() >= 2
    expect(
        page.get_by_test_id("channel-link").filter(has_text="#general")
    ).to_be_visible()
    assert page.get_by_test_id("message").count() >= 3


def test_ui_post_visible_to_sdk(page: Page, client, unique):
    channel = open_general(page)
    text = f"from the ui {unique}"
    page.get_by_test_id("composer-input").fill(text)
    page.get_by_test_id("send").click()

    expect(page.get_by_test_id("messages").get_by_text(text)).to_be_visible()
    expect(page.get_by_test_id("composer-input")).to_have_value("")  # composer resets
    history = client.conversations_history(channel=channel, limit=5)["messages"]
    assert text in [m["text"] for m in history]


def test_sdk_post_appears_in_ui(page: Page, client, unique):
    channel = open_general(page)
    text = f"from the sdk {unique}"
    client.chat_postMessage(channel=channel, text=text)
    expect(page.get_by_test_id("messages").get_by_text(text)).to_be_visible(
        timeout=POLL
    )


def test_thread_reply_roundtrip(page: Page, client, unique):
    channel = open_general(page)
    root_text, reply_text = f"root {unique}", f"reply {unique}"
    root = client.chat_postMessage(channel=channel, text=root_text)["ts"]

    page.reload()
    message = page.get_by_test_id("message").filter(has_text=root_text)
    message.get_by_test_id("thread-link").click()
    pane = page.get_by_test_id("thread-pane")
    expect(pane).to_be_visible()
    pane.get_by_test_id("reply-input").fill(reply_text)
    pane.get_by_test_id("reply-send").click()
    expect(pane.get_by_text(reply_text)).to_be_visible()

    replies = client.conversations_replies(channel=channel, ts=root)["messages"]
    assert [m["text"] for m in replies] == [root_text, reply_text]
    # Top-level view shows the reply count, not the reply itself.
    page.goto(f"/ui/c/{channel}")
    expect(page.get_by_test_id("message").filter(has_text=root_text)).to_contain_text(
        "1 reply"
    )
    expect(page.get_by_test_id("messages").get_by_text(reply_text)).to_have_count(0)


def test_dm_via_quick_switcher(page: Page, client, unique):
    open_general(page)
    page.get_by_test_id("switcher-input").fill("localslack-bot")
    page.get_by_test_id("switcher-result").filter(
        has_text="@localslack-bot"
    ).get_by_role("button").click()
    page.wait_for_url("**/ui/c/D*")
    dm = channel_id(page)
    expect(
        page.get_by_test_id("dm-link").filter(has_text="localslack-bot")
    ).to_be_visible()

    text = f"dm {unique}"
    page.get_by_test_id("composer-input").fill(text)
    page.get_by_test_id("send").click()
    expect(page.get_by_test_id("messages").get_by_text(text)).to_be_visible()

    assert dm in {c["id"] for c in client.conversations_list(types="im")["channels"]}
    assert client.conversations_history(channel=dm)["messages"][0]["text"] == text


def test_acting_as_changes_author(page: Page, client, unique):
    channel = open_general(page)
    select = page.get_by_test_id("acting-as")
    options = select.locator("option")
    users = [(o.get_attribute("value"), o.text_content()) for o in options.all()]
    chosen = next(u for u in users if "(bot)" in u[1])  # differs from the default human
    select.select_option(chosen[0])
    page.wait_for_load_state()

    text = f"as bot {unique}"
    page.get_by_test_id("composer-input").fill(text)
    page.get_by_test_id("send").click()
    expect(page.get_by_test_id("messages").get_by_text(text)).to_be_visible()
    newest = client.conversations_history(channel=channel, limit=1)["messages"][0]
    assert (newest["text"], newest["user"]) == (text, chosen[0])
