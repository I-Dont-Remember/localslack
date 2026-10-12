"""Demo Bolt app for LocalSlack: mention -> pong, "hello" -> greeting, reaction -> "saw :x:".

Run via `make demo` (credentials come from `localslack apps env`). Environment:
  SLACK_API_URL, SLACK_BOT_TOKEN, SLACK_SIGNING_SECRET  (always)
  SLACK_APP_TOKEN                                        (DEMO_MODE=socket)
  DEMO_MODE=http|socket (default http), DEMO_PORT (default 3000)
"""

from __future__ import annotations

import os
import re

from slack_bolt import App
from slack_sdk import WebClient


def build_app() -> App:
    client = WebClient(
        token=os.environ["SLACK_BOT_TOKEN"], base_url=os.environ["SLACK_API_URL"]
    )
    app = App(client=client, signing_secret=os.environ["SLACK_SIGNING_SECRET"])

    @app.event("app_mention")
    def pong(event, say):
        say(text="pong", thread_ts=event.get("thread_ts") or event["ts"])

    @app.message(re.compile(r"\bhello\b", re.IGNORECASE))
    def greet(message, say):
        if message.get("bot_id"):
            return  # only greet humans, so two bots never greet each other forever
        say(text=f"Hello <@{message['user']}>! :wave:")

    @app.event("reaction_added")
    def saw(event, client):
        item = event["item"]
        client.chat_postMessage(
            channel=item["channel"], text=f"saw :{event['reaction']}:"
        )

    @app.event("message")
    def ignore_other_messages():
        pass  # Bolt warns about unhandled message events otherwise

    return app


def main() -> None:
    app = build_app()
    if os.environ.get("DEMO_MODE", "http") == "socket":
        from slack_bolt.adapter.socket_mode import SocketModeHandler

        SocketModeHandler(app, os.environ["SLACK_APP_TOKEN"]).start()
    else:
        app.start(port=int(os.environ.get("DEMO_PORT", "3000")), path="/slack/events")


if __name__ == "__main__":
    main()
