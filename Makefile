# Playwright browsers live in the (gitignored) repo tmp/ so setup/e2e never touch $$HOME.
export PLAYWRIGHT_BROWSERS_PATH ?= $(CURDIR)/tmp/pw-browsers

# Use system Chrome when Playwright's bundled Chromium is not installed (e.g. downloads blocked).
E2E_ARGS ?= $(if $(wildcard $(PLAYWRIGHT_BROWSERS_PATH)/chromium-*),,$(if $(shell command -v google-chrome),--browser-channel chrome))

.PHONY: demo demo-check tools-test redact-check docs evidence-gaps lint-py build run dev tidy clean test conformance conformance-gaps conformance-update-gaps smoke smoke-reset e2e test-all verify setup fmt lint vet check review merge wsl-cover

AIR 		  := air
APP           := localslack
CMD_DIR       := ./cmd/$(APP)
BIN_DIR       := ./bin
BIN           := $(BIN_DIR)/$(APP)
GOTESTFLAGS   ?= -race -count=1 -shuffle=on
LDFLAGS       ?= -s -w
UNIT_PKGS     := $$(go list ./... | grep -v /tests/)

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

build: $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD_DIR)

run:
	go run $(CMD_DIR) start

dev:
	$(AIR)

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)

test:
	go test $(GOTESTFLAGS) $(UNIT_PKGS)

conformance:
	go test $(GOTESTFLAGS) ./tests/conformance/...

# Rewrite gap_diffs of known_gap fixtures from the observed differences (e.g. after recording).
conformance-update-gaps:
	LOCALSLACK_UPDATE_GAPS=1 go test -count=1 ./tests/conformance/...

conformance-gaps:
	@go test -v -count=1 ./tests/conformance/... | grep -A12 'KNOWN GAP' || echo 'no known gaps'

# Regenerate docs/COMPATIBILITY.md from the capabilities manifest (a test fails if it is stale).
docs:
	go run ./cmd/capctl docs

# Shipped methods that still need a real-Slack recording (weakest evidence first).
evidence-gaps:
	@go run ./cmd/capctl gaps

# Fail if any conformance fixture could leak tokens, emails, workspace URLs or real IDs.
redact-check:
	uv run --project tests/smoke python tests/conformance/redact_check.py

# Tests for the recording/redaction tools (records against a live isolated LocalSlack).
tools-test:
	scripts/with-server.sh uv run --project tests/smoke pytest -c tests/smoke/pyproject.toml tests/conformance/ -v

smoke:
	scripts/with-server.sh uv run --project tests/smoke pytest tests/smoke/ -v --ignore=tests/smoke/reset_test.py

# Suites that reset global state get their own server (skipped by `smoke`, and by pytest unless the env var is set).
smoke-reset:
	LOCALSLACK_RESET_SUITE=1 scripts/with-server.sh uv run --project tests/smoke pytest tests/smoke/reset_test.py -v

# Browser tests (Playwright/Chromium). Failure screenshots land in tmp/e2e-artifacts.
e2e:
	scripts/with-server.sh uv run --project tests/smoke pytest -c tests/smoke/pyproject.toml tests/e2e/ -v \
		--screenshot only-on-failure --output tmp/e2e-artifacts $(E2E_ARGS)

# Hands-on demo: LocalSlack + two Bolt apps on :3232 (Ctrl-C to stop); DEMO_MODE=socket for Socket Mode.
demo:
	scripts/demo.sh

# Non-interactive demo smoke: boots everything, mentions the bot, expects "pong".
demo-check:
	scripts/demo.sh --check

test-all: test conformance tools-test smoke smoke-reset e2e

# single "am I done" command
verify: check test-all

setup:
	mise install
	pre-commit install
	uv sync --project tests/smoke
	uv run --project tests/smoke playwright install chromium || echo "playwright chromium download failed; make e2e will use system Chrome if present"

fmt:
	golangci-lint fmt ./...
	uv run --project tests/smoke ruff format tests examples

lint:
	golangci-lint run ./...
	ast-grep test
	ast-grep scan

lint-py:
	uv run --project tests/smoke ruff check tests examples
	uv run --project tests/smoke ruff format --check tests examples

vet:
	go vet ./...

check:
	golangci-lint fmt --diff ./...
	$(MAKE) vet
	$(MAKE) lint
	$(MAKE) lint-py
	$(MAKE) redact-check

review:
	git diff main HEAD | difit

merge:
	@BRANCH=$$(git rev-parse --abbrev-ref HEAD) && \
	printf "Merge '$$BRANCH' into main? [y/N] " && read CONFIRM && [ "$$CONFIRM" = "y" ] && \
	git checkout main && \
	git merge --no-ff $$BRANCH

# abstract this later, but gotta do BS on WSL to open file in browser
wsl-cover:
	go test $(GOTESTFLAGS) -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	explorer.exe coverage.html
