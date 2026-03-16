.PHONY: build build-convert install test test-convert test-convert-integration lint clean deps

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

## Build probe-convert tool
build-convert:
	cd tools/probe-convert && go build -o ../../$(OUT_DIR)/probe-convert .

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

## Run unit tests (probe CLI)
test:
	go test ./...

## Run probe-convert unit tests
test-convert:
	cd tools/probe-convert && go test ./convert/...

## Run probe-convert integration tests (golden files + lint + dry-run verify)
## Requires probe binary — builds it first if missing.
test-convert-integration: build build-convert
	cd tools/probe-convert && go test -v -run "TestGoldenFiles|TestLintGeneratedOutput|TestVerifyDryRun" .

## Lint .probe test files in the tests/ directory
lint:
	./$(OUT_DIR)/$(BINARY) lint tests/ || true

## Clean build artifacts
clean:
	rm -rf $(OUT_DIR)
