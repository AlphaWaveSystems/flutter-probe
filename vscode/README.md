# FlutterProbe for Visual Studio Code

High-performance E2E testing for Flutter apps — write tests in natural language, run them locally or on cloud device farms, and catch visual regressions automatically.

![FlutterProbe](resources/probe-icon.png)

## Features

### ProbeScript Language Support

Full editor support for `.probe` files:

- **Syntax highlighting** — keywords, strings, selectors, tags, comments
- **IntelliSense** — autocomplete for commands, permissions, tags, and recipes
- **Snippets** — 19 built-in snippets (`test`, `tap`, `type`, `see`, `wait`, `recipe`, `if`, `repeat`, `dart`, `mock`, and more)
- **Lint on save** — real-time diagnostics from the ProbeScript parser
- **CodeLens** — inline Run / Debug buttons above every test

### Test Runner

Run tests directly from the editor:

- **Run at cursor** (`Cmd+Shift+T`) — run the test under your cursor
- **Run file** (`Cmd+Shift+R`) — run all tests in the current file
- **Run by tag** — filter tests by `@smoke`, `@critical`, or custom tags
- **Run with options** — interactive picker for device, timeout, format, video
- **Run Profile panel** — full configuration webview with local and cloud options

### Session Manager

Test on multiple devices in parallel:

- Create named sessions, each bound to a device and port
- Run tests on a single session or all sessions simultaneously (`Cmd+Shift+D`)
- Per-session CodeLens buttons appear inline above each test
- Sessions persist in `.vscode/flutterprobe-sessions.json`

### Cloud Device Testing

Run tests on real devices in the cloud — bring your own account:

| Provider | Type | Auth |
|---|---|---|
| **BrowserStack** | Real devices via Appium | Username + Access Key |
| **Sauce Labs** | Real Device Cloud via Appium | Username + Access Key |
| **LambdaTest** | Real devices via Appium | Username + Access Key |
| **AWS Device Farm** | Managed device sessions | Access Key ID + Secret |
| **Firebase Test Lab** | Batch test execution | Service Account JSON |

Configure via VS Code settings or the Run Profile panel. Cloud credentials can also be set in `probe.yaml` under the `cloud:` section.

### Test Recording

Record user interactions and generate ProbeScript automatically:

1. **Start Recording** — select a device, the agent captures taps, swipes, and text input
2. **Stop Recording** — generates a `.probe` file with natural language steps
3. Gaps longer than 2 seconds automatically insert `wait N seconds`

### Visual Regression

Catch unintended UI changes:

- `compare screenshot "name"` — takes a screenshot and compares against a stored baseline
- Configurable threshold (default 0.5%) and pixel delta (default 8)
- Works in both local and cloud mode (base64 transfer for cloud devices)

### Studio

Launch the interactive widget inspector:

- **Open Studio** — starts a local server and opens the browser
- Live widget tree exploration with tap-to-select
- Selector suggestions for writing tests

### Test Conversion

Migrate from other frameworks with zero manual rewrite:

- **Maestro** YAML
- **Gherkin** / Cucumber `.feature` files
- **Robot Framework** `.robot` files
- **Detox** JS/TS
- **Appium** Python, Java, and JS

### Test Explorer

Sidebar tree view of all tests and recipes in your workspace:

- Organized by folder and file
- Shows test names and tags
- Click to navigate to source
- Refresh button to rescan

## Getting Started

### Prerequisites

