# LocalSlack

A project to imitate the Slack API locally for use in integration testing scenarios, inspired by [LocalStack](https://github.com/localstack/localstack), used for AWS emulation.

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
