"""Browser test: the react button posts through the service and the SDK sees it."""

from conftest import channel_id
from playwright.sync_api import Page, expect


def test_react_button_adds_reaction_visible_to_sdk(page: Page, client, unique):
    page.goto("/")
    expect(page.get_by_test_id("channel-title")).to_have_text("#general")
    channel = channel_id(page)
    text = f"react to me {unique}"
    ts = client.chat_postMessage(channel=channel, text=text)["ts"]

    page.reload()
    message = page.get_by_test_id("message").filter(has_text=text)
    message.get_by_test_id("react-button").click()
    message.get_by_test_id("react-option-tada").click()

    expect(message.get_by_test_id("reaction")).to_have_text("🎉 1")
    sent = client.conversations_history(
        channel=channel, latest=ts, inclusive=True, limit=1
    )
    reactions = sent["messages"][0]["reactions"]
    assert [(r["name"], r["count"]) for r in reactions] == [("tada", 1)]
