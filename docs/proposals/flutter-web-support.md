# Flutter Web support (scope only)

**Status:** Proposal, scope only. No code, no estimate of effort beyond the slices below, no commitment to build.
**Touches (if built):** `probe_agent` (Dart), `internal/probelink`, `internal/device`, `internal/runner`, the ProbeScript dictionary surfaces.
**Related:** `docs/proposals/webview-content-automation.md` (the same loopback and cleanup rules apply to anything that talks to a browser).

## The ask

Run ProbeScript tests against a Flutter app compiled for the web, in a desktop browser, with the same steps and selectors a mobile test uses.

## What exists today (checked on `origin/main`)

- The agent is a Dart package embedded in the app (`probe_agent`). It runs an HTTP and WebSocket **server** inside the app process (`probe_agent/lib/src/server.dart` binds an `HttpServer`), and the CLI connects to it (`internal/probelink`). On a device the CLI reaches it through a forward (`adb forward`) or the simulator's loopback.
- The agent imports `dart:io` in `agent.dart`, `executor.dart`, `http_capture.dart` and `perf.dart`. `dart:io` does not exist in a browser build, so the package does not compile for the web as it stands.
- The agent's steps run inside the app: it reads the widget tree and dispatches taps, typing and scrolling itself, so no operating-system input is involved for ordinary steps.
- Everything platform-specific around it (device discovery, permissions, native UI, app restart, clearing app data) is in `internal/device` and `internal/runner/device_context.go` and is written for Android, iOS simulators and iOS devices. There is no browser notion in `device.Platform`.

## What is different on the web

1. **A browser page cannot listen on a port.** The agent's server design does not carry over. A web agent has to connect **out** (a WebSocket client) to something the CLI starts, or be driven from outside.
2. **There is no `dart:io`.** Parts of the agent that use it (HTTP capture and mocking, performance counters, file access for screenshots) need web replacements or must be switched off with a clear message.
3. **The page is drawn to a canvas.** The widget tree exists inside the Dart runtime, but the browser sees a canvas. Flutter can also publish an accessibility (semantics) tree as DOM elements, which is off until it is enabled. Whether and how a test run enables it, and what identifiers it exposes, must be confirmed by a spike.
4. **App lifecycle is a browser tab.** "Restart the app", "clear app data", "kill the app" mean reload the page, clear site storage, and close the tab. Permissions (notifications, location) are browser permissions with their own prompts.
5. **Browsers differ.** A first version should name one browser and one rendering mode, not promise all.

## Options

### A. In-page agent connects out to the CLI

The CLI starts a loopback WebSocket server and opens the page (a browser started by the CLI, a URL carrying a one-time token). The agent, compiled for the web, connects to it and speaks the same protocol as today.

- Keeps one source of truth for selectors and steps (the Dart widget tree).
- Needs the agent split into a platform-neutral core and two transports (server for mobile, client for web), and `dart:io` removed from the core.
- The browser must be allowed to open a WebSocket to loopback from the app's origin (mixed-content and CORS-like rules apply to a page served over `https`); a test run serves the app itself over `http` on loopback to avoid this.

### B. The CLI drives the browser through the Chrome DevTools Protocol, with no agent

Select through the semantics DOM or the canvas by coordinates; tap and type through CDP input events; screenshot through CDP.

- Needs no change to the app.
- Depends on the semantics tree being on and carrying identifiers, and loses direct widget-tree access: no `#id` of a widget key, no widget-level reads, no `dump tree`.
- The CDP client from the WebView work (`internal/device`, loopback only, pid-free here) is reusable.

### C. Both: the agent for the tree, CDP for what the page cannot do

The agent (option A) answers selector and tree questions; CDP supplies what an in-page agent cannot: screenshots, real input where needed, page load and reload, browser permissions, downloads, and clearing site data.

## Recommendation

Do a **spike first, with option C in mind**, in this order. Do not write the platform split until the spike answers the questions below.

1. **Spike 1, drive a page by CDP only (option B).** A tiny Flutter Web app, one browser, one renderer. Questions: can the semantics tree be enabled from outside and what identifiers does it expose; do CDP mouse and key events reach Flutter; can a screenshot be taken reliably. Evidence: a recorded run that taps a button and reads the result.
2. **Spike 2, an agent that connects out (option A).** Same app. Questions: which parts of `probe_agent` compile for the web after removing `dart:io`; can the existing protocol run over a client WebSocket; what must be stubbed.
3. Decide A, B or C from the two spikes, and write the slice list then.

## Slices, if the spikes succeed (each a separate PR)

- S1. `device.Platform` gains a web value; a browser launcher that starts one browser with a remote-debugging port on loopback and a fresh profile directory, and removes only what it started.
- S2. The platform-neutral agent core, and a web transport (client) next to the server transport.
- S3. Steps on the web: tap, type, scroll, see, wait, screenshot; the parts of `DeviceContext` that make sense for a browser (reload, clear site data, close).
- S4. Browser permissions and dialogs; downloads.
- S5. Grammar and dictionary: a way to name the browser and window size in the test or the run command; every dictionary surface (CLI help, docs, MCP, VS Code, Studio).
- S6. Report and artifacts: screenshots and traces for web runs.

## Not in scope

- Several browsers or browser versions in the first version; mobile browsers; WebAssembly-specific tuning.
- Testing a plain web page that is not a Flutter app.
- Cloud or remote browsers.
- Anything that reads or changes a page outside the app under test.

## Rules that carry over from the WebView proposal

- Loopback only; no listener on another interface.
- The CLI starts the browser, so it owns it: it closes only what it started and removes only the profile directory it created.
- A one-time token on every connection; the token is never logged.
- No raw script from a `.probe` file runs in the page.
- Typed values and page text follow the existing redaction rules.

## Open questions for the owner

- Which browser and renderer first?
- Does a web app test run against a deployed URL, a local build served by the CLI, or both?
- Is the web agent shipped in the same package as the mobile agent, or as a separate one?
