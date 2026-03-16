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

# Run with custom agent port and extended timeouts (CI/CD)
bin/probe test tests/ --port 48686 --dial-timeout 45s --token-timeout 60s -v -y

# Generate HTML report from JSON results
bin/probe report --input reports/results.json -o reports/report.html --open
```

## Architecture

The system has two main components that communicate via WebSocket + JSON-RPC 2.0:

1. **Go CLI (`cmd/probe/`)**: Parses `.probe` files, manages devices, orchestrates test runs, generates reports
2. **Dart ProbeAgent (`probe_agent/`)**: Runs on-device as a WebSocket server (port 48686), executes commands against the live Flutter widget tree

### Connection Flow

- **Android**: CLI → `adb forward tcp:48686 tcp:48686` → WebSocket connect → authenticate via one-time token (extracted from `adb logcat` matching `PROBE_TOKEN=...`) → dispatch JSON-RPC commands
- **iOS Simulator**: CLI → `localhost:48686` directly (simulator shares host loopback) → authenticate via token file at `~/Library/Developer/CoreSimulator/Devices/<UDID>/data/tmp/probe/token` (fast path) or log stream (fallback)

### Go CLI internals (`internal/`)

- **`cli/`** — Cobra command definitions (`probe test`, `probe init`, `probe lint`, `probe device`, `probe record`, `probe report`, `probe migrate`, `probe generate`, `probe studio`)
- **`parser/`** — Indent-aware lexer + recursive-descent parser producing an AST. ProbeScript is Python-like with indent-based blocks. Key types: `Program → UseStmt | RecipeDef | HookDef | TestDef`, each containing `Step` nodes
- **`runner/`** — Test orchestration: `runner.go` loads recipes/files/tags, `executor.go` walks AST steps and dispatches to ProbeLink, `reporter.go` outputs Terminal/JUnit/JSON, `device_context.go` handles platform-level operations (restart, clear data, permissions, reconnection)
- **`probelink/`** — JSON-RPC 2.0 WebSocket client. `DialWithOptions()` for full config control. Methods like `probe.tap`, `probe.type`, `probe.see`, `probe.wait`, `probe.screenshot`, etc.
- **`device/`** — ADB integration for Android emulator management, port forwarding, token extraction, permission grant/revoke. `permissions.go` maps human-readable names to platform constants.
- **`ios/`** — iOS simulator management via `xcrun simctl`: boot, install, launch, log streaming, token reading
- **`config/`** — `probe.yaml` parsing with sections: `project`, `defaults`, `agent`, `device`, `video`, `visual`, `tools`, `devices`, `environment`. All timeouts and tuning constants are configurable.
- **`ai/`** — Self-healing selectors via fuzzy matching against widget tree (strategies: text_fuzzy, key_partial, type_position, semantic)
- **`migrate/`** — Converts Maestro YAML test flows to ProbeScript (legacy; see `tools/probe-convert/` for multi-format converter)
- **`report/`** — HTML report generation with relative artifact paths for portability
- **`plugin/`** — YAML-based custom command system; plugins define new ProbeScript commands that dispatch to Dart handlers
- **`visual/`** — Screenshot-based visual regression testing with configurable threshold and pixel delta

### Dart Agent internals (`probe_agent/lib/src/`)

- **`server.dart`** — WebSocket server with token auth; prints `PROBE_TOKEN=` every 3 seconds and writes token to a file for CLI pickup
- **`executor.dart`** — Command dispatcher (largest file); resolves selectors, executes actions (tap, type, see, swipe, screenshot, etc.)
- **`finder.dart`** — Widget selector engine walking the live widget tree via `WidgetsBinding.instance.rootElement`. Supports text, `#id` (ValueKey), type, ordinal (`1st "Item"`), positional (`"text" in "Container"`)
- **`sync.dart`** — Triple-signal synchronization (frames + animations + microtasks) for flake prevention
- **`gestures.dart`** — Tap, swipe, scroll, drag, pinch, rotate handlers
- **`recorder.dart`** — Recording engine for `probe record`: intercepts pointer events via `GestureBinding.pointerRouter.addGlobalRoute()`, classifies gestures (tap/swipe/long_press), identifies widgets from touch coordinates via hit-testing, tracks text input via controller listeners, streams events back to CLI as JSON-RPC notifications

## Configuration

All settings follow the resolution order: **CLI flag > probe.yaml > built-in default**.

### probe.yaml sections

- **`agent:`** — WebSocket port (`48686`), dial timeout (`30s`), ping interval (`5s`), token read timeout (`30s`), reconnect delay (`2s`)
- **`device:`** — Emulator boot timeout (`120s`), simulator boot timeout (`60s`), boot poll interval (`2s`), token file retries (`5`), restart delay (`500ms`)
- **`video:`** — Resolution (`720x1280`), framerate (`2` fps), screenrecord cycle (`170s`)
- **`visual:`** — Threshold (`0.5`%), pixel delta (`8`)
- **`defaults:`** — Platform, per-step timeout, screenshots, video, retry count, permission auto-grant
- **`tools:`** — Custom paths for `adb` and `flutter` binaries

