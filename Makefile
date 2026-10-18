BINARY_NAME=spedious
CMD_DIR=./cmd/spedious
BIN_DIR=./bin

.PHONY: all build test test-race lint vet run clean release help

all: build

build:
	@go build -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)

test:
	@go test -v ./...

test-race:
	@go test -v -race ./...

vet:
	@go vet ./...

lint:
	@golangci-lint run ./... || echo "golangci-lint not found or failed, running go vet" && go vet ./...

run: build
	@$(BIN_DIR)/$(BINARY_NAME) $(ARGS)

clean:
	@rm -rf $(BIN_DIR) coverage.out coverage.html dist

release:
	@goreleaser release --snapshot --clean

help:
	@echo "Spedious Makefile targets:"
	@echo "  build      Build Spedious binary into ./bin"
	@echo "  test       Run unit tests"
	@echo "  test-race  Run tests with race detection"
	@echo "  vet        Run go vet"
	@echo "  lint       Run golangci-lint"
	@echo "  run        Run binary with ARGS"
	@echo "  clean      Remove build artifacts"
	@echo "  release    Create snapshot release using goreleaser"
