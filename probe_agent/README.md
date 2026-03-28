# FlutterProbe Agent

On-device E2E test agent for [FlutterProbe](https://flutterprobe.dev). Embeds in your Flutter app and receives test commands from the `probe` CLI via WebSocket or HTTP.

[![pub package](https://img.shields.io/pub/v/flutter_probe_agent.svg)](https://pub.dev/packages/flutter_probe_agent)
[![Publisher](https://img.shields.io/pub/publisher/flutter_probe_agent.svg)](https://pub.dev/publishers/alphawavesystems.com)

## What is FlutterProbe?

FlutterProbe lets you write E2E tests in plain English:

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

Tests execute with sub-50ms command round-trips via direct widget-tree access — no UI automation layer, no WebDriver, no accessibility bridge.

## Installation

Add to your `pubspec.yaml`:

```yaml
dev_dependencies:
  flutter_probe_agent: ^0.5.1
```

## Setup

Initialize the agent in your app's `main.dart`:

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

Build with the agent enabled:

```bash
flutter run --dart-define=PROBE_AGENT=true
```

The agent is **completely inactive** unless `PROBE_AGENT=true` is passed at build time. It adds zero overhead to your production app.

## Physical Device Testing

For physical iOS devices, WiFi testing is recommended:

```bash
# Build with WiFi enabled
flutter build ios --profile \
  --dart-define=PROBE_AGENT=true \
  --dart-define=PROBE_WIFI=true

# Run tests over WiFi
probe test tests/ --host <device-ip> --token <probe-token>
```

## Features

- **WebSocket + HTTP transports** — persistent connection for simulators, stateless HTTP for physical devices
- **Profile mode support** — works on physical iOS devices (not just debug)
- **Release mode safeguards** — blocked by default, opt-in with `allowReleaseBuild: true`
- **Ping/pong keepalive** — prevents idle connection drops
- **WiFi testing** — bind to `0.0.0.0` with `PROBE_WIFI=true` for cable-free testing
- **Pre-shared restart token** — `restart the app` works over WiFi without USB log reading

## Documentation

- [Getting Started](https://flutterprobe.dev/getting-started/installation/)
- [ProbeScript Syntax](https://flutterprobe.dev/probescript/syntax/)
- [ProbeScript Dictionary](https://flutterprobe.dev/probescript/dictionary/)
- [iOS Integration Guide](https://flutterprobe.dev/platform/ios/)
- [CLI Reference](https://flutterprobe.dev/tools/cli-reference/)

## Requirements

- Flutter 3.19+ (tested up to 3.41)
- Dart 3.3+
- [FlutterProbe CLI](https://github.com/AlphaWaveSystems/flutter-probe) (`probe` binary)

## License

[Business Source License 1.1](LICENSE)

## Publisher

Built by [Alpha Wave Systems](https://alphawavesystems.com) in Guadalajara, Mexico.
