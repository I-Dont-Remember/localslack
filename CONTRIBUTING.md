# Contributing

Start with `README.md` (quickstart) and `agent-docs/workflow.md` (the required process for changes;
agents and humans follow the same one). `make verify` is the "am I done" command.

## Recording real Slack responses (the most valuable contribution)

LocalSlack's goal is to behave like real Slack, and most of what we know about Slack today is
**unverified** (see the `llm`/`none` rows in [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md)).
A response recorded from a real workspace upgrades a method to `recorded` evidence.

1. **Use a throwaway workspace** (a free one is fine) and a token with only the scopes you need.
   Never use a workspace that holds real conversations.
2. Pick work: `make evidence-gaps` lists implemented methods that still lack a recording.
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
