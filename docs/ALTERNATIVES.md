# Alternatives

When should you use something other than LocalSlack? Last checked: 2026-10-09, from each project's README only (not hands-on). Re-check before re-ranking `agent-docs/ROADMAP.md`. LocalSlack's own status: `COMPATIBILITY.md`.

## The tools

| | LocalSlack | [vercel-labs/emulate](https://github.com/vercel-labs/emulate) | [slack-mock](https://github.com/Skellington-Closet/slack-mock) | [slack-testing-library](https://github.com/chrishutchinson/slack-testing-library) |
|---|---|---|---|---|
| Model | stateful emulator server | stateful emulator server | in-process HTTP interception | intercept server, UI-screen simulation |
| Runtime | single Go binary | Node (npm); suite of 15+ services | Node | Node / TypeScript, Jest |
| Web API breadth | small, Bolt/SDK core; evidence-graded | broad (~60 methods incl. files, pins, bookmarks, views) | canned responses | canned responses |
| Events over HTTP, signed | planned | yes | simulated | simulated |
| Socket Mode | planned | not implemented | RTM only | no |
| Slash commands / interactions | planned (later) | not implemented | yes | yes (App Home, buttons) |
| OAuth | not planned | yes | no | no |
| Scope enforcement | planned (permissive default) | opt-in `strict_scopes` | no | no |
| Rate limits / fault injection | planned | not documented | no | no |
| Call log / assertions | planned | inspector (event deliveries) | `slackMock.web.calls` | via test library |
| Reset / seeded scenarios | seed + reset planned | YAML seed | n/a | n/a |
| Fidelity evidence per method | yes (recorded/docs/llm) | not documented | no | no |

"Planned" = see `agent-docs/ROADMAP.md`; not shipped.

## When to use X instead

- **emulate:** you need broad method coverage or OAuth today, or already use its other service emulators.
- **slack-testing-library:** Jest tests of interactive UI flows (App Home, buttons) in-process.
- **slack-mock:** simple call assertions in a Node test, no workspace state needed.
- **LocalSlack:** non-Node CI (single binary), stateful workspace you can reset and seed, and (as the roadmap lands) Socket Mode, deterministic fault injection, and per-method trust levels.

Competitor gaps feed roadmap ranking: roadmap items that close one are tagged `(gap)`.
