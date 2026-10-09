# Conformance fixtures

Each `fixtures/*.json` replays one request against an in-process LocalSlack and compares the response.

## Format
```json
{
  "state": "passing | known_gap",
  "note": "free text",
  "request":  {"method": "POST", "path": "/api/auth.test", "token": "xoxb-test", "params": {"k": "v"}},
  "response": {"status": 200, "body": {...}, "ignore": ["key", "anywhere"]}
}
```
- `params` are sent form-encoded; `token` as `Authorization: Bearer`.
- Body matching is strict (no extra/missing keys) except `ignore` keys. String values may be matchers:
  `<string>` non-empty, `<ts>` Slack timestamp, `<id:C>` ID with that prefix letter, `<any>` anything.
- `known_gap`: documents real Slack behavior LocalSlack lacks. Mismatch = skipped, with the diff
  (`go test -v ./tests/conformance/ | grep -A8 'KNOWN GAP'`, or `make conformance-gaps`).
  When it starts matching the test fails: flip `state` to `passing`.

## Recording from real Slack
Use a throwaway workspace and a token with the needed scopes. Never commit tokens or real data.
```
SLACK_TOKEN=xoxb-... uv run --project tests/smoke python tests/conformance/record.py conversations.list limit=2
```
`record.py` sanitizes timestamps, IDs and free-form strings into matchers. Review the file by hand
(arrays are matched by length, so trim them to the elements you care about), then commit.
