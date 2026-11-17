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
> - Live step highlight: the editor gutter follows the executing step, step rows nest under each test, `step N of M` in the toolbar, Cancel
> - Run scope: the whole file, the test at the cursor, a tag, or the tests that failed last time; results group failures first
> - Performance lines (memory, CPU, frames, network) under each result that uses `start measuring`
> - Network tab: the requests the app made during a run, read from the agent's request log, with credentials hidden before they reach the window
> - Send request (in the Network tab): one request from this machine; the first request to each host and port in a session asks first and says where the name leads; no redirect to another host is followed; ten a minute, one at a time
> - Save as step (in the Send request panel): inserts the typed request into the open test as a `call` step (GET, POST, PUT or DELETE; headers are not saved) with `${PROBE_SAVED_*}` placeholders for query values and anything that looks like a credential, a card number or an email address; a typed `${NAME}` or `<name>`, an invisible character or a method the grammar has no word for is refused; values the save cannot recognise are written as typed, so read the step before saving the file
> - Suggest a fix (next to Explain failures): one ProbeScript line for the failed step of the last run, on an advisory card; Studio shows what it would send first and sends only that, once, after Send (a hosted destination every time, one on this machine once per session); not applied, and it changes no result. The card's Replace the line puts it in the editor as one undo step (not saved, not run) after Studio's checks in replace_line.go: the same file and line text, one step with the same verb and flags from a closed list (tap, long press, double tap, type into, see, don't see, wait until appears or disappears), and nothing new such as a comment, a `;` or a `${ENV}`.
> - Explain failures (under Results, while AI is on): advice on the failed tests of the last run; for a hosted provider it shows what it would send before every Explain; for a model on this machine, the first time in a session and again after the destination, the workspace, the provider settings or the saved key change; it sends only after you confirm
> - Replace the line and Re-run (on the Suggest a fix card): Replace puts the suggested line into the editor as one undo step after Studio checks it is one step with the same verb; Re-run runs the failed test once after you save, and shows passed or still failing; up to three suggestions per failed step
> - A click on a failing row, a failed test, a card or a note opens the failing line in the right file
> - A run that takes long: the running step's elapsed time after three seconds, "Still waiting for <step>: Cancel stops the run." after ten, and "no signal" next to Cancel when the device sends no screen
> - Workspace note (Workspace pane): no probe.yaml, a probe.yaml that cannot be read, or no project.app, in one line; the refresh button reads it again
> - Result notes (under a test): that a retry ran it more than once (a flaky pass, or a failure on every attempt) with the earlier failures; why the app died (crash evidence, Android, opt-in), that the test timed out and the limit, and a hint to turn crash evidence on after a lost connection; text only, never sent to an AI provider
> - Problems (tab under the editor): what `probe lint` finds in the open file, from the editor's text, saved or not (a recipe or data file that is not there, a test with no steps, a count check), marked on its line and listed; read only from inside the workspace
> - First-run guidance: a line under the toolbar names the one next step (open a workspace, open a `.probe` file, boot a device, pick one, Connect), every disabled primary button says why, and a gone or empty folder is named
> - Runs like `probe test`: the device context comes from the same code and the workspace `probe.yaml`, so `restart the app` works and the live view follows it; `clear app data` asks in a dialog that names the app (from `probe.yaml`) and the device (default Cancel, Clear data armed after half a second, a second step asks again, no answer in 60 s is a no; Connect is refused during a run)
> - Keyboard shortcuts, toast notifications, dark theme
>
> Stability is "beta" — expect rough edges, especially around physical
> devices (supported; the USB, manual WiFi and Android paths are not yet verified on a real phone) and parallel multi-device authoring.

## E2E tests

`make studio-e2e-native` drives the real Studio window on macOS through the
accessibility tree and screenshots against a bundled fixture app
(`native-test-apps/studio-fixture/`). See [`e2e/README.md`](e2e/README.md),
and [`e2e/DOGFOOD.md`](e2e/DOGFOOD.md) to run the same suite against your own app.

Studio reads `probe.yaml` from the open workspace (agent port, timeout,
device ids). `PROBE_STUDIO_WORKSPACE=/path` opens a workspace at launch
without the folder picker.

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

- Go 1.26.10+
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

- Editor pane with Monaco + ProbeScript syntax highlighting, completions, hover help and snippets (generated from the ProbeScript docs by `npm run gen:probescript`; `npm run check:probescript` checks the generated module is current)
- Device pane: a read-only view fed by repeated screenshot + widget-tree requests to the
  on-device agent (about 13 to 14 frames per second on an idle fixture app on an Apple M5 Max,
  see the Studio docs page for the method; Flutter layer only, no system UI). It does not
  forward clicks: the agent has no coordinate tap, so the pane cannot tap or type
- Optional shared device view (`PROBE_STUDIO_DEVICE_VIEW=1`): the pane draws a loopback hub's frames with the shared
  component and relays tap, swipe, typing, hover and Alt+click selection to the app, "+ tap" writes a tap step for the selected element at the editor cursor, and the header says when the device is view-only; off by default
- Device settings panel ("Settings" in the device pane header): dark mode, font size, app language, orientation (Android) and deep links for the connected emulator or simulator, through `internal/devsettings`
- Recorder: records taps made on the device itself into ProbeScript lines
- Lint markers (live, on every edit)
- "Run all" button → results timeline driven by in-process runner

## Supported connections

| Target | Transport | Notes |
|---|---|---|
| iOS Simulator | local | Token from simctl filesystem; no tunnel |
| Android Emulator | adb forward | Token from cache, /data/local/tmp, or logcat |
| Physical iOS (USB) | iproxy tunnel | Requires `brew install libimobiledevice`; token via idevicesyslog |
| Physical Android (USB) | adb forward | Same path as emulator |
| Physical iOS (WiFi) | direct | Auto-discovered via mDNS (needs a `ProbeAdvertiser` in the app, see the Studio docs); user pastes token from app logs |
| Physical Android (WiFi) | direct | Auto-discovered via mDNS (needs a `ProbeAdvertiser` in the app, see the Studio docs); user pastes token from app logs |

WiFi discovery requires `flutter_probe_agent` v0.7.0+ in your Flutter
app's pubspec, and the app must run with `--dart-define=PROBE_WIFI=true`
so the agent advertises itself as `_flutterprobe._tcp` on the LAN.

## Deferred to v0.7.x

- A video-based device view and input injection (candidates: scrcpy on Android, `simctl io` on the iOS
  simulator); not started
- Multi-device side-by-side
- Time-travel widget tree per step
- AI chat pane (calls the configured provider directly: the Anthropic API, OpenAI, or a local endpoint named in `probe.yaml`; it does not run `probe-mcp`). The first message to a destination you have not confirmed in this window shows what it would send and waits for **Send**. Workspace settings has an **AI provider** section that writes the `ai:` block, with **Test connection**; Settings never takes a key, and **Save** leaves a plain `api_key` in `probe.yaml` as it is

## Testing Studio

- **A selector fix sends only from a preview.** `studio/ai_selector_fix_test.go`: a preview sends nothing; the send is the previewed text; a hosted destination needs a preview for every request, a local one is confirmed once per session and again after a change of model, endpoint, key, workspace or switches; anything that changes between the preview and the send (including the failed step) stops it; previews expire after ten minutes and a session keeps four; there is no one-call method; every field is redacted and the screen bounded.
- **The run tracker has direct tests.** `studio/runtrack_test.go` holds `runTracker`, which the window and the automation endpoint read to show a run, to its contract: the history keeps the newest 2,000 steps in order (checked at 1,999, 2,000, 2,001 and 5,000), a step that has only started is the current step and not history, every new run starts clean (steps, results, the error, the verdict, the current step and the plan), `finish` replaces the results only when it is given some, a snapshot is a copy in both directions, steps and results are JSON lists and never null, and eight writers with four readers under `-race` find no unlocked access.
- **Save as step has a fuzz test.** `go test -run FuzzSaveStep ./studio` runs a seed corpus (a recognisable secret planted in each of twelve positions, and the classes the reviews found); `go test -fuzz=FuzzSaveStep -fuzztime=2m` explores more and writes any failing input to `studio/testdata/fuzz/FuzzSaveStep/`, which then runs as a regression test. The properties: no panic, a saved line is one line that the real parser reads back as a call with the typed method, a request with a `${NAME}` or `<name>` is never saved, and a planted secret is never in the step or in an error. The seed corpus runs in CI with `go test ./...`; only exploratory fuzzing (`-fuzz`) is not part of CI.
- **Adding a method to `App` is a decision.** Every exported `App` method is callable from the window and, in the headless build, from any page script with the bridge token. `studio/bound_methods.json` records every one: its category (pure, reads or writes the workspace, keychain, network, AI provider, device, run control), what it does in one sentence, who may call it (the window, the headless bridge, the automation endpoint), what it touches (files, devices, network, keychain) and whether it needs a confirmation. `studio/bound_methods_test.go` reads it and fails, saying what entry to add, when a method has no entry, an entry has no method, the generated Wails bindings differ, the headless bridge blocks a different set than the table says, or the automation endpoint reaches a different set of names or methods (and never one in a category that must not be reachable from it). It also fails when an entry contradicts the code (the bridge's block list, the automation endpoint's methods, a secret that touches no keychain, a run that does not ask). Add your method's entry to `bound_methods.json` in the same change.
- `make studio-e2e-web`: the real UI in headless Chromium through Studio's headless mode (`PROBE_STUDIO_HEADLESS=1`), no window or focus involved; the device-free half runs on any CI runner. See `e2e-web/README.md`.
- `make studio-e2e-native`: the real window, driven through macOS accessibility (smoke set; needs a second display named by `STUDIO_E2E_DISPLAY`, and focus). `make studio-e2e` is a deprecated alias for one release.
- CI (`.github/workflows/studio-ci.yml`) runs on changes to `studio/` or `vscode/`: the Studio Go tests (after the frontend build that the binary embeds), the frontend build and its `node --test` suites, and the VS Code extension tests. The frontend suites run on Node 26: they pass a directory to `node --test`, which Node 22 does not accept.

The headless suite sets `PROBE_STUDIO_KEYCHAIN=memory` in `e2e-web/playwright.config.ts`, which keeps API keys in memory so no test reads or writes the real keychain. Only the headless build reads that variable.

Test variables, read by the headless build only (a normal launch ignores them):

- `PROBE_STUDIO_SEED_RUN`: the finished tests (a JSON list in the shape of the run results) that Studio starts with. The file must be a regular file inside the workspace, after symlinks are resolved; strict JSON, at most 1 MB and 50 tests. Anything else is ignored with a note.
- `PROBE_STUDIO_SEED_EXCHANGES`: the request log (the shape `probe.http_log` answers with) that Studio serves when no app is connected. A regular file inside the workspace, strict JSON, at most 1 MB and 1,000 entries.
- `PROBE_STUDIO_LOCAL_ENDPOINT`: the default target of Find local models. Only a loopback endpoint is honoured; any other value is ignored, with a note on stderr.
- `PROBE_STUDIO_TEST_SEND_RATE`: the Send request limit per minute, a whole number from 1 to 1000. Anything else keeps the real limit of ten a minute.
- `PROBE_STUDIO_TEST_ALLOW_LOCAL`: comma-separated host names whose addresses on this machine, a private network or a reserved range Send request may call in the tests. Those addresses are refused for any other host in this build.

## Manual check of the native window (device view)

Automated tests cover the browser mode; these checks need a person at an unlocked screen with the real window. Use one simulator or emulator at a time, with a Probe-enabled app running and its agent port and token known.

1. Build and start Studio with `PROBE_STUDIO_DEVICE_VIEW=1 wails dev` (or the built app started the same way) and connect a device from the picker.
2. The pane shows the device screen within a few seconds; the fps label reads about 10 or more and does not climb into dropped frames.
3. Click a button in the pane: the app reacts. Drag across the pane: the app scrolls or swipes. Click a text field and type: the text appears in the app.
4. Hover over widgets: an outline follows the pointer. Alt+click one: a selector chip appears, it is on the clipboard, and the inspector search box holds it.
5. Rotate the device or change its size: the pane keeps the right proportions and clicks still land on the same widget.
6. Open a second window on the same device (for example `probe device view`): the first pane shows "taken over" with a Reconnect button and does not reconnect by itself. Close the second window and press Reconnect: the view returns.
7. Disconnect in Studio: the pane clears and no device view process or port stays behind (`lsof -iTCP -sTCP:LISTEN` shows no leftover loopback listener of Studio).
8. Start Studio without the variable and connect: the old screenshot pane works as before.

VS Code panel (extension from this repository, one device connected):

1. Run "FlutterProbe: Open Device View": the panel shows the screen; tap, swipe and typing work as in steps 3 and 4.
2. Open the same device in Studio's device pane: the VS Code panel shows "taken over"; after Reconnect it returns, or says another window just took over if you reconnect within a few seconds.
3. With the cursor in a `.probe` file, Alt+click a widget and run "FlutterProbe: Insert Tap Step from Device View": a `tap` step with a quoted selector is inserted at the cursor. Close the panel: the `probe device view` process exits.

## Automation endpoint

With `PROBE_STUDIO_AUTOMATION=1` Studio opens a loopback-only control endpoint (random per-launch token, discovery file `<user config dir>/flutter-probe-studio/automation.json`, mode 0600) that exposes the bound methods (`open_workspace`, `connect`, `run_file`, `run_state`, `results`, `screenshot`, ...) without a window or cursor. Off by default. The `studio_*` tools of `probe-mcp` use it. Automation can only list, read, run and open workspaces inside the folder a person opened in the window (or `PROBE_STUDIO_WORKSPACE`) plus `PROBE_STUDIO_AUTOMATION_ROOTS`; keep the discovery file at 0600. See the [Studio page](https://flutterprobe.dev/tools/studio/#automation-no-window-no-cursor).
