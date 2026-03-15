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

# Run E2E tests against a device
bin/probe test <test-path>/ --device <UDID-or-serial> --timeout 60s -v -y

# Run E2E tests with custom ADB path and JSON output for report generation
bin/probe test tests/ --device emulator-5554 --timeout 60s -v -y \
  --format json -o reports/results.json --adb /path/to/adb

# Generate HTML report from JSON results
bin/probe report --input reports/results.json -o reports/report.html --open
```

## Architecture

The system has two main components that communicate via WebSocket + JSON-RPC 2.0:

1. **Go CLI (`cmd/probe/`)**: Parses `.probe` files, manages devices, orchestrates test runs, generates reports
2. **Dart ProbeAgent (`probe_agent/`)**: Runs on-device as a WebSocket server (port 8686), executes commands against the live Flutter widget tree

### Connection Flow

- **Android**: CLI → `adb forward tcp:8686 tcp:8686` → WebSocket connect → authenticate via one-time token (extracted from `adb logcat` matching `PROBE_TOKEN=...`) → dispatch JSON-RPC commands
- **iOS Simulator**: CLI → `localhost:8686` directly (simulator shares host loopback) → authenticate via token file at `~/Library/Developer/CoreSimulator/Devices/<UDID>/data/tmp/probe/token` (fast path) or log stream (fallback)

### Go CLI internals (`internal/`)

- **`cli/`** — Cobra command definitions (`probe test`, `probe init`, `probe lint`, `probe device`, `probe record`, `probe report`, `probe migrate`, `probe generate`)
- **`parser/`** — Indent-aware lexer + recursive-descent parser producing an AST. ProbeScript is Python-like with indent-based blocks. Key types: `Program → UseStmt | RecipeDef | HookDef | TestDef`, each containing `Step` nodes
- **`runner/`** — Test orchestration: `runner.go` loads recipes/files/tags, `executor.go` walks AST steps and dispatches to ProbeLink, `reporter.go` outputs Terminal/JUnit/JSON, `device_context.go` handles platform-level operations (restart, clear data, permissions, reconnection)
- **`probelink/`** — JSON-RPC 2.0 WebSocket client. Methods like `probe.tap`, `probe.type`, `probe.see`, `probe.wait`, `probe.screenshot`, etc.
- **`device/`** — ADB integration for Android emulator management, port forwarding, token extraction, permission grant/revoke. `permissions.go` maps human-readable names to platform constants.
- **`ios/`** — iOS simulator management via `xcrun simctl`: boot, install, launch, log streaming, token reading
- **`config/`** — `probe.yaml` parsing (project name, app ID, platform, timeouts, device list, environment vars)
- **`ai/`** — Self-healing selectors via fuzzy matching against widget tree (strategies: text_fuzzy, key_partial, type_position, semantic)
- **`migrate/`** — Converts Maestro YAML test flows to ProbeScript
- **`plugin/`** — YAML-based custom command system; plugins define new ProbeScript commands that dispatch to Dart handlers
- **`visual/`** — Screenshot-based visual regression testing with configurable threshold

### Dart Agent internals (`probe_agent/lib/src/`)

- **`server.dart`** — WebSocket server with token auth; prints `PROBE_TOKEN=` every 3 seconds and writes token to a file for CLI pickup
- **`executor.dart`** — Command dispatcher (largest file); resolves selectors, executes actions (tap, type, see, swipe, screenshot, etc.)
- **`finder.dart`** — Widget selector engine walking the live widget tree via `WidgetsBinding.instance.rootElement`. Supports text, `#id` (ValueKey), type, ordinal (`1st "Item"`), positional (`"text" in "Container"`)
- **`sync.dart`** — Triple-signal synchronization (frames + animations + microtasks) for flake prevention
- **`gestures.dart`** — Tap, swipe, scroll, drag, pinch, rotate handlers

## Critical Implementation Details

### ProbeAgent does NOT use flutter_test

The Dart agent runs inside a **production Flutter app** using `WidgetsFlutterBinding` (not `TestWidgetsFlutterBinding`). This means:

