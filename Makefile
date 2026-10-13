# Playwright browsers live in the (gitignored) repo tmp/ so setup/e2e never touch $$HOME.
export PLAYWRIGHT_BROWSERS_PATH ?= $(CURDIR)/tmp/pw-browsers

# Use system Chrome when Playwright's bundled Chromium is not installed (e.g. downloads blocked).
E2E_ARGS ?= $(if $(wildcard $(PLAYWRIGHT_BROWSERS_PATH)/chromium-*),,$(if $(shell command -v google-chrome),--browser-channel chrome))

.PHONY: demo demo-check tools-test redact-check docs evidence-gaps lint-py build run dev tidy clean test conformance conformance-gaps conformance-update-gaps smoke smoke-reset e2e test-all verify setup fmt lint vet check check-raw review merge wsl-cover t stress lint-changed gen hooks-test trace-test trace-stats

AIR 		  := air
APP           := localslack
CMD_DIR       := ./cmd/$(APP)
BIN_DIR       := ./bin
BIN           := $(BIN_DIR)/$(APP)
GOTESTFLAGS   ?= -race -count=1 -shuffle=on
LDFLAGS       ?= -s -w
UNIT_PKGS     := $$(go list ./... | grep -v /tests/)
PYTEST_V      := $(if $(V),-v,-q)

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

# Quiet by default: one "ok" line on success, only failures on error (full log in tmp/logs/<name>.log).
# V=1 shows raw output.
Q := scripts/quiet.sh

test:
	@$(Q) test go test $(GOTESTFLAGS) $(UNIT_PKGS)

# Fast inner loop for one package: make t PKG=./internal/service [RUN=TestX]
t:
	@test -n "$(PKG)" || { echo "usage: make t PKG=./internal/x [RUN=TestX]"; exit 2; }
	@$(Q) t go test $(GOTESTFLAGS) $(if $(RUN),-run '$(RUN)') $(PKG)

# Flake hunt: make stress PKG=./internal/x RUN=TestX [N=50]
stress:
	@test -n "$(PKG)" -a -n "$(RUN)" || { echo "usage: make stress PKG=./internal/x RUN=TestX [N=50]"; exit 2; }
	@$(Q) stress go test -race -shuffle=on -count=$(or $(N),50) -run '$(RUN)' $(PKG)

# Lint only what changed vs BASE (default main; stacked branches: BASE=<parent-branch>).
BASE ?= main
lint-changed:
	golangci-lint run --new-from-rev=$(BASE) ./...
	ast-grep scan

# Regenerate all generated files (currently docs/COMPATIBILITY.md).
gen: docs

hooks-test:
	scripts/hooks_test.sh

# Tests for scripts/trace_stats.py. Usage of the tool: make trace-stats SESSION=<id|path> [VS=<id|path>]
trace-test:
	uv run --project tests/smoke pytest -c tests/smoke/pyproject.toml tests/trace -q

trace-stats:
	@test -n "$(SESSION)" || { echo "usage: make trace-stats SESSION=<id|path> [VS=<id|path>]"; exit 2; }
	python3 scripts/trace_stats.py $(SESSION) $(if $(VS),--compare $(VS))

conformance:
	@$(Q) conformance go test $(GOTESTFLAGS) ./tests/conformance/...

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
	@$(Q) tools-test scripts/with-server.sh uv run --project tests/smoke pytest -c tests/smoke/pyproject.toml tests/conformance/ $(PYTEST_V)

smoke:
	@$(Q) smoke scripts/with-server.sh uv run --project tests/smoke pytest tests/smoke/ $(PYTEST_V) --ignore=tests/smoke/reset_test.py

# Suites that reset global state get their own server (skipped by `smoke`, and by pytest unless the env var is set).
smoke-reset:
	@LOCALSLACK_RESET_SUITE=1 $(Q) smoke-reset scripts/with-server.sh uv run --project tests/smoke pytest tests/smoke/reset_test.py tests/smoke/scenarios_test.py $(PYTEST_V)

# Browser tests (Playwright/Chromium). Failure screenshots land in tmp/e2e-artifacts.
e2e:
	@$(Q) e2e scripts/with-server.sh uv run --project tests/smoke pytest -c tests/smoke/pyproject.toml tests/e2e/ $(PYTEST_V) \
		--screenshot only-on-failure --output tmp/e2e-artifacts $(E2E_ARGS)

# Hands-on demo: LocalSlack + two Bolt apps on :3232 (Ctrl-C to stop); DEMO_MODE=socket for Socket Mode.
demo:
	scripts/demo.sh

# Non-interactive demo smoke: boots everything, mentions the bot, expects "pong".
demo-check:
	scripts/demo.sh --check

# Same demo over Socket Mode (apps connect out to LocalSlack; no request URLs).
demo-socket:
	DEMO_MODE=socket scripts/demo.sh

demo-check-socket:
	DEMO_MODE=socket scripts/demo.sh --check

test-all: test conformance tools-test trace-test hooks-test smoke smoke-reset e2e

# single "am I done" command
verify: check test-all

setup:
	mise install
	pre-commit install
	uv sync --project tests/smoke
	uv run --project tests/smoke playwright install chromium || echo "playwright chromium download failed; make e2e will use system Chrome if present"

fmt:
	golangci-lint fmt ./...
	uv run --project tests/smoke ruff format tests examples scripts

lint:
	golangci-lint run ./...
	ast-grep test
	ast-grep scan

lint-py:
	uv run --project tests/smoke ruff check tests examples scripts
	uv run --project tests/smoke ruff format --check tests examples scripts

vet:
	go vet ./...

check:
	@$(Q) check $(MAKE) --no-print-directory check-raw

check-raw:
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
