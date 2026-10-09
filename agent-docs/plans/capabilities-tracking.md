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

## Arch review (done / deferred)
Done: record.py path/host allowlist, setup `ok` check, ok:false refusal, non-Slack host => `source: local` (never `recorded`), call listing before replay; evidence cell flags `(errors only)`; ok:true fixture required per shipped method (waivers need a reason); "SDK-called" wording (hint only); claims must land on main + conflict rule for COMPATIBILITY.md; redact_check catches embedded/any-prefix ids and 32-hex secrets; removed no-op test; `auth.test` tier 3; added `conversations.history` happy-path fixture.
Deferred: `claimed` date + stale-claim listing in capctl; `partial` <-> high-deviation check; global-deviation flag; `ref` URL/path validation (fixtures + behaviors), downgrade `get-methods-accept-post`/`form-and-json-bodies` to `llm`; deviation xfail masking setup errors (run setup in fixtures / match error code); per-row "unverified" marker for tier/scopes; verify `message-length-unbounded` (Slack may truncate at 40k, not error), `conversations.open` scopes; redact allowlist-by-key policy, epoch ints, ad hoc request params; Bolt negative control + M2 real delivery test; fixture `kind` for events (M2/M3); scopes any-of vs all-of (M4); `make new-method` / `capctl check <method>`; merge `conformance-update-gaps` into `conformance-gaps UPDATE=1`; slack-sdk version in `recorded`; 429 backoff on setup steps.
