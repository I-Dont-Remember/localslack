"""A second, tiny Bolt app so you can watch two apps share one workspace.

- "echo <text>" in a channel -> replies "<text>".
- It puts a :robot_face: on any other bot's "pong", which the demo app then notices (reaction_added).

Same environment as app.py, with ECHO_PORT (default 3001) instead of DEMO_PORT.
"""

from __future__ import annotations

import os

from slack_bolt import App
from slack_sdk import WebClient


def build_app() -> App:
    client = WebClient(
        token=os.environ["SLACK_BOT_TOKEN"], base_url=os.environ["SLACK_API_URL"]
    )
    app = App(client=client, signing_secret=os.environ["SLACK_SIGNING_SECRET"])

    @app.message("pong")
    def react_to_pong(message, client):
        if message.get("bot_id"):
            client.reactions_add(
                channel=message["channel"], timestamp=message["ts"], name="robot_face"
            )

    @app.message(r"^echo (.+)")
    def echo(message, context, say):
        say(text=context["matches"][0])

    return app


def main() -> None:
    app = build_app()
    if os.environ.get("DEMO_MODE", "http") == "socket":
        from slack_bolt.adapter.socket_mode import SocketModeHandler

        SocketModeHandler(app, os.environ["SLACK_APP_TOKEN"]).start()
    else:
        app.start(port=int(os.environ.get("ECHO_PORT", "3001")), path="/slack/events")


if __name__ == "__main__":
    main()
