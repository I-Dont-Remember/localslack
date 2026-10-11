# Contributing

Start with `README.md` (quickstart) and `agent-docs/workflow.md` (the required process for changes;
agents and humans follow the same one). `make verify` is the "am I done" command.

## Recording real Slack responses (the most valuable contribution)

LocalSlack's goal is to behave like real Slack, and most of what we know about Slack today is
**unverified** (see the `llm`/`none` rows in [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md)).
A response recorded from a real workspace upgrades a method to `recorded` evidence.

Recording is opt-in and best-effort. We don't provide a workspace, and many methods (admin.*,
Enterprise Grid, paid-plan features, anything destructive) may never be recorded. That's fine:
`docs`/`llm` evidence is shown honestly in the matrix. Only record what you can safely reach.

1. **Use a workspace you can afford to pollute** (a free personal one is ideal), a token with only the
   scopes you need, and prefer read-only methods. Never record from a workspace holding real
   conversations or data you don't have the right to publish. If you have no such workspace, skip
   recording; docs-based corrections (a `ref` URL on a fixture, a `[deviations.*]` entry) are welcome too.
2. Pick work: `make evidence-gaps` lists implemented methods that still lack a recording (methods
   marked `verify = "unlikely"` in the manifest are left out). The owner records the core
   `bolt-core`/`common` methods; other recordings are opportunistic, and nobody should chase the rest.
3. Record, replaying an existing fixture (its `setup` + `request`) against real Slack:
   ```
   export SLACK_TOKEN=xoxb-...   SLACK_WORKSPACE_TYPE=free
   uv run --project tests/smoke python tests/conformance/record.py \
       --from tests/conformance/fixtures/conversations_replies.json
   ```
   This writes `<name>__recorded.json` next to the fixtures and never overwrites. It creates real
   objects in the workspace (a random suffix is added to `name` params so reruns don't collide).
4. `make conformance-update-gaps` records how LocalSlack differs as `gap_diffs`
   (or flips the fixture to `passing` if it already matches).
5. **Review the file by hand**, then `make redact-check` (also part of `make check`). It fails on
   tokens, emails, workspace URLs, literal Slack IDs and long free text. Trim arrays to what matters
   (arrays are matched by length). Keep the `recorded` provenance block (date, workspace type, scopes).
6. `make docs` regenerates `docs/COMPATIBILITY.md` (the evidence level is derived from fixtures), then
   run `make verify` and open a PR. If the recording shows we are wrong about something that matters,
   add or update a `[deviations.*]` entry in `internal/capabilities/capabilities.toml`.

**Never commit tokens, workspace names or real user data.** If you are unsure whether something is
safe to publish, leave it out.

## Adding or changing a Slack method

Follow `agent-docs/endpoint_guide.md`. The capability manifest
(`internal/capabilities/capabilities.toml`, see `agent-docs/capabilities.md`) is test-enforced: a method
in the code but not the manifest, or vice versa, fails `make test`.

Realistic fake Slack tokens (`xoxb-...`, `xapp-...`) must come from `tests/conformance/fake_tokens.py`, never literals: GitHub push protection rejects them.
