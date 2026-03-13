.PHONY: build install test lint clean deps

BINARY  = probe
OUT_DIR = bin

## Download dependencies
deps:
	go mod tidy
	go mod download

## Build the probe binary
build: deps
	@mkdir -p $(OUT_DIR)
	go build -ldflags="-s -w" -o $(OUT_DIR)/$(BINARY) ./cmd/probe

## Install probe to GOPATH/bin
install: deps
	go install ./cmd/probe

## Cross-compile for all platforms
build-all: deps
	@mkdir -p $(OUT_DIR)
	GOOS=linux   GOARCH=amd64 go build -o $(OUT_DIR)/probe-linux-amd64     ./cmd/probe
	GOOS=darwin  GOARCH=amd64 go build -o $(OUT_DIR)/probe-darwin-amd64    ./cmd/probe
	GOOS=darwin  GOARCH=arm64 go build -o $(OUT_DIR)/probe-darwin-arm64    ./cmd/probe
	GOOS=windows GOARCH=amd64 go build -o $(OUT_DIR)/probe-windows-amd64.exe ./cmd/probe

## Run unit tests
test:
	go test ./...

## Lint .probe test files in the tests/ directory
lint:
	./$(OUT_DIR)/$(BINARY) lint tests/ || true

## Clean build artifacts
clean:
	rm -rf $(OUT_DIR)