1. Install the [FlutterProbe CLI](https://github.com/AlphaWaveSystems/flutter-probe):

```bash
# macOS / Linux
curl -fsSL https://flutterprobe.com/install.sh | sh

# Or build from source
git clone https://github.com/AlphaWaveSystems/flutter-probe.git
cd flutter-probe && make install
```

2. Add `probe_agent` to your Flutter app's `pubspec.yaml`:

```yaml
dependencies:
  probe_agent:
    git:
      url: https://github.com/AlphaWaveSystems/flutter-probe.git
      path: probe_agent
```

3. Initialize the agent in your `main.dart`:

```dart
import 'package:probe_agent/probe_agent.dart';

void main() {
  ProbeAgent.start(); // Only active in debug/profile builds
  runApp(MyApp());
}
```

### Initialize a Project

Run **FlutterProbe: Init Project** from the command palette (`Cmd+Shift+P`) to generate a `probe.yaml` configuration file.

### Write Your First Test

Create a file named `tests/login.probe`:

```
test "user can log in"
  @smoke @critical
  open the app
  wait until "Sign In" appears
  tap on "Sign In"
  type "user@example.com" into the "Email" field
  type "password123" into the "Password" field
  tap "Continue"
  see "Dashboard"
```

Press `Cmd+Shift+T` to run it.

## Configuration

All settings are under `flutterprobe.*` in VS Code settings. They follow the resolution order: **VS Code setting > probe.yaml > built-in default**.

### General

| Setting | Default | Description |
|---|---|---|
| `probePath` | `probe` | Path to the probe CLI binary |
| `convertPath` | `probe-convert` | Path to the probe-convert binary |
| `autoLint` | `true` | Lint `.probe` files on save |
| `defaultDevice` | | Default device serial |
| `defaultTimeout` | `30s` | Per-step timeout |
| `autoConfirm` | `false` | Auto-confirm destructive actions (`-y` flag) |
| `outputFormat` | `terminal` | Output format: `terminal`, `json`, or `junit` |
| `outputPath` | | Output file path for results |

### Agent Connection

| Setting | Default | Description |
|---|---|---|
| `agentPort` | `48686` | Agent WebSocket host port |
| `dialTimeout` | `30s` | WebSocket connection timeout |
| `tokenTimeout` | `30s` | Agent token wait timeout |
| `reconnectDelay` | `2s` | Post-restart reconnect delay |

### Video & Visual Regression

| Setting | Default | Description |
|---|---|---|
| `video` | `false` | Record video during test runs |
| `videoResolution` | `720x1280` | Video resolution |
| `videoFramerate` | `2` | Video framerate (fps) |
| `visualThreshold` | `0.5` | Visual regression threshold (%) |
| `visualPixelDelta` | `8` | Per-pixel delta tolerance |

### Cloud Providers

| Setting | Default | Description |
|---|---|---|
| `cloudProvider` | | Provider: `browserstack`, `saucelabs`, `lambdatest`, `aws`, `firebase` |
| `cloudDevice` | | Device string (e.g., `Google Pixel 7-13.0`) |
| `cloudApp` | | Path to `.apk` or `.ipa` binary |
| `cloudKey` | | Username or Access Key ID |
| `cloudSecret` | | Access Key or Secret (consider env vars for CI) |
| `relayUrl` | | Relay server URL for cloud E2E |
| `relayToken` | | Relay authentication token |

### Tool Paths

| Setting | Default | Description |
|---|---|---|
| `adbPath` | | Custom ADB binary path |
| `flutterPath` | | Custom Flutter binary path |
| `studioPort` | `9191` | Port for probe studio |

## Cloud Provider Setup

### BrowserStack

1. Sign up at [browserstack.com](https://www.browserstack.com)
2. Go to **App Automate** > **Account** to find your username and access key
3. In VS Code settings:
   ```json
   {
     "flutterprobe.cloudProvider": "browserstack",
     "flutterprobe.cloudKey": "YOUR_USERNAME",
     "flutterprobe.cloudSecret": "YOUR_ACCESS_KEY",
     "flutterprobe.cloudDevice": "Google Pixel 7-13.0",
     "flutterprobe.cloudApp": "/path/to/app.apk"
   }
   ```

### Sauce Labs

1. Sign up at [saucelabs.com](https://saucelabs.com)
2. Go to **Account** > **User Settings** for credentials
3. Configure the same way with `"flutterprobe.cloudProvider": "saucelabs"`

### LambdaTest

1. Sign up at [lambdatest.com](https://www.lambdatest.com)
2. Go to **Settings** > **Account Settings** for credentials
3. Configure with `"flutterprobe.cloudProvider": "lambdatest"`

### AWS Device Farm

1. Create an IAM user with `AWSDeviceFarmFullAccess`
2. Configure:
   ```json
   {
     "flutterprobe.cloudProvider": "aws",
     "flutterprobe.cloudKey": "AKIAIOSFODNN7EXAMPLE",
     "flutterprobe.cloudSecret": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
     "flutterprobe.cloudDevice": "Google Pixel 7-13.0"
   }
   ```

### Firebase Test Lab

1. Create a service account with **Firebase Test Lab Admin** role
2. Download the JSON key file
3. Configure:
   ```json
   {
     "flutterprobe.cloudProvider": "firebase",
     "flutterprobe.cloudKey": "/path/to/service-account.json"
   }
   ```

Firebase Test Lab requires relay mode. Set `relayUrl` and `relayToken`, or let the CLI create a relay session automatically.

### Using probe.yaml Instead

Cloud credentials can also be configured in `probe.yaml` to keep them out of VS Code settings:

```yaml
cloud:
  provider: browserstack
  device: "Google Pixel 7-13.0"
  app: "/path/to/app.apk"
  credentials:
    username: YOUR_USERNAME
    access_key: YOUR_ACCESS_KEY
```

## Keyboard Shortcuts

| Command | macOS | Windows/Linux |
|---|---|---|
| Run test at cursor | `Cmd+Shift+T` | `Ctrl+Shift+T` |
| Run file | `Cmd+Shift+R` | `Ctrl+Shift+R` |
| Lint file | `Cmd+Shift+L` | `Ctrl+Shift+L` |
| Run on all sessions | `Cmd+Shift+D` | `Ctrl+Shift+D` |

## Commands

All commands are available via the command palette (`Cmd+Shift+P`):

| Command | Description |
|---|---|
| FlutterProbe: Run Test at Cursor | Run the test under cursor |
| FlutterProbe: Run File | Run all tests in current file |
| FlutterProbe: Run by Tag | Run tests matching a tag |
| FlutterProbe: Run with Options... | Interactive run configuration |
| FlutterProbe: Configure Run Options | Open Run Profile webview |
| FlutterProbe: Lint File | Lint current `.probe` file |
| FlutterProbe: Lint Workspace | Lint all `.probe` files |
| FlutterProbe: Start Recording | Record interactions on device |
| FlutterProbe: Stop Recording | Stop recording and save |
| FlutterProbe: List Devices | Show connected devices |
| FlutterProbe: Start Device/Emulator | Boot emulator or simulator |
| FlutterProbe: Open Studio | Launch widget inspector |
| FlutterProbe: Generate HTML Report | Create report from JSON |
| FlutterProbe: Open Report | Open report in browser |
| FlutterProbe: Init Project | Generate `probe.yaml` |
| FlutterProbe: Convert Tests | Convert from other frameworks |
| FlutterProbe: Add Session | Add a device session |
| FlutterProbe: Run on All Sessions | Run on all sessions in parallel |

## Supported ProbeScript Constructs

```
test "name"               # Define a test
  @tag1 @tag2             # Tag tests for filtering
recipe "name" (params)    # Reusable step sequences
use "path/to/file.probe"  # Import recipes

# Actions
open the app
tap on "Button"
type "text" into the "Field" field
see "Expected text"
don't see "Hidden text"
wait until "Element" appears
wait 3 seconds
swipe up / down / left / right
scroll down until "Item" appears
take screenshot "name"
compare screenshot "name"

# Lifecycle
restart the app
clear app data
allow permission "camera"
deny permission "notifications"
grant all permissions

# Control flow
if "Element" is visible
  tap on "Element"
else
  tap on "Fallback"
repeat 3 times
  tap on "Next"

# Data-driven
test "login with credentials"
  type <email> into "Email"
  type <password> into "Password"
  with examples:
    | email              | password  |
    | user@example.com   | pass123   |
    | admin@example.com  | admin456  |

# Hooks
before each test
  open the app
after each test
  take screenshot "final"
on failure
  take screenshot "failure"

# Escape hatch
run dart:
  print('Custom Dart code');
```

## Requirements

- VS Code 1.85+
- [FlutterProbe CLI](https://github.com/AlphaWaveSystems/flutter-probe) (`probe` binary in PATH or workspace `bin/`)
- Flutter 3.19+ and Dart 3.3+
- For local testing: Android emulator or iOS simulator
- For cloud testing: account with a supported provider

## License

BSL 1.1 — free for all use except competing commercial hosted testing services. Converts to Apache 2.0 after 4 years per release.
