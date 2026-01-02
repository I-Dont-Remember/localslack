# LocalSlack

A project to imitate the Slack API locally for use in integration testing scenarios without cluttering up a real workspace, or to allow quickly creating arbitrary scenarios to run scripts against repeatably. Inspired by [LocalStack](https://github.com/localstack/localstack)'s AWS emulation.

## Quickstart

First, install any necessary packages for Go project.

Second, install the `smoke_tests/` Pipenv requirements. I've been using Python3.8 because I've been needing an old version of Pipenv on my machine, but you can use others.

```
make run
```

In a separate terminal, run a smoke test:

```
pipenv run ./run-smoke.sh
```

## Key Notes

- [`Slack APIs accepts any API method requests with POST methods`](https://github.com/slackapi/python-slack-sdk/blob/6fc2d5c082f70ca3e222a7644e6eb202f2140893/slack/web/base_client.py#L402), so at least the Python Slack client always sends requests as `POST` even if they are listed as `GET` in the docs, e.g. `users.list`.
