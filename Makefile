.PHONY: lint-py build run dev tidy clean test conformance smoke test-all verify setup fmt lint vet check review merge wsl-cover

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

smoke:
	scripts/with-server.sh uv run --project tests/smoke pytest tests/smoke/ -v

test-all: test conformance smoke

# single "am I done" command
verify: check test-all

setup:
	mise install
	pre-commit install
	uv sync --project tests/smoke

fmt:
	golangci-lint fmt ./...
	uv run --project tests/smoke ruff format tests

lint:
	golangci-lint run ./...

lint-py:
	uv run --project tests/smoke ruff check tests
	uv run --project tests/smoke ruff format --check tests

vet:
	go vet ./...

check:
	golangci-lint fmt --diff ./...
	$(MAKE) vet
	$(MAKE) lint
	$(MAKE) lint-py

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
