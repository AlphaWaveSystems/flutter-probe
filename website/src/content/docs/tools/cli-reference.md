---
title: CLI Reference
description: Complete reference for all probe CLI commands and flags.
---

## Commands

| Command | Description |
|---------|-------------|
| `probe init` | Scaffold `probe.yaml` and `tests/` directory |
| `probe test [path]` | Run `.probe` test files |
| `probe lint [path]` | Validate `.probe` files for syntax errors |
| `probe device list` | List connected devices/simulators |
| `probe device start` | Start an emulator/simulator |
| `probe record` | Record user interactions as ProbeScript |
| `probe report` | Generate HTML report from test results |
| `probe migrate` | Convert Maestro YAML flows to ProbeScript |
| `probe generate` | AI-assisted test generation |
| `probe studio` | Launch interactive test studio |

## probe test

Run `.probe` test files against a connected device.

### Usage

```bash
probe test [path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--device <serial>` | auto | Target device UDID or serial |
| `--host <ip>` | `127.0.0.1` | Agent host IP (use device IP for WiFi testing) |
| `--token <string>` | — | Agent auth token (skip auto-detection; for WiFi testing) |
| `--parallel` | `false` | Run tests in parallel across all connected devices |
| `--devices <serials>` | — | Comma-separated device serials for parallel execution |
| `--tag <tag>` | — | Run only tests matching this tag |
| `--timeout <duration>` | `30s` | Per-step timeout |
| `--format <fmt>` | `terminal` | Output format: `terminal`, `junit`, `json` |
| `-o, --output <path>` | — | Output file path for reports |
| `-v, --verbose` | `false` | Verbose output |
| `-y, --yes` | `false` | Auto-confirm destructive ops + auto-grant permissions |
| `--watch` | `false` | Watch mode — re-run on file changes |
| `--video` | `false` | Enable video recording per test |
| `--video-resolution` | `720x1280` | Android screenrecord resolution |
| `--video-framerate` | `2` | Video framerate (fps) |
| `--visual-threshold` | `0.5` | Max allowed pixel diff % for visual regression |
| `--visual-pixel-delta` | `8` | Pixel color delta tolerance |
| `--port <int>` | `48686` | Agent WebSocket port |
| `--dial-timeout` | `30s` | WebSocket connection timeout |
| `--token-timeout` | `30s` | Agent auth token wait timeout |
| `--reconnect-delay` | `2s` | Post-restart reconnect delay |
| `--app-path <path>` | — | Install app before testing |
| `--adb <path>` | `adb` | Override ADB binary path |
| `--flutter <path>` | `flutter` | Override Flutter binary path |
| `--config <path>` | `probe.yaml` | Config file path |
| `--shard <N/M>` | — | Run shard N of M (for parallel CI) |
| `--dry-run` | `false` | Parse and validate without executing |

### Examples

```bash
# Run all tests verbosely
probe test tests/ -v

# Run with custom timeout and auto-confirm
probe test tests/ --device emulator-5554 --timeout 60s -y

# Generate JUnit report for CI
probe test tests/ --format junit -o reports/results.xml

# Parallel CI sharding
probe test tests/ --shard 1/3
probe test tests/ --shard 2/3
probe test tests/ --shard 3/3

# WiFi testing on physical iOS device
probe test tests/ --host 192.168.1.100 --token abc123... --device <UDID>

# Parallel across multiple devices
probe test tests/ --parallel
probe test tests/ --devices emulator-5554,<iOS-UDID>
```

## probe lint

Validate `.probe` files without executing them.

```bash
probe lint tests/
probe lint tests/smoke/login.probe
```

## probe record

Record user interactions and generate ProbeScript.

```bash
probe record --device <UDID> --output tests/my_flow.probe
probe record --timeout 60s -o tests/flow.probe
```

## probe report

Generate an HTML report from JSON test results.

```bash
probe report --input reports/results.json -o reports/report.html
probe report --input reports/results.json -o reports/report.html --open
```

## probe device

Manage connected devices and emulators.

```bash
probe device list
probe device start --platform android
probe device start --platform ios
```

## probe-convert

Standalone tool for converting tests from other frameworks. See [probe-convert](/tools/probe-convert/) for full documentation.

```bash
probe-convert <file|dir> [flags]
probe-convert catalog [lang]
probe-convert formats
```
