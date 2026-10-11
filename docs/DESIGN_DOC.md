# Design Doc: LocalSlack

A Slack API emulator for CI/CD and local development, in the spirit of LocalStack.

Finalized to begin work: 03/22/2026
Rescoped: 10/09/2026 — UI + core endpoints pulled forward into M1 to get a playable slice early; later milestones renumbered.

---

## Goals & Non-Goals

### Goals

- Provide a service which lets developers & Slack admins have confidence in their code that interacts with Slack
- Provide an alternative to the "Dev" workspace people normally use for testing Slack apps & scripts
- Allow custom & repeatable scenario testing
  - With a Dev workspace, the state is often messy and hard to control. Allow users to craft specific scenarios they can run their code against, and make it repeatable so they can try again if there attempts have errors.
- It should be as simple as possible to run for users, ideally a single binary
- Cover the Slack API surface that Bolt/SDK apps actually use, and make the test loop superb (seed, run, trigger events, inspect, reset). Breadth beyond that is on demand.
- Users should be able to use the Slack SDK and Slack Bolt with LocalSlack
  - They should not have to change anything in their code to use LocalSlack except for the base URL
- Deliver this in milestones, so we don't get trapped with a 1/2 done tool

### Non-Goals

- Full Slack feature parity
- We do not want a perfect UI replica, or to cover every fancy feature Slack releases
  - UI exists to inspect state and trigger events (a testing instrument), not to be a polished chat client
- Hosting a service
  - this is meant to be run only locally or in CI/CD, not a cloud offering.
- Advanced observability: Prometheus or OpenTelemetry are overkill for this emulator.

---

## Architecture

It will be a Golang CLI with embedded server. End users shouldn't have to set up anything other than downloading a release file & running it, unless they have specific settings they want to tweak.

### User configuration

Users can configure LocalSlack using environment variables.

### Dev experience/ tooling

- `air` for live reload
- `mise` for Go version management
- golangci-lint
- pre-commit

### Backend language: Go

A performant typed language for web applications. Also one that I want to spend more time working with.

Use `slog` for structured logging.

### Frontend: Server-side templates at first

I want to keep this simple, but also give ourselves the ability to handle more complex scenarios in the future. I'd prefer to not have to maintain a build step for a React SPA unless it really makes the tool significantly better.

Decision (2026-10-09): `html/template` + vendored htmx, embedded with `go:embed`. No build step.

### Storage: tiered

- Database (SQLite) for core entitites which should persist like channels, workspaces, messages, etc.
- In-memory for things which aren't as important to retain between restarts/snapshots, or which could be confusing if user restarts and expects a fresh slate. Things like events in the queue, rate-limit state, active WebSocket connections, typing indicators, etc.
  - By adopting more in-memory storage, it reduces the write pressure on our SQLite instance and matches durability of real Slack API. We will likely benefit for these in-memory structs to use something along the lines of `sync.RWMutex`.

### Database: SQLite

Simplicity should rule the architecture of this application. SQLite can handle surprisingly large volumes of traffic and is a battle-tested too. I'd rather we find ways to use it to it's fullest than be forced to make a more complicated setup for users.

Schema: it should start with minimal columns & JSON blob, but have useful Go types where it helps in development of LocalSlack. We may adjust these to be more heavily typed in the future.

- No ORM usage, we'll use `modernc.org/sqlite` since it's a pure Go implementation.
- multi-tenant: we will allow multiple workspaces within single DB file
- DB file path should be configurable with `LOCALSLACK_DB_PATH`
- Migrations: use Goose if we need a migration tool

We also need to make sure SQLite can handle significant local write traffic & simultaneous read traffic. I believe that likely means we need:

- WAL mode + set busy timeout on connection open
- Keep a single `*sql.DB` for writes (acts kinda serialized), and we should be separating the readers from the writers.

We may need to investigate optimizations in the future if we aren't seeing good performance, since SQLite can be optimized in many ways.

### Random Seed Data

This can be disabled through an environment variable.

On startup, it should pick a random seed value, log it, and then generate & populate the storage layer with mock data. The first version should be quite simple, and have sane defaults of:

- 1 single workspace
- 1 - 10 users
- 2 - 50 channels
- 1 channel will have between 3 - 25 messages between up to 4 people.

TODO: eventually we will find a way for users to define custom seed data/scenarios, but that is a long-term roadmap and should not be built in the near future.

### API/ webserver

Webserver will be packaged with the CLI tool and use `go-chi` library.

