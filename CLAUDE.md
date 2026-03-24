# CLAUDE.md

Before coding, use AskUserQuestion to clarify requirements and constraints.

Agents MUST keep CLAUDE.md files concise & small. If docs need to be extensive, create a separate file.

Project name: `LocalSlack`, also referred to in code as `localslack`. It is NOT `local Slack` with a space when referring to this project.

See `STATUS.md` for current milestone and progress.

## Agent usage

- `agent-docs/` is intended for agents to use for gaining useful context about the repo & coding practices for it, as well as where new documentation should be written if it's intended for agents.
- `agent-docs/endpoint_guide.md` describes the workflow for implementing new Slack API endpoints.
- `agent-docs/workflow.md` is the required process for all repo changes — read it before starting work.
- `docs/` is for both humans & agents.
  - `docs/DESIGN_DOC.md` has the  initial plans which will get us the core of the application built out, including relevant milestones & technical choices.
- you can use `./tmp/` as a playground since I have it `gitignored`. Examples of use: if you need to write small scripts or things to test assumptions and see how bite-size pieces function, or drop some temporary log files, etc.

## Tech choices

- language: Go
- CLI with embedded web server
  - Kong for the CLI library

## Development

- DO NOT commit unless we are passing all formatters, linters, & other quality of life tools. Unit tests must be fully passing.
- Prefer simple solutions, unless the simple version creates more complexity elsewhere.
- Prefer small functions & concise files.
- all app code lives in `internal/`, there are no exported packages.
- Prefer automation to avoid back-n-forth during development to deal with mundane fixes like formatting, linting errors, etc.
  - Use scripts and the tools at our disposal such as pre-commit, golangci-lint, custom bash scripts, Makefile to encapsulate multi-step processes into single easy commands.

## Code Quality

- Use red-green TDD to ensure tests fail first.
- unit tests should be configured to run randomly & parallelized early on. Avoid accidental dependencies.
- Use table driven tests with subtests (t.Run)
- Use got/want vs actual expecter when naming identifiers in tests
- If a function returns a struct or slice, don't write test code that performs individual comparisons for each field. Instead compare against an expected struct/slice as Golang testing libraries should support comparison of entire struct/slice.
