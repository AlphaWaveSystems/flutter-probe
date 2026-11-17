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
  flutter_probe_agent: ^0.27.0
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
| `PROBE_APP_ID=<id>` | The application id the agent reports in `probe.ping` and `GET /probe/status`, for iOS or an app whose Android process has another name than its package. Optional: on Android the agent reads its process name. |
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
- **Two apps with the agent on one Android device.** The second app's agent moves to the next free port, and the CLI, which forwards the configured one, reaches the first app. With `project.app` set, the CLI looks for the port of the app under test **after a failed dial** (a 401, or a connection that does not answer for 3 seconds), read-only: it takes the one port in the 10 above the configured one that a socket owned by that app's package uid listens on, moves the `adb forward` to it and says `agent port N on the device is held by <package>; using M for <app>`. It does nothing when the app listens on none or on several of them, when the package shares its uid with another package, or when `/proc/net` cannot be read; the usual hint then names the holder. A token that was not read through `run-as` (the shared file or the log) does not prove the app, so in that case the CLI checks before the first dial (and again when a rejected token is read again and the read fell back to such a source), with three read-only adb calls, whether another app holds the port. When one does, it looks for the app's own port for the whole dial window (the app may still be starting) and, if it finds none, **refuses to connect** and names the holder, rather than dial an agent that may be the other app's. With a `run-as` token a run with one agent makes no extra adb call. Not covered: an iOS simulator (it has its own ports), Studio and its Wi-Fi target, `probe record`, the device view, the cloud path of `probe test` (they use the configured port), the first connect of a composite test (its reconnects follow). Still unprotected: when `/proc/net` is unreadable nobody can say who holds the port, so nothing is refused; with an agent too old to move its port and an app that is not debuggable (no `run-as`), the token and the port can both belong to another app. Each device of a parallel run looks for itself; nothing is shared.
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

