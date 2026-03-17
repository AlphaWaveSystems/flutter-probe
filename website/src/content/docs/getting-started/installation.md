---
title: Installation
description: Build FlutterProbe from source and integrate the Dart agent into your Flutter app.
---

## Prerequisites

- **Go 1.23+** — for building the probe CLI
- **Dart 3.3+ / Flutter 3.19+** — for the probe_agent package
- **Android**: ADB + Android SDK (for Android emulator testing)
- **iOS**: Xcode + `xcrun simctl` (for iOS simulator testing)

## Build the CLI

Clone the repository and build the `probe` binary:

```bash
git clone https://github.com/nicktesh/FlutterProbe.git
cd FlutterProbe
make build          # outputs bin/probe
```

To install to your `$GOPATH/bin` (so `probe` is available globally):

```bash
make install
```

## Add ProbeAgent to Your Flutter App

The Dart agent runs inside your Flutter app and provides the WebSocket server that the CLI connects to.

### 1. Add the dependency

In your Flutter app's `pubspec.yaml`:

```yaml
dev_dependencies:
  probe_agent:
    path: /path/to/FlutterProbe/probe_agent
```

### 2. Initialize in main.dart

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

The `PROBE_AGENT` flag is compiled into the binary via `--dart-define`. It must be set at build time — it cannot be toggled at runtime.

### 3. Build with the flag enabled

**iOS Simulator:**

```bash
flutter build ios --debug --simulator --dart-define=PROBE_AGENT=true
xcrun simctl install <UDID> build/ios/iphonesimulator/YourApp.app
xcrun simctl launch <UDID> com.example.myapp
```

**Android Emulator:**

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true
adb install -r build/app/outputs/flutter-apk/app-debug.apk
adb shell am start -n com.example.myapp/.MainActivity
```

## Initialize Your Project

From your Flutter project directory:

```bash
probe init
```

This creates:
- `probe.yaml` — project configuration
- `tests/` — directory for `.probe` test files with samples

## Build the Test Converter (Optional)

If you want to migrate tests from other frameworks:

```bash
make build-convert    # outputs bin/probe-convert
```

See [probe-convert](/tools/probe-convert/) for details.
