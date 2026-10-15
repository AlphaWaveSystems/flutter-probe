---
title: FlutterProbe Studio
description: Visual ProbeScript test authoring with embedded device view, live widget tree inspector, and in-process test execution. Cross-platform desktop app built on Wails.
---

> **Beta Preview** — shipped in **v0.6.0**. Surface area is feature-complete, including physical iOS and Android devices (v0.7.0); expect rough edges around parallel multi-device authoring.

FlutterProbe Studio is a standalone desktop application for designing, recording, and executing ProbeScript tests. It complements the `probe` CLI rather than replacing it: same parser, same runner, same agent — just a visual front-end for the same underlying engine.

## What you get

- **Monaco editor** with ProbeScript syntax highlighting, live lint markers, and the same TextMate grammar that powers the VS Code extension. Completions, hover help and snippets cover every command in the [ProbeScript dictionary](/probescript/dictionary/) and every reserved word in the [grammar](/probescript/grammar/); they are generated from those pages, so the editor and the docs agree.
- **Workspace folder picker** — point Studio at any project root with a `tests/` folder. Choice persists across launches.
- **Device picker** that lists booted simulators, online emulators, and physical iOS/Android devices side by side, with a `physical` tag and a clear hint when nothing is connectable.
- **Physical device support** — the same `internal/device` code the `probe` CLI uses: `iproxy` tunneling for physical iOS over USB, `adb forward` for physical Android (same path as emulators), and mDNS auto-discovery (`_flutterprobe._tcp`) for WiFi-connected devices. See [Physical devices](#physical-devices) below.
- **Live device pane** — the connected simulator, emulator, or physical device's Flutter screen, shown read-only from repeated screenshot requests to the on-device agent (no video stream; system UI and native views are not included). The title shows `N fps · M ms avg · K ms max`.
- **Live widget tree inspector** — every device frame is bundled with a `dump_widget_tree` snapshot, so the inspector pane updates per frame. No refresh button.
- **In-process test execution** — Studio's Go backend imports `internal/runner` directly. No subprocess shell-out, no JSON wire format. Results stream into the timeline as `run:result` events fire.
- **Live step highlight** — while a file runs, the editor gutter follows the executing step (▶ while running, then ✓ / ✗ / – stays on the line; a failed line shows its error on hover). Step rows nest under each test in the results timeline with line number, duration and attempt; click one to jump to its line. The toolbar shows `step N of M` and a **Cancel** button stops the run.
- **Toast notifications**, dark theme, draggable native title bar, About panel.
- **Keyboard shortcuts**: ⌘R run, ⌘S save, ⌘B connect/disconnect, ⌘P open workspace, ⌘K refresh devices, `?` help, `Esc` close help.

## Automation (no window, no cursor)

Start Studio with `PROBE_STUDIO_AUTOMATION=1` and scripts or AI agents can drive it without touching the mouse or the keyboard focus. Studio then listens on **loopback only**, makes a random token for that launch and writes both to a private discovery file (`<user config dir>/flutter-probe-studio/automation.json`, mode 0600; `PROBE_STUDIO_AUTOMATION_FILE` moves it, `PROBE_STUDIO_AUTOMATION_PORT` fixes the port). The endpoint is **off by default**.

`POST <url>/rpc` with `Authorization: Bearer <token>` and `{"method": "...", "params": {...}}` answers `{"result": ...}` or `{"error": "..."}`. Requests from a browser (any `Origin`, or `Sec-Fetch-Site: cross-site` or `same-site`), with a non-loopback `Host`, or without the token are refused. Methods: `open_workspace {path}`, `list_dir`, `read_file`, `list_devices`, `connect {deviceId}`, `connect_wifi {host, port, token}`, `test_wifi {host, port, token}`, `disconnect`, `status`, `run_file {path}` (returns at once), `cancel`, `run_state` (the running step, finished steps, plan, results), `results`, `ui_state`, `screenshot` (the connected device, base64 PNG), `widget_tree`.

**What the token grants.** Whoever holds the token can drive the connected device (taps, typing, screenshots, running tests), open workspaces, list folders and read and run `.probe` files, but **only inside the allowed folders**: the workspace a person opened in the window (or `PROBE_STUDIO_WORKSPACE`) plus the folders in `PROBE_STUDIO_AUTOMATION_ROOTS` (separated like `PATH`). With neither, `open_workspace`, `list_dir`, `read_file` and `run_file` are refused. Paths must be absolute; symlinks are resolved on both sides before the check, so a link that points out of an allowed folder does not get out of it, and `.probe` is checked on the resolved file. A workspace chosen through automation does not widen the allowed folders. Keep the discovery file at mode 0600 (Studio creates it that way) and treat the token like a password: anything that can read it can run tests on your device. The MCP client refuses to send the token to any address that is not loopback.

A workspace matters because Studio reads its `probe.yaml` (agent port, timeouts, device ids, and the AI and cloud endpoints) and runs its `.probe` files. Nothing in `probe.yaml` names a host command and Studio does not use the `tools:` paths, but a test file can make the host call a URL (`call GET ...`), copy a host file to the device (`add media`) or write screenshots and logs, which is why the folders are confined.

The MCP server exposes the same through `studio_open_workspace`, `studio_list_devices`, `studio_connect`, `studio_run_file`, `studio_cancel`, `studio_run_state`, `studio_results`, `studio_screenshot`, `studio_widget_tree` and `studio_ui_state` (the last two are read-only), so an AI client can open a workspace, connect, run a `.probe` file and read step-by-step progress.

## Testing Studio

Studio's own UI is tested two ways. `make studio-e2e-web` runs the real frontend and Go backend in headless Chromium (Studio's headless mode serves the UI over loopback; a small bridge stands in for the window runtime), so it needs no window, cursor or focus and its device-free half runs on a normal CI runner. `make studio-e2e-native` drives the real window through macOS accessibility for what needs one (window placement, focus, recording); it needs a second display named by `STUDIO_E2E_DISPLAY`. Headless mode is for tests only; to drive Studio from a script or an AI client use the [automation endpoint](#automation-no-window-no-cursor), which is confined to the project.

## Architecture

```
┌─────────────────────────────────────────────┐
│ Studio (Wails 2.12, single binary per OS)    │
│ ┌───────────────────────────────────────┐   │
│ │ WebView frontend (Vite + TypeScript)  │   │
│ │  - Monaco editor + ProbeScript grammar│   │
│ │  - Device pane (img stream)           │   │
│ │  - Inspector / results timeline       │   │
│ └─────────┬─────────────────────────────┘   │
│           │  Wails bindings + EventsEmit    │
│ ┌─────────▼─────────────────────────────┐   │
│ │ Go backend (App struct)               │   │
│ │  imports internal/runner              │   │
│ │  imports internal/lint                │   │
│ │  imports internal/probelink           │   │
│ │  imports internal/device              │   │
│ └─────────┬─────────────────────────────┘   │
└───────────┼─────────────────────────────────┘
            │  WebSocket / HTTP (existing transports)
            ▼
┌──────────────────────────────────┐
│ Flutter app under test            │
│  + flutter_probe_agent package    │
└──────────────────────────────────┘
```

The runner is a library, not a subprocess. The connection-stability work in v0.6.0 (configurable reconnect, signal handler, iproxy probe, serialization) applies to Studio without any extra wiring.

## System requirements

- macOS 11+ (Apple Silicon or Intel), Windows 10/11, or a recent Linux distribution
- A running Flutter app on a booted simulator, online emulator, or physical device, launched with `--dart-define=PROBE_AGENT=true`
- Linux only: WebKitGTK 4.1+ (`apt install libwebkit2gtk-4.1-dev`)
- Physical iOS over USB: `libimobiledevice` for `iproxy` (`brew install libimobiledevice` on macOS)
- Physical Android: `adb` on `PATH` (Android SDK platform-tools) — same requirement as emulators
- Physical devices over WiFi: `flutter_probe_agent` built with `--dart-define=PROBE_WIFI=true`, plus a `ProbeAdvertiser` in the app (agent 0.15.0+; before that mDNS shipped inside the agent) for mDNS discovery — see [WiFi auto-discovery](#wifi-auto-discovery)

## Physical devices

Studio connects to physical iOS and Android devices through the exact same `internal/device` package the `probe` CLI uses — there is no separate device-management code path for Studio.

- **USB** — plug in the device, launch the app with `--dart-define=PROBE_AGENT=true`, click ↻ to refresh the device picker, and pick the entry tagged `physical`. Physical iOS gets an `iproxy` tunnel automatically before Studio dials the agent; physical Android goes through `adb forward`, the same path as emulators. Connection errors for a missing `iproxy` or `adb` binary surface inline with an actionable fix (e.g. "install via: brew install libimobiledevice").
- **WiFi (preferred for iOS)** — matching the CLI's own documented preference, WiFi is more stable than USB-C for physical iOS: a USB-C cable's charge/data mode-flipping can drop the `iproxy` tunnel mid-run, where WiFi has no such failure mode. Build the Flutter app with `--dart-define=PROBE_WIFI=true` and pass an advertiser to `ProbeAgent.start` (see [WiFi auto-discovery](#wifi-auto-discovery)), click the 📡 button to open the WiFi discovery overlay, and pick the device once it appears via mDNS (`_flutterprobe._tcp`). Paste the token printed in the app's `PROBE_TOKEN=...` log line — Studio remembers it per device in local storage for next time (with a "forget" control).
- The exact conditions for a physical iPhone over WiFi, with a three-command check, are in [Physical iPhone over WiFi](/platform/ios/#physical-iphone-over-wifi-cli-and-studio-what-must-be-true).
- Devices that don't advertise over mDNS (no advertiser in the build, older agent versions, or networks that block mDNS): use **Connect manually** in the WiFi overlay (host, port, token) — see [Manual WiFi entry](#manual-wifi-entry) — or the CLI's `--host <ip> --token <token>`.

### The device picker and physical phones

Physical iPhones come from two sources and are labelled by how Studio can reach them: **USB** (seen by `libimobiledevice`, reached through `iproxy`) and **WiFi only** (paired and reachable over the local network, seen by `xcrun devicectl list devices`, Xcode 15 or later). Before this, a phone that was paired over the network but not on a cable simply did not appear. Each entry shows the name you gave the phone and its model (for example `Jane's phone (iPhone 15) · physical · WiFi only`).

Selecting a physical device shows what it needs in a line under the toolbar, instead of leaving you with a Connect button that fails: the app must be **running in the foreground with the agent** (a profile build made with `--dart-define=PROBE_AGENT=true`; add `--dart-define=PROBE_WIFI=true` for WiFi). A WiFi-only phone has Connect disabled, and the line and the button's tooltip say to use the 📡 button and **Connect manually**. An Android phone that is `unauthorized` or `offline` says to unlock it and accept the USB debugging prompt. When nothing is found at all, the message points at the 📡 button too.

## Quick start

1. **Boot a simulator/emulator, or connect a physical device.** Xcode → Devices, or `xcrun simctl boot <udid>` for iOS sim; Android Studio → Device Manager for Android emu; or plug in a physical iOS/Android device over USB (see [Physical devices](#physical-devices) for WiFi).
2. **Launch your Flutter app** with the agent enabled:
   ```bash
   flutter run --dart-define=PROBE_AGENT=true
   ```
3. **Open Studio.** Click the 📂 button in the Workspace pane and pick your project root (the folder containing `probe.yaml` and `tests/`).
4. **Pick the device** in the toolbar dropdown (or use the 📡 WiFi button for a WiFi-connected physical device) and click **Connect**. The device pane should fill with a live mirror; the inspector pane gets a green pulsing **LIVE** badge.
5. **Open a `.probe` file** from the Workspace pane (or write one inline) and click **Run**. Per-test results stream into the Results pane below.

## What the live view is, and its frame rate

The device pane is a **read-only** view. Studio asks the on-device agent for a screenshot and for the widget tree, over the same connection a test run uses, in a loop with no fixed delay: the next request goes out as soon as the previous answer arrives. There is no click-to-tap and no typing into the pane (the agent has no coordinate tap); use the widget tree and the recorder to build steps.

Because the image comes from the app's own renderer (`RenderRepaintBoundary.toImage`), the pane shows **only the Flutter layer**: the status bar, system dialogs, keyboards drawn by the OS and native views are not in it. The frames are PNG, sent as text, and the agent waits for the app to settle before capturing, so a screen with a running animation can capture slower than an idle one. The toolbar shows the frames per second and, per frame, the average and slowest capture time over the last second (`frameMs`, measured on the host around one screenshot and widget-tree pair; it does not include decoding and painting the image in the window).

Measured on one machine (Apple M5 Max, idle fixture app on a static screen, 150 frames each, the host also running other work, so treat the numbers as a rough guide):

| Device | Frames per second | Capture time per frame (median / 95th percentile) | PNG size |
|---|---|---|---|
| iOS simulator `ProbeBaseline-iPhone17` (iOS 26.5) | 14.1 | 68 ms / 86 ms | 52 KiB |
| Android emulator `ProbeRepro-API35` (Android 15) | 13.4 | 72 ms / 84 ms | 44 KiB |

A physical phone (USB or WiFi) uses the same agent request, so expect similar or lower rates depending on the tunnel or network; it has not been measured. A busier screen means larger frames and a lower rate. Faster, video-based views are a possible future change, not something Studio does today.

## Shared device view (preview, off by default)

Start Studio with `PROBE_STUDIO_DEVICE_VIEW=1` and the device pane uses the same interactive view as `probe device view` and the VS Code extension instead of the screenshot loop above. Studio starts a loopback-only hub for the connected device (a random token for this connection, never written to disk) and the pane draws its frames on a canvas. With it you can:

- **tap, swipe and type** in the app by clicking, dragging and typing in the pane (the agent must support the input calls; an older agent gives a read-only view);
- **hover** to outline the widget under the pointer, and **Alt+click** to select it: the selector (`#key`, else the text) is shown, copied to the clipboard and put in the inspector's search box;
- press **+ tap** (device pane header) to write a `tap` step for the selected element at the editor cursor: on a blank line it fills the line, otherwise it goes on a new line below with the same indentation. The selector is built by the same escaping rule as the clipboard copy, so text on the device screen cannot add a second statement;
- see frames at a lower cost: the pane asks the agent for the frame size it needs, and the agent captures at that size.

The pane header says what the device supports: "view only" when the agent has no input calls (an older `flutter_probe_agent`; taps, swipes and typing are off) and "no element select" when it cannot hit-test (hover and Alt+click are off); hover the label for the reason. The inspector text refreshes every 1.5 seconds in this mode (it is no longer bundled with each frame). If another window takes the view over (a second connection replaces the first), the pane shows "taken over" and a Reconnect button; it never reconnects by itself. Without the variable nothing changes: the old loop runs and no hub is started. The protocol is in `docs/studio-device-protocol.md`; this mode is a preview until it has run on a physical phone.

## Device settings

The **Settings** button in the device pane header opens a panel that changes OS-level settings of the connected emulator or simulator, so you can see a screen in dark mode, with a large font or in another language without leaving Studio:

- **Appearance** (light, dark; auto on Android), **font size** (an iOS content size category, or an Android factor such as 1.3), **app language** (needs the app id, which defaults to `project.app` in `probe.yaml`; the app restarts on iOS), **orientation** (Android only; iOS simulators have no command-line rotate) and **open link** (a deep link, delivered to the app when it claims the scheme).
- It always acts on the device Studio is connected to; there is no way to name another one. WiFi connections and physical iPhones are not supported (the panel says why), because the changes go through `adb` and `simctl`.
- Settings stay on the device until you change them back. The same operations are available from the command line (`probe device set`) and to AI clients (`set_device_setting`).

## Build from source

```bash
# Prerequisites
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor   # verify your toolchain

# Build the desktop binary for your current OS
cd studio
wails build

# Output:
#   studio/build/bin/flutter-probe-studio.app   (macOS)
#   studio/build/bin/flutter-probe-studio.exe   (Windows)
#   studio/build/bin/flutter-probe-studio       (Linux)
```

`wails dev` provides a live-reload development loop with browser dev tools available at `http://localhost:34115`.

## Known limitations

- No multi-device side-by-side view.
- No per-step replay / time-travel debugging — see the Results pane for pass/fail, but stack inspection requires re-running.
- The AI chat pane talks to the Anthropic API only, with your own key stored in the OS keychain. Each message sends the open file. The failures of the last run and the connected device's widget tree are sent only when you turn them on, under **AI chat context** in Workspace settings, or in `probe.yaml` under `studio.ai_context` (`run_results` and `widget_tree`). **Both default to off**: until you change them, the chat sends only the open file. When either is on, a line above the chat input says what is sent. Before the widget tree is sent, obscured fields and credential-looking text (API keys such as `sk-…`, GitHub tokens such as `ghp_…`, `Bearer` values, and long hex or base64 runs) are redacted. Each part is cut to a size limit and marked when it is cut. The chat is not exposed through the automation endpoint or the MCP server; use the standalone `probe-mcp` binary with Claude Desktop or Cursor for MCP-driven work.

## Configuration

Studio reuses `probe.yaml` from the workspace root. Connection-related fields (`agent.dial_timeout`, `agent.reconnect_attempts`, `agent.reconnect_backoff`) are honored exactly as the CLI uses them. See [Configuration Reference](/advanced/configuration/).

## Reporting issues

Studio bugs go to the same tracker as the CLI: [github.com/AlphaWaveSystems/flutter-probe/issues](https://github.com/AlphaWaveSystems/flutter-probe/issues). Please include the OS, Wails version (`wails version`), the connected device platform and OS version, and any relevant `studio/build/bin/.../*.log` output.

## Manual WiFi entry

The 📡 overlay has a **Connect manually (host, port, token)** section for a phone that does not show up in the discovered list.

- **Host**: an IPv4 address, an IPv6 address or a hostname (such as `iphone.local`): no `ws://`, path or port. **Port**: 1 to 65535, default 48686 (the port the app printed as `PROBE_PORT=` if it moved). **Token**: the value after `PROBE_TOKEN=` in the app's console (8 to 128 letters, digits and `_ . ~ -`; without the `PROBE_TOKEN=` part).
- **Test connection** dials the agent and pings it without making it the active connection, and says whether the agent is reachable and the token accepted. **Connect** does the same and then connects. The host and port are remembered for next time; the token never is.
- When it fails, the message says what to check: the phone and this computer on the same network (and `curl http://<host>:<port>/probe/status` from this computer), the app in the foreground with the screen unlocked, a **profile build made with `--dart-define=PROBE_AGENT=true` and `--dart-define=PROBE_WIFI=true`** (a production or debug build does not listen), the port, and a fresh token (a new one is created whenever the app restarts).
- An agent can do the same through the automation endpoint (`test_wifi`, `connect_wifi`) or the MCP tool `studio_connect` with `host`, `port` and `token` (and `test_only` to only check).

**Where Studio will connect.** The manual entry sends the agent token to the address you type, so it only accepts a target on this machine or a local network: loopback, link-local, private IPv4 (10.x, 172.16 to 31.x, 192.168.x), IPv6 unique-local, and hostnames such as `iphone.local` or a single-label name. A hostname is resolved and every address it resolves to must be local too (a name that resolves to a public address is refused like the address itself). To connect to a public address on purpose, start Studio with `PROBE_STUDIO_ALLOW_PUBLIC_TARGETS=1`; automation and MCP callers cannot switch this on. Test and connect attempts are limited to 6 per minute, and error messages never contain the token.

The full list of conditions on the phone and the network is in [Physical iPhone over WiFi](/platform/ios/#physical-iphone-over-wifi-cli-and-studio-what-must-be-true).

## WiFi auto-discovery

The core agent has **no native dependency**: a native plugin in your app's dependencies is linked into every
build of that app, release included. mDNS advertising needs one (Bonjour/NSD), so the agent only defines a hook,
`ProbeAdvertiser`, and you implement it in the build you test on devices. No extra FlutterProbe package is needed.

Add `bonsoir` to your app (ideally only in a dedicated flavor/target):

```yaml
dependencies:
  bonsoir: 6.0.2
```

```dart
import 'package:bonsoir/bonsoir.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

class BonsoirAdvertiser extends ProbeAdvertiser {
  BonsoirBroadcast? _broadcast;

  @override
  Future<void> start({required String name, required int port, required String agentVersion}) async {
    try {
      final broadcast = BonsoirBroadcast(
        service: BonsoirService(
          name: name,
          type: mdnsServiceType, // _flutterprobe._tcp
          port: port,
          // The auth token is deliberately NOT advertised.
          attributes: {'version': agentVersion, 'port': '$port'},
        ),
      );
      await broadcast.initialize();
      await broadcast.start();
      _broadcast = broadcast;
    } catch (e) {
      print('ProbeAgent: mDNS advertise failed: $e'); // never block the agent
    }
  }

  @override
  Future<void> stop() async {
    try {
      await _broadcast?.stop();
    } catch (_) {}
    _broadcast = null;
  }
}

Future<void> main() async {
  await ProbeAgent.start(advertiser: BonsoirAdvertiser());
  runApp(const MyApp());
}
```

This is the code of the repository's own fixture (`native-test-apps/flutter/lib/probe_advertiser.dart`), which builds for iOS and Android.
Use `bonsoir` 6.0.2 for now: its 7.1.x Android plugin (`bonsoir_android` 7.1.3) uses `NsdServiceInfo.hostname`, which did not compile against the
Android SDK platform 36 that was installed when this was checked (2026-10-11) (7.1.x was not tried further). On iOS, bonsoir needs a deployment target
of at least 13.0 ([its installation notes](https://github.com/Skyost/Bonsoir/blob/main/docs/content/docs.md)).

Run with `--dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true`. On iOS 14+, add `_flutterprobe._tcp` to
`NSBonjourServices` and a `NSLocalNetworkUsageDescription` in that build's `Info.plist`. It only advertises in
WiFi mode (agent bound to `0.0.0.0`). Without an advertiser, WiFi testing still works with
`probe test --host <ip> --token <token>`; the agent logs `PROBE_MDNS=off`.
