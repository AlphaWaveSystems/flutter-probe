# FlutterProbe for Visual Studio Code

High-performance E2E testing for Flutter apps — write tests in natural language, run them locally or on cloud device farms, and catch visual regressions automatically.

![FlutterProbe](resources/probe-icon.png)

## Features

### ProbeScript Language Support

Full editor support for `.probe` files:

- **Syntax highlighting** — keywords, strings, selectors, tags, comments
- **IntelliSense** — autocomplete for commands, permissions, tags, and recipes
- **Named devices** — run results show the named simulator/emulator each test ran on (`probe device start --name …`)
- **Performance** — snippets and completions for `start measuring`, `see memory below`, `see cpu below`, `see slow frames below`
- **Time budgets** — `within N seconds` highlighted; snippet `within`
- **Backend data** — snippets and completions for `wait for response`, `see response`, `if response`
- **ARB labels** — snippet for `l10n "key"` selectors
- **App language** — snippet for `set language "de"`; `probe test --locales de,ja,ar` runs a suite per language
- **Appearance** — snippet for `set appearance dark` (light, dark, or auto on Android)
- **Airplane mode** — snippet for `set airplane mode on` (Android emulators only; nothing restores the state)
- **Multi-language suites** — snippets for `see any of`, `wait until any of` and `wait until … appears matching`; the grammar highlights them
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
| **Firebase Test Lab** (deprecated by Google, shuts down 2027-09-30) | Batch test execution | Service Account JSON |

