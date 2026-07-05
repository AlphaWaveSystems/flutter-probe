---
title: App Lifecycle
description: Control app state with kill, open, restart, clear data, and permission management commands.
---

FlutterProbe provides commands to manage the app's lifecycle during tests.

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

If the app is already running and connected, this is a no-op (nothing to do). If the connection is down — most commonly right after `kill the app` — it launches the app and reconnects:
- Android: `adb shell am start -n <package>/<launcher-activity>`
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

## Native UI Boundary

FlutterProbe's `tap`, `type`, `see`, and every other selector-based verb operate on the **Flutter widget tree**, driven by an in-process Dart agent. Anything that isn't a Flutter widget — because it's rendered by the OS itself — is invisible to those verbs, by design, not as a bug to be worked around per-test:

- **Image/file/media pickers** (`PHPickerViewController` on iOS, the Android photo/document picker)
- **Share sheets** (`UIActivityViewController`, Android's share intent chooser)
- **System dialogs that can't be bypassed by an OS-level grant** — most permission prompts *are* handled without ever appearing (see above), but a few (e.g. iOS's notification permission prompt) require the native `UNUserNotificationCenter` dialog to actually be tapped, with no `simctl privacy`-style bypass available

`take screenshot` and video recording **do** capture this content (they record the full physical screen, not just the Flutter view) — so you can still visually confirm a picker or share sheet appeared and looks correct. There is currently no selector or verb that can tap a specific element inside one.

**If your test flow needs to cross into native UI** (uploading a photo, sharing via the system sheet), the current options are:
- Design the test to stop just short of the native surface (e.g. assert the picker/share sheet opened via a screenshot, without completing the flow through it)
- Have the app under test support a test-only bypass for that specific flow (e.g. a debug-mode "skip picker, use a fixture image" path), the same pattern already used for the iOS notification permission prompt

A design proposal for closing part of this gap (an Android-first `tap native`/`see native` verb pair via `uiautomator`, with iOS deferred to its own future proposal) is written up in [`docs/proposals/pt13-native-ui-bridging.md`](https://github.com/AlphaWaveSystems/flutter-probe/blob/main/docs/proposals/pt13-native-ui-bridging.md) — not yet implemented.

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
