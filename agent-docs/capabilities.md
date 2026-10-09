# Capability manifest (read this before adding or claiming work)

`internal/capabilities/capabilities.toml` is the **one** hand-edited inventory of what LocalSlack
supports, what is planned, which agent owns in-flight work, and where we knowingly differ from real
Slack. `docs/COMPATIBILITY.md` is generated from it for users. Do not keep method lists, status or
"known gaps" anywhere else (README, DESIGN_DOC, plans): link to the manifest or COMPATIBILITY.md.

## What lives where
| Thing | Source | Hand-written? |
|---|---|---|
| Method status, tier, scopes, deviations, plan | `capabilities.toml` `[methods.*]` | yes |
| Non-endpoint behaviors (rate limiting, scopes, Socket Mode, POST-for-GET, ...) | `[behaviors.*]` | yes (incl. `evidence`) |
| Known deviations from Slack | `[deviations.*]` (id, severity, one-sentence summary) | yes |
| Per-method **evidence level** (`recorded` > `docs` > `llm` > `none`) | best `source` among the method's conformance fixtures | **derived** |
| **SDK-called** (hint only) | scan of `tests/smoke`, `tests/e2e` for `client.<method_with_underscores>(` | **derived** |
| Client versions | `tests/smoke/uv.lock` | **derived** |
| `docs/COMPATIBILITY.md` | `make docs` | **generated** (a test fails if stale) |

Evidence levels: `recorded` = a sanitized real Slack response is committed; `docs` = checked against a
cited source (`ref`); `llm` = written from model knowledge, never checked (the honest default for anything
you write from memory); `none` = no fixture. Say `llm` when that is what it is: users rely on this table.

Optional per-method `verify = "unlikely"`: set it on methods we expect never to record (admin.*,
Enterprise-only, destructive). They drop out of `make evidence-gaps` and the matrix says so.

## Status lifecycle
`planned` -> `in_progress` -> `implemented` (or `partial` if a high-severity deviation means real apps will
notice, e.g. `chat.postMessage` ignoring blocks). **Claim work by setting `status = "in_progress"` and
`plan = "agent-docs/plans/<your plan>.md"`** (the file must exist), as its own small commit **on `main`**
first (claims on a feature branch are invisible to parallel agents). Before starting, check main:
`git fetch && git show origin/main:internal/capabilities/capabilities.toml | rg -B1 -A4 'in_progress'`.
If the method is `in_progress`, talk to the owner instead of duplicating it. Claims never expire on
their own: if one looks stale, ask the owner.

`docs/COMPATIBILITY.md` is generated and will conflict between branches: on conflict, take either side
and run `make docs`. Never hand-merge it.

## Checklist when you add or change behavior
1. Register the method in `internal/api/api.go` `registry()` (see `endpoint_guide.md`).
2. Set/adjust its entry in `capabilities.toml` (status, tier, scopes from Slack's docs, deviations). Add a
   `[deviations.<id>]` for anything you knowingly do differently; remove ones you fix.
3. Add a conformance fixture in `tests/conformance/fixtures/` with a truthful `source` (see
   `tests/conformance/RECORDING.md`); a shipped method without one fails the tests unless you set
   `fixtures = "waived: <reason>"`.
4. Call it through the real SDK in a smoke test (this is what flips SDK-called).
5. `make docs`, then `make verify`.

## What fails, and why
- `TestRegistryParity`: the manifest and `api.MethodNames()` disagree (method added/removed/status wrong).
- `TestEmbeddedManifestIsValid`: bad enum, unknown deviation id, `in_progress` without an existing plan file,
  behavior without summary/evidence, `docs` evidence without a `ref`.
- `TestEveryShippedMethodHasAFixture`, `TestRepoFixturesAreValid`: fixture missing or provenance incomplete.
- `TestCompatibilityDocIsCurrent`: run `make docs` and commit the result (never edit COMPATIBILITY.md by hand).
- `make redact-check`: a fixture could leak a token, email, workspace URL, real ID or long free text.

## Notes
- `tier` and `scopes` are transcribed from Slack's docs from memory and are **unverified** until a recording
  or a cited doc confirms them. M4 (scopes) and M5 (rate limits) should read them from this embedded file
  rather than adding a second table.
- `make evidence-gaps` lists shipped methods still lacking a real recording (see `CONTRIBUTING.md`).