All providers are implemented but not yet verified on a live account, and cloud runs need a relay service. See [Cloud device farms](https://flutterprobe.dev/tools/cloud-farms/).

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

### Testing view

Tests also appear in VS Code's built-in **Testing** view, with a run button next to each test and in the editor gutter. A run executes the test's file with `probe test --yes --format json` and takes each test's pass or fail from the CLI's report. The CLI has no per-test filter in this version, so a run executes the whole file, and only the tests you selected are reported. Output streams to the run's output panel. The run uses the `flutterprobe.defaultDevice` setting when it is set.

## Getting Started

### Prerequisites

1. Install the [FlutterProbe CLI](https://github.com/AlphaWaveSystems/flutter-probe):

```bash
# macOS / Linux
curl -fsSL https://flutterprobe.dev/install.sh | sh

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
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

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
| `probePath` | `probe` | Path to the probe CLI binary. User or remote settings only. Without it: `probe` on PATH (see Tool Paths) |
| `convertPath` | `probe-convert` | Path to the probe-convert binary. User or remote settings only. Without it: `probe-convert` on PATH (see Tool Paths) |
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
| `cloudProvider` | | Provider: `browserstack`, `saucelabs`, `lambdatest`, `aws`, `firebase`. User or remote settings only |
| `cloudDevice` | | Device string (e.g., `Google Pixel 7-13.0`) |
| `cloudApp` | | Path to `.apk` or `.ipa` binary |
| `cloudKey` | | Username or Access Key ID. User or remote settings only |
| `cloudSecret` | | Access Key or Secret. User or remote settings only |
| `relayUrl` | | Relay server URL for cloud E2E. User or remote settings only |
| `relayToken` | | Relay authentication token. User or remote settings only |

**The cloud key, the cloud secret and the relay token never go on the command line.** A command line is visible to other users in the process list, so the extension gives these three to `probe test` as environment variables of that run (`PROBE_CLOUD_KEY`, `PROBE_CLOUD_SECRET`, `PROBE_RELAY_CLI_TOKEN`), which the CLI reads when the flag is not given.

**What else the programs inherit.** Every program the extension starts inherits VS Code's environment. The settings' secrets are given only to test runs (the terminal runs and Test Explorer runs). The device list, lint and the device view get VS Code's environment without the three variables, in any casing. The other runs (Studio, record, convert, init, lint, report, devices) get no extra variables, but still inherit VS Code's own environment, so if VS Code itself was started with one of the three variables set, those runs have it too. Nothing else is removed: a `${NAME}` in a test reads any variable by design. The boundary is which binaries run and in which workspaces (see Tool Paths).

**Where they live.** The values are in the environment of that one `probe test` process and of the programs it starts (`adb`, `flutter`), for as long as they run. The FlutterProbe terminals are not shells: nothing typed in them runs as a command, and a later run gets its own environment. A setting's value wins over a variable of the same name in VS Code's own environment.

**What a workspace cannot set.** `cloudKey`, `cloudSecret` and `relayToken`, and the two settings that decide where they are sent, `relayUrl` (the relay token is sent to it) and `cloudProvider` (the key and secret are sent to it), can be set only in your user (or remote) settings. A repository's `.vscode/settings.json` cannot set them, and Settings Sync leaves them out. This covers the extension's settings only: `probe test` also reads the workspace's `probe.yaml`, whose `cloud:` section can name a provider and a relay service URL. Which binary the extension runs is a separate set of settings, restricted the same way (see Tool Paths); `probe.yaml`'s `tools:` section can still name `adb` and `flutter`. VS Code stores settings in plain text; on a shared machine or in CI, set the environment variables instead.

> **Changed in this version:** a value of `cloudKey`, `cloudSecret`, `relayToken`, `relayUrl` or `cloudProvider` in a workspace's `.vscode/settings.json` is now ignored, and the extension says so once for that workspace. Move it to your user settings (Preferences: Open User Settings (JSON)), or set the environment variable.

### Tool Paths

| Setting | Default | Description |
|---|---|---|
| `adbPath` | | Custom ADB binary path. User or remote settings only |
| `flutterPath` | | Custom Flutter binary path. User or remote settings only |
| `useWorkspaceBinaries` | `false` | Run `bin/probe` and `bin/probe-convert` from the open workspace when `probePath` or `convertPath` is not set. User or remote settings only |
| `studioPort` | `9191` | Port for probe studio |

**Which binaries run.**

- The settings that name a binary (`probePath`, `convertPath`, `adbPath`, `flutterPath`) and `useWorkspaceBinaries` can be set only in your user settings, or in the **remote** settings when VS Code is connected to a remote machine (SSH, WSL, a container): set them there, since a value in the workspace's `.vscode/settings.json` is ignored. A repository cannot point the extension at a binary of its own.
- The extension resolves the binary itself and runs it by its absolute path: the `probePath` you set (an absolute path, or a command name looked up on PATH), else `probe` on PATH. Only the absolute folders of PATH are searched, never the workspace: on Windows the system would otherwise look in the current folder first (Node's `spawn` and cmd.exe do), so a `probe.exe` in a repository could run instead of yours. When `probe` is on no absolute folder of PATH, the extension says so (on every system) rather than run a bare name that the system could find in the workspace. The extension runs the binary itself, with its arguments as a list and no shell, and shows the output in its own terminal: no shell parses, expands or line-edits a file name, under any terminal shell, system or locale (typing a quoted command line into a shell could not be made safe: under `LC_ALL=C`, bash's line editor acts on bytes of non-ASCII characters). The output stays after the run ends; a second run in the same terminal starts when the first ends; what you type there goes to the running tool line by line. Ctrl+C on macOS and Linux sends SIGINT to the tool and the programs it started, as in a shell (`probe record` saves what it recorded); on Windows, which has no Ctrl+C for another process, it ends the tool and every program it started (a recording is not saved). Closing the terminal does the same, and a tool that has not ended 5 seconds later is killed with the programs it started, so nothing is left running. A `.bat` or `.cmd` file as the tool is refused on Windows (Windows runs it only through cmd.exe): use the `.exe`. Not checked on Windows here: the tree kill, the batch refusal and the `.exe` runs were tested on macOS only, with the Windows paths covered by unit tests. The tool's output is written to the terminal as it is, its colors and other escape sequences included. The run gets VS Code's environment, not the one your shell profile builds.
- A `bin/probe` inside the workspace is used only when you turn on `useWorkspaceBinaries` and the workspace is trusted. Trusting a folder in VS Code is not consent to run its binaries.
- The extension runs only in a trusted workspace (VS Code's Restricted Mode keeps it off until you trust the folder). `probe test` still reads the workspace's `probe.yaml`, whose `tools:` section can name `adb` and `flutter` binaries: trust a folder only if you trust what it runs.

> **Changed in this version:** a value of `probePath`, `convertPath`, `adbPath` or `flutterPath` in a workspace's `.vscode/settings.json` is now ignored: move it to your user (or remote) settings. A `bin/probe` in the workspace is no longer picked up on its own: turn on `useWorkspaceBinaries` in your user settings to use it.

## Cloud Provider Setup

### BrowserStack

1. Sign up at [browserstack.com](https://www.browserstack.com)
2. Go to **App Automate** > **Account** to find your username and access key
3. In your VS Code **user** settings (the key and the secret are not read from workspace settings):
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
2. Configure, in your **user** settings:
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
| FlutterProbe: Run Test File at Cursor | Run the file that contains the cursor (the CLI has no per-test filter yet) |
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
don't see "Hidden text"      # "dont see" works too
wait until "Element" appears
wait 3 seconds
swipe up / down / left / right
swipe up over 40 percent
swipe right on "Menu" from 2 percent
press tab
tap "Contact support" in webview
type "hi" into css:"#message" in webview
see "Getting Started" in webview
don't see css:".error" in webview
wait until "Message sent" appears in webview
store text of css:"#total" in webview as total
scroll down until "Item" appears
wait for idle
tap "Allow" in system dialog       # OS permission alerts / sign-in sheets
type "$ENV_VAR" into system field "Password"   # masked
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
  type "<email>" into "Email"
  type "<password>" into "Password"
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

## Device View (preview)

`FlutterProbe: Open Device View` opens a connected simulator, emulator or phone in an editor tab (Flutter layer only). Alt+click an element to get its selector and insert a `tap` step into the open `.probe` file. It starts `probe device view` locally and does not work in remote windows yet.
