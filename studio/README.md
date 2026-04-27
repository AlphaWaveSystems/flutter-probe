# FlutterProbe Studio

A standalone desktop application for designing, recording, and executing
ProbeScript tests with embedded simulator/emulator views. Built on
[Wails 2.10+](https://wails.io/) (Go backend + WebView frontend).

> **Status: Beta Preview** (shipped in v0.6.0). The current build provides:
> - Monaco editor with ProbeScript syntax highlighting and live lint markers
> - File browser with workspace folder picker
> - Device picker that filters to booted simulators / online emulators
> - Live device stream + widget tree inspector (event-driven, parallel RPCs)
> - In-process test execution with a streaming results timeline
> - Keyboard shortcuts, toast notifications, dark theme
>
> Stability is "beta" — expect rough edges, especially around physical
> devices (not yet supported) and parallel multi-device authoring.

## Run from source

From this directory (`studio/`):

```bash
# Install Wails CLI once (Go 1.23+):
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Verify your toolchain:
wails doctor

# Live-reload dev mode (browser dev tools at http://localhost:34115):
wails dev

# Production build (produces a single binary per OS):
wails build
# → studio/build/bin/flutter-probe-studio.app   (macOS)
# → studio/build/bin/flutter-probe-studio.exe   (Windows)
# → studio/build/bin/flutter-probe-studio       (Linux)
```

The Studio module imports `internal/...` packages from the parent module
via a `replace` directive in `go.mod` (always builds against the local
sibling, not a pinned proxy version).

## Why Wails (not Flutter Desktop / Electron / Tauri)?

| Requirement                              | Wails | Flutter Desktop | Tauri | Electron |
| ---------------------------------------- | :---: | :-------------: | :---: | :------: |
| Cross-platform (macOS / Windows / Linux) |   ✅   |        ✅        |   ✅   |     ✅    |
| Reuses existing Go runner/lint packages  |   ✅   |        ❌        |   ❌   |     ❌    |
| Monaco editor (industry standard)        |   ✅   |        ❌        |   ✅   |     ✅    |
| Single-language toolchain                |   ✅   |        ✅        |   ❌   |     ❌    |
| Binary size                              | ~20MB |     ~50MB+      | ~10MB |  ~150MB  |

Wails wins because the Studio backend can directly import
`internal/runner`, `internal/lint`, `internal/probelink`, and
`internal/device` — no subprocess shell-out, no JSON wire format to
design. The runner becomes a library, not a child process.

## Prerequisites (when implementation lands)

- Go 1.26+
- [Wails CLI v2.10+](https://wails.io/docs/gettingstarted/installation)
- Platform-specific WebView runtime:
  - **macOS**: WebKit (built in)
  - **Windows**: WebView2 (auto-installed on Win11; bundled on older)
  - **Linux**: WebKitGTK 4.1+ (`apt install libwebkit2gtk-4.1-dev`)

## Architecture

```
┌─────────────────────────────────────────────┐
│ Studio (Wails 2.10+, single binary per OS)   │
│ ┌───────────────────────────────────────┐   │
│ │ WebView frontend (HTML/CSS/JS)        │   │
│ │  - Monaco editor + ProbeScript grammar│   │
│ │  - Device view (img stream + taps)    │   │
│ │  - Inspector / results timeline       │   │
│ └─────────┬─────────────────────────────┘   │
│           │  Wails bindings + EventsEmit    │
│ ┌─────────▼─────────────────────────────┐   │
│ │ Go backend (App struct)               │   │
│ │  imports internal/runner              │   │
│ │  imports internal/lint                │   │
│ │  imports internal/probelink           │   │
│ │  imports internal/device              │   │
│ └───────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

## Layout (target)

```
studio/
├── main.go              # Wails app entry, binds App to WebView
├── app.go               # App struct: Run/Lint/Connect/Disconnect methods
├── services/
│   ├── runner.go        # wraps internal/runner for in-process exec
│   ├── device.go        # wraps internal/device.Manager
│   └── lint.go          # wraps internal/lint
├── frontend/            # Vite + TS web UI
│   ├── package.json
│   ├── index.html
│   ├── src/
│   │   ├── main.ts
│   │   ├── editor.ts    # Monaco wrapper, loads probescript.tmLanguage.json
│   │   ├── device.ts    # screenshot stream renderer + tap forwarding
│   │   └── inspector.ts # widget tree pane
│   └── public/
│       └── probescript.tmLanguage.json  # symlink/copy from vscode/syntaxes/
├── wails.json           # Wails project config (Wails v2.10+ schema)
└── README.md            # this file
```

## v0.7.0 MVP scope

- Editor pane with Monaco + ProbeScript syntax highlighting
- Device pane with ~10 FPS screenshot stream (via existing
  `take_screenshot` RPC — works on iOS sim, Android emu, physical iOS
  over WiFi, physical Android with zero new agent code)
- Tap forwarding (click in pane → coord translation → existing `tap_at`
  RPC)
- Lint markers (live, on every edit)
- "Run all" button → results timeline driven by in-process runner

## Deferred to v0.7.x

- Native scrcpy embed (Android, 60 FPS)
- `simctl io recordVideo` H.264 stream (iOS sim, 60 FPS)
- Multi-device side-by-side
- Time-travel widget tree per step
- AI chat pane (spawns `probe-mcp` and proxies)