- **No `flutter_test` finders** — `find.text()`, `find.byKey()`, etc. will crash with null check errors because they require `TestWidgetsFlutterBinding`. Instead, `finder.dart` walks the widget tree manually via `WidgetsBinding.instance.rootElement`.
- **No `TestGesture`** — The `TestGesture` class is test-framework only. Use `GestureBinding.handlePointerEvent()` with `PointerDownEvent`/`PointerUpEvent` directly.
- **No `tester.enterText()`** — Platform text input channels don't work outside the test framework. Instead, find the `TextEditingController` in the widget tree and set `.text` directly.

### Text input strategy

The `_typeText` method in `executor.dart`:
1. Finds the target element by selector
2. Walks up/down the element tree to find a `TextEditingController` (from `EditableText` or `TextField`)
3. Sets `controller.text = value` and `controller.selection = TextSelection.collapsed(offset: value.length)`
4. If no controller found on the target, taps the field first to focus it, then finds the focused controller

### iOS simulator token reading

`internal/ios/simctl.go` `ReadToken()`:
1. **Fast path**: Reads token from file at `~/Library/Developer/CoreSimulator/Devices/<UDID>/data/tmp/probe/token`
2. **Fallback**: Streams simulator logs with predicate `eventMessage CONTAINS "PROBE_TOKEN="` and parses the token from matching lines
3. Server prints `PROBE_TOKEN=<token>` every 3 seconds to handle late-connecting CLI
4. Must filter out the log stream header line which itself contains "PROBE_TOKEN=" — check `len(token) >= 16 && !strings.HasPrefix(token, "\"")`

### OS-level permission handling

ProbeAgent can only interact with **Flutter widgets**, not native OS permission dialogs. FlutterProbe handles these at the platform level:

- **Android**: `adb shell pm grant/revoke <package> <permission>` — works on debug builds, handles all runtime permissions
- **iOS**: `xcrun simctl privacy grant/revoke <udid> <service> <bundleID>` — works on simulators (iOS 14+)
- **ProbeScript commands**: `allow permission "notifications"`, `deny permission "camera"`, `grant all permissions`, `revoke all permissions`
- **Auto-grant on clear**: When `-y` flag is used or `grant_permissions_on_clear: true` in probe.yaml, all permissions are automatically granted after `clear app data` — preventing permission dialogs from blocking tests
- **Permission map**: Human-readable names (`notifications`, `camera`, `location`, `microphone`, `storage`, `contacts`, `phone`, `calendar`, `sms`, `bluetooth`) are mapped to platform-specific constants in `internal/device/permissions.go`

### App lifecycle operations

- **`restart the app`** — force-stops via `am force-stop` (Android) / `simctl terminate` (iOS), relaunches, reconnects WebSocket. Preserves app data.
- **`clear app data`** — wipes all data via `pm clear` (Android) / container subdirectory deletion (iOS), relaunches, reconnects. Requires `-y` flag or interactive confirmation.
- Both operations are handled by `DeviceContext` in `internal/runner/device_context.go` and transparently reconnect the WebSocket client after the app restarts.
- iOS data clearing uses path validation (`validateIOSDataPath`) to prevent accidental deletion of non-container paths.

### Configurable tool paths

The `--adb` and `--flutter` CLI flags (or `tools:` in probe.yaml) allow overriding binary paths for CI/CD or non-standard installations. Resolution order: CLI flag → probe.yaml → PATH.

### iOS build considerations

- When Xcode SDK version differs from simulator iOS version, `flutter run` may fail to match the destination. Use `xcodebuild` directly with `-destination 'platform=iOS Simulator,id=<UDID>'` as a workaround.
- DART_DEFINES must be base64-encoded when passed to xcodebuild: `DART_DEFINES=$(echo 'PROBE_AGENT=true' | base64)`
- Use `clear app data` in tests to reset persisted login state between runs (no need for uninstall/install cycles)

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

Key constructs: `test`, `recipe` (reusable steps with params), `before each`/`after each` hooks, `on failure` hooks, `if`/`else` conditionals, `repeat N times` loops, `Examples:` blocks for data-driven tests with `<var>` substitution, `dart:` blocks for escape-hatch Dart code, `when ... respond with` for HTTP mocking, `clear app data` / `restart the app` for lifecycle control, `allow permission` / `deny permission` / `grant all permissions` for OS permission management.

## Key Dependencies

- Go 1.23, Dart 3.3+, Flutter 3.19+
- `github.com/gorilla/websocket` — WebSocket client
- `github.com/spf13/cobra` — CLI framework
- `gopkg.in/yaml.v3` — YAML parsing
