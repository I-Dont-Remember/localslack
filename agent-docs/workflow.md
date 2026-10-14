# Agent Workflow

## 0. Resuming work

Read `STATUS.md` -> active plan in `plans/` -> any `## Handoff` -> first unchecked task. Tick plan boxes in the commit that completes them.

## Commands

| Command | Purpose |
|---|---|
| `make setup` | one-time: mise tools, pre-commit hooks, uv env |
| `make check` | gofmt/gofumpt diff, vet, golangci-lint, ast-grep rules, ruff |
| `make test` | Go unit tests (`-race -shuffle=on`, excludes `tests/`); pre-commit only runs the changed packages |
| `make conformance` / `make conformance-gaps` | fixture replay / list known gaps vs real Slack |
| `make conformance-update-gaps` | rewrite `gap_diffs` of `known_gap` fixtures from observed differences (after recording) |
| `make smoke` | Python SDK + Bolt tests against a live isolated server (incl. red tests for known deviations) |
| `make tools-test` / `make redact-check` | tests for the recording tools / fail if a fixture could leak tokens, emails, IDs |
| `make docs` / `make evidence-gaps` | regenerate `docs/COMPATIBILITY.md` from the capability manifest / list methods lacking a real recording |
| `make e2e` | Playwright browser tests (UI + SDK) against a live isolated server; failure screenshots in `tmp/e2e-artifacts` |
| `make verify` | everything above; the "am I done" command |
| `scripts/with-server.sh <cmd>` | run any command against an isolated live server (`$LOCALSLACK_URL`) |
| `make run` / `make dev` | run server (:3232) / hot reload with air |
| `make t PKG=./internal/x [RUN=TestY]` | fast package-scoped tests (the inner loop) |
| `make lint-changed` | golangci-lint on changes since `BASE` (default `main`; set to the parent branch when stacked) + ast-grep |
| `make stress PKG=... RUN=TestY` | `-race -count=50` flake hunt |
| `make gen` | regenerate generated files (`docs/COMPATIBILITY.md`); run it when `TestCompatibilityDocIsCurrent` fails |
| `make debt` | report-only consolidation targets: near-duplicates (dupl + art-dupl), expired `//art-dupl:accept` markers, dead code, repeated patterns (`report-rules/`), size outliers, churn |
| `make hooks-test` / `make trace-test` | tests for `scripts/` tooling (hooks, quiet.sh, merge-stream.sh, trace_stats.py) |
| `make trace-stats SESSION=<id>` | token/efficiency stats of a Claude session (`VS=<id>` to compare); see `reviews/` |

`make check|test|smoke|e2e|...` are **quiet**: one `ok` line on success, only failures otherwise; the full log is `tmp/logs/<name>.log`. `V=1` shows raw output.

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
- Check `internal/capabilities/capabilities.toml` before starting feature work and keep it current in the same commits (`agent-docs/capabilities.md`); `make test` fails if it drifts from the code, and `make docs` regenerates `docs/COMPATIBILITY.md`.
- Inner loop: `make t PKG=./internal/x` while coding -> `make lint-changed` -> `make check` before each commit -> `make verify` once before handing back. Don't run the full suite after every edit.
- Edit files with the Edit/Write tools and read with Read (offset/limit); use Bash for `make`/`git`/`go`/`rg`. A PreToolUse hook (`scripts/claude-hooks/`) blocks `sed -i`, python/`cat` heredoc writes and long sleeps.
- Test idioms that avoid the lint failures agents hit most: fixtures as funcs, not package-level vars (`gochecknoglobals`); `errors.Is/As`, not `==`/type assertions (`errorlint`); `t.Helper()` in helpers (`thelper`); `defer resp.Body.Close()` (`bodyclose`); `t.Parallel()` in the test and each `t.Run` (`paralleltest`); no unused params/returns (`unparam`); poll a condition, never a bare `time.Sleep` (`no-time-sleep-bare-in-tests`).

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

Before handback run `make debt BASE=<parent branch>`. If the branch adds a duplicate cluster or the third instance of a tracked pattern (`report-rules/`), launch the `dry-consolidator` agent (`.claude/agents/dry-consolidator.md`); it commits on a separate `consolidate-<branch>` branch for the owner to take or drop.

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
