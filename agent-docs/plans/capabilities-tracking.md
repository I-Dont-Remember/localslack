# Capability tracking, evidence provenance & real-client coverage (branch `capabilities-tracking`)

Enabling work after M1 so many agents can extend LocalSlack in parallel without duplicating work or
letting docs drift, and so users can see how faithful each piece is. Decided with the owner: TOML
manifest enforced by tests; Python slack_sdk + Bolt as the gated clients; recordings from the owner
and community contributors.

## Done
- [x] `internal/capabilities`: embedded `capabilities.toml` (methods, behaviors, deviations), strict parser, validator, parity test against `api.MethodNames()`
- [x] Fixture provenance schema (`source`/`ref`/`recorded`), validation, shared by the conformance runner; fixtures migrated honestly (`llm`/`local`); missing fixtures added
- [x] Derived facts: evidence level (best fixture source), SDK-tested (scan), deviation red tests (scan), client versions (`uv.lock`); generated `docs/COMPATIBILITY.md` with a golden test; `make docs`, `make evidence-gaps`, `cmd/capctl`
- [x] `record.py` v2 (`--from`, provenance, header subset, no overwrite), `make conformance-update-gaps`, `make redact-check`, `make tools-test`, `CONTRIBUTING.md`
- [x] Client suite: SDK golden path (iterators, error contract, payload variants, retries), async client, Bolt app boot + signed-event dispatch, 14 red tests for known deviations, `slack-sdk` pinned, `slack-bolt` + `aiohttp` added
- [x] Docs: `agent-docs/capabilities.md`, endpoint guide + workflow rewritten, stale DESIGN_DOC passages fixed, README links to the generated matrix

## Follow-ups (not started)
- [ ] Get the first real recordings (`make evidence-gaps` is the list; start with `auth.test`, `conversations.create/history`, `chat.postMessage`)
- [ ] Verify tier/scopes against recordings or cited docs (currently unverified transcriptions)
- [ ] Priority backlog in the manifest (`priority = "bolt-core"`): `api.test`, `conversations.info/members`, `users.info`, `chat.update/delete/postEphemeral`, `reactions.add`
- [ ] OpenAPI spec validation as a fourth derived signal (M6)
- [ ] M4/M5: read `tier`/`scopes` from the embedded manifest instead of a second table
- [ ] Optional: Node (`@slack/web-api`, `@slack/bolt`) or Go (`slack-go`) client suites; a scheduled CI job against the latest `slack-sdk`/`slack-bolt`
