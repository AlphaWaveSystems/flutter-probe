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

## Features

- **WebSocket + HTTP transports** — persistent connection for simulators, stateless HTTP for physical devices
- **Profile mode support** — works on physical iOS devices (not just debug)
- **Release mode safeguards** — blocked by default, opt-in with `allowReleaseBuild: true`
- **WiFi testing** — bind to `0.0.0.0` with `PROBE_WIFI=true` for cable-free testing
- **Pre-shared restart token** — `restart the app` works over WiFi without USB log reading
- **`tap "X" if visible`** — conditional actions that skip silently when widget is not found

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
