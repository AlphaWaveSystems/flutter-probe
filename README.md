# FlutterProbe

A high-performance local E2E testing framework for Flutter mobile apps. Write tests in natural language, execute them with sub-50ms command round-trips via direct widget-tree access.

```
test "user can log in"
  open the app
  see "Email"
  type "user@example.com" into "Email"
  type "secret" into "Password"
  tap "Sign In"
  see "Dashboard"
```

## How It Works

FlutterProbe has two components:

1. **`probe` CLI** (Go) — parses `.probe` test files, manages devices, orchestrates runs, generates reports
2. **`probe_agent`** (Dart) — embedded in your Flutter app as a dev dependency; runs a WebSocket server on port 48686 and executes commands directly against the live widget tree

```
┌──────────────┐     WebSocket / JSON-RPC 2.0      ┌─────────────────┐
│  probe CLI   │ ──────────────────────────────────▶│  ProbeAgent     │
│  (your Mac)  │   localhost:48686                   │  (in Flutter app│
│              │   • tap, type, see, wait, swipe    │   on device)    │
│  parses .probe│   • screenshot, dump_tree         │                 │
│  manages devices│  • one-time token auth          │  walks widget   │
│  generates reports│                               │  tree directly  │
└──────────────┘                                    └─────────────────┘
```

**Android**: CLI → `adb forward tcp:<host-port> tcp:<device-port>` → WebSocket on `localhost:<host-port>` → ProbeAgent
**iOS Simulator**: CLI → `localhost:<port>` directly (simulator shares host loopback — no port forwarding needed)

