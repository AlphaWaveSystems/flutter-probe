---
title: Installation
description: Install the probe CLI and integrate the Dart agent into your Flutter app.
---

## Step 1: Install the CLI

### Option A — Homebrew (macOS + Linux, recommended)

```bash
brew tap AlphaWaveSystems/tap
brew install probe
```

### Option B — Pre-built binary (all platforms, good for CI)

Download the binary for your platform from [GitHub Releases](https://github.com/AlphaWaveSystems/flutter-probe/releases/latest):

| Platform | Binary |
|---|---|
| macOS (Apple Silicon) | `probe-darwin-arm64` |
| macOS (Intel) | `probe-darwin-amd64` |
| Linux (x86-64) | `probe-linux-amd64` |
| Windows (x86-64) | `probe-windows-amd64.exe` |

Make the binary executable and place it on your `PATH`:

```bash
chmod +x probe-darwin-arm64
mv probe-darwin-arm64 /usr/local/bin/probe
```

### Option C — `go install` (requires Go 1.26+)

```bash
go install github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest
```

This is a good option for CI environments that already have Go set up.

Verify:

```bash
probe version
```

## Step 2: Add the Agent to Your App

Add `flutter_probe_agent` to your Flutter app's `pubspec.yaml`:

```yaml
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

The `bool.fromEnvironment` check ensures the agent is only active when built with `--dart-define=PROBE_AGENT=true`. It adds zero overhead to your production app.

## Step 3: Build and Run

**iOS Simulator:**

```bash
flutter run --dart-define=PROBE_AGENT=true
# In another terminal:
probe test tests/login.probe --device <your-device> -v
```

**Android Emulator:**

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true
adb install -r build/app/outputs/flutter-apk/app-debug.apk
adb shell am start -n com.example.myapp/.MainActivity
probe test tests/login.probe --device emulator-5554 -v
```

**Physical iOS (WiFi mode — recommended):**

```bash
flutter build ios --profile --dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true
xcrun devicectl device install app --device <UDID> build/ios/iphoneos/Runner.app
xcrun devicectl device process launch --device <UDID> <bundle-id>
# Find PROBE_TOKEN in app console, then:
probe test tests/ --host <device-ip> --token <probe-token> -v
```

## Initialize Your Project

From your Flutter project directory:

```bash
probe init
```

This creates:
- `probe.yaml` — project configuration
- `tests/` — directory for `.probe` test files with samples

## Prerequisites

| Requirement | When needed |
|---|---|
| Dart 3.3+ / Flutter 3.19+ | Always (for the agent) |
| Android SDK + ADB | Android emulator/device testing |
| Xcode + `xcrun simctl` | iOS simulator testing |
| `libimobiledevice` (`brew install libimobiledevice`) | Physical iOS device testing via USB |

## Next Steps

- [Write your first test](/probescript/syntax/)
- [ProbeScript Dictionary](/probescript/dictionary/) — all commands and modifiers
- [iOS Integration Guide](/platform/ios/) — physical device setup
- [CI/CD with GitHub Actions](/ci-cd/github-actions/)
