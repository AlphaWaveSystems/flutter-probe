---
title: The On-Device Agent
description: How the flutter_probe_agent package works, how to add it to your app safely, every build flag, ports and tokens, the RPC surface, and troubleshooting.
---

`flutter_probe_agent` ([pub.dev](https://pub.dev/packages/flutter_probe_agent)) is the half of FlutterProbe that
lives **inside your Flutter app**. The `probe` CLI parses `.probe` files and sends commands; the agent receives them
and acts on the live widget tree. Both are required.

```
probe CLI (Go)  ──  JSON-RPC 2.0 over WebSocket / HTTP  ──  flutter_probe_agent (in your app)
```

The agent reads widgets directly instead of going through the platform accessibility layer, which is why a command
takes tens of milliseconds. It cannot see anything outside your app (permission alerts, share sheets, the StoreKit
sign-in sheet): those are handled by [System dialogs](/tools/system-dialogs/).

## Add it to your app

```yaml
# pubspec.yaml
dependencies:
  flutter_probe_agent: ^0.16.0
```

```dart
import 'package:flutter/widgets.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await ProbeAgent.start();   // a no-op unless built with --dart-define=PROBE_AGENT=true
  runApp(const MyApp());
}
```

`ProbeAgent.start()` does nothing unless the app is built with `--dart-define=PROBE_AGENT=true`, so the call is safe to
leave in `main`. Run with:

```bash
flutter run --dart-define=PROBE_AGENT=true
probe test tests/
```

The package has **no native plugin dependency**, so adding it links nothing native into your builds.

## Build flags

All are `--dart-define=NAME=value` at build time.

| Define | Effect |
|---|---|
| `PROBE_AGENT=true` | Enables the agent. Required; without it `start()` returns immediately. |
| `PROBE_WIFI=true` | Binds to `0.0.0.0` instead of loopback so a CLI on another machine can connect (`probe test --host <ip> --token <token>`). Debug/profile builds only. |
| `PROBE_PORT=<n>` | Move the agent off the default port 48686. Pair with `probe test --agent-port <n>`. |
| `PROBE_AGENT_FORCE=true` | Silences the console warning in an explicitly allowed release build. |
| `PROBE_RELAY_URL`, `PROBE_RELAY_TOKEN` | Relay mode for cloud device farms: the agent connects *out* to a relay instead of listening. |

## Build modes and safety

| Mode | Behavior |
|---|---|
| Debug | Works with `PROBE_AGENT=true`. |
| Profile | Works; required for cold-launching on **physical iOS** devices. |
| Release | **Blocked by default** (the agent opens a debug server). `ProbeAgent.start(allowReleaseBuild: true)` overrides, and prints a warning unless `PROBE_AGENT_FORCE=true`. Never ship that to users. |

Because `ProbeAgent.start` is guarded by a compile-time constant, builds without the define tree-shake the agent out.

## Connecting: ports and tokens

- The agent listens on `127.0.0.1:48686`. If the port is busy it tries up to 10 consecutive ports and prints
  `PROBE_PORT=<n>`; when it finds another probe agent there it logs `PROBE_PORT_BUSY=<port> (another probe agent is running)`.
- It prints `PROBE_TOKEN=<token>` every 3 seconds (so a CLI that connects late still sees it) and also writes the
  token to a file the CLI reads. A pre-shared token (`set_next_token`) lets `restart the app` reconnect over WiFi
  without reading device logs.
- **Simulators and emulators** use WebSocket. **Physical devices** automatically use stateless HTTP POST instead,
  which cannot drop a connection. WiFi is the most reliable way to test a physical iOS device.
- The CLI sends its version on connect and warns if the agent's version differs by more than a minor release.

If a stale process holds the port, a failed connection now says so: `agent port 48686 is held by pid 4242 (Runner)`.

## What the agent can do (RPC surface)

Everything the CLI does is a JSON-RPC 2.0 method. You normally never call these yourself; they are listed so tools
and AI agents can reason about behavior.

| Group | Methods |
|---|---|
| Lifecycle | `probe.ping` (version handshake), `probe.settled`, `probe.open`, `probe.close`, `probe.device_action` |
| Interaction | `probe.tap`, `probe.double_tap`, `probe.long_press`, `probe.type`, `probe.clear`, `probe.swipe`, `probe.scroll`, `probe.drag` |
| Assertions and waiting | `probe.see`, `probe.wait`, `probe.visible_summary`, `probe.selector_bounds` |
| Inspection | `probe.screenshot`, `probe.dump_tree`, `probe.save_logs` |
| App control | `probe.run_dart`, `probe.mock`, `probe.open_link`, `probe.set_time_dilation`, `probe.set_next_token`, clipboard and recording methods |
| Signals from tests | `probe.biometric_signal`, `probe.signal`, `probe.set_output`, `probe.drain_output` |

Errors use JSON-RPC codes: `-32000` timeout, `-32001` widget not found, `-32002` assertion failed. A timeout or
not-found error lists the texts and keys visible at that moment, which is usually enough to see why a selector missed.

## Selectors

| Selector | Example | Matches |
|---|---|---|
| text | `tap "Sign In"` | a visible `Text`/`RichText`/`EditableText` containing the string |
| id | `tap #email_field` | a widget with `ValueKey('email_field')`, or `Semantics(identifier: 'email_field')` |
| type | `tap <ElevatedButton>` | by widget type name |
| ordinal | `tap 2nd "Add"` | the n-th match |
| positional | `tap "Edit" in "Settings"` | a match inside another widget |

Only widgets on the **current route** and not behind `Offstage`/`Visibility(false)` are matched, so a screen
underneath the one the user sees cannot produce a false positive.

## Behaviors worth knowing

- **Typing** goes through the same path as the keyboard (`EditableTextState.userUpdateTextEditingValue`), so
  `onChanged`, `inputFormatters` and form validation run exactly as for a user.
- **Scrolling**: `scroll down` reveals later content. `scroll down until "X" appears` scrolls half a viewport at a
  time and brings the target fully on screen. Lists build rows lazily, so a row that is off screen does not exist yet.
- **Taps** wait up to 2 seconds for an in-flight dialog/sheet/page transition, then fire. `wait for idle` waits for
  transitions, frames, animations and HTTP requests.
- **Sync**: after every action the agent waits until frames, tracked animations and in-flight HTTP are idle.
- **Semantics-wrapped buttons**: if a synthetic tap does not reach a `GestureDetector` under `Semantics`, put the
  `ValueKey` on the `GestureDetector`.

## Hooks for native flows

- `awaitBiometricResult()` returns the result of a scripted `biometric match` / `no match`, for apps whose
  `local_auth` call does not resolve on newer iOS simulators.
- `awaitSignal(name)` lets a test deliver a value into a flow that is blocked on a native prompt
  (`deliver signal "<name>" ...`).
- `ProbeAdvertiser` is a hook for Studio's WiFi auto-discovery (mDNS). The agent itself includes no mDNS plugin; see
  [Studio](/tools/studio/#wifi-auto-discovery) for a copy-paste implementation using `bonsoir`.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `connection refused` / agent never found | The app was not built with `--dart-define=PROBE_AGENT=true`, or `ProbeAgent.start()` is not reached. Look for `PROBE_TOKEN=` in the app log. |
| `agent rejected token (HTTP 401)` | You are connected to a *different* agent (often a leftover simulator app on 48686). The error names the process holding the port; stop it or use `--agent-port` with `PROBE_PORT`. |
| `agent port 48686 is held by an adb port forward` | A forward left over from an earlier run (or another Android device is forwarded to it): `adb forward --remove tcp:48686`, or `--agent-port`. |
| `agent port 48686 is held by an iOS simulator app` | A simulator app from an iOS run is still running. The message names the simulator: `xcrun simctl terminate <udid> <bundle-id>`. |
| A tap "does nothing" | The CLI now prints `tap target ... is covered by another widget`: something is on top at that point (a loading overlay, a sticky bar). The visible texts/keys in the message show what the screen contained. |
| `401` / `unexpected EOF` right after launching the app | The device still had the previous run's token. The CLI re-reads it and retries for up to 12 seconds; an explicit `--token` is not retried. |
| Two simulators collide | Build each with its own `--dart-define=PROBE_PORT=<n>` and pass `--agent-port <n>`. |
| Everything times out on a physical iPhone | Use a profile build and WiFi: `--dart-define=PROBE_WIFI=true`, then `--host <ip> --token <token>`. |
| `version mismatch` warning | Update the agent and the CLI to the same minor version. |
| Red screen after closing a dialog (agent 0.10 – 0.14) | Fixed in 0.15.0; upgrade the agent. |

## Versioning

The agent and the CLI are versioned together (`0.16.0` ships both). Upgrade the CLI with `brew upgrade probe` and the
agent with `flutter pub upgrade flutter_probe_agent`. Release notes: the
[agent changelog](https://pub.dev/packages/flutter_probe_agent/changelog).
