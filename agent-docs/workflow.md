# Agent Workflow

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

## 2. Make changes

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

When done, push the branch and open a PR:

```bash
git push -u origin <branch>
gh pr create --title "<title>" --body "<what and why>"
```

The PR is reviewed locally by the user via `gh pr checkout`.
