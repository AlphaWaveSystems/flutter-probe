---
title: iOS
description: iOS simulator setup, token reading, simctl integration, and platform-specific notes.
---

## Requirements

- macOS with Xcode installed
- `xcrun simctl` command-line tools
- Flutter app built for simulator with `--dart-define=PROBE_AGENT=true`

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

## Connection Flow

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

agent:
  port: 48686
  dial_timeout: 30s
  token_read_timeout: 30s
```

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
