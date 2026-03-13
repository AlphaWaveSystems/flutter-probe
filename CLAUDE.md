# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

FlutterProbe is a high-performance local E2E testing framework for Flutter mobile apps. It uses a natural language test syntax called ProbeScript (`.probe` files) with sub-50ms command execution via direct widget-tree access.

## Build & Development Commands

```bash
make build              # Build probe binary → bin/probe
make install            # Install to $GOPATH/bin
make test               # Run all Go unit tests (go test ./...)
make deps               # go mod tidy + download
make lint               # Lint .probe files in tests/ (requires build first)
make clean              # Remove bin/

# Run a single test package
go test -v ./internal/parser/
go test -v -run TestName ./internal/runner/

# Run with race detection and coverage
go test -v -race -coverprofile=coverage.out ./...

# Dart agent tests
cd probe_agent && flutter test
```

## Architecture

The system has two main components that communicate via WebSocket + JSON-RPC 2.0:

1. **Go CLI (`cmd/probe/`)**: Parses `.probe` files, manages devices, orchestrates test runs, generates reports
2. **Dart ProbeAgent (`probe_agent/`)**: Runs on-device as a WebSocket server (port 8686), executes commands against the live Flutter widget tree

**Connection flow**: CLI → ADB port-forward 8686 → WebSocket connect → authenticate via one-time token (extracted from logcat `PROBE_TOKEN=...`) → dispatch JSON-RPC commands

### Go CLI internals (`internal/`)

- **`cli/`** — Cobra command definitions (`probe test`, `probe init`, `probe lint`, `probe device`, `probe record`, `probe report`, `probe migrate`, `probe generate`)
- **`parser/`** — Indent-aware lexer + recursive-descent parser producing an AST. ProbeScript is Python-like with indent-based blocks. Key types: `Program → UseStmt | RecipeDef | HookDef | TestDef`, each containing `Step` nodes
- **`runner/`** — Test orchestration: `runner.go` loads recipes/files/tags, `executor.go` walks AST steps and dispatches to ProbeLink, `reporter.go` outputs Terminal/JUnit/JSON
- **`probelink/`** — JSON-RPC 2.0 WebSocket client. Methods like `probe.tap`, `probe.type`, `probe.see`, `probe.wait`, `probe.screenshot`, etc.
- **`device/`** — ADB integration for Android emulator management, port forwarding, token extraction
- **`config/`** — `probe.yaml` parsing (project name, app ID, platform, timeouts, device list, environment vars)
- **`ai/`** — Self-healing selectors via fuzzy matching against widget tree (strategies: text_fuzzy, key_partial, type_position, semantic)
- **`migrate/`** — Converts Maestro YAML test flows to ProbeScript
- **`plugin/`** — YAML-based custom command system; plugins define new ProbeScript commands that dispatch to Dart handlers
- **`visual/`** — Screenshot-based visual regression testing with configurable threshold

### Dart Agent internals (`probe_agent/lib/src/`)

- **`server.dart`** — WebSocket server with token auth
- **`executor.dart`** — Command dispatcher (largest file); resolves selectors, executes actions
- **`finder.dart`** — Widget selector engine (text, `#id`, type, ordinal like `1st "Item"`, positional like `"text" in "Container"`)
- **`sync.dart`** — Triple-signal synchronization (frames + animations + microtasks) for flake prevention
- **`gestures.dart`** — Tap, swipe, scroll, drag, pinch, rotate handlers

## ProbeScript Language

Tests use natural language syntax with indent-based blocks:
```
test "user can log in"
  @smoke @critical
  open the app
  wait until "Sign In" appears
  tap on "Sign In"
  type "user@example.com" into the "Email" field
  tap "Continue"
  see "Dashboard"
```

Key constructs: `test`, `recipe` (reusable steps with params), `before each`/`after each` hooks, `on failure` hooks, `Examples:` blocks for data-driven tests with `<var>` substitution.

## Key Dependencies

- Go 1.23, Dart 3.3+, Flutter 3.19+
- `github.com/gorilla/websocket` — WebSocket client
- `github.com/spf13/cobra` — CLI framework
- `gopkg.in/yaml.v3` — YAML parsing
