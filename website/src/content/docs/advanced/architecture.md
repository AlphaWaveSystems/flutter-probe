---
title: Architecture
description: Deep dive into FlutterProbe's Go CLI + Dart Agent architecture and JSON-RPC protocol.
---

FlutterProbe consists of two main components that communicate via WebSocket + JSON-RPC 2.0.

## System Overview

```
┌──────────────┐     WebSocket / JSON-RPC 2.0      ┌─────────────────┐
│  probe CLI   │ ──────────────────────────────────▶│  ProbeAgent     │
│  (your Mac)  │   localhost:48686                   │  (in Flutter app│
│              │   • tap, type, see, wait, swipe    │   on device)    │
│  parses .probe│   • screenshot, dump_tree         │                 │
│  manages devices│  • one-time token auth          │  walks widget   │
│  generates reports│                               │  tree directly  │
└──────────────┘                                    └─────────────────┘
```

## Go CLI (`cmd/probe/`, `internal/`)

The CLI is written in Go and handles everything outside the device.

### Key Packages

| Package | Responsibility |
|---------|---------------|
| `cli/` | Cobra command definitions (`probe test`, `probe init`, `probe lint`, etc.) |
| `parser/` | Indent-aware lexer + recursive-descent parser producing an AST |
| `runner/` | Test orchestration: loads files, walks AST, dispatches to ProbeLink |
| `probelink/` | JSON-RPC 2.0 WebSocket client |
| `device/` | ADB integration, port forwarding, token extraction, permissions |
| `ios/` | iOS simulator management via `xcrun simctl` |
| `config/` | `probe.yaml` parsing and validation |
| `ai/` | Self-healing selectors via fuzzy matching |
| `report/` | HTML report generation |
| `visual/` | Screenshot-based visual regression |
| `plugin/` | YAML-based custom command system |
| `migrate/` | Maestro YAML to ProbeScript converter |
| `studio/` | Interactive test studio (HTTP server) |

### Parser Architecture

The parser uses an indent-aware lexer (similar to Python's) combined with a recursive-descent parser:

- **Lexer**: Tracks indentation levels, emits INDENT/DEDENT tokens
- **Parser**: Produces an AST with types: `Program` containing `UseStmt`, `RecipeDef`, `HookDef`, `TestDef`, each containing `Step` nodes
- **Steps**: Each step represents a single ProbeScript command (tap, type, see, etc.)

### Test Execution Flow

1. `runner.go` loads all `.probe` files, resolves recipes and imports
2. `executor.go` walks the AST for each test, executing steps sequentially
3. Each step dispatches a JSON-RPC call via `probelink/`
4. `reporter.go` collects results and outputs in the requested format
5. `device_context.go` handles platform-level operations (restart, clear data, permissions, reconnection)

## Dart ProbeAgent (`probe_agent/`)

The agent runs inside the Flutter app as a WebSocket server. It does **not** use `flutter_test` — it runs in a production `WidgetsFlutterBinding`.

### Key Files

| File | Responsibility |
|------|---------------|
| `server.dart` | WebSocket server with token auth on port 48686 |
| `executor.dart` | Command dispatcher (tap, type, see, swipe, screenshot, etc.) |
| `finder.dart` | Widget selector engine — walks the tree via `WidgetsBinding.instance.rootElement` |
| `sync.dart` | Triple-signal synchronization (frames + animations + microtasks) |
| `gestures.dart` | Gesture handlers using `GestureBinding.handlePointerEvent()` |
| `recorder.dart` | Recording engine for `probe record` |
| `protocol.dart` | JSON-RPC types and method constants |

### Why Not flutter_test?

The agent runs in a **production app**, not a test harness. This means:

- No `find.text()` or `find.byKey()` — these require `TestWidgetsFlutterBinding` and crash at runtime
- No `TestGesture` — uses `GestureBinding.handlePointerEvent()` with raw pointer events instead
- No `tester.enterText()` — finds `TextEditingController` in the widget tree and sets `.text` directly

This design enables testing against real app behavior with real rendering, real animations, and real network calls.

## JSON-RPC Protocol

All 22 method names are defined as constants in both Go (`internal/probelink/protocol.go`) and Dart (`ProbeMethods` class). No string literals in dispatchers.

### Example Methods

| Method | Description |
|--------|-------------|
| `probe.tap` | Tap a widget by selector |
| `probe.type` | Enter text into a field |
| `probe.see` | Assert a widget is visible |
| `probe.wait` | Wait for a condition |
| `probe.swipe` | Perform a swipe gesture |
| `probe.screenshot` | Capture a screenshot |
| `probe.dump_tree` | Dump the widget tree |
| `probe.start_recording` | Begin recording interactions |

### Authentication

- The agent generates a one-time 32-character random token
- Token is printed to stdout/logcat (`PROBE_TOKEN=...`) every 3 seconds
- On iOS, also written to a file in the simulator's tmp directory
- CLI picks up the token and passes it as a query parameter on the WebSocket URL
- Token is masked in error messages for security

## Connection Flow

### Android

1. CLI runs `adb forward tcp:<host-port> tcp:<device-port>`
2. Connects to `ws://127.0.0.1:<host-port>/probe?token=...`
3. Token extracted from `adb logcat` matching `PROBE_TOKEN=`

### iOS Simulator

1. Simulator shares host loopback — no port forwarding needed
2. Connects to `ws://127.0.0.1:<port>/probe?token=...`
3. Token read from file (fast path) or log stream (fallback)

## Security

- WebSocket only on loopback (`127.0.0.1`)
- One-time token auth
- Token masked in error messages
- Studio HTTP server: localhost-only, CORS protection, XSS prevention
- Input validation on bundle IDs and device serials
- `VideoRecorder` uses `sync.Mutex` for thread safety

## Performance

| Metric | Target |
|--------|--------|
| Command round-trip | < 50ms |
| CLI cold start | < 100ms |
| 50-test suite | < 90 seconds |
| Flake rate | < 0.5% |
