# Installing the Claude Code hooks

The scripts here are tested (`make hooks-test`) but only run once wired into
`.claude/settings.json`, which agents can't edit (sandbox-protected). Merge this by hand:

```json
{
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash", "hooks": [{ "type": "command", "command": "${CLAUDE_PROJECT_DIR}/scripts/claude-hooks/pre-bash-guard.sh" }] }
    ],
    "PostToolUse": [
      { "matcher": "Edit|Write", "hooks": [{ "type": "command", "command": "${CLAUDE_PROJECT_DIR}/scripts/claude-hooks/post-edit-fmt.sh" }] }
    ]
  },
  "sandbox": {
    "filesystem": { "allowWrite": ["~/.cache/pre-commit"] }
  }
}
```

(Keep the existing `allowWrite` entries; add `~/.cache/pre-commit` so `git commit` works sandboxed.)
Verify the syntax against https://code.claude.com/docs/en/hooks.md; in worktrees
`$CLAUDE_PROJECT_DIR` is the worktree, so the scripts resolve per-worktree.

Notes:
- Block events are appended to `tmp/logs/hook-events.jsonl` (see the review's measurement section).
- Loopback for `make smoke|e2e`: `sandbox.network.allowLocalBinding` is reported macOS-only; on Linux
  keep running those unsandboxed.
- Optional skill `.claude/skills/orchestrate/SKILL.md` (also protected):

```markdown
---
name: orchestrate
description: Use before fanning out parallel subagents or worktree streams for LocalSlack work
---
Read agent-docs/orchestration.md and follow it: max 3 agents, ~40-60 call streams, contract settled
before code, briefs <=1.5k chars with doc line ranges, merge via scripts/merge-stream.sh.
```
