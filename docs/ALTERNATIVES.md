# Alternatives

When should you use something other than LocalSlack? Last checked: 2026-10-09 from READMEs; emulate re-checked hands-on 2026-10-10 (v0.12.1, see "Hands-on: emulate"). Others README-only. Re-check before re-ranking `agent-docs/ROADMAP.md`. LocalSlack's own status: `COMPATIBILITY.md`.

## The tools

| | LocalSlack | [vercel-labs/emulate](https://github.com/vercel-labs/emulate) | [slack-mock](https://github.com/Skellington-Closet/slack-mock) | [slack-testing-library](https://github.com/chrishutchinson/slack-testing-library) |
|---|---|---|---|---|
| Model | stateful emulator server | stateful emulator server | in-process HTTP interception | intercept server, UI-screen simulation |
| Runtime | single Go binary | Node (npm); suite of 15+ services | Node | Node / TypeScript, Jest |
| Web API breadth | small, Bolt/SDK core; evidence-graded | broad (~75 methods incl. files, pins, bookmarks, views) | canned responses | canned responses |
| Events over HTTP, signed | yes (retries, redelivery, injection) | signed payloads, but no CLI way to register a subscriber | simulated | simulated |
| Socket Mode | yes (`events_api` envelopes) | not implemented | RTM only | no |
| Slash commands / interactions | planned (later) | not implemented (open issue #289) | yes | yes (App Home, buttons) |
| OAuth | not planned | yes | no | no |
| Scope enforcement | partial (membership + `chat:write.public`; permissive otherwise) | opt-in `strict_scopes` | no | no |
| Rate limits / fault injection | fault injection yes (429, errors, delay); rate limits planned | none | no | no |
| Call log / assertions | yes (`/_localslack/calls`, long-poll) | none in CLI (event-delivery inspector tab only) | `slackMock.web.calls` | via test library |
| Reset / seeded scenarios | seed, reset, scenario files, snapshots + diff | YAML seed; no reset; random IDs | n/a | n/a |
| Fidelity evidence per method | yes (recorded/docs/llm) | not documented | no | no |

"Planned" = see `agent-docs/ROADMAP.md`; not shipped. Shipped capabilities: `docs/COMPATIBILITY.md`.

**Stance:** we know emulate is broader and moves fast, and we are not trying to match its method count. LocalSlack is built to explore the gaps above (the test loop) and as a deliberate exercise in building a service like this. Where we need breadth, emulate is the reference.

## Hands-on: emulate (v0.12.1, Slack service, CLI mode)

Ran it, our `tests/smoke` suite against it, and dozens of probes. Corrects the table above.

- **Strong:** ~75 method routes (files/pins/bookmarks/views present; only views probed live); blocks round-trip; private channels; `limit` honored; `strict_scopes` gives real `missing_scope` (`needed`/`provided`); OAuth authorize page + client validation (full token exchange not run); 200 concurrent posts ordered, unique `ts`; seeded YAML.
- **Events are unreachable from the CLI:** no seed key, route or API registers a subscriber (inspector: "No Slack event subscriptions are registered"). Dispatch code exists, so a Bolt app receives nothing. No `url_verification`, retries or injection.
- **No test loop:** `/_emulate` (request log, reset) is 404 for built-in Slack; no call log, reset, faults, snapshot. Rate limiting none (300/300 `200`).
- **Not reproducible:** non-seeded IDs random per run; state is in-memory.
- **Fidelity gaps (same as ours unless noted):** any token accepted (maps to first user); GitHub-style `x-ratelimit-*` headers, no `x-slack-req-id`; unknown method = `404 {"message":"Not Found"}`, not Slack-shaped; invalid cursor/`types` and missing thread return `ok:true`; private-channel membership is enforced (`not_in_channel`; hidden from lists) but a non-member can post to public channels without `chat:write.public`; no `bot_id`; Node-only.
- **Upstream momentum (GitHub, checked 2026-10-10):** very active (1.9k stars, release every ~1-2 weeks, Slack PRs weekly). Shipped in v0.12.0: signed event callbacks, full envelopes. Open: #293 payload fidelity, #294 retry redelivery (`/_slack/simulate/event-retry`), #263 `app_mention`, #289 interactivity (`block_actions`/`view_submission` simulate endpoints, `response_url`), PRs for `users.conversations`, `search.*`, usergroups, `conversations.history` oldest/latest. These come from outside users wrapping the dispatcher in-process; none adds CLI subscriber registration, a call log, reset, faults or Socket Mode. No issue/PR mentions Socket Mode, fault injection or rate limits.- **vs our red tests:** emulate already passes `no-blocks`, `users-list-unpaginated`, `no-private-channels`, token-in-body.

## When to use X instead

- **emulate:** you need broad method coverage or OAuth today, or already use its other service emulators.
- **slack-testing-library:** Jest tests of interactive UI flows (App Home, buttons) in-process.
- **slack-mock:** simple call assertions in a Node test, no workspace state needed.
- **LocalSlack:** non-Node CI (single binary), stateful workspace you can reset and seed, and Socket Mode, deterministic fault injection, a call log, snapshots and per-method trust levels.

Competitor gaps feed roadmap ranking: roadmap items that close one are tagged `(gap)`.