We will include a new namespace in the API prefixed with `_localslack/`. It should include a health endpoint. This namespace will allow us to add features in the future that help users interacting with LocalSlack, especially in cases where the Slack API might make validating a change difficult.

Example: Slack does not offer an API endpoint to search channels by name - we can offer this as a utility so it's easier in a case where someone is validating changes to many channel names at once.

- Endpoints which we haven't handled yet should respond with a "not implemented error" if they get requested
- We are aiming for response-shape parity with Slack

### Rate-limiting

Decision (2026-10-09, from `ignored/` user research): **deterministic fault injection comes first**; realistic tier limiting is opt-in and later. Time-based limits are flaky in CI and only show that a limit was hit. Faults are triggered on demand via `_localslack/` ("the 3rd `chat.postMessage` returns 429, `Retry-After: 2`"), also covering 5xx, timeouts and Slack error codes.

Opt-in (env var, **off by default**) realistic limiting: emulate Slack's per-method tiers, with a few outliers outside the tier. Tiers are not user-configurable. When in doubt, be more aggressive than Slack. No "burst" capacity.

A rough example of Slack response with rate-limiting:
```
HTTP/1.1 429 Too Many Requests
Retry-After: 30
```

And some non-exhaustive documentation of the Slack rate limits per minute - _"applied on a "per API method per workspace/team per app" basis"_:

```
rate_limits:
  # fallback if a method has no tier (optional)
  default_per_minute: 60

  # standard Slack tiers https://docs.slack.dev/apis/web-api/rate-limits/
  tiers:
    tier1: 1
    tier2: 20
    tier3: 50
    tier4: 100

  # only methods that deviate from tier limits
  special_methods:
    auth.test: 5
```

> The per-method tier is data now: `tier` (and `scopes`) per method in `internal/capabilities/capabilities.toml`, which M4/M5 read rather than keeping a second table. The numbers above are from Slack's docs and unverified until recorded.

### Events API Design

An internal in-memory event-driven architecture that emulates the Slack Events API. This is where LocalSlack app registers an event subscription and the LocalSlack service sends out event notifications through either HTTP or Socket Mode. It should allow us to register events when a user takes an action without blocking the response to their request.

### CLI

A Go CLI that is built with Kong and https://github.com/charmbracelet/bubbletea. Functionality we'll need:

- Managing the embedded web server - starting & stopping, and it should handle Ctrl+C gracefully. Also should be able to be started in the background with similar devex as Docker CLI so people don't have to learn new commands.
- An option to clear/reset the state of all the data, e.g. clear out all the messages, channels, apps, etc. to start a test from a clean slate
- Room for more in the future

### Observability

We are keeping this simple since it's a locally running tool for testing rather than a self-hosted product. We will include:

- structured request & response logging
- admin inspection API: `_localslack/` which exposes internal state for inspection, such as registered apps, pending events, etc.
- Event delivery trace log: a log of every event that was generated and whether it was successfully delivered to the app's endpoint. Separate from general request logging because the delivery happens async and the causality chain (message posted → event generated → webhook called → ack received) is otherwise invisible.
- A basic UI which enables visual inspection & manual interaction

---

## User Experience & Features

### Single binary CLI

Users only have to download a single CLI, run `localslack start`, and update their code to point to it's `localhost` URL to begin testing against it.

### LocalSlack API for Inspection

With the meta `_localslack/` namespace available in the API, we can add helper/utility endpoints that are useful for developing against LocalSlack. Allow them to peek "under the hood" when it is useful.

Depending on usage patterns, it might also be useful down the line to add some sort of "assertion" endpoint, e.g. (pseudocode) `/assert/ {channel_id == "c12335"}`.

### Events API

Supports emulation of the Slack Events API where LocalSlack will send event notifications to registered localhost endpoints or through Socket MOde.

### User Validation

To validate their code is working as intended & write assertions, users can:

- use "Read" endpoints of the Slack API or our helper endpoints under `/_localslack/` to check the state.
- Examine the structured logs we will emit

### Random State from seed

Each time LocalSLack starts, it will pick a random seed, log it, and then use that to generate a random set of data to populate LocalSlack with.

---

## Validation & Testing Strategy

We want to have layers of automated testing to ensure agents have the tools they need to iterate quickly with high quality, as well as providing confidence to the humans that this application is working as intended & not just tricking the test suite, and also that LocalSlack is conforming to it's ultimate goal of emulating Slack as near as possible.

### Unit tests

- These should be kept simple, mainly for regression.
- We should not go overboard writing unit tests.
- They must be configured to shuffle order & run in parallel, so we avoid any accidential test dependencies on one another.
- They must be passing 100% at each commit.

