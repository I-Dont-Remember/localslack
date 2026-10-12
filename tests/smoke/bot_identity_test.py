"""The bot's identity is stored: auth.test, users and messages agree on bot_id."""


def test_bot_messages_carry_the_auth_test_bot_id(client, unique):
    channel = client.conversations_create(name=f"botid-{unique}")["channel"]["id"]
    bot_id = client.auth_test()["bot_id"]
    client.chat_postMessage(channel=channel, text="who am i")
    message = client.conversations_history(channel=channel)["messages"][0]
    assert bot_id.startswith("B")
    assert message["bot_id"] == bot_id
