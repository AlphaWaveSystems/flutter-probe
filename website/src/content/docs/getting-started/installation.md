---
title: Installation
description: Install the probe CLI and integrate the Dart agent into your Flutter app.
---

**In a hurry?** On macOS or Linux with Flutter installed:

```bash
brew tap AlphaWaveSystems/tap && brew install probe     # 1. the CLI
# 2. add flutter_probe_agent to your app (Step 2 below) and run it with --dart-define=PROBE_AGENT=true
probe init && probe lint tests                          # 3. scaffold and check the sample
probe test tests/example.probe --device <id> -v         # 4. <id> from: probe device list
```

The steps below explain each part, and the [login test tutorial](/tutorials/flutter-login-test/) writes a real test.

## Step 1: Install the CLI

### Option A — Homebrew (macOS + Linux, recommended)

```bash
brew tap AlphaWaveSystems/tap
brew install probe
```

### Option B — Pre-built binary (all platforms, good for CI)

Each release ships three artifacts per OS / architecture: `probe` (CLI test runner, always required), `probe-mcp` (optional standalone MCP server for AI agents), and a Studio Beta Preview archive (optional desktop app). Download what you need from [GitHub Releases](https://github.com/AlphaWaveSystems/flutter-probe/releases/latest):

| Platform | CLI | MCP server |
|---|---|---|
| macOS (Apple Silicon) | `probe-darwin-arm64` | `probe-mcp-darwin-arm64` |
| macOS (Intel) | `probe-darwin-amd64` | `probe-mcp-darwin-amd64` |
| Linux (x86-64) | `probe-linux-amd64` | `probe-mcp-linux-amd64` |
| Windows (x86-64) | `probe-windows-amd64.exe` | `probe-mcp-windows-amd64.exe` |

Make the binaries executable and place them on your `PATH`:

```bash
chmod +x probe-darwin-arm64 probe-mcp-darwin-arm64
mv probe-darwin-arm64     /usr/local/bin/probe
mv probe-mcp-darwin-arm64 /usr/local/bin/probe-mcp
```

### Option C — `go install` (requires Go 1.26+)

```bash
go install github.com/AlphaWaveSystems/flutter-probe/cmd/probe@latest
go install github.com/AlphaWaveSystems/flutter-probe/cmd/probe-mcp@latest   # optional
```

This is a good option for CI environments that already have Go set up.

Verify:

```bash
probe version
```

## Optional: Install Studio

[FlutterProbe Studio](/tools/studio/) is a cross-platform desktop app for visual test authoring with an embedded device view. Download the archive for your platform from the GitHub Release:

```bash
# macOS — extract the .app bundle and move to Applications
unzip flutter-probe-studio-<version>-darwin-arm64.zip
mv flutter-probe-studio.app /Applications/

# Linux — single binary
tar xzf flutter-probe-studio-<version>-linux-amd64.tar.gz
mv flutter-probe-studio /usr/local/bin/

# Windows — unzip and double-click the .exe
```

Studio is **Beta Preview** — production-ready for editor + lint workflows but expect rough edges around physical device support and parallel multi-device authoring.

## Try a canary build

Before a stable release, pre-release builds named `vX.Y.Z-next.N` are published so you can try upcoming changes early. A canary never replaces the stable install: it is a GitHub pre-release (not "latest"), and pub.dev keeps its stable version.

```bash
# CLI via Homebrew (cannot be installed next to the stable "probe" formula)
brew uninstall probe          # only if the stable formula is installed
brew install AlphaWaveSystems/tap/probe@next
probe version                 # prints e.g. 0.25.2-next.1

# or download the binary from the pre-release page
# https://github.com/AlphaWaveSystems/flutter-probe/releases (pick the vX.Y.Z-next.N entry)
```

The iOS system-dialog driver (`probe ios-driver install`) comes from the same pre-release (`probe-ios-driver.zip`). It is built after the CLI binaries, so it can appear a few minutes later; if it is missing, `probe ios-driver install` reports a 404. Canaries do not include Studio, the `.mcpb` extension or Windows binaries; use a stable release for those.

The canary CLI and the agent must come from the same build. The agent is **not** published to pub.dev for a canary, so take it from the tagged source with a git dependency in your app's `pubspec.yaml`:

```yaml
dev_dependencies:
  flutter_probe_agent:
    git:
      url: https://github.com/AlphaWaveSystems/flutter-probe.git
      ref: v0.25.2-next.1      # the canary tag
      path: probe_agent
```

To go back to stable, `brew uninstall probe@next && brew install probe` and restore the pub.dev version constraint. See [Compatibility](/advanced/compatibility/) for what a CLI and agent of different versions can do together.

## Step 2: Add the Agent to Your App

Add `flutter_probe_agent` to your Flutter app's `pubspec.yaml`:

```yaml
dev_dependencies:
  flutter_probe_agent: ^0.25.0
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

## Step 3: Initialize Your Project

From your Flutter project directory:

```bash
probe init
```

This creates `probe.yaml` (set `project.app` to your app's bundle id or package name), `tests/example.probe` and a sample recipe under `tests/recipes/`. Check that they parse before you run anything:

```bash
probe lint tests
```

## Step 4: Build and Run

The app must be running on the device with the agent enabled (`--dart-define=PROBE_AGENT=true`) before `probe test` connects. If `probe test` cannot connect, it tells you what it was waiting for; the usual causes are the app not running, a build without the flag, or another process holding the agent port.

**iOS Simulator:**

```bash
flutter run --dart-define=PROBE_AGENT=true
# In another terminal:
probe test tests/example.probe --device <your-device> -v
```

**Android Emulator:**

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true
adb install -r build/app/outputs/flutter-apk/app-debug.apk
adb shell am start -n com.example.myapp/.MainActivity
probe test tests/example.probe --device emulator-5554 -v
```

**Physical iOS (WiFi mode — recommended):**

```bash
flutter build ios --profile --dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true
xcrun devicectl device install app --device <UDID> build/ios/iphoneos/Runner.app
xcrun devicectl device process launch --device <UDID> <bundle-id>
# Find PROBE_TOKEN in app console, then:
probe test tests/ --host <device-ip> --token <probe-token> -v
```

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
