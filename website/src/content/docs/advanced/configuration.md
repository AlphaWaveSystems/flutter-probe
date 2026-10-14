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

probe_version: 0.25.x          # pin the FlutterProbe CLI release; probe test stops before any test on another one

defaults:
  platform: ios
  timeout: 30s
  screenshots: on_failure
  video: false
  retry_failed_tests: 1
  grant_permissions_on_clear: true
  match: exact                 # text matching: exact (default) or loose (fold case, accents, apostrophes, whitespace); --match-loose overrides
  implicit_wait: 0s            # retry tap/type/see on "not found" for up to this long (Maestro-style); 0 = off. --implicit-wait overrides

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
  reconnect_attempts: 4
  reconnect_backoff: 1s
  launch_timeout: 120s
  # driver_port: 49100     # iOS system-dialog driver port (default: derived from the simulator UDID)

device:
  emulator_boot_timeout: 120s
  simulator_boot_timeout: 60s
  boot_poll_interval: 2s
  token_file_retries: 5
  restart_delay: 500ms
  ios_device_id: A1B2C3D4-E5F6-7890-ABCD-EF1234567890  # optional: pin to a simulator UDID
  android_device_id: emulator-5554                      # optional: pin to an emulator serial

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

# AI-powered assertions (see ... with ai, assert no visual defects with ai,
# read ... with ai into <var>) — optional, no default provider. Nothing is
# sent anywhere unless this block is configured AND a test actually uses
# `with ai`.
ai:
  provider: anthropic       # openai | anthropic | local — no default, must be set explicitly
  api_key: ${ANTHROPIC_API_KEY}
  model: claude-sonnet-4-20250514
  timeout: 60s
  redact:
    - selector: "#credit_card_field"

recipes_folder: tests/recipes
reports_folder: reports

l10n:
  dir: lib/l10n             # folder with your .arb files, for l10n "key" in tests
  default: en               # fallback language / language before any set language

environment:
  TEST_USER: "admin@test.com"
  API_BASE: "http://localhost:8080"
