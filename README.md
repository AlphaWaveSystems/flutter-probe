# FlutterProbe

A high-performance E2E testing framework for Flutter mobile apps. Write tests in plain English, execute with sub-50ms command round-trips via direct widget-tree access — no UI automation layer, no WebDriver overhead.

[![License: BSL 1.1](https://img.shields.io/badge/License-BSL%201.1-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-0.1.0-green.svg)](CHANGELOG.md)

```
test "user can log in"
  @smoke
  open the app
  wait until "Sign In" appears
  type "user@example.com" into the "Email" field
  type "secret" into the "Password" field
  tap "Sign In"
  see "Dashboard"
```

## Table of Contents

- [How It Works](#how-it-works)
- [Installation](#installation)
- [ProbeScript Language](#probescript-language)
- [CLI Commands](#cli-commands)
- [Visual Regression](#visual-regression)
- [Test Recording](#test-recording)
- [CI/CD Integration](#cicd-integration)
- [Cloud Device Farms](#cloud-device-farms)
- [Migrating from Other Frameworks](#migrating-from-other-frameworks)
- [VS Code Extension](#vs-code-extension)
- [Configuration (`probe.yaml`)](#configuration-probeyaml)
- [Self-Healing Selectors](#self-healing-selectors)
- [Repository Structure](#repository-structure)
- [Documentation](#documentation)
- [License](#license)

## How It Works

FlutterProbe has two components that communicate over WebSocket + JSON-RPC 2.0:

```
┌──────────────┐     WebSocket / JSON-RPC 2.0      ┌─────────────────┐
│  probe CLI   │ ──────────────────────────────────▶│  ProbeAgent     │
│  (Go)        │   localhost:48686                   │  (Dart, on      │
│              │   tap, type, see, wait, swipe       │   device)       │
│  Parses .probe│  screenshot, dump_tree             │                 │
│  Manages devs │  One-time token auth               │  Walks widget   │
│  Reports     │                                     │  tree directly  │
└──────────────┘                                    └─────────────────┘
```

- **Android**: CLI → `adb forward` → WebSocket → ProbeAgent
- **iOS Simulator**: CLI → `localhost` directly (simulator shares host loopback)

The **ProbeAgent** is a Dart package you add to your Flutter app as a dev dependency. It runs a WebSocket server inside your app and executes commands against the live widget tree — no `flutter_test`, no `TestWidgetsFlutterBinding`, no external driver.

## Installation

### Option A — Download pre-built binary (recommended for CI/CD)

Pre-built binaries for Linux, macOS (Intel + Apple Silicon), and Windows are attached to every [GitHub Release](https://github.com/AlphaWaveSystems/flutter-probe/releases).

```bash
# Linux (amd64)
curl -Lo probe https://github.com/AlphaWaveSystems/flutter-probe/releases/latest/download/probe-linux-amd64
chmod +x probe && sudo mv probe /usr/local/bin/

# macOS Apple Silicon
curl -Lo probe https://github.com/AlphaWaveSystems/flutter-probe/releases/latest/download/probe-darwin-arm64
chmod +x probe && sudo mv probe /usr/local/bin/

# macOS Intel
curl -Lo probe https://github.com/AlphaWaveSystems/flutter-probe/releases/latest/download/probe-darwin-amd64
chmod +x probe && sudo mv probe /usr/local/bin/
```

### Option B — Build from source

```bash
git clone https://github.com/AlphaWaveSystems/flutter-probe.git
cd flutter-probe
make build        # → bin/probe
make install      # → $GOPATH/bin/probe (optional)
```

**Requirements:** Go 1.26+, Dart 3.3+ / Flutter 3.19+, ADB (Android), Xcode (iOS)

### 2. Add ProbeAgent to your Flutter app

In your app's `pubspec.yaml`:

```yaml
dev_dependencies:
  probe_agent:
    path: /path/to/flutter-probe/probe_agent
```

In your `main.dart`, start the agent before `runApp`:

```dart
import 'package:probe_agent/probe_agent.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await ProbeAgent.start();   // starts WebSocket server on port 48686
  runApp(MyApp());
}
```

### 3. Initialize your project and run tests

```bash
cd your-flutter-app
probe init          # creates probe.yaml and tests/ scaffold
probe test tests/   # run all tests
```

## ProbeScript Language

Tests are written in `.probe` files using natural English with Python-style indentation.

### Basic test

```
test "checkout flow"
  @smoke @checkout
  open the app
  wait until "Cart" appears
  tap "Checkout"
  type "John Doe" into the "Name" field
  type "john@example.com" into the "Email" field
  tap "Place Order"
  see "Order confirmed"
  take a screenshot called "order_confirmation"
```

### Selectors

```
tap "Sign In"                   # by visible text
tap #login_button               # by ValueKey
tap "Submit" in "LoginForm"     # positional (widget inside a parent)
tap on the 1st "ListItem"       # ordinal
```

### Recipes — reusable step sequences

```
# tests/recipes/auth.probe
recipe "sign in" (email, password)
  open the app
  wait until "Sign In" appears
  type <email> into the "Email" field
  type <password> into the "Password" field
  tap "Sign In"

# tests/login.probe
use "tests/recipes/auth.probe"

test "user can access dashboard"
  sign in "user@example.com" and "secret"
  see "Dashboard"
```

### Hooks

```
before each
  clear app data
  open the app

after each
  take a screenshot called "after"

on failure
  take a screenshot called "failure"
  dump the widget tree
```

### Data-driven tests

```
test "login with <email>"
  open the app
  type <email> into the "Email" field
  tap "Continue"
  see <result>
  Examples:
    | email              | result      |
    | "user@example.com" | "Dashboard" |
    | "bad@example.com"  | "Error"     |
```

### Conditionals and loops

```
if "Onboarding" is visible
  tap "Skip"
repeat 3 times
  swipe up
  wait 1 seconds
```

### App lifecycle

```
clear app data          # wipe storage, relaunch, reconnect
restart the app         # force-stop, relaunch, reconnect (data preserved)
```

### OS-level permissions

```
allow permission "camera"
deny permission "notifications"
grant all permissions
revoke all permissions
```

### Dart escape hatch

```
run dart:
  final version = await PackageInfo.fromPlatform();
  print('App version: ${version.version}');
```

## CLI Commands

| Command | Description |
|---|---|
| `probe init` | Scaffold `probe.yaml` and `tests/` in your project |
| `probe test [path]` | Run tests (file, directory, or glob) |
| `probe test --tag smoke` | Run tests by tag |
| `probe test --format json -o results.json` | Output JSON results |
| `probe lint [path]` | Validate `.probe` syntax |
| `probe record` | Record interactions → generate ProbeScript |
| `probe report --input results.json` | Generate HTML report |
| `probe device list` | List connected devices and simulators |
| `probe studio` | Open interactive widget tree inspector |
| `probe generate --prompt "test login flow"` | AI-generate a test from a description |
| `probe version` | Print CLI version |
| `probe-convert` | Convert tests from other frameworks |

### Key flags for `probe test`

| Flag | Default | Description |
|---|---|---|
| `--device <serial>` | — | Device serial or simulator UDID |
| `--timeout <duration>` | `30s` | Per-step timeout |
| `--format terminal\|json\|junit` | `terminal` | Output format |
| `-o <path>` | — | Output file for JSON/JUnit results |
| `--video` | off | Record video during run |
| `-v` | off | Verbose step output |
| `-y` | off | Auto-confirm destructive operations |
| `--tag <tag>` | — | Run only tests with this tag |
| `--name <pattern>` | — | Run only tests matching name |
| `--adb <path>` | PATH | Custom ADB binary |

## Visual Regression

Capture baseline screenshots and compare on every run:

```
test "main screen looks correct"
  open the app
  wait 2 seconds
  take a screenshot called "main_screen"   # first run: saves baseline
                                            # subsequent runs: compares
```

Configure sensitivity in `probe.yaml`:
```yaml
visual:
  threshold: 0.5    # max % of pixels allowed to differ
  pixel_delta: 8    # per-pixel color tolerance (0–255)
```

## Test Recording

Capture real interactions and generate ProbeScript automatically:

```bash
probe record --device emulator-5554 --output tests/recorded.probe
```

Tap, swipe, and type in your app — FlutterProbe writes the `.probe` file as you go.

## CI/CD Integration

No need to clone this repo in your own CI pipelines. Download the pre-built binary directly from GitHub Releases:

```yaml
# .github/workflows/e2e.yml
jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install FlutterProbe
        run: |
          curl -Lo probe https://github.com/AlphaWaveSystems/flutter-probe/releases/latest/download/probe-linux-amd64
          chmod +x probe
          sudo mv probe /usr/local/bin/

      - name: Start Android emulator
        uses: reactivecircus/android-emulator-runner@v2
        with:
          api-level: 33
          script: |
            probe test tests/ \
              --device emulator-5554 \
              --format junit \
              -o results.xml \
              --timeout 60s -v -y

      - name: Upload test results
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: test-results
          path: results.xml
```

Pin to a specific version for reproducible builds:

```yaml
- name: Install FlutterProbe v0.1.0
  run: |
    curl -Lo probe https://github.com/AlphaWaveSystems/flutter-probe/releases/download/v0.1.0/probe-linux-amd64
    chmod +x probe && sudo mv probe /usr/local/bin/
```

Generate a portable HTML report from JSON output:

```bash
probe test tests/ --format json -o reports/results.json
probe report --input reports/results.json -o reports/report.html --open
```

## Cloud Device Farms

Run tests on real devices without managing your own device lab. Bring your own account:

| Provider | Flag value |
|---|---|
| BrowserStack App Automate | `browserstack` |
| Sauce Labs Real Device Cloud | `saucelabs` |
| LambdaTest Real Devices | `lambdatest` |
| AWS Device Farm | `aws` |
| Firebase Test Lab | `firebase` |

```bash
probe test tests/ \
  --cloud-provider browserstack \
  --cloud-device "Google Pixel 7-13.0" \
  --cloud-app app-release.apk \
  --cloud-key YOUR_KEY \
  --cloud-secret YOUR_SECRET
```

## Migrating from Other Frameworks

`probe-convert` translates tests from 7 formats at 100% construct coverage:

```bash
probe-convert tests/maestro/          # Maestro YAML
probe-convert tests/features/         # Gherkin / Cucumber
probe-convert tests/robot/            # Robot Framework
probe-convert tests/detox/            # Detox (JS/TS)
probe-convert tests/appium/           # Appium (Python, Java, JS)
```

## VS Code Extension

Install from source (`vscode/`) for:

- Syntax highlighting for `.probe` files
- Code completion for all ProbeScript commands, selectors, tags, and recipes
- CodeLens **Run** buttons above every `test` block
- Run by tag, run with options, auto-lint on save
- Session Manager — configure and run across multiple devices simultaneously
- Test Explorer sidebar
- Integrated recording and Studio launcher

## Configuration (`probe.yaml`)

```yaml
project:
  app: com.example.myapp

defaults:
  platform: android
  timeout: 30s
  screenshots: true
  video: false
  retry: 0

agent:
  port: 48686
  dial_timeout: 30s
  token_timeout: 30s

device:
  boot_timeout: 120s

visual:
  threshold: 0.5
  pixel_delta: 8

tools:
  adb: /path/to/adb          # optional override
  flutter: /path/to/flutter  # optional override

ai:
  api_key: sk-ant-...        # for probe generate and self-healing
  model: claude-sonnet-4-6
```

## Self-Healing Selectors

When a selector fails, FlutterProbe automatically tries to find a replacement using:

1. **Fuzzy text match** — Levenshtein similarity against visible text
2. **Partial key match** — Substring match on ValueKey
3. **Semantic match** — Accessibility label similarity
4. **LLM-assisted** — Claude API suggestion with confidence score (requires `ai.api_key`)

## Repository Structure

```
cmd/probe/              CLI entry point
internal/
  cli/                  Cobra command implementations
  parser/               ProbeScript lexer + parser (AST)
  runner/               Test orchestration + reporting
  probelink/            JSON-RPC 2.0 WebSocket client
  device/               ADB integration (Android)
  ios/                  xcrun simctl integration (iOS)
  cloud/                Device farm provider integrations
  ai/                   Self-healing + AI test generation
  visual/               Screenshot visual regression
  plugin/               YAML-defined custom commands
  report/               HTML report generation
probe_agent/            Dart package (runs on-device)
tools/probe-convert/    Multi-format test converter
vscode/                 VS Code extension
website/                Documentation site (Starlight/Astro)
tests/                  E2E test suites and health checks
```

## Documentation

Full documentation: [alphawavesystems.github.io/flutter-probe](https://alphawavesystems.github.io/flutter-probe/)

- [Installation Guide](https://alphawavesystems.github.io/flutter-probe/getting-started/installation/)
- [Quick Start](https://alphawavesystems.github.io/flutter-probe/getting-started/quick-start/)
- [ProbeScript Syntax](https://alphawavesystems.github.io/flutter-probe/probescript/syntax/)
- [CLI Reference](https://alphawavesystems.github.io/flutter-probe/tools/cli-reference/)
- [CI/CD Integration](https://alphawavesystems.github.io/flutter-probe/ci-cd/github-actions/)
- [Configuration Reference](https://alphawavesystems.github.io/flutter-probe/advanced/configuration/)
- [Cloud Providers](https://alphawavesystems.github.io/flutter-probe/advanced/configuration/)
- [Visual Regression](https://alphawavesystems.github.io/flutter-probe/advanced/visual-regression/)

## License

[Business Source License 1.1](LICENSE) — free for all use except competing commercial hosted testing services. Converts to Apache 2.0 after 4 years per release.

Copyright © 2026 Alpha Wave Systems S.A. de C.V.