### Key CLI flags (probe test)

- `--port` — Agent WebSocket port
- `--dial-timeout` — WebSocket connection timeout
- `--token-timeout` — Agent token wait timeout
- `--reconnect-delay` — Post-restart reconnect delay
- `--video-resolution` / `--video-framerate` — Video recording settings
- `--visual-threshold` / `--visual-pixel-delta` — Visual regression tuning

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
- Restart delay and reconnect delay are configurable via `device.restart_delay` and `agent.reconnect_delay` in probe.yaml.

### Test recording (`probe record`)

The recording feature captures user interactions and generates ProbeScript:

1. CLI sends `probe.start_recording` RPC to the Dart agent
2. Agent's `ProbeRecorder` installs a global pointer route via `GestureBinding.instance.pointerRouter.addGlobalRoute()`
3. Each `PointerDownEvent`/`PointerUpEvent` is classified: displacement < 20px = tap/long_press, >= 20px = swipe
4. Widget identification: hit-tests at touch position, walks hit test entries looking for user-meaningful selectors (ValueKey > Text > Semantics > type), skips framework internals like `NotificationListener`, `Padding`, `Container`
5. Text input: periodically scans for `EditableText` elements, attaches controller listeners, debounces (300ms), emits `type` events
6. Events are streamed as `probe.recorded_event` JSON-RPC notifications
7. CLI collects events, prints real-time feedback, and on stop generates a `.probe` file with auto-inserted `wait N seconds` for gaps > 2s

**Platform detection**: `record.go` detects iOS vs Android from the device list (same pattern as `test.go`) — uses `ReadTokenIOS()` without port forwarding for iOS, `ForwardPort()` + `ReadToken()` for Android.

### Report output paths

The `reports_folder` setting in `probe.yaml` (default: `"reports"`) controls where screenshots and videos are saved. Resolution order:
- `-o` flag directory takes precedence (video/screenshot subdirs created relative to it)
- Otherwise uses `cfg.Reports` from probe.yaml
- Fallback: `"reports"`

Both JSON and HTML reports use **relative paths** for artifacts (videos, screenshots) so the `reports/` directory is fully portable — upload to CI artifact storage, S3, or share without breaking references. The JSON reporter relativizes paths against its output file location; the HTML reporter does the same against the HTML file location.

### Video recording codec

iOS simulator videos use `--codec=h264` (not HEVC) for browser compatibility in HTML reports. Android videos are MP4 (H.264 by default).

## Security

### Input validation

- **`project.app`** (bundle ID) is validated against `^[a-zA-Z][a-zA-Z0-9_.]*$` at config load time. Invalid values are rejected with a clear error.
- **`--device` serial** is validated against `^[a-zA-Z0-9._:/-]+$` via `config.ValidateDeviceSerial()` before being passed to any shell command.
- **Selector text** from recorded events is sanitized (newlines stripped) before writing to `.probe` files to prevent syntax injection.

### Token handling

- The ProbeAgent auth token is a one-time 32-char random string, only valid on loopback (`127.0.0.1`).
- Token is **masked in error messages** — dial errors show `ws://host:port/probe?token=***` instead of the real token.
- Token is intentionally printed to stdout/logcat for CLI pickup — this is by design for the auth flow.

### Studio security

- Studio HTTP server binds to `127.0.0.1` only (not exposed to network).
- **CORS protection**: API requests from origins other than the Studio's own (`http://127.0.0.1:<port>`) are rejected with 403.
- **XSS prevention**: All user-controlled content (widget types, keys, error messages) is HTML-escaped via `escHtml()` before rendering in the DOM.

### Thread safety

- `VideoRecorder` uses a `sync.Mutex` to protect `cmd`, `segments`, `frameIdx`, and `remotePath` fields that are accessed by background goroutines (screenrecord chaining and screencap capture).

### Constants

- All 22 JSON-RPC method names are defined as constants in both Go (`internal/probelink/protocol.go`) and Dart (`ProbeMethods` class in `probe_agent/lib/src/protocol.dart`). No string literals for method names in dispatchers.
- Notification methods (`probe.recorded_event`, `probe.exec_dart`, `probe.restart_app`) are also constants in both languages.

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

### Recipes and reuse

Recipes define reusable step sequences. Use `use` statements to import recipes from other files:
```
# tests/recipes/common.probe
recipe "sign in with" (email, password)
  open the app
  wait until "Sign In" appears
  tap on "Sign In"
  type <email> into the "Email" field
  type <password> into the "Password" field
  tap "Continue"

# tests/login.probe
use "tests/recipes/common.probe"

test "a user can sign in"
  sign in with "user@example.com" and "mypassword"
  see "Dashboard"
```

