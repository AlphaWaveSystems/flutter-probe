# iOS Integration Guide

## Setup

### 1. Add ProbeAgent to Your App

```yaml
# pubspec.yaml
dev_dependencies:
  probe_agent:
    path: ../FlutterProbe/probe_agent  # or from pub.dev
```

### 2. Initialize in main.dart

```dart
import 'package:probe_agent/probe_agent.dart';

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