### Smoke tests

- Smoke tests must be run against a live LocalSlack instance, there should be no unit tests involved.
- Python 3.12 managed with `uv` (`tests/smoke/pyproject.toml`) installs the `slack-sdk` & Slack Bolt dependencies. Do not place this at the top level of the repo.
- A passing smoke test should prove:
  - We can connect a basic script with Slack SDK webclient, it should be show the APIs for `auth.test`, `converstions.create`, `chat.postMessage`, and `conversations.list` are functioning.
  - a basic Bolt app boots against LocalSlack and replies through it (done: `tests/smoke/bolt_test.py`); receiving real events over HTTP is proven in M2
  - a basic Socket Mode app connects and receives events (M3)
  - (M5) a basic check of one of the rate-limiting tiers
  - the Slack SDK's own behaviors that apps rely on (pagination iterators, async client, error types) work: `tests/smoke/sdk_golden_path_test.py`, `async_client_test.py`
  - what real Slack does that we do not yet is encoded as strict-xfail "red tests" tagged with a deviation id (`tests/smoke/known_deviations_test.py`)

### Anti-drift / API Parity: conformance test suite

We need a way to have a "ground truth" of the real Slack API to validate our API responses against, even if they seem to be working in our smoke tests, end-to-end tests, etc. This can be our "conformance test suite".

For now, there are 3 pieces - recorded snapshots, real SDK apps, and the openAPI spec.

#### conformance: recorded snapshots

These will be real API requests against a Slack instance which can be serialized to JSON & replayed against this emulator.

Much like LocalStack does for AWS, we will assume that this will be a work in progress, and will initially only have a small number of real Slack API responses for the simplest endpoints. We will sanitize the real API responses to make our conformance testing easier, e.g. we don't want it to fail because a date field had different days, that is still correct.

#### conformance: provenance and the capability manifest

Every fixture declares where its expected response came from: `recorded` (a sanitized real Slack response, with date/workspace type/scopes), `docs` (checked against a cited source), `llm` (written from model knowledge, unverified), or `local` (LocalSlack-only behavior). A method's evidence level is the best source among its fixtures, derived rather than hand-written. `internal/capabilities/capabilities.toml` is the single test-enforced inventory of methods, non-endpoint behaviors (rate limiting, scopes, Socket Mode, POST-for-GET, ...) and known deviations; `docs/COMPATIBILITY.md` is generated from it so users can see exactly how far to trust each piece. Recordings are expected from the owner and community contributors (`CONTRIBUTING.md`), gated by an automated redaction check.

### conformance: real SDK apps

This is somewhat covered by the smoke tests, but could be potentially fleshed out to be more comprehensive for running real workflows end to end.

#### conformance: OpenAPI spec

We can validate the LocalSlack responses against the published OpenAPI spec for the Slack API automatically. This only validates shape not behavior, but gives us an easy starting point, even if there have been some historical inaccuracies with it.

This does not have to be 100% passing, but we should be able to easily understand the remaining gaps.


#### conformance: Detecting API changes

In early stages, we will manually detect when the real API has changed by watching the change-log for the Slack Python SDK. This piece does not need to be automated yet.

### CI test (?)

Unclear if this one actually provides us any benefit. Theoretically we should ensure we can run this in a CI environment, but if I'm already running the other types of tests against an instance running on a linux server, how different would the CI environment be?

This one is only worthwhile if we can write a version that provides us different information than our other test suites. Even if it is, we will only run it very rarely.

---

## Milestones

### Milestone 0: repo quality tools

initial repo quality of life setup for a best-practices Golang CLI & web server (formatting, linting, vetting, pre-commit, etc.)
  Agents developing in this repo should have lots of automated checks which help them avoid dealing with simple changes & also catch simple to solve issues without having to be directed.

### Milestone 0.1: scaffolding

This should set us up with the basic building blocks future milestones will build on top of. Putting together the basic CLI and server with the few commands we already know we'll need (including running the server in the background as an option `-d` like Docker does for containers, and a way to kill that background process). Also other remaining scaffolding for a best practices Golang repo so I can have a "hello world" CLI & server with unit tests passing.

### Milestone 0.5: testing infrastructure in place (trimmed 2026-10-09)

Tooling + test harnesses so agents catch basic errors automatically and can test the app end to end with one command (`make verify`): linters, pre-commit, ast-grep rules, isolated live-server script, conformance fixture framework (matchers, known-gap reporting), smoke test harness with strict-xfail tests for M1 endpoints (project-level red-green).

