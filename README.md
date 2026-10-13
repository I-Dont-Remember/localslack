# LocalSlack

A Slack API emulator for use in CI/CD or local development to avoid cluttering up a real workspace, or to allow quickly creating arbitrary scenarios to run scripts against repeatably. Inspired by [LocalStack](https://github.com/localstack/localstack)'s AWS emulation.

**Scope:** a deliberately small slice of Slack's API (what Bolt and SDK apps commonly use). For broad method coverage use [vercel-labs/emulate](https://github.com/vercel-labs/emulate); LocalSlack focuses on the test loop (events, Socket Mode, call log, reset, faults). See [docs/ALTERNATIVES.md](docs/ALTERNATIVES.md). To see which capabilities/endpoints are verified vs best guess, review [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md).

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
- **What's supported:** see [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md), generated from the capability manifest: every implemented and planned method, how well each is verified against real Slack, behaviors like rate limiting and scopes, and known deviations. Unimplemented methods return `200 unknown_method` with an `X-LocalSlack-Not-Implemented: 1` header.

## Try the test loop

```
make demo               # LocalSlack on :3232 + two real Bolt apps (needs go, uv); DEMO_MODE=socket or `make demo-socket` for Socket Mode
```

- Open http://localhost:3232/ui, post `@demo-bolt ping` in `#general` (the bot replies `pong`), then watch it all at **http://localhost:3232/ui/inspector** (API calls, event deliveries, apps, faults, sockets, snapshots).
- Guided 45-minute walkthrough with expected results and feedback prompts: [docs/QA.md](docs/QA.md). API contract: [docs/TESTING_API.md](docs/TESTING_API.md).
- CLI niceties (server running on :3232):
  - `localslack apps add manifest.json` / `apps set-url <app> http://localhost:3000` (fixes a `blocked` remote request URL) / `apps env <app>` (prints `SLACK_API_URL`, `SLACK_BOT_TOKEN`, `SLACK_SIGNING_SECRET`, `SLACK_APP_TOKEN` for your app).
  - `localslack reset [--all]`: re-seed workspace data, keeping apps; `--all` also removes apps.
  - `localslack start --scenario examples/scenarios/acme.json` (also `reset --scenario f`): exact, reproducible users/channels/messages.

## Key Notes

- [`Slack APIs accepts any API method requests with POST methods`](https://github.com/slackapi/python-slack-sdk/blob/6fc2d5c082f70ca3e222a7644e6eb202f2140893/slack/web/base_client.py#L402), so at least the Python Slack client always sends requests as `POST` even if they are listed as `GET` in the docs, e.g. `users.list`.

## Reference

- In a similar ballpark https://github.com/chrishutchinson/slack-testing-library

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), including how to record real Slack responses as conformance evidence.

Notes on how things work.

### Storage

SQLite via `modernc.org/sqlite` (no cgo, no ORM), migrations with `goose` (embedded). Layering: `store` (SQL only) -> `service` (rules, typed errors) -> `api` / `web` (translation only); depguard enforces that `api`/`web` never import `store` or each other. See `agent-docs/` for the agent-facing guides.

### Misc

Other thoughts.
