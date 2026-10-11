---
title: FlutterProbe Studio
description: Visual ProbeScript test authoring with embedded device view, live widget tree inspector, and in-process test execution. Cross-platform desktop app built on Wails.
---

> **Beta Preview** — shipped in **v0.6.0**. Surface area is feature-complete, including physical iOS and Android devices (v0.7.0); expect rough edges around parallel multi-device authoring.

FlutterProbe Studio is a standalone desktop application for designing, recording, and executing ProbeScript tests. It complements the `probe` CLI rather than replacing it: same parser, same runner, same agent — just a visual front-end for the same underlying engine.

## What you get

- **Monaco editor** with ProbeScript syntax highlighting, live lint markers, and the same TextMate grammar that powers the VS Code extension.
- **Workspace folder picker** — point Studio at any project root with a `tests/` folder. Choice persists across launches.
- **Device picker** that lists booted simulators, online emulators, and physical iOS/Android devices side by side, with a `physical` tag and a clear hint when nothing is connectable.
- **Physical device support** — the same `internal/device` code the `probe` CLI uses: `iproxy` tunneling for physical iOS over USB, `adb forward` for physical Android (same path as emulators), and mDNS auto-discovery (`_flutterprobe._tcp`) for WiFi-connected devices. See [Physical devices](#physical-devices) below.
- **Live device pane** — the connected simulator, emulator, or physical device's screen mirrored at the device's native pace via repeated `take_screenshot` RPCs. Frame metadata (`N fps · Mms/frame`) shown in the title.
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

The MCP server exposes the same through `studio_open_workspace`, `studio_list_devices`, `studio_connect`, `studio_run_file`, `studio_cancel`, `studio_run_state`, `studio_results` and `studio_screenshot`, so an AI client can open a workspace, connect, run a `.probe` file and read step-by-step progress.

## Testing Studio

Studio's own UI is tested two ways. `make studio-e2e-web` runs the real frontend and Go backend in headless Chromium (Studio's headless mode serves the UI over loopback; a small bridge stands in for the window runtime), so it needs no window, cursor or focus and its device-free half runs on a normal CI runner. `make studio-e2e` drives the real window through macOS accessibility for what needs one (native dialogs, focus, recording). Headless mode is for tests only; to drive Studio from a script or an AI client use the [automation endpoint](#automation-no-window-no-cursor), which is confined to the project.

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

## Frame rate expectations

The live device pane is bottlenecked by the underlying screenshot API:

| Source | Frames per second (approx) |
|---|---|
| iOS simulator (`simctl io screenshot`) | 4–6 FPS |
| Android emulator (`adb exec-out screencap`) | 6–10 FPS |
| Physical device (USB or WiFi) | Same `take_screenshot` RPC, over an `iproxy` tunnel (iOS USB) or a direct WebSocket (WiFi) — expect roughly emulator-class FPS or a little lower, depending on tunnel/network latency; not separately benchmarked yet |

Each frame is a parallel screenshot + widget-tree pair, so frame time is `max(screenshot, tree)`, not `screenshot + tree`. Higher framerate (30–60 FPS via H.264 video stream) is on the roadmap for a follow-up release.

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
- The MCP-driven AI chat pane is designed for but not built; use the standalone `probe-mcp` binary with Claude Desktop or Cursor in the meantime.

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
  bonsoir: ^5.1.10
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
      await broadcast.ready;
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

Run with `--dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true`. On iOS, add `_flutterprobe._tcp` to
`NSBonjourServices` and a `NSLocalNetworkUsageDescription` in that build's `Info.plist`. It only advertises in
WiFi mode (agent bound to `0.0.0.0`). Without an advertiser, WiFi testing still works with
`probe test --host <ip> --token <token>`; the agent logs `PROBE_MDNS=off`.
