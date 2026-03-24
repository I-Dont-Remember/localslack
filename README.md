# LocalSlack

A Slack API emulator for use in CI/CD or local development to avoid cluttering up a real workspace, or to allow quickly creating arbitrary scenarios to run scripts against repeatably. Inspired by [LocalStack](https://github.com/localstack/localstack)'s AWS emulation.

## Quickstart

First, install any necessary packages for Go project.

Second, install the `smoke_tests/` Pipenv requirements. I've been using Python3.8 because I've been needing an old version of Pipenv on my machine, but you can use others.

Recommended to install `air` with `mise use -g air`.

```
make run
```

In a separate terminal, run a smoke test:

```
pipenv run ./run-smoke.sh
```

## Key Notes

- [`Slack APIs accepts any API method requests with POST methods`](https://github.com/slackapi/python-slack-sdk/blob/6fc2d5c082f70ca3e222a7644e6eb202f2140893/slack/web/base_client.py#L402), so at least the Python Slack client always sends requests as `POST` even if they are listed as `GET` in the docs, e.g. `users.list`.

## Why do this? Couldn't this have been auto-generated faster somehow?

Probably off of an openAPI spec and/or Claude Code, but I wanted a structured project so I can learn Go again. If it ends up useful somewhere, that's a happy bonus but not a requirement.

## Reference

- In a similar ballpark https://github.com/chrishutchinson/slack-testing-library

---

## Contributing

Notes on how things work.

### Storage

Keeping things in SQLite, but with minimal fields to start with to avoid having to make constant changes to schema - majority will live as json blob in a `string` field.

- `repo` depends on both `core` (for our type definitions) and `sqlc` for generated code. Maps between DB Rows and Slack object structs.
- Not using an ORM for now, but considering it for later.
- Considering have `goose` or a very simple setup for basic migrations - doesn't seem like a ton of overhead if we only have one or two files, and if we end up wanting later it seems more difficult to try to fit it into an app that wasn't meant for it.

### Misc

Other thoughts.
