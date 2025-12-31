.PHONY: run build test tidy clean dev

AIR 		  := air
APP           := localslack
CMD_DIR       := ./cmd/$(APP)
BIN_DIR       := ./bin
BIN           := $(BIN_DIR)/$(APP)
GOTESTFLAGS   ?= -race -count=1
LDFLAGS       ?= -s -w

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

.PHONY: build
build: $(BIN_DIR)
	go build -ldflags "$(LDFLAGS) $(LDVERSION)" -o $(BIN) $(CMD_DIR)

run:
	go run $(CMD_DIR)

dev:
	$(AIR)

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)

test:
	go test $(GOTESTFLAGS) ./...

# abstract this later, but gotta do BS on WSL to open file in browser
wsl-cover:
	 go test $(GOTESTFLAGS) -coverprofile=coverage.out ./...
	 go tool cover -html=coverage.out -o coverage.html
	explorer.exe coverage.html