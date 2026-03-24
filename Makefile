.PHONY: run build test tidy clean dev fmt lint vet check review merge smoke conformance test-all

AIR 		  := air
APP           := localslack
CMD_DIR       := ./cmd/$(APP)
BIN_DIR       := ./bin
BIN           := $(BIN_DIR)/$(APP)
GOTESTFLAGS   ?= -race -count=1 -shuffle=on
LDFLAGS       ?= -s -w

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

.PHONY: build
build: $(BIN_DIR)
	go build -ldflags "$(LDFLAGS) $(LDVERSION)" -o $(BIN) $(CMD_DIR)

run:
	go run $(CMD_DIR) start

dev:
	$(AIR)

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)

test:
	go test $(GOTESTFLAGS) ./...

conformance:
	go test ./tests/conformance/...

smoke: build
	@./bin/localslack start -d; \
	sleep 1; \
	uv run --project tests/smoke pytest tests/smoke/ -v; EXIT=$$?; \
	./bin/localslack stop; \
	exit $$EXIT

test-all: test conformance smoke

fmt:
	golangci-lint fmt ./...

lint:
	golangci-lint run ./...

vet:
	go vet ./...

check:
	golangci-lint fmt --diff ./...
	$(MAKE) vet
	$(MAKE) lint

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
