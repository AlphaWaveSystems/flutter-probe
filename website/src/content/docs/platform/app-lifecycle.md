---
title: App Lifecycle
description: Control app state with kill, open, restart, clear data, and permission management commands.
---

FlutterProbe provides commands to manage the app's lifecycle during tests. These run at the platform level (ADB or simctl), not through the Dart agent.

## Kill the App

```
kill the app
```

Force-stops the app **without relaunching**:
- Android: `adb shell am force-stop <package>`
- iOS: `xcrun simctl terminate <udid> <bundle-id>`

The WebSocket connection is closed. Use `open the app` to relaunch and reconnect.

## Open the App

```
open the app
```

Launches the app from the CLI side and reconnects the WebSocket:
- Android: `adb shell monkey -p <package> -c android.intent.category.LAUNCHER 1`
- iOS: `xcrun simctl launch <udid> <bundle-id>`

Use this after `kill the app` to restart from a cold state, or at the beginning of a test to ensure the app is running.

## Restart the App

```
restart the app
```

Combines kill + open in one step:
1. Force-stops the app
2. Relaunches the app
3. Reconnects the WebSocket connection automatically

App data is **preserved** — SharedPreferences, databases, and files remain intact.

## Clear App Data

```
clear app data
```

This command:
1. Wipes all app data (SharedPreferences, databases, files)
   - Android: `adb shell pm clear <package>`
   - iOS: Deletes container subdirectories (Documents, Library, tmp)
2. Relaunches the app
3. Reconnects the WebSocket connection

This is a **destructive operation** and requires either:
- The `-y` CLI flag: `probe test tests/ -y`
- Interactive confirmation at runtime
- `grant_permissions_on_clear: true` in `probe.yaml`

When `-y` is used, all permissions are automatically re-granted after clearing to prevent permission dialogs from blocking tests.

## Permission Management

OS-level permission dialogs (notifications, camera, location) are outside the Flutter widget tree, so the Dart agent cannot interact with them. FlutterProbe handles these at the platform level.

### Granting permissions

```
allow permission "notifications"
allow permission "camera"
grant all permissions
```

### Revoking permissions

```
deny permission "camera"
revoke all permissions
```

### Available permissions

| Permission | Android | iOS |
|------------|---------|-----|
| `notifications` | `POST_NOTIFICATIONS` | `notifications` |
| `camera` | `CAMERA` | `camera` |
| `location` | `ACCESS_FINE_LOCATION` | `location` |
| `microphone` | `RECORD_AUDIO` | `microphone` |
| `storage` | `READ_EXTERNAL_STORAGE` | `photos` |
| `contacts` | `READ_CONTACTS` | `contacts` |
| `phone` | `CALL_PHONE` | — |
| `calendar` | `READ_CALENDAR` | `calendar` |
| `sms` | `READ_SMS` | — |
| `bluetooth` | `BLUETOOTH_CONNECT` | — |

### Auto-grant on clear

When `grant_permissions_on_clear: true` is set in `probe.yaml` (or `-y` is used), all known permissions are automatically granted after `clear app data`. This prevents permission dialogs from appearing and blocking tests.

## Configuration

```yaml
# probe.yaml
device:
  restart_delay: 500ms       # delay after force-stop before relaunch

agent:
  reconnect_delay: 2s        # delay before reconnecting WebSocket after restart

defaults:
  grant_permissions_on_clear: true
```

## Typical Usage Pattern

```
test "fresh install experience"
  clear app data
  open the app
  see "Welcome"
  see "Create Account"

test "returning user"
  restart the app
  see "Welcome Back"
  see "Sign In"
```
