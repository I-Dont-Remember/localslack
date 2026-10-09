# Conformance fixtures

Each `fixtures/*.json` replays one request against an in-process LocalSlack and compares the response.

## Format
```json
{
  "state": "passing | known_gap",
  "note": "free text",
  "gap_diffs": ["exact diff strings, known_gap only"],
  "setup": [{"method": "POST", "path": "/api/conversations.create", "token": "xoxb-test",
             "params": {"name": "x"}, "capture": {"name": "channel.name"}}],
  "request":  {"method": "POST", "path": "/api/auth.test", "token": "xoxb-test", "params": {"k": "v"}},
  "response": {"status": 200, "body": {...}, "ignore": ["key", "anywhere"]}
}
```
- `params` are sent form-encoded; `token` as `Authorization: Bearer`.
- Body matching is strict (no extra/missing keys) except `ignore` keys. String values may be matchers:
  `<string>` non-empty, `<ts>` Slack timestamp, `<id:C>` ID with that prefix letter, `<any>` anything.
- Each fixture gets its own fresh app (pinned seed + clock). `setup` requests run first, in order;
  `capture` maps a name to a dotted path in that response, usable as `{{name}}` in later `path`/`params`.
- `known_gap`: documents real Slack behavior LocalSlack lacks. `gap_diffs` must list the exact set of
  differences; the test fails if the observed set changes in either direction (so partly-right ->
  broken is caught, and so is progress: update `gap_diffs`, or flip `state` to `passing` once empty).
  List them with `make conformance-gaps` (`go test -v ./tests/conformance/ | grep -A8 'KNOWN GAP'`).

## Recording from real Slack
Use a throwaway workspace and a token with the needed scopes. Never commit tokens or real data.
```
SLACK_TOKEN=xoxb-... uv run --project tests/smoke python tests/conformance/record.py conversations.list limit=2
```
`record.py` sanitizes timestamps, IDs and free-form strings into matchers. Review the file by hand
(arrays are matched by length, so trim them to the elements you care about), then commit.
