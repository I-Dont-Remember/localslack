# LocalSlack

A Slack API emulator for use in CI/CD or local development to avoid cluttering up a real workspace, or to allow quickly creating arbitrary scenarios to run scripts against repeatably. Inspired by [LocalStack](https://github.com/localstack/localstack)'s AWS emulation.

## Quickstart

Install tools with [mise](https://mise.jdx.dev), then:

```
make setup    # mise install, pre-commit hooks, uv env
make run      # start server on :3232
make verify   # check + unit + conformance + smoke (+ e2e) against an isolated live server
```

## Try it

```
make run                      # serves on http://localhost:3232 (loopback only; seeded demo workspace, DB under XDG data dir)
```

- **Browser:** open http://localhost:3232. Pick channels, post, reply in threads, jump to people/channels with the quick-switcher, and switch "Acting as" to post as another user. The page polls every 2s, so messages sent from scripts show up live.
- **Slack SDK:** point it at the emulator, e.g. Python:

  ```python
  from slack_sdk import WebClient
  client = WebClient(token="xoxb-anything", base_url="http://localhost:3232/api/")
  channel = client.conversations_create(name="demo")["channel"]["id"]
  client.chat_postMessage(channel=channel, text="hello from the SDK")
  ```

  Tokens: `xoxb-<anything>` acts as the bot; `xoxp-<userID>` acts as that user (ids from `users.list`).
- **Config (env):** `LOCALSLACK_ADDR` (default `127.0.0.1:3232`; auth is "any token works", so only expose with `0.0.0.0:3232` deliberately, e.g. in a container), `LOCALSLACK_DB_PATH`, `LOCALSLACK_SEED` (same seed, same demo data; a random one is logged otherwise), `LOCALSLACK_NO_SEED=1`.
- **Inspect state:** `curl localhost:3232/_localslack/state/conversations`.
- **Implemented methods:** `auth.test`, `users.list`, `conversations.list|create|open|history|replies|archive`, `chat.postMessage`. Anything else returns `404 not_implemented`.

## Key Notes

- [`Slack APIs accepts any API method requests with POST methods`](https://github.com/slackapi/python-slack-sdk/blob/6fc2d5c082f70ca3e222a7644e6eb202f2140893/slack/web/base_client.py#L402), so at least the Python Slack client always sends requests as `POST` even if they are listed as `GET` in the docs, e.g. `users.list`.

## Reference

- In a similar ballpark https://github.com/chrishutchinson/slack-testing-library

---

## Contributing

Notes on how things work.

### Storage

SQLite via `modernc.org/sqlite` (no cgo, no ORM), migrations with `goose` (embedded). Layering: `store` (SQL only) -> `service` (rules, typed errors) -> `api` / `web` (translation only); depguard enforces that `api`/`web` never import `store` or each other. See `agent-docs/` for the agent-facing guides.

### Misc

Other thoughts.
