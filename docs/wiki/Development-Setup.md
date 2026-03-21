# Development Setup

## Prerequisites

- Go 1.26+
- Dart 3.3+ / Flutter 3.19+
- Android SDK with `adb` in PATH
- Xcode with `xcrun simctl` (macOS only, for iOS testing)

## Build

```bash
make build              # Build probe binary -> bin/probe
make install            # Install to $GOPATH/bin
make build-convert      # Build probe-convert -> bin/probe-convert
```

## Test

```bash
# Go unit tests
make test               # go test ./...
go test -v -race -coverprofile=coverage.out ./...

# Single package
go test -v ./internal/parser/
go test -v -run TestName ./internal/runner/

# Dart agent tests
cd probe_agent && flutter test

# probe-convert tests
make test-convert                 # Unit tests
make test-convert-integration     # Golden files + lint + dry-run

# E2E health check (requires emulator/simulator)
./tests/e2e_cli_params/health_check.sh
```

## E2E Testing

### Quick Start

```bash
# 1. Build the CLI
make build

# 2. Start an Android emulator or iOS simulator

# 3. Build your Flutter app with ProbeAgent
cd your-flutter-app
flutter run --debug --dart-define=PROBE_AGENT=true

# 4. Run tests
bin/probe test tests/ --device emulator-5554 -y -v
```

### Health Check

The health check script validates all CLI parameters against local devices:

```bash
./tests/e2e_cli_params/health_check.sh        # Run all phases
./tests/e2e_cli_params/health_check.sh 1       # Offline tests only
./tests/e2e_cli_params/health_check.sh 2       # Android tests only
./tests/e2e_cli_params/health_check.sh 3       # iOS tests only
```

Results are saved to `tests/e2e_cli_params/results.html`.

## Project Structure

```
cmd/probe/              CLI entry point
internal/
  cli/                  Cobra commands
  parser/               ProbeScript lexer + parser
  runner/               Test execution engine
  probelink/            WebSocket JSON-RPC client
  device/               Android ADB integration
  ios/                  iOS simctl integration
  config/               probe.yaml handling
  cloud/                Cloud provider integration
  report/               HTML report generation
  ai/                   Self-healing + test generation
  visual/               Visual regression testing
  plugin/               Custom command plugins
  migrate/              Maestro YAML migration (legacy)
probe_agent/            Dart on-device agent
tools/probe-convert/    Multi-format test converter
website/                Documentation site (Starlight)
tests/                  E2E test suites
```

## Code Style

- Go: standard `gofmt`, no external linter config needed
- Dart: standard `dart format`
- Commits: Conventional Commits format (`feat:`, `fix:`, `chore:`, etc.)
- No Co-Authored-By lines in commits

## Release

```bash
./scripts/release.sh 0.2.0    # Bumps VERSION, pubspec.yaml, package.json, commits, tags
git push origin main --tags    # Triggers release workflow
```
