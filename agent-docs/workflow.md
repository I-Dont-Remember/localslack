# Agent Workflow

## 0. Resuming work

Read `STATUS.md` -> active plan in `plans/` -> any `## Handoff` -> first unchecked task. Tick plan boxes in the commit that completes them.

## Commands

| Command | Purpose |
|---|---|
| `make setup` | one-time: mise tools, pre-commit hooks, uv env |
| `make check` | gofmt/gofumpt diff, vet, golangci-lint, ast-grep rules, ruff |
| `make test` | Go unit tests (`-race -shuffle=on`, excludes `tests/`) |
| `make conformance` / `make conformance-gaps` | fixture replay / list known gaps vs real Slack |
| `make smoke` | Python SDK tests against a live isolated server |
| `make verify` | everything above; the "am I done" command |
| `scripts/with-server.sh <cmd>` | run any command against an isolated live server (`$LOCALSLACK_URL`) |
| `make run` / `make dev` | run server (:3232) / hot reload with air |

Gotchas: the repo root has untracked unrelated dotfiles (`.bashrc`, `.idea`, ...) - never `git add -A` at root; add specific paths. Never commit `.claude/settings.local.json`. Playground dir: `./tmp/` (gitignored; ast-grep and git ignore it, but golangci-lint does not - delete stray `.go` files there).

## 1. Validate initial state

Before any changes, confirm the repo is healthy:

```bash
make check && make test
```

Both must exit 0. If they don't, fix or report before proceeding.

Also run:

```bash
git status
```

Note any uncommitted changes. Don't clobber in-progress work.

**Sandbox note:** if `uv` fails with a read-only cache error, set `UV_CACHE_DIR=$TMPDIR/uv` for that command. Commits run pre-commit (cache outside the sandbox allowlist), so `git commit` needs an unsandboxed run/approval.

## 2. Make changes

- Before creating a feature branch, check the current branch. If it is **not** `main`, use `AskUserQuestion` to ask whether to proceed (stacking on the existing branch) or stop so they can merge the current branch first.
- Work in a feature branch: `git checkout -b <short-description>`
- Follow TDD: write a failing test first, then implement.
- After each logical chunk of changes, re-run `make check && make test` to stay green.

## 3. Verify functionality

For every change, verify the actual behavior works — not just that linters pass:

- **Server/handler changes:** start the server (`make run`) and curl the affected endpoint.
- **CLI changes:** run the binary and exercise the changed command.
- **Pure logic changes:** ensure the new/updated tests cover the real behavior.

Don't ship a change that only passes linters without demonstrating it works.

## 4. Commit and open for review

Commits must be atomic — one logical change per commit, passing `make check && make test` at each.

```bash
git add <specific files>   # never git add -A blindly
git commit -m "<concise imperative summary>"
```

When done, open the branch for review with `difit`:

```bash
make review          # pipes git diff main..HEAD into difit
```

Or manually:

```bash
git diff main HEAD | difit
```

The user reviews the diff locally. **Agents must not run `make merge`** — merging into main is a human-only action.

Before opening for review, update `agent-docs/STATUS.md` (current milestone only) and add an entry to `agent-docs/CHANGELOG.md` if a milestone completed.
