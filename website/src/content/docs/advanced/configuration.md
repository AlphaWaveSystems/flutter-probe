---
title: Configuration Reference
description: Complete probe.yaml reference with all sections and settings.
---

FlutterProbe is configured via `probe.yaml` in your project root. All settings follow the resolution order: **CLI flag > probe.yaml > built-in default**.

## Full Example

```yaml
project:
  name: "My App"
  app: com.example.myapp

defaults:
  platform: ios
  timeout: 30s
  screenshots: on_failure
  video: false
  retry_failed_tests: 1
  grant_permissions_on_clear: true

devices:
  - name: iPhone 16 Pro
    serial: A1B2C3D4-E5F6-7890-ABCD-EF1234567890
  - name: Pixel 7
    serial: emulator-5554

agent:
  port: 48686
  device_port: 48686
  dial_timeout: 30s
  ping_interval: 5s
  token_read_timeout: 30s
  reconnect_delay: 2s

device:
  emulator_boot_timeout: 120s
  simulator_boot_timeout: 60s
  boot_poll_interval: 2s
  token_file_retries: 5
  restart_delay: 500ms

video:
  resolution: 720x1280
  framerate: 2
  screenrecord_cycle: 170s

visual:
  threshold: 0.5
  pixel_delta: 8

tools:
  adb: /usr/local/bin/adb
  flutter: /usr/local/bin/flutter

recipes_folder: tests/recipes
reports_folder: reports

environment:
  TEST_USER: "admin@test.com"
  API_BASE: "http://localhost:8080"
```

## Sections

### project

| Key | Type | Description |
|-----|------|-------------|
| `name` | string | Project display name |
| `app` | string | Bundle ID (iOS) or package name (Android). Validated against `^[a-zA-Z][a-zA-Z0-9_.]*$` |

### defaults

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `platform` | string | — | `android`, `ios`, or `both` |
| `timeout` | duration | `30s` | Per-step timeout |
| `screenshots` | string | `on_failure` | `always`, `on_failure`, or `never` |
| `video` | bool | `false` | Enable video recording |
| `retry_failed_tests` | int | `0` | Number of retries for failed tests |
| `grant_permissions_on_clear` | bool | `false` | Auto-grant permissions after `clear app data` |

### devices

List of target devices:

| Key | Type | Description |
|-----|------|-------------|
| `name` | string | Device display name |
| `serial` | string | UDID (iOS) or serial (Android). Use `auto` for auto-detection |

### agent

WebSocket connection settings:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `port` | int | `48686` | Host-side WebSocket port |
| `device_port` | int | same as `port` | On-device port (allows different host/device ports for parallel testing) |
| `dial_timeout` | duration | `30s` | WebSocket connection timeout |
| `ping_interval` | duration | `5s` | WebSocket keepalive interval |
| `token_read_timeout` | duration | `30s` | Max time to wait for auth token |
| `reconnect_delay` | duration | `2s` | Delay before reconnecting after app restart |

### device

Device/emulator management:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `emulator_boot_timeout` | duration | `120s` | Android emulator boot timeout |
| `simulator_boot_timeout` | duration | `60s` | iOS simulator boot timeout |
| `boot_poll_interval` | duration | `2s` | Polling interval during boot |
| `token_file_retries` | int | `5` | Retries for reading token file |
| `restart_delay` | duration | `500ms` | Delay after force-stop before relaunch |

### video

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `resolution` | string | `720x1280` | Android screenrecord resolution |
| `framerate` | int | `2` | Frames per second |
| `screenrecord_cycle` | duration | `170s` | Max segment length (chains to avoid 180s limit) |

### visual

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `threshold` | float | `0.5` | Max allowed pixel diff percentage |
| `pixel_delta` | int | `8` | Per-pixel color delta tolerance |

### tools

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `adb` | string | `adb` (PATH) | Path to ADB binary |
| `flutter` | string | `flutter` (PATH) | Path to Flutter binary |

### environment

Key-value pairs available as environment variables during test execution:

```yaml
environment:
  TEST_USER: "admin@test.com"
  API_BASE: "http://localhost:8080"
```

## Platform-Specific Configs

Use separate config files for parallel platform testing:

```bash
probe test tests/ --config probe.ios.yaml --device <IOS_UDID> &
probe test tests/ --config probe.android.yaml --device emulator-5554 &
wait
```
