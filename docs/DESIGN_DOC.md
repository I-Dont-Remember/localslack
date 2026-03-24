# Design Doc: LocalSlack

A Slack API emulator for CI/CD and local development, in the spirit of LocalStack.

Finalized to begin work: 03/22/2026

---

## Goals & Non-Goals

### Goals

- Provide a service which lets developers & Slack admins have confidence in their code that interacts with Slack
- Provide an alternative to the "Dev" workspace people normally use for testing Slack apps & scripts
- Allow custom & repeatable scenario testing
  - With a Dev workspace, the state is often messy and hard to control. Allow users to craft specific scenarios they can run their code against, and make it repeatable so they can try again if there attempts have errors.
- It should be as simple as possible to run for users, ideally a single binary
- We should have coverage of the vast majority of Slack API endpoints
- Users should be able to use the Slack SDK and Slack Bolt with LocalSlack
  - They should not have to change anything in their code to use LocalSlack except for the base URL
- Deliver this in milestones, so we don't get trapped with a 1/2 done tool

### Non-Goals

- Full Slack feature parity
- We do not want a perfect UI replica, or to cover every fancy feature Slack releases
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

I leave it up to the front-end agent to pick a lightweight library such as alpine.js or similar for keeping the app simple but providing for useful web app experiences.

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

We will include a new namespace in the API prefixed with `localslack/`. It should include a health endpoint. This namespace will allow us to add features in the future that help users interacting with LocalSlack, especially in cases where the Slack API might make validating a change difficult.

Example: Slack does not offer an API endpoint to search channels by name - we can offer this as a utility so it's easier in a case where someone is validating changes to many channel names at once.

- Endpoints which we haven't handled yet should respond with a "not implemented error" if they get requested
- We are aiming for response-shape parity with Slack

### Rate-limiting

This can be disabled through an environment variable, but is on by default.

