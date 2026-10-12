# QA guide

Hands-on walkthroughs for the test-loop features (agent-docs/plans/test-loop.md). Each page lists setup, steps, expected results and what feedback is wanted. API shapes live in [TESTING_API.md](TESTING_API.md).

| Area | Covers | Status |
|---|---|---|
| [Events over HTTP](qa/events-http.md) | Apps, signed event delivery, retries, redeliver, delivery controls | ready to QA |
| [Bolt core methods](qa/bolt-core.md) | conversations.info, users.info, conversations.members, api.test, chat.update/delete/postEphemeral, reactions.add | ready to QA |
| [Call log and reset](qa/calls-reset.md) | GET/DELETE /_localslack/calls, POST /_localslack/reset, `localslack reset` | ready to QA |
| [Fault injection](qa/faults.md) | Rate limits, server errors, delays and nth-call faults | ready to QA |
| [Blocks, attachments, metadata](qa/blocks.md) | Rich message content round-trips | ready to QA |
| [Socket Mode](qa/socket-mode.md) | apps.connections.open, WebSocket envelopes, acks, disconnects | not yet implemented |
| [Access edge cases](qa/access.md) | Membership, private channels, archived channels | ready to QA |
| [Scenarios, snapshots, diff](qa/scenarios.md) | Fixtures, generated data, snapshot/restore, diff | not yet implemented |
| [Inspector UI](qa/inspector.md) | /ui/inspector live panels | not yet implemented |

Each stream fills in its own page; pages keep the sections Setup, Steps, Expected, Feedback prompts.