Key constructs: `test`, `recipe` (reusable steps with params), `use` (import recipes), `before each`/`after each` hooks, `on failure` hooks, `if`/`else` conditionals, `repeat N times` loops, `Examples:` blocks for data-driven tests with `<var>` substitution, `dart:` blocks for escape-hatch Dart code, `when ... respond with` for HTTP mocking, `clear app data` / `restart the app` for lifecycle control, `allow permission` / `deny permission` / `grant all permissions` for OS permission management.

## probe-convert Tool (`tools/probe-convert/`)

Standalone multi-format test converter that translates tests from 7 source formats into ProbeScript. All languages are at **100% construct coverage** (no Manual-level constructs remaining).

### Build & Test Commands

```bash
# From repo root
make build-convert                # Build → bin/probe-convert
make test-convert                 # Unit tests (converter packages only)
make test-convert-integration     # Golden files + probe lint + probe test --dry-run

# From tools/probe-convert/
make test                         # Unit tests
make test-all                     # Unit + integration
make update-golden                # Regenerate golden files after intentional changes
```

### Architecture

- **`convert/`** — Common types (`Converter` interface, `Result`, `Warning`, `ProbeWriter`)
- **`convert/maestro/`** — Maestro YAML → ProbeScript (26 constructs, 100% coverage)
- **`convert/gherkin/`** — Gherkin/Cucumber `.feature` → ProbeScript (34 constructs)
- **`convert/robot/`** — Robot Framework `.robot` → ProbeScript (29 constructs)
- **`convert/detox/`** — Detox JS/TS → ProbeScript (22 constructs)
- **`convert/appium/`** — Appium Python/Java/JS → ProbeScript (39 constructs across 3 variants)
- **`catalog/`** — Formal grammar construct catalog per language with EBNF, examples, ProbeScript mappings, and coverage levels
- **`cmd/`** — Cobra CLI: root convert command + `catalog` and `formats` subcommands
- **`ui/`** — Terminal output (colors, spinners, syntax-highlighted dry-run)

### Conversion Levels

- **Full** — Lossless 1:1 mapping (e.g., `tapOn` → `tap on`)
- **Partial** — Lossy but valid ProbeScript with guidance comments (e.g., `evalScript` → `run dart:` block, `ifdef` → platform guard in dart block, `setLocation` → GPS mock comments + dart block)
- **Manual** — Emitted as `# TODO` comment (none remaining — all promoted)
- **Skip** — Intentionally ignored (imports, boilerplate)

### Key Maestro Mappings (recently promoted)

| Maestro Construct | ProbeScript Output | Level |
|---|---|---|
| `evalScript: "console.log('x')"` | `run dart:` block with JS→Dart transpilation (`console.log`→`print`, `===`→`==`, `const`→`final`) | Full |
| `setAirplaneMode: true` | `toggle wifi off` | Full |
| `ifdef: {platform: android}` | `run dart:` block with platform guard comment (`Platform.isAndroid`) | Partial |
| `setLocation: {lat, lng}` | Comments with `adb`/`simctl` GPS commands + `run dart:` block | Partial |

### Integration Test Contract

`integration_test.go` runs 3 test suites across all 15 example files (all 7 formats):

1. **TestGoldenFiles** — Converter output matches `testdata/golden/` snapshots
2. **TestLintGeneratedOutput** — Generated `.probe` files pass `probe lint` (ProbeScript parser accepts them)
3. **TestVerifyDryRun** — Generated files pass `probe test --dry-run` (full parse + recipe resolution)

### Format Auto-Detection

`convert/detect.go` guesses format from file extension + content markers:
- `.feature` → Gherkin, `.robot` → Robot
- `.yaml`/`.yml` → Maestro (if contains `appId`, `tapOn`, `launchApp`, etc.)
- `.js`/`.ts` → Detox (if contains `element(by.` or `device.launchApp`) else Appium JS
- `.py` → Appium Python, `.java`/`.kt` → Appium Java

### CLI Flags

- `--from/-f` — Force source format (maestro|gherkin|robot|detox|appium)
- `--output/-o` — Output directory or file
- `--dry-run` — Preview to stdout
- `--recursive/-r` — Recurse into subdirectories
- `--lint` — Validate with `probe lint` after conversion
- `--verify` — Validate with `probe test --dry-run` after conversion
- `--probe-path` — Path to probe binary (auto-detected)

## Key Dependencies

- Go 1.23, Dart 3.3+, Flutter 3.19+
- `github.com/gorilla/websocket` — WebSocket client
- `github.com/spf13/cobra` — CLI framework
- `gopkg.in/yaml.v3` — YAML parsing
