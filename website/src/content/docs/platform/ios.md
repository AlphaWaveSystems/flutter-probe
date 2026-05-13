---
title: iOS
description: iOS simulator setup, token reading, simctl integration, and platform-specific notes.
---

## Requirements

- macOS with Xcode installed
- `xcrun simctl` command-line tools
- Flutter app built for simulator with `--dart-define=PROBE_AGENT=true`
- **Physical devices only:** `iproxy` from `libimobiledevice` for USB port forwarding (see [Physical Device Setup](#physical-device-setup) below)

## Simulator Setup

### List available simulators

```bash
probe device list
xcrun simctl list devices available
```

### Boot a simulator

```bash
probe device start --platform ios
```

Or manually:

```bash
xcrun simctl boot <UDID>
```

### Build and install your app

```bash
flutter build ios --debug --simulator --dart-define=PROBE_AGENT=true
xcrun simctl install <UDID> build/ios/iphonesimulator/YourApp.app
xcrun simctl launch <UDID> com.example.myapp
```

For cases where Xcode SDK and simulator iOS versions differ, use `xcodebuild` directly:

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

## Physical Device Setup

Testing on a physical iOS device requires USB port forwarding because, unlike simulators, physical devices do not share the host's loopback network. FlutterProbe uses `iproxy` (from the `libimobiledevice` suite) to forward the agent's WebSocket port over USB.

### Install iproxy

```bash
brew install libimobiledevice  # macOS
```

Verify the installation:

```bash
iproxy --help
```

### How it works

When targeting a physical device, the CLI runs `iproxy <host-port> <device-port> -u <UDID>` to create a TCP tunnel over USB. The CLI then connects to `ws://127.0.0.1:<host-port>/probe?token=...` just like it would for a simulator.

:::note
`iproxy` is **not** needed for iOS simulators (they share the host loopback network) or Android devices (which use `adb forward` for port forwarding).
:::

## Connection Flow (Simulator)

The iOS simulator shares the host's `localhost` network. No port forwarding is needed.

1. The CLI connects directly to `ws://127.0.0.1:<port>/probe?token=...`
2. Token is read via **fast path**: file at `~/Library/Developer/CoreSimulator/Devices/<UDID>/data/tmp/probe/token`
3. If the file is not available, the **fallback** streams simulator logs with `xcrun simctl spawn <UDID> log stream` and parses `PROBE_TOKEN=` lines
4. The ProbeAgent prints the token every 3 seconds to handle late-connecting CLI instances

## Permissions

iOS simulator permissions are managed via `xcrun simctl privacy`:

```
allow permission "notifications"   # simctl privacy grant <UDID> notifications <bundleID>
deny permission "camera"           # simctl privacy revoke <UDID> camera <bundleID>
grant all permissions
revoke all permissions
```

This works on iOS 14+ simulators.

:::caution[Notification permissions cannot be pre-granted]
Apple does not support granting notification permissions via `simctl privacy`. If your app requests notification permission (e.g. via `UNUserNotificationCenter` or Firebase Messaging), the native dialog will block the Flutter UI and prevent tests from proceeding.

**Solution:** Guard notification permission requests in your app's `main.dart`:

```dart
const probeEnabled = bool.fromEnvironment('PROBE_AGENT');
if (!probeEnabled) {
  await requestNotificationPermission();
  await FirebaseMessaging.instance.requestPermission();
}
```

Build with `--dart-define=PROBE_AGENT=true` to skip these requests during testing.
:::

## Biometric Authentication (Face ID / Touch ID)

`enroll biometric`, `biometric match`, and `biometric no match` drive Face ID and Touch ID flows on iOS Simulator via `xcrun simctl spawn booted notifyutil`.

```
before all tests
  enroll biometric

test "Face ID unlocks the app"
  open the app
  tap "Sign in with Face ID"
  wait until "Sign in with Face ID" appears
  tap "Sign in with Face ID"
  biometric match
  wait until "Dashboard" appears
  see "Dashboard"

test "failed Face ID shows error"
  open the app
  tap "Sign in with Face ID"
  wait until "Sign in with Face ID" appears
  tap "Sign in with Face ID"
  biometric no match
  wait until "Authentication failed" appears
  see "Authentication failed"
```

:::caution[iOS 26+ simulator — use `awaitBiometricResult()` in your app]
On iOS 26 / Xcode 26.5, the `faceCapture.no-match` notifyutil notification **no longer resolves `LAContext.evaluatePolicy`**. If your app calls `local_auth.authenticate()` directly, no-match tests will hang indefinitely.

**Fix:** In PROBE_AGENT builds, use `awaitBiometricResult()` from `flutter_probe_agent` instead. The CLI delivers the result via `probe.biometric_signal` after every `biometric match` / `biometric no match` step:

```dart
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

final ok = const bool.fromEnvironment('PROBE_AGENT')
    ? await awaitBiometricResult()
    : await LocalAuthentication().authenticate(localizedReason: 'Sign in');
```

This pattern works on all iOS versions and requires no changes to `.probe` test files.
:::

**Multiple simulators:** If more than one simulator is booted, the biometric notifyutil fires on `booted` (which selects an arbitrary device). Always boot only the target simulator, or pass `--device <UDID>` so the CLI targets the right one.

## Video Recording

iOS simulator video is recorded via `xcrun simctl io <UDID> recordVideo`. Videos use the `h264` codec (not HEVC) for browser compatibility in HTML reports.

```bash
probe test tests/ --video
```

## Configuration

```yaml
# probe.yaml
device:
  simulator_boot_timeout: 60s
  boot_poll_interval: 2s
  token_file_retries: 5
  restart_delay: 500ms
  ios_device_id: A1B2C3D4-E5F6-7890-ABCD-EF1234567890  # pin to a simulator UDID (skips auto-detection)

agent:
  port: 48686
  dial_timeout: 30s
  token_read_timeout: 30s
```

Set `ios_device_id` to avoid passing `--device <UDID>` on every run. The `--device` flag takes precedence when provided.

## Troubleshooting

### Token not found

If the CLI times out waiting for the auth token:
- Verify the app is running: `xcrun simctl list devices booted`
- Check the token file: `cat ~/Library/Developer/CoreSimulator/Devices/<UDID>/data/tmp/probe/token`
- Check logs manually: `xcrun simctl spawn <UDID> log stream --predicate 'eventMessage CONTAINS "PROBE_TOKEN="'`

### Build fails with SDK mismatch

When your Xcode SDK version differs from the target simulator iOS version, use `xcodebuild` with explicit `-destination` instead of `flutter build`.

### Notification permission dialog blocks tests

If a native notification permission dialog appears and blocks the Flutter UI, your app is requesting notification permissions without checking for `PROBE_AGENT`. See the [Permissions section](#permissions) above for the fix.

### Data clearing on iOS

`clear app data` deletes the container subdirectories (Documents, Library, tmp) rather than uninstalling the app. Path validation prevents accidental deletion of non-container paths.
