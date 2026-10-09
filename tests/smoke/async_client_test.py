"""The asyncio SDK client (aiohttp) must behave like the sync one against LocalSlack."""

import asyncio

from slack_sdk.errors import SlackApiError
from slack_sdk.web.async_client import AsyncWebClient


def run(coro):
    return asyncio.run(coro)


def test_async_round_trip(api_url, unique):
    async def scenario():
        client = AsyncWebClient(token="xoxb-test", base_url=api_url)
        assert (await client.auth_test())["ok"]
        channel = (await client.conversations_create(name=f"as-{unique}"))["channel"]["id"]
        posted = await client.chat_postMessage(channel=channel, text="hello async")
        history = (await client.conversations_history(channel=channel))["messages"]
        assert [(m["ts"], m["text"]) for m in history] == [(posted["ts"], "hello async")]

    run(scenario())


def test_async_pagination_iterator(api_url, unique):
    async def scenario():
        client = AsyncWebClient(token="xoxb-test", base_url=api_url)
        channel = (await client.conversations_create(name=f"asp-{unique}"))["channel"]["id"]
        posted = [
            (await client.chat_postMessage(channel=channel, text=f"m{i}"))["ts"] for i in range(5)
        ]
        walked = []
        async for page in await client.conversations_history(channel=channel, limit=2):
            walked += [m["ts"] for m in page["messages"]]
        assert walked == list(reversed(posted))

    run(scenario())


def test_async_errors_are_slack_api_errors(api_url, unique):
    async def scenario():
        client = AsyncWebClient(token="xoxb-test", base_url=api_url)
        await client.conversations_create(name=f"asd-{unique}")
        try:
            await client.conversations_create(name=f"asd-{unique}")
        except SlackApiError as e:
            assert (e.response.status_code, e.response["error"]) == (200, "name_taken")
        else:
            raise AssertionError("expected name_taken")

    run(scenario())
