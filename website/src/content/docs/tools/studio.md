---
title: FlutterProbe Studio
description: Visual ProbeScript test authoring with embedded device view, live widget tree inspector, and in-process test execution. Cross-platform desktop app built on Wails.
---

> **Beta Preview** — shipped in **v0.6.0**. Surface area is feature-complete; expect rough edges around physical devices (not yet supported in Studio) and parallel multi-device authoring.

FlutterProbe Studio is a standalone desktop application for designing, recording, and executing ProbeScript tests. It complements the `probe` CLI rather than replacing it: same parser, same runner, same agent — just a visual front-end for the same underlying engine.

## What you get

- **Monaco editor** with ProbeScript syntax highlighting, live lint markers, and the same TextMate grammar that powers the VS Code extension.
- **Workspace folder picker** — point Studio at any project root with a `tests/` folder. Choice persists across launches.
- **Device picker** that filters to booted simulators / online emulators, with a clear hint when nothing is bootable.
- **Live device pane** — the connected simulator or emulator's screen mirrored at the device's native pace via repeated `take_screenshot` RPCs. Frame metadata (`N fps · Mms/frame`) shown in the title.
- **Live widget tree inspector** — every device frame is bundled with a `dump_widget_tree` snapshot, so the inspector pane updates per frame. No refresh button.
- **In-process test execution** — Studio's Go backend imports `internal/runner` directly. No subprocess shell-out, no JSON wire format. Results stream into the timeline as `run:result` events fire.
- **Toast notifications**, dark theme, draggable native title bar, About panel.
- **Keyboard shortcuts**: ⌘R run, ⌘S save, ⌘B connect/disconnect, ⌘P open workspace, ⌘K refresh devices, `?` help, `Esc` close help.

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
- A running Flutter app on a booted simulator or emulator, launched with `--dart-define=PROBE_AGENT=true`
- Linux only: WebKitGTK 4.1+ (`apt install libwebkit2gtk-4.1-dev`)

Physical iOS / Android devices are **not** supported in this Beta Preview — the CLI's `probe test` flow handles them; Studio support lands in a follow-up release.

## Quick start

1. **Boot a simulator or emulator.** Xcode → Devices, or `xcrun simctl boot <udid>` for iOS; Android Studio → Device Manager, or your IDE's emulator launcher for Android.
2. **Launch your Flutter app** with the agent enabled:
   ```bash
   flutter run --dart-define=PROBE_AGENT=true
   ```
3. **Open Studio.** Click the 📂 button in the Workspace pane and pick your project root (the folder containing `probe.yaml` and `tests/`).
4. **Pick the booted device** in the toolbar dropdown and click **Connect**. The device pane should fill with a live mirror; the inspector pane gets a green pulsing **LIVE** badge.
5. **Open a `.probe` file** from the Workspace pane (or write one inline) and click **Run**. Per-test results stream into the Results pane below.

## Frame rate expectations

The live device pane is bottlenecked by the underlying screenshot API:

| Source | Frames per second (approx) |
|---|---|
| iOS simulator (`simctl io screenshot`) | 4–6 FPS |
| Android emulator (`adb exec-out screencap`) | 6–10 FPS |
| Physical device over WiFi | n/a (Studio doesn't support physical yet) |

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

- Physical devices are not supported (no iproxy management in Studio yet).
- No multi-device side-by-side view.
- No per-step replay / time-travel debugging — see the Results pane for pass/fail, but stack inspection requires re-running.
- The MCP-driven AI chat pane is designed for but not built; use the standalone `probe-mcp` binary with Claude Desktop or Cursor in the meantime.

## Configuration

Studio reuses `probe.yaml` from the workspace root. Connection-related fields (`agent.dial_timeout`, `agent.reconnect_attempts`, `agent.reconnect_backoff`) are honored exactly as the CLI uses them. See [Configuration Reference](/advanced/configuration/).

## Reporting issues

Studio bugs go to the same tracker as the CLI: [github.com/AlphaWaveSystems/flutter-probe/issues](https://github.com/AlphaWaveSystems/flutter-probe/issues). Please include the OS, Wails version (`wails version`), the connected device platform and OS version, and any relevant `studio/build/bin/.../*.log` output.
