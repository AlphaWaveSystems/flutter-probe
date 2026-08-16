---
title: Android
description: Android emulator setup, ADB integration, and platform-specific configuration.
---

## Requirements

- Android SDK with platform tools (ADB)
- An Android emulator or connected device
- Flutter app built with `--dart-define=PROBE_AGENT=true`

## Emulator Setup

### List available devices

```bash
probe device list
```

### Start an emulator

```bash
probe device start --platform android
```

Or start manually:

```bash
emulator -avd Pixel_7_API_34 -no-snapshot-load
```

### Build and install your app

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true
adb install -r build/app/outputs/flutter-apk/app-debug.apk
adb shell am start -n com.example.myapp/.MainActivity
```

## Connection Flow

1. The CLI runs `adb forward tcp:<host-port> tcp:<device-port>` to bridge the emulator network
2. The CLI connects via WebSocket to `ws://127.0.0.1:<host-port>/probe?token=...`
3. The auth token is extracted from `adb logcat` output matching `PROBE_TOKEN=`
4. Once authenticated, the CLI dispatches JSON-RPC commands to the Dart agent

By default, both host and device ports are `48686`. For parallel testing, set different host ports per device.

## Port Forwarding

```yaml
# probe.yaml
agent:
  port: 48686         # host-side port
  device_port: 48686  # on-device port (what ProbeAgent listens on)
```

For parallel testing with multiple devices:

```yaml
# probe.android-1.yaml
agent:
  port: 48687
  device_port: 48686
```

This maps `adb forward tcp:48687 tcp:48686`, keeping the device port the same while using a unique host port.

## Custom ADB Path

If ADB is not on your PATH:

```bash
probe test tests/ --adb /path/to/platform-tools/adb
```

Or in `probe.yaml`:

```yaml
tools:
  adb: /path/to/platform-tools/adb
```

## Permissions

Android runtime permissions are granted via `adb shell pm grant`:

```
allow permission "camera"          # adb shell pm grant <pkg> android.permission.CAMERA
deny permission "location"         # adb shell pm revoke <pkg> android.permission.ACCESS_FINE_LOCATION
grant all permissions              # grants all known runtime permissions
```

Available permissions: `notifications`, `camera`, `location`, `microphone`, `storage`, `contacts`, `phone`, `calendar`, `sms`, `bluetooth`.

## Native UI Automation

Android is currently the only platform where probe can reach **outside the Flutter widget tree**
into native, OS-owned UI — pickers, share sheets, and any other surface the Dart agent can never
see. Elements are matched against uiautomator's text or resource-id and driven via
`uiautomator dump` + `input tap`/`input text` — no new dependencies, `adb` already ships both:

```
tap native "Choose from Gallery"
see native "IMG_0001.jpg"
don't see native "Error"
type native "wifi" into "Search settings"
```

Notes from real-device verification:

- Matching is a case-insensitive substring against both `text` and `resource-id`.
- If the native element is reached via a screen transition, add a `wait` step first — the same
  idiom used after Flutter navigation.
- `take a screenshot` (the Dart-agent verb) cannot capture native UI — it renders only Flutter's
  own tree. Use `adb exec-out screencap` externally to visually verify native state.
- API 35 pitfall for your own native screens: forced edge-to-edge can place a top-of-screen
  element's reported bounds center underneath the app bar, which silently eats the tap — handle
  window insets (`fitsSystemWindows` or equivalent).

A purpose-built native fixture app with stable ids lives in the repo under `native-test-apps/`.

## Device Media

```
add media "fixtures/photo.jpg"     # adb push + MEDIA_SCANNER_SCAN_FILE broadcast
```

The file lands in `/sdcard/Pictures/` and is MediaStore-indexed, so it is genuinely visible to
image pickers — combine with the native UI verbs above to actually select it.

## Deep Links

```
open link "myapp://profile/42" in the app
```

Dispatches `am start -a android.intent.action.VIEW`, so a custom scheme or App Links URL
registered by your app is delivered to the app itself — including cold-launching it from fully
terminated (unlike iOS Simulator; see the iOS page). Plain `open link "https://..."` (no suffix)
still opens the external browser via `url_launcher`.

## Location

```
set location 48.1351, 11.5820      # adb emu geo fix (emulators only)
```

Simulate movement through an ordered route with `travel to` — interpolates between waypoints and
calls the same `adb emu geo fix` primitive repeatedly at ~1-second intervals, rather than jumping
instantly:

```
travel to
  48.1351, 11.5820
  48.1451, 11.5920
over 10 seconds
```

## Video Recording

Android uses the built-in `screenrecord` command. Videos are recorded as MP4 (H.264). The CLI auto-chains recordings to work around the 180-second limit.

```bash
probe test tests/ --video --video-resolution 720x1280 --video-framerate 2
```

If `scrcpy` is installed, it is used as the preferred backend for higher quality recordings.

If `ffmpeg` is installed, multi-segment recordings are stitched into a single file. Without it, segments are kept as separate files.

## Configuration

```yaml
# probe.yaml
device:
  emulator_boot_timeout: 120s
  boot_poll_interval: 2s
  token_file_retries: 5
  restart_delay: 500ms

agent:
  port: 48686
  dial_timeout: 30s
  ping_interval: 5s
  token_read_timeout: 30s
  reconnect_delay: 2s
```