Smoke tests must prove persistence, not just plausible responses: write, then read back.

Deferred to the milestone that ships the feature: Bolt smoke (M2), Socket Mode smoke (M3), rate-limit smoke (M5), OpenAPI validator (M6). Plan: `agent-docs/plans/m0.5-hardening.md`.

### Milestone 1: vertical slice (API + UI)

A playable full system, thin but end to end. Rescoped 2026-10-09 so there is something concrete to play with early (UI moved up from the old M5).

- SQLite storage + random seed data
- Fake auth: `xoxb-` token -> bot user, `xoxp-<userID>` -> that user
- Endpoints: `auth.test`, `users.list`, `conversations.list/create/history/replies/open/archive`, `chat.postMessage` (incl. `thread_ts`). The live inventory is `docs/COMPATIBILITY.md` (generated); do not maintain lists here.
- htmx UI: channel/DM sidebar, quick-switcher, channel view, composer, thread pane, "acting as" user switcher
- SDK smoke tests with persistence checks; Playwright E2E tests

Plan: `agent-docs/plans/m1-vertical-slice.md`.

### Milestone 2: Events API (HTTP) + Bolt app

Track events and deliver to subscribed HTTP endpoints; Bolt app smoke test.

### Milestone 3: Socket Mode

Same as M2 delivery, through Socket Mode; Socket Mode smoke test.

### Milestone 4: mock OAuth & scopes

Build the mocked out version of the OAuth experience that apps go through, and support emulation of scope errors so they find out while testing against LocalSlack. Endpoints have the right scopes tied to them and block tokens without them.

Inspired by Stripe's 424242... credit card for testing, a special "God Mode" token identity has permission to do everything. Users don't have to add app registration steps to every script, only where they care to test oauth/scopes/auth.

### Milestone 5: fault injection + rate limiting

Fault injection API first, then opt-in per-method tiers (see Rate-limiting); smoke check for each.

### Milestone 6: breadth + polish

Long-tail endpoints (the bolt-core ones moved up: see `agent-docs/ROADMAP.md`), search, OpenAPI conformance validator, `reset` CLI, UI polish.

### Milestones TBD

There is more that will need to be done, but the first few milestones get us a lot of the way towards a complete system.

---

## Alternatives Considered

I wanted this to exist: tired of cluttering up a "Dev" Slack workspace with random messages and channels which make it harder to test future scenarios.

Other tools exist (notably vercel-labs/emulate, a broader Node-based Slack emulator). See `docs/ALTERNATIVES.md` for the comparison and when to use them instead. We do not compete on method breadth; we compete on the test loop: single Go binary, Socket Mode, fault injection, event-delivery controls, reproducible scenarios, and evidence-graded fidelity.

---

## Open Questions (Unresolved)

A parking lot for questions that don't fit above yet.

- ~~Chaos API~~ Decided yes: deterministic fault injection (see Rate-limiting).
- ~~Call capture API~~ Decided yes: call log + reset ship first (ROADMAP item 5). See [Appendix](#appendix)
- State snapshot/diff: still open, planned together with scenario seeding. See [Appendix](#appendix)
- Versioning: does LocalSlack need to version its API to track Slack API versions?
- UI - should we handle Block Kit / Interactivity (button clicks, modals, etc.)
- Is there a better way to handle conformance testing suite than we currently are? Something that is simpler & easier to maintain, or with higher confidence?

---

## Appendix

### Assertion/ call-capture API

Depending on usage patterns, it might be useful down the line to add some sort of "assertion" endpoint, e.g. (pseudocode) `/assert/ {channel_id == "c12335"}`. Basically what if we can help take the place of `assert/ expect(x).toBe`, or at least make it easier.

Why? Don't make the user grep logs to assert behavior.

Example:
```
# After your code runs, assert side effects without touching Slack state
GET /_localslack/calls?method=chat.postMessage&channel=C123

# Returns: [{ts: "...", payload: {text: "Hello"}}]
# assert len(calls) == 1 and calls[0]["payload"]["text"] == "Hello".
```

### State snapshot & creation

We already toyed with an idea for allowing a seed value to allow users to generate randomized scenarios, but be able to repeat against the same scenario as before by using the same seed. This is a cool concept, but it doesn't cover the case where they construct a specific scenario themselves and then want to make it repeatable without running the same giant script again. Localstack does something similar to this to make CI/CD checks fast.

This also has me wanting to allow people to create scenarios using like a toml file or something instead of writing a script - e.g. I want 500 random channels with names that include numbers. This might be difficult to make flexible enough, but I bet we can find a way.