By default both ports are `48686`. For parallel testing, the host port can differ from the device port (e.g., `adb forward tcp:48687 tcp:48686`) — see [Parallel Testing](#parallel-testing-ios--android).

## Quick Start

### 1. Build the CLI

```bash
git clone <this-repo>
cd FlutterProbe
make build          # outputs bin/probe
# or: make install  # installs to $GOPATH/bin
```

### 2. Add ProbeAgent to your Flutter app

In your app's `pubspec.yaml`:

```yaml
dev_dependencies:
  probe_agent:
    path: /path/to/FlutterProbe/probe_agent
```

Then in your `lib/main.dart`:

```dart
import 'package:probe_agent/probe_agent.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  const probeEnabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  if (probeEnabled) {
    await ProbeAgent.start();
  }

  runApp(const MyApp());
}
```

> **Important**: The `PROBE_AGENT` flag is compiled into the binary via `--dart-define`. This means you must build the app with the flag — you cannot toggle it at runtime.

### 3. Initialize your project

```bash
cd your-flutter-project
probe init
```

This creates `probe.yaml` and a `tests/` directory with sample files.

### 4. Configure `probe.yaml`

```yaml
project:
  name: "My App"
  app: com.example.myapp          # bundle ID (iOS) or package name (Android)

defaults:
  platform: ios                    # android | ios | both
  timeout: 30s
  screenshots: on_failure          # always | on_failure | never
  video: false
  retry_failed_tests: 1

devices:
  - name: iPhone 16 Pro
    serial: <UDID>                 # from `xcrun simctl list devices`
  - name: Pixel 7
    serial: emulator-5554          # or `auto`

recipes_folder: tests/recipes
reports_folder: reports

environment:
  TEST_USER: "admin@test.com"
  API_BASE: "http://localhost:8080"
```

### 5. Build and run your app with ProbeAgent

**iOS Simulator:**

```bash
# Build for simulator with ProbeAgent enabled
flutter build ios --debug --simulator --dart-define=PROBE_AGENT=true

# Install and launch
xcrun simctl install <UDID> build/ios/iphonesimulator/YourApp.app
xcrun simctl launch <UDID> com.example.myapp
```

Or use `xcodebuild` for more control (e.g., when Xcode SDK and simulator iOS versions differ):

```bash
xcodebuild -workspace ios/Runner.xcworkspace \
  -scheme Runner \
  -sdk iphonesimulator \
  -configuration Debug \
  -destination 'platform=iOS Simulator,id=<UDID>' \
  DART_DEFINES=$(echo 'PROBE_AGENT=true' | base64) \
  -derivedDataPath build/ios \
  build
```

**Android Emulator:**

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true
adb install -r build/app/outputs/flutter-apk/app-debug.apk
adb shell am start -n com.example.myapp/.MainActivity
```

### 6. Run tests

```bash
# Run all tests
probe test tests/

# Run a specific file
probe test tests/smoke/login.probe

# Run tests by tag
probe test tests/ --tag smoke

# Run against a specific device
probe test tests/ --device <UDID-or-serial>

# Specify timeout per step
probe test tests/ --timeout 60s

# JUnit output for CI
probe test tests/ --format junit --output reports/results.xml

# Watch mode (re-runs on file change)
probe test tests/ --watch

# Shard tests for parallel CI
probe test tests/ --shard 1/3
```

## Writing Tests

### ProbeScript Syntax

ProbeScript uses natural language with indent-based blocks (like Python). Save files as `.probe`.

#### Basic test

```
test "user sees welcome screen"
  open the app
  wait 3 seconds
  see "Welcome"
  don't see "Error"
```

#### Tags

```
test "critical login flow"
  @smoke @critical
  open the app
  tap "Sign In"
  see "Dashboard"
```

#### Text input

```
type "hello@world.com" into "Email"
type "secret123" into the "Password" field
```

#### Selectors

ProbeScript supports multiple selector strategies:

| Selector | Syntax | Example |
|----------|--------|---------|
| Text match | `"text"` | `tap "Submit"` |
| Widget key | `#keyName` | `tap #loginButton` |
| Type | `<TypeName>` | `tap <ElevatedButton>` |
| Ordinal | `1st "Item"`, `2nd "Item"` | `tap 2nd "Add"` |
| Positional | `"text" in "Container"` | `tap "Edit" in "Settings"` |

#### Assertions

```
see "Dashboard"                    # text is visible
don't see "Error"                  # text is NOT visible
see 3 "Item"                       # exactly 3 matches
see "Submit" is enabled            # widget state check
see "Terms" is checked
see "Price" contains "$9.99"       # partial text match
```

#### Gestures

```
tap "Button"
double tap "Image"
long press "Item"
swipe left
swipe up on "Card"
scroll down
scroll up on "ListView"
drag "Item A" to "Item B"
```

#### Wait commands

```
wait 5 seconds
wait until "Dashboard" appears
wait until "Loading" disappears
wait for the page to load
wait for network idle
```

#### Conditionals

```
if "Accept Cookies" appears
  tap "Accept Cookies"
```

With else:

```
if "Welcome Back" appears
  tap "Continue"
else
  tap "Sign In"
```

#### Loops

```
repeat 3 times
  swipe left
  wait 1 second
```

#### Recipes (reusable steps)

Define in a separate file (e.g., `tests/recipes/auth.probe`):

```
recipe "log in as" (email, password)
  open the app
  wait until "Sign In" appears
  tap "Sign In"
  type <email> into "Email"
  type <password> into "Password"
  tap "Continue"
  see "Dashboard"
```

Use in tests:

```
use "recipes/auth.probe"

test "logged-in user can view profile"
  log in as "user@test.com" with "secret123"
  tap "Profile"
  see "user@test.com"
```

#### Data-driven tests

```
test "login validation"
  open the app
  type <email> into "Email"
  type <password> into "Password"
  tap "Continue"
  see <expected>

with examples:
  email              password     expected
  "user@test.com"    "pass123"    "Dashboard"
  ""                 "pass123"    "Email is required"
  "user@test.com"    ""           "Password is required"
```

#### Hooks

```
before each
  open the app
  wait for the page to load

after each
  take screenshot "after_test"

on failure
  take screenshot "failure"
  save logs
  dump tree
```

#### HTTP mocking

```
when the app calls POST "/api/auth/login"
  respond with 503 and body "{ \"error\": \"Service Unavailable\" }"
```

#### Dart escape hatch

```
dart:
  final prefs = await SharedPreferences.getInstance();
  await prefs.clear();
```

#### App lifecycle commands

```
clear app data                     # wipe all app data (SharedPreferences, databases, files) and relaunch
restart the app                    # force-stop and relaunch (preserves data)
```

`clear app data` uses `pm clear` on Android and container deletion on iOS. It requires the `-y` flag or interactive confirmation since it's a destructive operation. After clearing, the app is relaunched and the WebSocket connection is re-established automatically.

#### Permission handling

OS-level permission dialogs (notifications, camera, location) are outside the Flutter widget tree, so the Dart agent can't interact with them. FlutterProbe handles these at the platform level via ADB `pm grant`/`pm revoke` (Android) or `simctl privacy` (iOS):

```
allow permission "notifications"   # grant notification permission
deny permission "camera"           # revoke camera permission
grant all permissions              # grant all known runtime permissions
revoke all permissions             # revoke all permissions
```

Available permission names: `notifications`, `camera`, `location`, `microphone`, `storage`, `contacts`, `phone`, `calendar`, `sms`, `bluetooth`

When using `-y` flag (or `grant_permissions_on_clear: true` in `probe.yaml`), all permissions are automatically granted after `clear app data` to prevent permission dialogs from blocking tests.

#### Other commands

```
take screenshot "checkout_page"    # saves PNG to screenshots folder
dump tree                          # dumps widget tree (for debugging)
save logs                          # saves app logs
go back                            # device back button
rotate landscape                   # rotate device
log "checkpoint reached"           # print to test output
pause                              # 1-second pause
```

## iOS Simulator Setup Notes

- The iOS simulator shares `localhost` with the host Mac — no port forwarding needed
- ProbeAgent writes a token file to `~/Library/Developer/CoreSimulator/Devices/<UDID>/data/tmp/probe/token` and also prints `PROBE_TOKEN=<token>` to stdout periodically
- The CLI reads the token via the file (fast path) or by streaming simulator logs (fallback)
- **Native permission dialogs** are handled via `simctl privacy` — use `allow permission "notifications"` in your test or `-y` flag to auto-grant all permissions after `clear app data`

## Android Emulator Setup Notes

- The CLI uses `adb forward tcp:48686 tcp:48686` to bridge the host and emulator network
- Token is extracted from `adb logcat` output matching `PROBE_TOKEN=`
- Use `probe device list` to see connected devices
- Use `probe device start --platform android` to launch an emulator

## CLI Commands

| Command | Description |
|---------|-------------|
| `probe init` | Scaffold `probe.yaml` and `tests/` directory |
| `probe test [path]` | Run `.probe` test files |
| `probe test --tag <tag>` | Run tests matching a tag |
| `probe test --watch` | Watch mode — re-runs on file change |
| `probe test --format junit -o results.xml` | JUnit XML output |
| `probe test --format json -o results.json` | JSON output (for HTML reports) |
| `probe test -y` | Auto-confirm destructive ops + auto-grant permissions |
| `probe test --config probe.ios.yaml` | Use a specific config file (for parallel platform runs) |
| `probe test --video` | Enable video recording per test |
| `probe test --video-resolution 720x1280` | Set Android screenrecord resolution |
| `probe test --visual-threshold 0.5` | Max allowed pixel diff % for visual regression |
| `probe test --port 48687` | Override ProbeAgent WebSocket port |
| `probe test --dial-timeout 45s` | WebSocket connection timeout |
| `probe test --token-timeout 60s` | Agent auth token wait timeout |
| `probe test --app-path app.apk` | Install app before testing |
| `probe test --adb /path/to/adb` | Override ADB binary path |
| `probe test --flutter /path/to/flutter` | Override Flutter binary path |
| `probe test --shard N/M` | Run shard N of M (for parallel CI) |
| `probe lint [path]` | Validate `.probe` files for syntax errors |
| `probe device list` | List connected devices/simulators |
| `probe device start` | Start an emulator/simulator |
| `probe record` | Record user interactions as ProbeScript (tap, type, swipe) |
| `probe record -o tests/flow.probe` | Record to a specific output file |
| `probe report` | Generate HTML report from test results |
| `probe migrate` | Convert Maestro YAML flows to ProbeScript |
| `probe generate` | AI-assisted test generation |
| `probe-convert <file\|dir>` | Convert tests from other frameworks to ProbeScript |
| `probe-convert catalog [lang]` | Show grammar construct catalog and coverage stats |

## Recording Tests

FlutterProbe can generate `.probe` files by recording your interactions on a real device or simulator:

```bash
# Start recording (interact with your app, then Ctrl+C to stop)
probe record --device <UDID-or-serial> --output tests/my_flow.probe

# Record with a timeout (auto-stops after 60s)
probe record --timeout 60s -o tests/my_flow.probe
```

The recorder intercepts real touch events on the running app:
- **Taps** are captured with widget selectors (text, key, or type)
- **Swipes** are detected with direction (up/down/left/right)
- **Long presses** are distinguished from taps by hold duration
- **Text input** is tracked via controller listeners with debounce
- **Wait steps** are auto-inserted when gaps between actions exceed 2 seconds

Real-time feedback is printed as you interact:
```
  ● Recording on 909F49AD... — interact with your app
  ✓ tap "Sign In"
  ✓ type "user@test.com" into "Email"
  ✓ swipe up
  ✓ tap "Submit"
  ✓ Recorded 4 events → tests/my_flow.probe
```

> **Note**: The recorder identifies widgets by walking up from the touch target looking for Text content, ValueKey, or Semantics labels. Icon-only buttons may resolve to framework widget types and need manual cleanup.

## Custom Plugins

Define custom commands via YAML in the `plugins/` directory:

```yaml
# plugins/auth_bypass.yaml
command: "bypass login as"
method: "probe.plugin.auth_bypass"
description: "Authenticate directly using a dev token"
params:
  token: "${1}"
```

These dispatch to corresponding Dart handlers in the ProbeAgent.

## Reports & Artifacts

FlutterProbe supports three output formats for CI/CD integration:

```bash
# Terminal output (default)
probe test tests/

# JUnit XML (GitHub Actions, Jenkins, CircleCI)
probe test tests/ --format junit -o reports/results.xml

# JSON with artifacts (for HTML report generation)
probe test tests/ --format json -o reports/results.json --video

# Generate interactive HTML report from JSON
probe report --input reports/results.json -o reports/report.html --open
```

### Report Metadata

JSON and HTML reports include a `metadata` section with device and environment info for CI/CD traceability:

```json
{
  "metadata": {
    "device_name": "iPhone 16 Pro",
    "device_id": "909F49AD-EE6A-4263-AFED-BAC0FC5C8B40",
    "platform": "ios",
    "os_version": "iOS 18.6",
    "app_id": "com.example.myapp",
    "app_version": "1.2.16",
    "config_file": "probe.ios.yaml"
  }
}
```

The HTML report renders this in the header: `Device: iPhone 16 Pro · OS: iOS 18.6 · Platform: ios · App: com.example.myapp v1.2.16`

Metadata is collected automatically — iOS version from the simulator runtime, Android version from `getprop`, and app version from `dumpsys package`.

### Report Artifacts

The `reports/` folder is **fully self-contained and portable**:

```
reports/
├── results.json           # Test results with relative artifact paths
├── report.html            # Interactive HTML dashboard
├── screenshots/           # Failure & on-demand screenshots (PNG)
│   ├── failure_login_1234.png
│   └── main_menu_5678.png
└── videos/                # Per-test screen recordings (H.264 MOV/MP4)
    ├── login_test.mov
    └── navigation_test.mov
```

All artifact paths in `results.json` are **relative** to the JSON file — the entire folder can be uploaded to CI artifact storage, S3, or shared without breaking references.

The `reports_folder` setting in `probe.yaml` controls the default output directory:

```yaml
reports_folder: reports    # default; change to "ci/output" etc.
```

### Viewing the HTML Report

The HTML report embeds screenshots and videos using relative paths. To view it with media:

```bash
# Serve locally (required for video/screenshot playback)
cd reports && python3 -m http.server 8080
# Then open http://localhost:8080/report.html

# Or use probe report --open (opens file directly, media may not load in some browsers)
probe report --input reports/results.json -o reports/report.html --open
```

## CI / GitHub Actions

### Example Workflow

```yaml
name: E2E Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - uses: subosito/flutter-action@v2
        with:
          flutter-version: '3.19.0'

      - name: Build probe CLI
        run: make build

      - name: Run Go unit tests
        run: go test -v -race ./...

      - name: Boot iOS simulator
        run: |
          DEVICE=$(xcrun simctl list devices available -j | jq -r '.devices | to_entries[] | .value[] | select(.name | contains("iPhone")) | .udid' | head -1)
          xcrun simctl boot "$DEVICE"
          echo "DEVICE_UDID=$DEVICE" >> $GITHUB_ENV

      - name: Build & launch app with ProbeAgent
        run: |
          cd your-flutter-app
          flutter build ios --debug --simulator --dart-define=PROBE_AGENT=true
          xcrun simctl install $DEVICE_UDID build/ios/iphonesimulator/YourApp.app
          xcrun simctl launch $DEVICE_UDID com.example.yourapp

      - name: Run E2E tests
        run: |
          bin/probe test tests/ \
            --device $DEVICE_UDID \
            --timeout 60s -v -y \
            --video \
            --format json -o reports/results.json

      - name: Generate HTML report
        if: always()
        run: bin/probe report --input reports/results.json -o reports/report.html

      - name: Upload test artifacts
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: test-reports
          path: reports/

      # Optional: also produce JUnit for GitHub's test summary
      - name: Run tests (JUnit)
        if: always()
        run: |
          bin/probe test tests/ \
            --device $DEVICE_UDID \
            --timeout 60s -y \
            --format junit -o reports/results.xml
        continue-on-error: true

      - name: Publish test results
        if: always()
        uses: dorny/test-reporter@v1
        with:
          name: E2E Test Results
          path: reports/results.xml
          reporter: java-junit
```

### Android CI (Self-hosted or Docker)

```yaml
  android-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - name: Build probe CLI
        run: make build

      - name: Start Android emulator
        uses: reactivecircus/android-emulator-runner@v2
        with:
          api-level: 34
          script: |
            adb install -r your-app-debug.apk
            adb shell am start -n com.example.yourapp/.MainActivity
            sleep 10
            bin/probe test tests/ \
              --device emulator-5554 \
              --timeout 60s -v -y \
              --video \
              --format json -o reports/results.json
            bin/probe report --input reports/results.json -o reports/report.html

      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: android-reports
          path: reports/
```

A Docker setup is also available in `docker/` for self-hosted CI with Android emulators.

### Parallel Testing (iOS + Android)

Run tests on both platforms simultaneously using platform-specific configs with the `--config` flag:

```bash
# Create platform-specific configs
# probe.ios.yaml — uses port 48686, reports to reports/ios/
# probe.android.yaml — uses host port 48687 → device port 48686, reports to reports/android/

# Run in parallel
bin/probe test tests/ --config probe.ios.yaml --device <IOS_UDID> -v -y &
bin/probe test tests/ --config probe.android.yaml --device emulator-5554 -v -y &
wait
```

Key config differences for parallel runs:

```yaml
# probe.ios.yaml
agent:
  port: 48686        # iOS simulator shares host loopback, no port forwarding

# probe.android.yaml
agent:
  port: 48687        # different host port to avoid conflict with iOS
  device_port: 48686 # on-device port stays the same (what ProbeAgent listens on)
```

The `device_port` field allows the host-side port to differ from the on-device port. This is needed because `adb forward` maps `host:48687 → device:48686`, while the iOS simulator uses `localhost:48686` directly.

## Migrating From Other Frameworks (`probe-convert`)

FlutterProbe includes a standalone converter tool that translates tests from 7 source formats into ProbeScript with **100% construct coverage** across all supported languages.

### Supported Formats

| Source Framework | Extensions | Constructs | Full | Partial | Coverage |
|------------------|-----------|------------|------|---------|----------|
| Maestro | `.yaml`, `.yml` | 26 | 24 | 2 | 100% |
| Gherkin (Cucumber) | `.feature` | 34 | 34 | 0 | 100% |
| Robot Framework | `.robot` | 29 | 28 | 1 | 100% |
| Detox | `.js`, `.ts` | 22 | 22 | 0 | 100% |
| Appium (Python) | `.py` | 14 | 13 | 1 | 100% |
| Appium (Java) | `.java`, `.kt` | 12 | 12 | 0 | 100% |
| Appium (JS/WebdriverIO) | `.js` | 13 | 13 | 0 | 100% |

### Build & Install

```bash
make build-convert      # outputs bin/probe-convert
# or from tools/probe-convert:
cd tools/probe-convert && make build
```

### Usage

```bash
# Convert a single file (auto-detects format)
bin/probe-convert tests/login.yaml

# Convert a directory recursively
bin/probe-convert -r maestro_tests/ -o probe_tests/

# Force a specific source format
bin/probe-convert --from maestro flow.yml

# Preview without writing files
bin/probe-convert --dry-run tests/login.yaml

# Convert and validate with probe lint
bin/probe-convert --lint tests/login.yaml -o output/

# Convert and verify with probe test --dry-run
bin/probe-convert --verify tests/login.yaml -o output/

# Show catalog for a language
bin/probe-convert catalog maestro

# Show full catalog summary table
bin/probe-convert catalog
```

### Conversion Examples

**Maestro YAML → ProbeScript:**
```yaml
# login.yaml (Maestro)
appId: com.example.app
---
- launchApp
- tapOn: "Sign In"
- inputText: "user@test.com"
- assertVisible: "Dashboard"
- evalScript: "console.log('done')"
- setAirplaneMode: true
```
```
# login.probe (generated)
test "login"
  open the app
  tap on "Sign In"
  type "user@test.com"
  see "Dashboard"
  run dart:
    // Migrated from Maestro evalScript — review and adapt JS → Dart
    // Original: console.log('done')
    print('done')
  toggle wifi off
```

**Gherkin → ProbeScript:**
```gherkin
Feature: Login
  Background:
    Given the app is launched

  Scenario: Valid login
    When I tap on "Sign In"
    And I type "user@test.com" into "Email"
    Then I should see "Dashboard"
```
```
# login.probe (generated)
before each
  open the app

test "Valid login"
  tap on "Sign In"
  type "user@test.com" into "Email"
  see "Dashboard"
```

**Detox JS → ProbeScript:**
```js
describe('Login', () => {
  it('should sign in', async () => {
    await element(by.id('email')).typeText('user@test.com');
    await element(by.text('Sign In')).tap();
    await expect(element(by.text('Dashboard'))).toBeVisible();
  });
});
```
```
# login.probe (generated)
test "should sign in"
  type "user@test.com" into #email
  tap on "Sign In"
  see "Dashboard"
```

### Conversion Levels

- **Full** — Lossless 1:1 mapping to ProbeScript (e.g., `tapOn` → `tap on`)
- **Partial** — Lossy but usable; emits valid ProbeScript with guidance comments (e.g., `evalScript` → `run dart:` block, `ifdef` → platform guard, `setLocation` → GPS mock comments)

### Grammar Catalog

Every construct in every supported language is formally catalogued with EBNF rules, examples, and ProbeScript mappings. View with:

```bash
bin/probe-convert catalog            # summary table
bin/probe-convert catalog maestro    # full Maestro construct catalog
bin/probe-convert catalog gherkin    # full Gherkin construct catalog
bin/probe-convert catalog --markdown # Markdown output for docs
```

### Testing Converted Output

```bash
# Run converter unit tests
make test-convert

# Run full integration suite (golden files + lint + dry-run verify)
make test-convert-integration

# From tools/probe-convert directory:
cd tools/probe-convert
make test          # unit tests
make test-all      # unit + integration
make update-golden # regenerate golden files after intentional changes
```

## VS Code Extension

A VS Code extension is included in `vscode/` providing:

- ProbeScript syntax highlighting
- Code snippets for common patterns
- Commands: Run Test, Run File, Lint File, Start Recording, Open Studio

## Project Structure

```
FlutterProbe/
├── cmd/probe/            # CLI entry point (main.go)
├── internal/
│   ├── ai/               # Self-healing selectors (fuzzy match against widget tree)
│   ├── cli/              # Cobra command definitions
│   ├── config/           # probe.yaml parsing
│   ├── device/           # ADB + Android emulator management
│   ├── ios/              # iOS simulator management (simctl)
│   ├── migrate/          # Maestro YAML → ProbeScript converter
│   ├── parser/           # ProbeScript lexer + recursive-descent parser → AST
│   ├── plugin/           # YAML-based custom command system
│   ├── probelink/        # JSON-RPC 2.0 WebSocket client
│   ├── report/           # HTML report generation
│   ├── runner/           # Test orchestration, executor, reporter
│   ├── studio/           # Interactive test studio
│   └── visual/           # Screenshot-based visual regression testing
├── probe_agent/          # Dart package (on-device agent)
│   └── lib/src/
│       ├── agent.dart    # Top-level ProbeAgent.start() API
│       ├── server.dart   # WebSocket server + token auth
│       ├── executor.dart # Command dispatcher (tap, type, see, etc.)
│       ├── finder.dart   # Widget selector engine (text, key, type, ordinal, positional)
│       ├── sync.dart     # Triple-signal sync (frames + animations + microtasks)
│       ├── gestures.dart # Gesture handlers
│       ├── recorder.dart # Gesture recording engine (for probe record)
│       └── protocol.dart # JSON-RPC types
├── plugins/              # Custom command definitions (YAML)
├── tests/                # Example .probe test files
├── tools/
│   └── probe-convert/    # Multi-format test converter (Maestro, Gherkin, Robot, Detox, Appium)
│       ├── convert/      # Converter implementations per format
│       ├── catalog/      # Grammar construct catalogs with EBNF rules
│       ├── examples/     # Example input files for all formats
│       └── testdata/     # Golden files for integration tests
├── docker/               # CI Docker setup for Android emulators
├── vscode/               # VS Code extension for ProbeScript
├── .github/workflows/    # GitHub Actions CI pipeline
└── Makefile              # Build, test, lint, clean targets
```

## Performance Targets

| Metric | Target |
|--------|--------|
| Command round-trip | < 50ms |
| CLI cold start | < 100ms |
| 50-test suite | < 90 seconds |
| Flake rate | < 0.5% |

## Requirements

### Core

- Go 1.23+
- Dart 3.3+ / Flutter 3.19+
- Android: ADB + Android SDK
- iOS: Xcode + `xcrun simctl`

### Video recording

| Platform | Tool | Required? | Notes |
|----------|------|-----------|-------|
| iOS | `xcrun simctl` | Built-in | Uses `simctl io recordVideo --codec=h264` |
| Android | `screenrecord` | Built-in | On-device, auto-chains to avoid 180s limit |
| Android | `scrcpy` | Optional | Preferred backend if installed (higher quality) |
| Both | `ffmpeg` | Optional | Required to stitch multi-segment Android recordings and screencap frames into a single video. Without it, segments are kept as separate files |

### Screenshots

Screenshots are captured by the Dart agent directly (no external tools needed). They are saved as PNG files on the device and pulled to `reports/screenshots/` automatically.

### HTML reports

The HTML report uses relative paths for videos and screenshots. To view with embedded media:

- **Recommended**: Serve via HTTP — `cd reports && python3 -m http.server 8080`
- **Direct open**: `open reports/report.html` — screenshots work, videos may not load in all browsers due to `file://` security restrictions
