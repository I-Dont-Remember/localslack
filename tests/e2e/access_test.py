"""Browser tests for access rules: UI auto-join on post, lock icon on private channels."""

from playwright.sync_api import Page, expect
from slack_sdk import WebClient


def ui_human(page: Page, client: WebClient, base_url: str) -> WebClient:
    """SDK client for the user the UI acts as (first human, the default actor)."""
    page.goto("/")
    human = next(u["id"] for u in client.users_list()["members"] if not u["is_bot"])
    return WebClient(token=f"xoxp-{human}", base_url=f"{base_url}/api/")


def test_ui_post_to_sdk_created_channel_auto_joins(page: Page, client, unique):
    channel = client.conversations_create(name=f"ui-join-{unique}")["channel"][
        "id"
    ]  # bot-only
    page.goto(f"/ui/c/{channel}")
    text = f"ui joins {unique}"
    page.get_by_test_id("composer-input").fill(text)
    page.get_by_test_id("send").click()

    expect(page.get_by_test_id("messages").get_by_text(text)).to_be_visible()
    members = client.conversations_members(channel=channel)["members"]
    assert len(members) == 2  # the bot and the UI actor


def test_private_channel_shows_lock_for_members_only(
    page: Page, client, base_url, unique
):
    human = ui_human(page, client, base_url)
    mine = human.conversations_create(name=f"lock-{unique}", is_private=True)[
        "channel"
    ]["id"]
    client.conversations_create(name=f"hidden-{unique}", is_private=True)  # bot-only

    page.goto(f"/ui/c/{mine}")
    link = page.get_by_test_id("channel-link").filter(has_text=f"lock-{unique}")
    expect(link.get_by_test_id("private-lock")).to_be_visible()
    expect(
        page.get_by_test_id("channel-link").filter(has_text=f"hidden-{unique}")
    ).to_have_count(0)
