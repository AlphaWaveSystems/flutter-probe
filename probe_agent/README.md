# FlutterProbe Agent

On-device E2E test agent for [FlutterProbe](https://flutterprobe.dev). Embeds in your Flutter app and receives test commands from the `probe` CLI via WebSocket or HTTP.

[![pub package](https://img.shields.io/pub/v/flutter_probe_agent.svg)](https://pub.dev/packages/flutter_probe_agent)
[![Publisher](https://img.shields.io/pub/publisher/flutter_probe_agent.svg)](https://pub.dev/publishers/alphawavesystems.com)

## How FlutterProbe Works

FlutterProbe is a **two-part system**:

1. **This package** (`flutter_probe_agent`) — embeds in your Flutter app, listens for test commands
2. **The CLI** (`probe`) — a Go binary that parses `.probe` test files and sends commands to the agent

Both are required. The agent alone does nothing without the CLI to drive it.

```
┌──────────────┐    WebSocket / HTTP    ┌─────────────────────┐
│  probe CLI   │ ◄──────────────────► │  flutter_probe_agent  │
│  (Go binary) │    JSON-RPC 2.0       │  (in your app)        │
└──────────────┘                        └─────────────────────┘
```

## Step 1: Install the CLI

The `probe` CLI is a Go binary. Install via one of:

```bash
# Option A: Homebrew (macOS + Linux — recommended)
brew tap AlphaWaveSystems/tap
brew install probe

# Option B: Go install (requires Go 1.26+)
go install github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest

# Option C: Download from GitHub Releases (all platforms)
# https://github.com/AlphaWaveSystems/flutter-probe/releases/latest
```

Verify:

```bash
probe version
```

## Step 2: Add the Agent to Your App

```yaml
# pubspec.yaml
dev_dependencies:
  flutter_probe_agent: ^0.5.3
```

Initialize in your `main.dart`:

```dart
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  const probeEnabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  if (probeEnabled) {
    await ProbeAgent.start();
  }

  runApp(const MyApp());
}
```

The agent is **completely inactive** unless `PROBE_AGENT=true` is passed at build time. It adds zero overhead to your production app.

## Step 3: Write a Test

Create `tests/login.probe`:

```
test "user can log in"
  tap "Email"
  type "user@test.com" into "Email"
  tap "Password"
  type "secret123" into "Password"
  tap "Sign In"
  wait until "Dashboard" appears
  see "Welcome"
```

## Step 4: Run It

```bash
# Start your app with the agent enabled
flutter run --dart-define=PROBE_AGENT=true

# In another terminal, run the test
probe test tests/login.probe --device <your-device> -v
```

Tests execute with sub-50ms command round-trips via direct widget-tree access — no UI automation layer, no WebDriver, no accessibility bridge.

## Physical Device Testing

For physical iOS devices, **WiFi is recommended** (USB-C causes intermittent connection drops):

```bash
# Build with WiFi enabled
flutter build ios --profile \
  --dart-define=PROBE_AGENT=true \
  --dart-define=PROBE_WIFI=true

# Install and launch on device
xcrun devicectl device install app --device <UDID> build/ios/iphoneos/Runner.app
xcrun devicectl device process launch --device <UDID> <bundle-id>

# Run tests over WiFi (find token in app console: PROBE_TOKEN=...)
probe test tests/ --host <device-ip> --token <probe-token> -v
```

## Biometric Authentication Testing (v0.9.7+)

`enroll biometric`, `biometric match`, and `biometric no match` ProbeScript steps drive Face ID / Touch ID / fingerprint flows on iOS Simulator and Android emulator. On iOS 26+ simulator, the notifyutil `no-match` notification no longer resolves `LAContext.evaluatePolicy`. Use `awaitBiometricResult()` in your screen widget to receive the result from the CLI via a Dart `Completer`:

```dart
import 'package:flutter_probe_agent/flutter_probe_agent.dart';
import 'package:local_auth/local_auth.dart';

Future<bool> _signIn() async {
  if (const bool.fromEnvironment('PROBE_AGENT')) {
    // CLI delivers true (match) or false (no-match) via probe.biometric_signal.
    // Works on all iOS simulator versions including iOS 26+.
    return awaitBiometricResult();
  }
  return LocalAuthentication().authenticate(localizedReason: 'Sign in');
}
```

The CLI automatically sends `probe.biometric_signal` after every `biometric match` / `biometric no match` step — no changes to `.probe` test files are needed.

## Agent reference

Everything you need to run the agent safely. Full docs: [flutterprobe.dev/tools/agent](https://flutterprobe.dev/tools/agent/).

### Build flags (`--dart-define=NAME=value`)

| Define | Effect |
|---|---|
| `PROBE_AGENT=true` | Enables the agent. Required; without it `ProbeAgent.start()` is a no-op, so the call is safe to leave in `main`. |
| `PROBE_WIFI=true` | Bind to `0.0.0.0` so the CLI on another machine can connect (`probe test --host <ip> --token <token>`). Debug/profile only. |
| `PROBE_PORT=<n>` | Move off the default port 48686 (pair with `probe test --agent-port <n>`). |
| `PROBE_AGENT_FORCE=true` | Silence the warning in an explicitly allowed release build. |
| `PROBE_RELAY_URL`, `PROBE_RELAY_TOKEN` | Relay mode for cloud device farms (the agent connects out). |

### Build modes

- **Debug / profile**: work with `PROBE_AGENT=true` (profile is required to cold-launch on physical iOS).
- **Release**: blocked by default because the agent opens a debug server. `ProbeAgent.start(allowReleaseBuild: true)`
  overrides it. Never ship that to users.

The package has **no native plugin dependency**; nothing native is linked into your app.

### Ports and tokens

The agent listens on `127.0.0.1:48686` (falling back through the next 9 ports and printing `PROBE_PORT=<n>`), prints
`PROBE_TOKEN=<token>` every 3 seconds and writes it to a file the CLI reads. Simulators/emulators use WebSocket;
physical devices use stateless HTTP POST. If another process holds the port, a failed connection names it
(`agent port 48686 is held by pid 4242 (Runner)`).

### What it does for the CLI

JSON-RPC 2.0 methods for tap, type, scroll (including `scroll ... until ... appears`), assertions, waiting
(`wait for idle`), screenshots, widget-tree dumps, mocks and signals. `type` goes through the keyboard path, so
`onChanged` and `inputFormatters` run. Failures list the visible texts and keys. Only widgets on the current route are
matched.

### Not covered by the agent

OS dialogs (permission alerts, the StoreKit sign-in sheet) are outside the Flutter widget tree; the CLI drives them
separately: see [System dialogs](https://flutterprobe.dev/tools/system-dialogs/). Studio's WiFi auto-discovery uses an
optional `ProbeAdvertiser` hook that you implement in your app (no mDNS plugin is bundled here).

### Troubleshooting

| Symptom | Fix |
|---|---|
| CLI cannot find the agent | Build with `--dart-define=PROBE_AGENT=true` and look for `PROBE_TOKEN=` in the log. |
| `agent rejected token (HTTP 401)` | You reached a different agent (a leftover app on 48686). Stop it, or use `PROBE_PORT` + `--agent-port`. |
| Red screen after closing a dialog on 0.10 – 0.14 | Fixed in 0.15.0: upgrade. |
| Version-mismatch warning | Use the same minor version of the CLI and the agent. |

## Features

- **WebSocket + HTTP transports** — persistent connection for simulators, stateless HTTP for physical devices
- **Profile mode support** — works on physical iOS devices (not just debug)
- **Release mode safeguards** — blocked by default, opt-in with `allowReleaseBuild: true`
- **WiFi testing** — bind to `0.0.0.0` with `PROBE_WIFI=true` for cable-free testing. Studio auto-discovery (mDNS) is an optional `ProbeAdvertiser` hook you implement in your app (a copy-paste `bonsoir` example is in the [Studio docs](https://flutterprobe.dev/tools/studio/#wifi-auto-discovery)), so this package has **no native plugin dependency** and nothing native is linked into release builds
- **Pre-shared restart token** — `restart the app` works over WiFi without USB log reading
- **`tap "X" if visible`** — conditional actions that skip silently when widget is not found
- **`PROBE_PORT`** — `--dart-define=PROBE_PORT=48700` moves the agent off 48686 (pair with `probe test --agent-port 48700`)
- **`scroll … until … appears` / `wait for idle`** — agent-side scroll-into-view and route-transition settling (0.15.0)
- **Text input like a keyboard** — `type`/`clear` run through `EditableTextState`, so `onChanged` and `inputFormatters` fire
- **Actionable failures** — `Widget not found` / `Timed out waiting for` list the visible texts and keys; `probe.visible_summary` RPC
- **Waiting and retries** — step timeout, implicit wait, per-step `within N seconds` budgets, `optional`/`if visible`, `retry N times` and retried tests; the same budgets apply to backend `wait for response` steps. Guide: [Timeouts, waiting and retries](https://flutterprobe.dev/advanced/timeouts-and-retries/)
- **Backend awareness** — the agent records the app's `dart:io` HTTP traffic (redacted headers, 64 KB bodies, last 300 exchanges; `PROBE_HTTP_CAPTURE=false` disables) and applies `mock` rules for real, so tests can `wait for response`, assert on status/JSON, branch on server data and simulate slow or failing backends. Start the agent before the app creates its HTTP clients. Guide: [Testing against backend data](https://flutterprobe.dev/advanced/backend-data/)
- **ARB labels** — `tap l10n "saveButton"` resolves the text from your `gen_l10n` ARB files (`probe.yaml` `l10n.dir`) in the app's current language
- **Named devices** — every simulator/emulator probe opens is named (`probe device start --name …`), and every test result records the device name and id it ran on
- **Multi-language apps** — `set language "de"` / `probe test --locale de` / `--locales de,ja,ar` change the app language per test or run; text selectors can match loosely (case, accents, typographic apostrophes, whitespace folded; `probe test --match-loose`), `#key` selectors never depend on the language, and `see any of "Save", "Speichern"` takes alternatives. Guide: [Testing localized apps](https://flutterprobe.dev/advanced/multi-language/)
- **Port-range fallback** — auto-tries ports 48686–48695 if preferred port is busy; logs `PROBE_PORT_BUSY=N (another probe agent is running)` when collision is with a sibling agent

## Requirements

- **Flutter** 3.19+ (tested up to 3.41)
- **Dart** 3.3+
- **FlutterProbe CLI** — [install instructions](https://github.com/AlphaWaveSystems/flutter-probe#installation)

## Documentation

- [Getting Started](https://flutterprobe.dev/getting-started/installation/)
- [ProbeScript Syntax](https://flutterprobe.dev/probescript/syntax/)
- [ProbeScript Dictionary](https://flutterprobe.dev/probescript/dictionary/)
- [CLI Reference](https://flutterprobe.dev/tools/cli-reference/)
- [iOS Integration Guide](https://flutterprobe.dev/platform/ios/)

## License

[MIT](LICENSE) — free to embed in any app, including commercial and proprietary.

The FlutterProbe CLI (the Go binary that drives tests) is licensed separately under [BSL 1.1](https://github.com/AlphaWaveSystems/flutter-probe/blob/main/LICENSE).

## Publisher

Built by [Alpha Wave Systems](https://alphawavesystems.com) in Guadalajara, Mexico.
