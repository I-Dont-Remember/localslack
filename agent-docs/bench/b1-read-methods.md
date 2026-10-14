# Bench task: B1 read methods

Fixed task for A/B replays (`scripts/agent-bench.sh`). Base ref `9d20c27` (test-loop right before stream B1
merged; Wave 0 extension points exist). Original run: 124k peak context, 39 turns, ~7M cache-read tokens.
Adapted from the Wave 1 stream B1 brief; only the branch/orchestrator wording changed.

---

You are implementing the Bolt-core read methods in the LocalSlack repo (Go Slack emulator). Commit on the current branch.

READ FIRST: CLAUDE.md files (root, agent-docs, internal), agent-docs/workflow.md, coding_standards.md, endpoint_guide.md (THE workflow for each method), capabilities.md, agent-docs/plans/test-loop.md (stream B1 + Wave 0 handoff extension points), docs/qa/bolt-core.md (shared stub: add only a "Read methods" section; stream B2 adds its own section — keep your edits to a clearly delimited section to ease merging).

COMMON RULES: red-green TDD, table-driven got/want tests, parallel + shuffled; atomic commits each passing `make check && make test`; explicit `git add` paths only; never commit .claude/settings.local.json; commits run pre-commit (run unsandboxed if sandbox blocks); terse commit messages ending `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`; no literal xox*/xapp- tokens in repo; scratch in $TMPDIR/./tmp. Stay inside files you own; use the Wave 0 extension points (a.<x>Methods registration funcs in api/methods_<x>.go, one registry() line, errors_<stream>.go). capabilities.toml: only flip YOUR method entries (already claimed in_progress with plan) to implemented/partial; never hand-edit docs/COMPATIBILITY.md — run `make docs`. Smoke needs loopback: rerun unsandboxed on sandbox-evidence failures. Python tests in new tests/smoke/*_test.py; leave conftest.py alone.

SCOPE (follow endpoint_guide.md for each: handler test with storetest.NewApp, service/store code, DTO mappers, conformance fixture with truthful `source`, SDK smoke call, manifest update, deviations + red test if any):
- `conversations.info` -> {channel: conversationDTO}; include_num_members; channel_not_found.
- `users.info` -> {user: userDTO}; user_not_found.
- `conversations.members` -> {members:[ids], response_metadata:{next_cursor}} using existing cursor pagination; channel_not_found.
- `api.test` -> authNone spec; {ok:true,args:{...echo of params}}; with `error=x` -> {ok:false,error:"x",args}.
Do not implement membership strictness (stream X later). Use the Wave 0 service-owned types (Conversation.IsPrivate, User.BotID) in DTOs where they apply. Fill the "Read methods" section of docs/qa/bolt-core.md for a human QA-ing (Setup/Steps with slack_sdk or curl/jq, Expected, Feedback prompts).
Finish: `make verify` green in your worktree. REPORT: commits, done/not done.