We want to emuluate Slack's rate limiting tiers which are done per method, with a few unique outliers outside the tier. Users will not be allowed to configure the rate-limit tiers, only if it's on or off. The goal is for LocalSlack emulation to be able to rate-limit your API calls at the same point you would experience with the real Slack (when in doubt, we will be more aggressive, since it's better if we err on the side of caution for our users and they end up pleasantly surprised). We won't wory about adding the "burst" capacity Slack often provides, just regular rate limiting will suffice.

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

### Smoke tests

- Smoke tests must be run against a live LocalSlack instance, there should be no unit tests involved.
- You'll likely need Python 3.12 and to use Pipenv to install the `slack-sdk` & Slack Bolt dependencies. Do not place this at the top level of the repo.
- A passing smoke test should prove:
  - We can connect a basic script with Slack SDK webclient, it should be show the APIs for `auth.test`, `converstions.create`, `chat.postMessage`, and `conversations.list` are functioning.
  - we can connect a basic Bolt app
  - We can connect a basic Socket Mode app
  - (eventually) does a basic check of one of the rate-limiting tiers

## CI test (?)

Unclear if this one actually provides us any benefit. Theoretically we should ensure we can run this in a CI environment, but if I'm already running the other types of tests against an instance running on a linux server, how different would the CI environment be?

This one is only worthwhile if we can write a version that provides us different information than our other test suites. Even if it is, we will only run it very rarely.

### Anti-drift / API Parity: conformance test suite

We need a way to have a "ground truth" of the real Slack API to validate our API responses against, even if they seem to be working in our smoke tests, end-to-end tests, etc. This can be our "conformance test suite".

For now, there are 3 pieces - recorded snapshots, real SDK apps, and the openAPI spec.

#### conformance: recorded snapshots

These will be real API requests against a Slack instance which can be serialized to JSON & replayed against this emulator.

Much like LocalStack does for AWS, we will assume that this will be a work in progress, and will initially only have a small number of real Slack API responses for the simplest endpoints. We will sanitize the real API responses to make our conformance testing easier, e.g. we don't want it to fail because a date field had different days, that is still correct.

### conformance: real SDK apps

This is somewhat covered by the smoke tests, but could be potentially fleshed out to be more comprehensive for running real workflows end to end.

#### conformance: OpenAPI spec

We can validate the LocalSlack responses against the published OpenAPI spec for the Slack API automatically. This only validates shape not behavior, but gives us an easy starting point, even if there have been some historical inaccuracies with it.


#### conformance: Detecting API changes

In early stages, we will manually detect when the real API has changed by watching the change-log for the Slack Python SDK. This piece does not need to be automated yet.

---

## Milestones

### Milestone 0: repo quality tools

initial repo quality of life setup for a best-practices Golang CLI & web server (formatting, linting, vetting, pre-commit, etc.)
  Agents developing in this repo should have lots of automated checks which help them avoid dealing with simple changes & also catch simple to solve issues without having to be directed.

### Milestone 0.5:  & testing infrastructure in place

Testing setup for LocalSlack (conformance testing, end-2-end testing, smoke tests, etc.)

I want us to treat this like a project level red-green TDD almost. I want to lay the groundwork for a well architected stack of tools for testing & validation. We should have things in place for conformance test suite, smoke tests, unit test infrastructure (not all of them written of course, but the architecture should be in place so they can easily be added with future features).

Not everything will pass at the first milestone, that is expected, we aren't going to 1-shot the entire API with all features at once, but with these in place we can have confidence we are improving over time.

The smoke tests should be written so it give us confidence not only that API responses appear to be coming back correct, but that things are actually being persisted as well. It would be easy for an endpoint to send back a fake hardcoded response that looks right but doesn't actually store a new channel, message, etc.

### Milestone 1: passing basic tests for simple common actions through HTTP

At this point, we should have a CLI which starts a server that a Slack Bolt app can connect to through both HTTP API and successfully complete some simple smoke tests of basic endpoints. Auth can be faked out for now. The storage layer should be built out enough that CRUD actions against entities actually happen, and endpoints are lying with hardcoded responses.

### Milestone 2: socket mode

Basically the same as Milestone 1, but the app connects through Socket Mode.

### Milestone 3: mock OAuth & scopes

Build the mocked out version of the OAuth experience that apps go through, and it should enable us to support emulation of scope errors so they can find out while testing against LocalSlack before they try against real Slack. Endpoints should have the right scopes tied to them, and block tokens without the right scopes.

Inspired by Stripe's 424242... credit card for testing, let's also have a special "God Mode" token identity which has permission to do everything. This way users of LocalSlack don't have to add app registration steps to every script or app they want to test, only the ones where they care to test the oauth setup/scopes/auth permissions.

### Milestone 4: Events API (outbound webhooks)

Building the capability to track events happening inside the system and then send notifications to subscribed endpoints/socket mode connections.

### Milestone 5: simple UI for most common actions in Slack

If we end up with a UI that allows a user to do the most common actions in Slack, then that's a good enough milestone until we find out deeper use cases. Users should be able to view a channel sidebar, search for a conversation to start/open (any type of channel or DM), and send messages in them (including threads). That's 90% of my day as a real user of Slack, and it handles our use case of LocalSlack for users being able to trigger specific events & feel comfortable that their code is working through visual inspection of the fake Slack workspace state.

### Milestones TBD

There is more that will need to be done, but the first few milestones get us a lot of the way towards a complete system.

---

## Alternatives Considered

None! I wanted this to exist. Tired of cluttering up a "Dev" Slack workspace with a bunch of random messages and channels which can then make it harder to test future scenarios.

---

## Open Questions (Unresolved)

A parking lot for questions that don't fit above yet.

- Do we want to include a "Chaos API" that helps users test their code for when Slack is having system issues?
- what if we exposed some sort of call capture API? See [Appendix](#appendix)
- what if instead of just the random seed concept, we also enabled a state snapshot? See [Appendix](#appendix)
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