The `probe.visible_summary` answer lists what a person could see now: `texts` (each visible `Text` and the value of each visible field that is not obscured, de-duplicated by trimmed text) and `keys` (string `ValueKey`s), each cut at 25 in tree order. It also carries keys an older CLI ignores: `editable_texts` (the value of every visible field that is not obscured, recorded even when a `Text` with the same text is already in `texts`, since a text selector also matches a field by its value), `rich_texts` (the texts of visible `RichText`s that no `Text` and no `Icon` built, which a text selector also matches; an `Icon`'s `RichText` holds only its glyph, a private-use character. The glyph is still matchable by a text selector made of that character, but it is not listed, so a reader must refuse text selectors made of private-use characters), `max` (the cap, 25), `counts` (how many distinct items each list saw, cut or not) and `truncated` (true when any list saw more than `max`). The agent never cuts or leaves out an item for its length; an obscured field adds nothing to any list.

**Which control a state check reads.** `is enabled`, `is disabled` and `is checked` read the control the target is, then the first control it holds (an id on a wrapper such as `Semantics(identifier: ...)` reads the button inside), then the control around it (a text inside a button reads the button). Before the fix for issue #860, `is enabled` did not look inside the target, so a wrapper around a disabled button read as enabled.

The `probe.ping` answer carries the agent's version and capabilities, and, when the app's build knows them, `flutter_version`, `flutter_channel`,
`framework_revision` and `dart_version`: the Flutter and Dart that built the app (the Flutter tool records them in the app at build time; they are not the
Flutter that published `probe_agent`). Each field is optional and left out when unknown. The CLI keeps only plain version text (letters, digits, dots, dashes
and plus signs, at most 64 characters) and ignores the rest. A fork or a custom embedder can report values that are not the real ones.

The answer also carries `app_id`, and so does `GET /probe/status` (which needs no token, like the version it reports): the application id of the app the agent runs in. On
Android the agent reads it from its own process name (`/proc/self/cmdline`), which is the package name unless the app sets `android:process`; on iOS, or when the process has
another name, build with `--dart-define=PROBE_APP_ID=<id>`. It is left out when it is not a plain application id (dot-separated segments of letters, digits, `_` and `-`, at
most 255 characters), and nothing is guessed. When two apps with the agent share a device, the CLI can then tell which one answered: after the handshake it prints a **warning**
(not a refusal) when the agent's `app_id` differs from `project.app` in `probe.yaml` (compared exactly on Android, where package names are case-sensitive, and without regard to case on iOS, where bundle ids are not), for example `the agent that answered reports app com.other.app, but project.app is
com.example.app`. It is the app's own statement, not a proof, and an agent that does not send it (an older one) says nothing.

**One visible text is one match.** A text selector matches a `Text`, a `Text.rich`, a `RichText` or a text field's value, by the whole text or any part
of it. A `Text` builds a `RichText` inside it; the agent counts that pair once, so `see exactly 1 "<text>"` passes for one `Text`, an ordinal (`2nd "Row"`)
counts separate texts, and a button's label is one match. Before the fix for issue #800 (`probe_agent` 0.27.0 and earlier), a plain `Text` counted twice: `see exactly 2` passed for one `Text`, and `2nd "8"` found the inner paragraph of a single "8"; with the fix both fail, and the real count and `tap "8"` are what to write.

**A widget with no text is found, and read, by its label.** When no visible text matches, a text selector falls back to a `Semantics` label (an `Icon` or an `Image` with a `semanticLabel`) or a `Tooltip` message (an icon button). That widget reads as its label, so `see "Logo" contains "Logo"` and `see "Add item" contains "Add"` check it; before the fix for issue #818 (`probe_agent` 0.27.0 and earlier) it read as an empty text. A text field under a labelled `Semantics` or a `Tooltip` still reads as the field's text, and an obscured one as nothing, as before. Changed for an element that is itself a labelled `Semantics` and has no text field below it, when it is the match (by `#id` on a `Semantics(identifier:)`): it reads as its label where it read as `""` (`Semantics(identifier: "tot", label: "Total label", child: Text("42.00"))` found by `#tot` reads "Total label"; it never read "42.00"). A widget found by `#id` whose only text is a tooltip still reads as empty.

**The agent answers once the app's view has its real size.** After a relaunch, an app can draw its first frames with a 0×0 view for about a second (measured on an
Android 15 emulator), and a list laid out then holds only its first item: a step that ran in that window saw half the screen (issue #793). The size changes the moment
the engine reports it, but the widgets are laid out at it only in the next frame, so the agent records the size each frame was laid out at, and waits, before it
answers any request, until a frame has been laid out at the view's real size. Once it has, this is a synchronous check that costs nothing; after a resize (a
rotation) a request waits for at most one frame. The wait
is capped at 2.5 seconds, well under the handshake's limits (the dial timeout is 5 seconds in Studio and 30 by default in the CLI); a size that does not come within
the cap is not waited for again until the view's metrics change. It never waits for an app with no view, or one that is not in the foreground (paused, hidden,
inactive). The `probe.ping` answer says how long it waited, in `view_ready_ms` (0 when it did not wait), and the agent reports the capability
`waits_for_view_size`. Both are for diagnostics: no step needs them, and an older agent sends neither.

Errors use JSON-RPC codes: `-32000` timeout, `-32001` widget not found, `-32002` assertion failed. A timeout or
not-found error lists the texts and keys visible at that moment, which is usually enough to see why a selector missed. The value of an obscured (password) field is never matched by a selector and never listed there; the field is found by its id, label or hint (see Password fields in the ProbeScript syntax page).

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
  transitions, frames, animations and HTTP requests, but not for app-level async loads (for example a list fetched in
  `initState`): after navigation use `wait until "<text>" appears`.
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
| `connection refused` / agent never found | The app was not built with `--dart-define=PROBE_AGENT=true`, or `ProbeAgent.start()` is not reached. Look for `PROBE_TOKEN=` in the app log. On Android the CLI re-creates the `adb forward` while refused; check `adb forward --list` if it persists. |
| `agent rejected token (HTTP 401)` | You are connected to a *different* agent (often a leftover simulator app on 48686). The error names the process holding the port; stop it or use `--agent-port` with `PROBE_PORT`. On Android with `project.app` set, the CLI first looks for the app's own port and says `using <n> for <app>` when it finds it (see Connecting). |
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
