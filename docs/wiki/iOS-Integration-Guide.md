# iOS Integration Guide

## Setup

### 1. Add ProbeAgent to Your App

```yaml
# pubspec.yaml
dev_dependencies:
  flutter_probe_agent:
    path: ../FlutterProbe/probe_agent  # or from pub.dev
```

### 2. Initialize in main.dart

```dart
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  const probeEnabled = bool.fromEnvironment('PROBE_AGENT');
  if (probeEnabled) {
    await ProbeAgent.start();
  }

  // IMPORTANT: Guard notification permissions
  if (!probeEnabled) {
    await requestNotificationPermission();
    await FirebaseMessaging.instance.requestPermission();
  }

  runApp(const MyApp());
}
```

### 3. Build and Run

```bash
flutter run --debug --dart-define=PROBE_AGENT=true \
  -d <simulator-UDID>
```

### 4. Create probe.yaml

```yaml
project:
  app: com.example.myapp  # Your bundle ID

agent:
  port: 48686

defaults:
  platform: ios
```

## iOS-Specific Considerations

### Permission Handling

FlutterProbe pre-grants supported permissions via `xcrun simctl privacy`:

| Permission | simctl Service | Supported |
|---|---|---|
| Camera | `camera` | Yes |
| Microphone | `microphone` | Yes |
| Location | `location` | Yes |
| Photos | `photos` | Yes |
| Contacts | `contacts-limited` | Yes |
| Calendar | `calendar` | Yes |
| **Notifications** | - | **No** (Apple limitation) |

Since notifications cannot be pre-granted, your app MUST skip notification permission requests when `PROBE_AGENT=true`. See [Troubleshooting](Troubleshooting#notification-permission-dialog-blocks-tests).

### Token Reading

The ProbeAgent writes a token file to the app container:
```
<container>/tmp/probe/token
```

The CLI resolves this via:
```bash
xcrun simctl get_app_container <UDID> <bundleID> data
```

The token is a 32-character random string, printed to logs every 3 seconds as fallback.

### Data Clearing

`clear app data` on iOS:
1. Terminates the app
2. Deletes `Documents/`, `Library/`, `tmp/` from the app container
3. Resets the simulator Keychain (`xcrun simctl keychain <UDID> reset`)
4. Re-grants permissions
5. Relaunches the app

Note: Keychain reset affects ALL apps on the simulator, not just the target app.

### Video Recording

iOS simulator videos use `xcrun simctl io recordVideo` with `--codec=h264` for browser compatibility in HTML reports.

### Xcode Version Mismatch

If Xcode SDK version differs from the simulator's iOS version, `flutter run` may fail. Workaround:
```bash
xcodebuild -project Runner.xcodeproj \
  -scheme Runner \
  -destination 'platform=iOS Simulator,id=<UDID>' \
  build
```

With DART_DEFINES:
```bash
DART_DEFINES=$(echo 'PROBE_AGENT=true' | base64)
xcodebuild ... GCC_PREPROCESSOR_DEFINITIONS='$(inherited)' DART_DEFINES="$DART_DEFINES"
```

## Physical iOS Devices

Physical device testing requires additional tools and has some limitations compared to simulator testing.

### Prerequisites

```bash
# Install libimobiledevice for device communication
brew install libimobiledevice

# Verify device is detected
idevice_id -l
```

### Build and Run

Physical iOS devices require **debug** or **profile** mode (release mode blocks ProbeAgent by default):

```bash
# Debug mode (USB-connected)
flutter run --debug --dart-define=PROBE_AGENT=true \
  --device-id <UDID>

# Profile mode (better performance, no debug overhead)
flutter run --profile --dart-define=PROBE_AGENT=true \
  --device-id <UDID>
```

### Connection Modes

> **WiFi is the recommended connection mode for physical iOS devices.** USB-C connections
> are unstable — the cable switches between charging and data transfer modes, causing
> intermittent connection drops that interrupt test execution. WiFi provides a stable,
> drop-free connection throughout the entire test run.

**WiFi mode** (recommended):
```bash
# Build with WiFi enabled
flutter build ios --profile --flavor <flavor> \
  --dart-define=PROBE_AGENT=true \
  --dart-define=PROBE_WIFI=true

# Run tests over WiFi (no USB cable needed)
probe test tests/ --host <device-ip> --token <probe-token> --device <UDID>
```

To find the token, check the app's console output for `PROBE_TOKEN=...` (printed every 3 seconds).

**`restart the app` over WiFi**: The CLI automatically pre-shares a token with the agent before restarting. After restart, the agent uses the pre-shared token instead of generating a new one — no manual token re-reading needed.

**USB mode** (fallback — may experience intermittent drops with USB-C):
1. FlutterProbe starts `iproxy` to forward port 48686 from host to device
2. CLI reads the agent token from `idevicesyslog`
3. Uses HTTP POST transport with auto-reconnect (up to 2 retries per step)

> **Note**: USB-C cables cause connection drops when the device switches between
> charging and data transfer modes. If you experience frequent reconnects, switch
> to WiFi mode. USB-A cables do not have this issue.

### How It Works

1. **USB**: `iproxy` forwards port 48686; CLI reads token via `idevicesyslog`; HTTP POST transport
2. **WiFi**: Agent binds to `0.0.0.0` (via `PROBE_WIFI=true`); CLI connects directly to device IP; no iproxy needed
3. **App lifecycle**: `xcrun devicectl` handles launch/terminate (instead of `simctl`)
4. **Keepalive**: WebSocket ping/pong frames every 5s prevent idle connection drops (USB mode)
5. **Auto-reconnect**: If the connection drops mid-test, the CLI reconnects transparently (up to 2 retries)

### Limitations on Physical Devices

| Operation | Physical iOS | Notes |
|---|---|---|
| `tap`, `type`, `see`, etc. | Works | Via agent RPC |
| `restart the app` | Works | Via `xcrun devicectl` |
| `take screenshot` | Works | Via agent RPC |
| `clear app data` | Skipped | No filesystem access; uninstall/reinstall as workaround |
| `allow/deny permission` | Skipped | Requires MDM or manual action |
| `set location` | Skipped | No CLI tool for GPS mocking on real hardware |

Skipped operations print a warning and continue — they do not fail the test.

### Troubleshooting Physical Devices

**"iproxy not found"**: Install libimobiledevice: `brew install libimobiledevice`

**"token not found within 30s"**: Ensure the app is running with `--dart-define=PROBE_AGENT=true`. Check `idevicesyslog -u <UDID> | grep PROBE_TOKEN` to verify the agent is printing tokens.

**Stale iproxy processes**: FlutterProbe automatically kills stale iproxy processes matching your device UDID. If you still have port conflicts, run: `pkill -f "iproxy.*<UDID>"`

**White/grey screen on launch**: Profile builds may fail if Firebase or other native SDKs aren't configured for the device. Try debug mode instead.

## Common probe.yaml for iOS

```yaml
project:
  app: com.example.myapp

agent:
  port: 48686
  dial_timeout: 30s
  token_read_timeout: 30s
  reconnect_delay: 2s

device:
  simulator_boot_timeout: 60s

defaults:
  platform: ios
  timeout: 30s
  grant_permissions_on_clear: true

video:
  resolution: 720x1280
  framerate: 2
```
