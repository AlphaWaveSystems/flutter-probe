# Architecture Overview

FlutterProbe consists of two main components that communicate via WebSocket + JSON-RPC 2.0.

## System Diagram

```
+------------------+       WebSocket (JSON-RPC 2.0)       +------------------+
|   Go CLI         | <----------------------------------> |   Dart Agent     |
|   (probe)        |       ws://127.0.0.1:48686           |   (ProbeAgent)   |
+------------------+                                       +------------------+
|                  |                                       |                  |
| - Parser         |                                       | - Server         |
| - Runner         |                                       | - Executor       |
| - ProbeLink      |                                       | - Finder         |
| - Device mgmt    |                                       | - Sync           |
| - Reporter       |                                       | - Gestures       |
| - Config         |                                       | - Recorder       |
+------------------+                                       +------------------+
        |                                                          |
        v                                                          v
  .probe files                                             Flutter Widget Tree
  probe.yaml                                               (live app)
```

## Go CLI (`cmd/probe/`, `internal/`)

The CLI is the orchestrator. It parses `.probe` test files, manages device connections, executes tests, and generates reports.

### Key Packages

| Package | Responsibility |
|---|---|
| `cli/` | Cobra command definitions (test, init, lint, device, record, report, etc.) |
| `parser/` | Indent-aware lexer + recursive-descent parser producing AST |
| `runner/` | Test orchestration: loads files, walks AST, dispatches to ProbeLink |
| `probelink/` | JSON-RPC 2.0 WebSocket client |
| `device/` | ADB integration for Android (port forwarding, permissions, token extraction) |
| `ios/` | iOS simulator management via `xcrun simctl` |
| `config/` | `probe.yaml` parsing with layered resolution (CLI flag > config > default) |
| `report/` | HTML report generation with portable relative paths |
| `cloud/` | Cloud provider integration (BrowserStack, SauceLabs, AWS, Firebase, LambdaTest) |
| `ai/` | Self-healing selectors and LLM-based test generation |
| `visual/` | Screenshot-based visual regression testing |
| `plugin/` | YAML-based custom command system |

## Dart Agent (`probe_agent/`)

The agent runs **inside the production Flutter app** using `WidgetsFlutterBinding` (NOT `TestWidgetsFlutterBinding`). This is a critical design decision — see [Flutter Test Incompatibility](#flutter-test-incompatibility).

### Key Files

| File | Responsibility |
|---|---|
| `server.dart` | WebSocket server with token auth |
| `executor.dart` | Command dispatcher (tap, type, see, swipe, screenshot, etc.) |
| `finder.dart` | Widget selector engine walking the live widget tree |
| `sync.dart` | Triple-signal synchronization for flake prevention |
| `gestures.dart` | Touch event handlers |
| `recorder.dart` | Recording engine for `probe record` |

## Connection Flow

### Android

1. CLI runs `adb forward tcp:<host-port> tcp:<device-port>`
2. CLI reads token from `adb logcat` (matching `PROBE_TOKEN=...`)
3. CLI connects: `ws://127.0.0.1:<host-port>/probe?token=<token>`
4. JSON-RPC commands are dispatched over the WebSocket

### iOS Simulator

1. Simulator shares host loopback — no port forwarding needed
2. CLI reads token from app container: `<container>/tmp/probe/token`
3. CLI connects: `ws://127.0.0.1:<port>/probe?token=<token>`
4. Fallback: parse token from `simctl spawn ... log show`

### Cloud (Relay Mode)

1. CLI creates relay session (gets URL + token)
2. App is built with `--dart-define=PROBE_RELAY_URL=<url> --dart-define=PROBE_RELAY_TOKEN=<token>`
3. Both agent and CLI connect outbound to the relay
4. `ws://` auto-upgraded to `wss://` for non-localhost hosts

## Flutter Test Incompatibility

The ProbeAgent does NOT use `flutter_test`. It runs in a production app with `WidgetsFlutterBinding`. This means:

- **No `find.text()`** — crashes with null check errors (requires `TestWidgetsFlutterBinding`)
- **No `TestGesture`** — test-framework only class
- **No `tester.enterText()`** — platform channels don't work outside test framework

Instead:
- `finder.dart` walks `WidgetsBinding.instance.rootElement` manually
- `gestures.dart` uses `GestureBinding.handlePointerEvent()` directly
- `executor.dart` sets `TextEditingController.text` directly for text input

## Configuration Resolution

All settings follow: **CLI flag > probe.yaml > built-in default**

Key config sections: `project`, `defaults`, `agent`, `device`, `video`, `visual`, `tools`, `devices`, `environment`