```

## Sections

### probe_version

A top-level key, not inside a section. It pins the FlutterProbe CLI release the project is tested with. Leave it out and nothing changes.

| Value | Meaning |
|-------|---------|
| `0.25.1` | exactly this release |
| `0.25.x` | any patch release of 0.25 |

A leading `v` is accepted (`v0.25.1`). A pre-release or build version such as `0.25.1-rc.1` cannot be pinned. Any other value fails when probe.yaml loads, and the error names `probe_version`.

`probe test` checks the running CLI before anything else. If the CLI does not meet the pin, the run stops before a device is touched or a test starts. The message names the installed CLI, the required version and how to install the right one: the release page, or `brew install probe`, which installs the latest stable release. Nothing is downloaded and nothing is upgraded automatically.

A pre-release or build suffix on the installed CLI does not count against the pin: `0.25.1-fp3` meets `0.25.1`. A development build (`probe version` prints `dev`) meets no pin.

The agent is not part of the pin. `probe test` prints `probe CLI X / agent A` on stderr once the first agent answers the handshake. The connect-time handshake still compares the agent with the CLI: it warns on a different minor or patch version and refuses a different major version.

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
| `retry_failed_tests` | int | `0` | Re-run a failing test up to this many extra times (0 = off); also `--retry-failed N` |
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
| `reconnect_attempts` | int | `4` | Max auto-reconnect attempts after a connection drop mid-test |
| `reconnect_backoff` | duration | `1s` | Base for exponential reconnect backoff: `delay = base << (attempt-1)` capped at 8s, ±20% jitter |
| `launch_timeout` | duration | `120s` | Max time for `restart the app`/`clear app data` to force-stop, relaunch, and reconnect — distinct from `dial_timeout` since a real app's cold-launch path (e.g. Firebase App Check init) can cost far more than a warm reconnect. Raise this if `restart the app` times out on a slow-starting app. Also settable per-run with `--launch-timeout` |
| `driver_port` | int | derived | Loopback port of the iOS system-dialog driver (1024-65535). Derived per simulator from its UDID (48790-48989) when unset; pin it when a port range is reserved per test lane. Also settable with `--driver-port`, which wins. Single-device runs only |

### device

Device/emulator management:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `emulator_boot_timeout` | duration | `120s` | Android emulator boot timeout |
| `simulator_boot_timeout` | duration | `60s` | iOS simulator boot timeout |
| `boot_poll_interval` | duration | `2s` | Polling interval during boot |
| `token_file_retries` | int | `5` | Retries for reading token file |
| `restart_delay` | duration | `500ms` | Delay after force-stop before relaunch |
| `ios_device_id` | string | — | Preferred iOS simulator UDID or physical device UDID. Overridden by `--device` flag |
| `android_device_id` | string | — | Preferred Android emulator serial or device serial. Overridden by `--device` flag |

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

### ai

Settings for the [AI-powered assertion commands](/probescript/dictionary/#ai-powered-assertions)
(`see ... with ai`, `assert no visual defects with ai`, `read ... with ai into <var>`). Not
required unless a test uses `with ai` — omitting this block entirely is the default, and no
screenshot ever leaves the device/CI runner without it. There is no default provider by design:
every "with ai" call goes directly from the CLI to whichever provider you configure, never through
a FlutterProbe-operated relay (see `docs/research/maestro-ai-assertions-investigation.md` for why
this is a deliberate departure from Maestro's mandatory-cloud-upload model).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `provider` | string | — | `openai`, `anthropic`, or `local`. No default — must be set explicitly, and a test using `with ai` fails fast (before any device connects) if this is unset |
| `api_key` | string | — | Provider API key. Supports `${ENV_VAR}` syntax. Not required for `provider: local` |
| `model` | string | `claude-sonnet-4-20250514` | Model name. Required, no default, for `provider: local` |
| `endpoint` | string | — | OpenAI-compatible base URL (e.g. `http://localhost:11434/v1`) for `provider: local`. Supports `${ENV_VAR}`. Required when `provider: local` — a test using `with ai` fails fast if it's missing |
| `timeout` | duration | `60s` | Per-request HTTP timeout for AI provider calls. Raise this for slow local models — the step timeout alone doesn't cover the provider HTTP call |
| `vision` | bool | `true` | Set `false` for a text-only model (most small local models have no image support). `see "..." with ai` is then judged from the screen's visible texts and keys instead of a screenshot; `assert no visual defects with ai` and `read ... with ai` need pixels and fail with a clear message. Refused when `redact` rules exist, since those can't be applied to text. See [AI & local models](/tools/ai/) |
| `redact` | list | — | Widgets to black out in the screenshot before it's sent to any provider (local or cloud). Each entry: `selector:` — a ProbeScript selector (`"#credit_card_field"` or `"Card Number"`) |

```yaml
ai:
  provider: local
  endpoint: http://localhost:11434/v1   # e.g. Ollama
  redact:
    - selector: "#credit_card_field"
    - selector: "Email"
```

### hosted

Opt-in report upload to the hosted service (Probe Cloud, preview). Unrelated to the device-farm `cloud:` block. See [Probe Cloud (preview)](/tools/probe-cloud-preview/).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `upload_report` | bool | `false` | Upload the scrubbed report after `probe test` (same as `--upload-report`); needs a license with the `probe_cloud` plan |
| `url` | string | placeholder, not live | Service base URL; https required (http only for localhost) |
| `label` | string | empty | Label for this upload (suite, shard or matrix leg); needed when one CI job uploads more than once |
| `timeout` | duration | `5s` | Upload time limit; an upload never fails the run |

### perf

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `measure_all` | bool | `false` | Wrap every test in a measuring window named after the test (same as `--measure-all`) |
| `frame_budget_ms` | number | `16.7` | Build+raster time above which a frame counts as slow (`8.3` for a 120 Hz display, a looser value for debug builds). Needs agent 0.25+; an older agent keeps 16.7 and the run says so. Windows measured with different budgets are not compared on `slow_frames` |

### l10n

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `dir` | string | — | Folder with the app's `.arb` files (`app_en.arb`, `app_de.arb`, ...). Enables `l10n "key"` in tests |
| `default` | string | — | Language used before any `set language` / `--locale`, and the fallback for keys missing from a translation |

See [Testing localized apps](/advanced/multi-language/#your-arb-files-l10n-key).

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
