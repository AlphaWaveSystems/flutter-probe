.PHONY: build install test test-integration test-all update-golden deps clean lint

build:
	go build -o ../../bin/probe-convert .

install:
	go install .

## Run unit tests (fast, no external deps)
test:
	go test ./convert/...

## Run integration tests (requires 'probe' binary built at ../../bin/probe)
test-integration: build
	@test -f ../../bin/probe || (echo "error: probe binary not found — run 'make build' from the FlutterProbe root first" && exit 1)
	go test -v -run "TestGoldenFiles|TestLintGeneratedOutput|TestVerifyDryRun" .

## Run all tests
test-all: test test-integration

## Update golden files after intentional converter changes
update-golden:
	go test -run TestGoldenFiles -update .

deps:
	go mod tidy
	go mod download

clean:
	rm -f ../../bin/probe-convert

lint:
	go vet ./...
