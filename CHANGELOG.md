# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.8.0] - 2026-05-02

### Added
- **Studio: ProbeScript recorder** — new ● Record button (⌘⇧R) starts an interactive recording session. The agent streams `probe.recorded_event` notifications; Studio converts each interaction to a ProbeScript step in real time in the editor. Gaps >2 seconds between actions automatically insert `wait N seconds` steps. Requires a WebSocket connection (simulators and emulators); returns a clear error on physical-device (HTTP) connections.
- **Studio: AI chat pane (BYOK)** — ✦ button (⌘⇧A) opens a 260px chat panel at the bottom of the Studio window. Chat with `claude-sonnet-4-6` about the open `.probe` file — the current file contents are injected into the system prompt for context. Your Anthropic API key is stored in the **platform keychain** (macOS Keychain, Windows Credential Manager, Linux libsecret) via `zalando/go-keyring`. Key is never written to disk or sent anywhere except `api.anthropic.com`. Running cost counter (input tokens / output tokens / estimated USD at Sonnet 4.6 rates).
- **Studio: WiFi token memory** — Studio remembers the agent token per discovered device in localStorage. Subsequent mDNS sessions for that device prefill automatically with a "🔑 saved" tag and forget button.
- **Studio: workspace settings overlay** — ⚙ button opens a form for `agent.port`, `defaults.timeout`, iOS UDID, Android serial. Saves back to `probe.yaml` preserving all other keys.
- **Studio: diagnostics polish** — error toasts include actionable hints for missing `iproxy`, `adb`, or `PROBE_AGENT=true`. Status tooltip shows device ID and transport. Inspector search scrolls to first match.
- **Security**: bumped 8 vulnerable dependencies (vite, @vscode/vsce, golang.org/x/crypto, net, sys, text).

## [0.7.0] - 2026-05-02

### Added
- **MCP device lifecycle tools** — `probe-mcp` now exposes 5 new tools that let AI agents discover and manage simulators/emulators end-to-end without leaving chat: `list_devices` (booted/connected sims, emulators, physical devices), `list_simulators` (all iOS sims including shutdown), `list_avds` (Android Virtual Device names), `start_device` (boot Android emulator by AVD name or iOS simulator by UDID, blocks until ready), `shutdown_device` (iOS simulator only). Brings the total tool count from 10 to 15.
- **`device` argument on existing MCP tools** — `get_widget_tree`, `take_screenshot`, `run_script`, and `run_tests` accept an optional `device` (serial or UDID) so the agent can pin a target when multiple devices are connected. Previously the agent had to smuggle this through the undocumented `flags` string.
- **Studio: physical-device support over USB** — Studio's Connect flow now handles physical iOS (via `iproxy` tunnel + `idevicesyslog` token read) and physical Android (via `adb forward`, same path as emulators). The picker shows a `physical` tag so cabled devices are obvious next to sims and emulators. Requires `brew install libimobiledevice` for physical iOS.
- **Studio: WiFi physical-device discovery via mDNS** — Studio now browses for `_flutterprobe._tcp` on the LAN and lets you connect to discovered devices with one click + token paste. Requires `flutter_probe_agent` v0.7.0+ in your Flutter app. The token is intentionally NOT advertised over mDNS (anyone on the network would be able to read it) — the user pastes it from the app's `PROBE_TOKEN=...` log line.
- **`flutter_probe_agent` v0.7.0**: agent advertises itself over Bonjour/NSD when running in WiFi mode (`PROBE_WIFI=true`). New dependency: `bonsoir: ^5.1.10`. Localhost-bound agents skip mDNS entirely so simulator-only apps pay zero overhead.
- **Studio: new Wails methods** `StartWiFiDiscovery`, `StopWiFiDiscovery`, `ConnectWiFi(host, port, token)`. Backed by `github.com/grandcat/zeroconf`.

## [0.6.0] - 2026-04-26

### Added
- **Configurable auto-reconnect policy** — `agent.reconnect_attempts` (default 4) and `agent.reconnect_backoff` (default 1s base) in `probe.yaml`. Replaces the previous fixed 2-attempt, 1s-sleep policy with capped exponential backoff plus jitter (1s → 2s → 4s → 8s, ±20%, ~15s total budget). Slow devices and brief USB-C cable mode flips now recover transparently instead of failing the step.
- **iproxy tunnel TCP health check** — physical iOS startup now verifies the iproxy tunnel is actually forwarding via a 127.0.0.1 probe (up to 3s) before the first dial. Dead-tunnel-on-live-process is detected immediately instead of failing later as a 30s WebSocket handshake timeout.
- **`probe-mcp` standalone binary** — the MCP (Model Context Protocol) server now ships as its own binary alongside `probe`. Configure your MCP client (Claude Desktop, Cursor, etc.) to call `probe-mcp` directly. Same 10 tools, same protocol, smaller per-binary surface. Available via Homebrew (`brew install probe`) and GitHub release artifacts.
- **`probe test --stream`** — when combined with `--format json`, emits one ndjson line per test as it completes (`{"type":"test_result","result":{...}}`), in addition to the final report. Built for live consumption by Studio, CI dashboards, and other tooling that wants real-time progress.
- **FlutterProbe Studio (Beta Preview)** — new `studio/` directory containing a [Wails 2.12+](https://wails.io/) desktop app for visual ProbeScript test authoring. Cross-platform (macOS / Windows / Linux). Marked Beta Preview because the surface area (editor, lint, device pane, run integration) is feature-complete but stability work is ongoing. Features:
  - Monaco editor with ProbeScript syntax highlighting (keywords, strings, variables, tags, comments) and live lint markers driven by the runner's parser
  - File browser with workspace folder picker (persists across sessions in localStorage)
  - Device picker backed by `internal/device.Manager` (simulators and emulators)
  - **Live device view** at ~10 FPS via the existing `take_screenshot` RPC — no new agent code, works on all sim/emu platforms
  - **In-process test execution** by importing `internal/runner` directly — no subprocess shell-out, no JSON wire format
  - Live results timeline streamed via Wails event bus as tests complete
  - Widget tree inspector (refresh on demand)
  - Connection status indicator with semantic colors (connected / connecting / error / disconnected)
  - Toast notifications, keyboard shortcuts (⌘R run, ⌘S save, ⌘B connect, ⌘P workspace, ⌘K refresh devices, `?` help)
  - Native macOS dark appearance, draggable title bar, About panel
  Build with `cd studio && wails build`. Physical device support, scrcpy/simctl native video, multi-device side-by-side, time-travel debugging, and AI chat pane via MCP are deferred to follow-ups.

### Changed
- **`probe mcp-server` is deprecated** — the subcommand still works for backwards compatibility (runs the same server code embedded in `probe`) but prints a one-time deprecation notice on stderr. Migrate your MCP client config to `probe-mcp`. Will be removed in a future release.
- **MCP server reports binary version** — the `initialize` response's `serverInfo.version` now reflects the installed binary version (set at build time) instead of a hardcoded `0.5.7`.

### Fixed
- **Ctrl-C no longer leaks iproxy / `idevicesyslog` / ADB forwards.** The `probe test` command now installs a `SIGINT` / `SIGTERM` handler that cancels the run context so deferred cleanup actually runs. Press Ctrl-C twice to force-exit.
- **Reconnect serialization** — concurrent steps (loops, conditionals) that both observe a dropped connection no longer race on the executor's client reference. A generation counter ensures only one reconnect runs at a time and late callers reuse the new client.

## [0.5.7] - 2026-04-26

### Added
- **Relational selectors** — `tap "Submit" below "Username"`, `see "Price" right of "Label"` — spatial anchoring via Flutter `RenderBox` positions (`below`, `above`, `left of`, `right of`)
- **`open link "url"`** — opens a URL in the default external browser via `url_launcher` platform channel
- **`wait for animations to end`** — polls `SchedulerBinding.hasScheduledFrame` until animations complete
- **`see "Field" is focused`** — asserts that a widget holds keyboard focus via `FocusManager.primaryFocus`
- **`store "value" as varName`** — stores a literal or `${variable}` value for use in later steps
- **`probe mcp-server`** — stdio MCP server (10 tools) for AI agent integration with Claude Desktop, Cursor, etc.: `get_widget_tree`, `take_screenshot`, `read_test`, `write_test`, `run_script`, `run_tests`, `list_files`, `lint`, `get_report`, `generate_test`; see [MCP Server docs](https://flutterprobe.dev/tools/mcp/)
- **`--disable-animations`** flag (also `defaults.disable_animations` in `probe.yaml`) — sets Flutter `timeDilation = 0` after connecting to skip animations and speed up tests
- **`probe.open_link`** RPC — agent-side handler that invokes url_launcher or records the URL for `verify_browser`
- **`probe.set_time_dilation`** RPC — sets `timeDilation` on the agent at runtime
- **`probe.set_output` / `probe.drain_output`** RPCs — inter-step output variable exchange between Dart and CLI
- `device.ios_device_id` / `device.android_device_id` in `probe.yaml` — set a preferred simulator UDID or emulator serial without requiring `--device` on every run

### Fixed
- Token acquisition reliability: `simctl` token reader now globs all app data containers, resolving stale-container mismatches after reinstalls or clear-data operations
- WebSocket dial now retries on transient errors (`connection refused`, reset, timeout) within the configured `dial_timeout` window, eliminating the race between token file write and agent server startup

## [0.5.6] - 2026-04-02

### Added
- Homebrew tap: `brew tap AlphaWaveSystems/tap && brew install probe` (macOS + Linux)
- Homebrew formula auto-updates on every release tag via `HOMEBREW_TAP_TOKEN`

## [0.5.5] - 2026-04-02

### Changed
- `flutter_probe_agent` Dart package re-licensed from BSL 1.1 to MIT (Go CLI remains BSL 1.1)
- CI: added Dart agent validation job — `dart analyze`, `flutter test`, `dart pub publish --dry-run`, CHANGELOG enforcement
- CI: added PR template with pub.dev and docs checklist

## [0.5.3] - 2026-03-28

### Added

- Automated pub.dev publishing via GitHub Actions using official `dart-lang/setup-dart` reusable workflow
- FAQ section on landing page (WiFi testing, physical devices, CI/CD, setup)
- ProbeScript Dictionary — complete reference of all keywords, commands, and modifiers
- Comprehensive third-party tool requirements documentation

### Changed

- Renamed Dart package from `probe_agent` to `flutter_probe_agent` for pub.dev branding
- Publish workflow chains after Release workflow (prevents publishing broken versions)
- Version badge auto-updates from git tags (no more hardcoded versions)

### Fixed

- Broken wiki link on landing page (`AlphaWaveSystems/wiki` → `flutter-probe/wiki`)
- Old domain references (`flutterprobe.com` → `flutterprobe.dev`)
- Old package name references in vscode README and docs
- pub.dev score: shorter description, dartdoc warning, clean public API

## [0.5.1] - 2026-03-26

### Added

- Pre-shared restart token (`probe.set_next_token`) — CLI sends a token to the agent before `restart the app`; agent persists it and uses it after restart, enabling WiFi reconnection without `idevicesyslog`
- `--host` flag for WiFi testing — connect directly to device IP, no iproxy needed
- `--token` flag to skip USB-dependent token auto-detection
- `PROBE_WIFI=true` dart-define — binds agent to `0.0.0.0` for network access
- HTTP POST fallback transport (`POST /probe/rpc`) — stateless per-request communication for physical devices
- `ProbeClient` interface — both WebSocket and HTTP clients satisfy it for transport-agnostic execution
- `tap "X" if visible` ProbeScript syntax — silently skips when widget is not found; works with tap, type, clear, long press, double tap
- Direct `onTap` invocation fallback for `Semantics`-wrapped `GestureDetector` widgets on physical devices
- `take screenshot "name"` now accepts name directly (no `called` keyword needed)
- Physical device E2E test suite for FlutterProbe Test App (12 tests covering all 10 screens)

### Fixed

- `clear app data` on physical iOS now skips immediately (before confirmation prompt) to avoid killing the agent
- Connection error detection in `if visible` — propagates connection errors for auto-reconnect instead of silently swallowing them
- Screenshot parser accepts `take screenshot "name"` without requiring `called` keyword

## [0.5.0] - 2026-03-26

### Added

- Physical iOS device support: launch/terminate via `xcrun devicectl`, token reading via `idevicesyslog`, port forwarding via `iproxy`
- Physical Android device validation: `EnsureADB()` verifies binary, device reachability, and cleans stale port forwards
- Physical device detection: `IsPhysicalIOS` (simctl list check) and `IsPhysicalAndroid` (ro.hardware property check)
- Physical iOS devices listed in `probe device list` via `idevice_id`
- WebSocket ping/pong keepalive (5s interval) — prevents idle connection drops on physical devices via iproxy
- Auto-reconnect on WebSocket connection loss — up to 2 transparent retries per step with full re-dial
- `EnsureIProxy()` — automatic iproxy lifecycle management: checks installation, kills stale processes, starts fresh, defers cleanup
- Visibility filtering in widget finder — off-screen widgets (behind routes, Offstage, Visibility) no longer match `see`/`if appears`
- Unique pointer IDs for synthetic gestures — prevents collision with real touch events on physical devices
- ProbeAgent profile mode support — `ProbeAgent.start()` works in profile builds (required for physical iOS)
- ProbeAgent release mode safeguards — blocked by default, opt-in via `allowReleaseBuild: true` + `PROBE_AGENT_FORCE=true`
- Test files for all packages: `cmd/probe`, `internal/cli`, `internal/ios`, `internal/device` (manager tests)
- HTTP POST fallback transport (`POST /probe/rpc`) — stateless alternative to WebSocket for physical devices, eliminates persistent connection drops
- `ProbeClient` interface — both WebSocket `Client` and `HTTPClient` satisfy it, enabling transport-agnostic test execution
- WiFi testing mode (`--host <ip>` + `--token <token>` + `--dart-define=PROBE_WIFI=true`) — test physical devices without USB, no iproxy needed
- `tap "X" if visible` ProbeScript syntax — silently skips tap when widget is not found, replaces verbose dialog-dismissal recipes
- Direct `onTap` invocation fallback for `Semantics`-wrapped widgets — fixes tap failures on physical devices where synthetic gestures don't reach `GestureDetector`
- `take screenshot "name"` now accepts name directly (previously required `called` keyword)

### Changed

- Operations unsupported on physical devices now skip gracefully with warnings instead of crashing:
  - `clear app data` on physical iOS → warning + skip
  - `allow/deny permission` on physical iOS → warning + skip
  - `set location` on any physical device → warning + skip
- `restart the app` on physical iOS uses `xcrun devicectl` instead of `simctl`
- iOS connection setup now branches: simulator path uses simctl permissions + loopback; physical path uses iproxy + idevicesyslog
- Android connection setup validates ADB availability and device state before port forwarding

## [0.4.2] - 2026-03-25

### Added

- Cross-platform parallel E2E execution: `--parallel --devices emulator-5554,<iOS-UDID>` runs tests on iOS + Android simultaneously
- `ResolveAppID`: auto-converts camelCase iOS bundle IDs to snake_case Android package names for cross-platform runs
- Per-device `AppID` field in `DeviceRun` for mixed-platform parallel testing
- Retry logic for parallel device connections (up to 2 retries with 5s backoff)
- Graceful per-device error handling — one device failing doesn't stop others
- Custom domain: site now lives at [flutterprobe.dev](https://flutterprobe.dev)
- SEO overhaul: sitemap.xml, robots.txt, JSON-LD structured data, Twitter Cards, OG image
- 7 comparison pages targeting search intent (Flutter E2E testing, integration_test alternative, Patrol alternative, etc.)
- 3 blog posts (Flutter E2E testing guide, Why We Built FlutterProbe, honest comparison)
- Copilot Code Review configuration with path-specific review instructions for parser, runner, agent, website, and CI
- Dependabot compatibility workflow: security audit (`govulncheck`, `npm audit`), license compliance (rejects GPL/AGPL/SSPL), backward compatibility (.probe file parsing), auto-merge for patch/minor updates
- Headless E2E CI/CD: fully wired Android (ubuntu + emulator) and iOS (macOS + simulator) workflows with 3-way sharding, automated app build/install/launch, and HTML report generation

### Fixed

- Parallel port assignment: Android gets `portBase+1` via ADB forward, iOS uses `portBase` directly
- Landing page version badge updated to current release

## [0.4.1] - 2026-03-25

### Fixed

- Fix `set location` decimal parsing — coordinates like `37.7749, -122.4194` were stripped of decimals and negative signs
- Fix Android app launch — replace `adb shell monkey` with `am start -n {package}/.MainActivity` (monkey fails silently on many emulators)
- Fix Android token reading — file-based token via `adb shell run-as` instead of unreliable logcat scanning
- Fix variable resolution in `see` assertions — data-driven variables like `<expected>` were not substituted
- Fix Dart agent url_launcher interceptor — use proper `MethodChannel.setMethodCallHandler` instead of mock-only API
- Increase Android reconnect delay to 5s (emulators need more boot time than iOS simulators)

### Added

- `--parallel` flag — auto-discover all connected devices, distribute test files round-robin, run in parallel goroutines
- `--devices serial1,serial2` flag — explicit device list for parallel execution
- `--shard N/M` flag — deterministic file-based sharding for CI matrix jobs (e.g. `--shard 1/3`)
- `ParallelOrchestrator` with per-device goroutines, independent WebSocket connections, port allocation, and result merging
- Per-device test attribution — `TestResult` includes `DeviceID` and `DeviceName`
- JSON reporter includes `device_id` and `device_name` per result
- Terminal output shows per-device summary table in parallel mode
- Lexer support for float literals (e.g. `37.7749`) and negative sign tokens

## [0.4.0] - 2026-03-25

### Added

- `before all` / `after all` hooks for suite-level setup and teardown (run once per file)
- `kill the app` command — force-stop without relaunch (CLI-side via ADB/simctl)
- `open the app` now performs CLI-side launch + reconnect when device context is available
- `copy "text" to clipboard` and `paste from clipboard` commands (agent-side via Dart Clipboard API)
- `set location lat, lng` command — set device GPS coordinates (ADB geo fix / simctl location)
- `verify external browser opened` command — checks url_launcher platform channel for external launches
- `call GET/POST/PUT/DELETE "url"` command — execute real HTTP requests from tests (Go-side net/http)
- `call ... with body "json"` — HTTP calls with request body, response stored in `<response.status>` and `<response.body>` variables
- `<random.email>`, `<random.name>`, `<random.phone>`, `<random.uuid>`, `<random.number(min,max)>`, `<random.text(length)>` data generators for form-heavy tests
- `with examples from "file.csv"` — load data-driven test data from external CSV files
- Unit tests for random data generators, CSV loader, all new parser commands
- E2E test files for all new features: hooks, clipboard, app lifecycle, location, random data, HTTP calls, CSV-driven tests

## [0.3.0] - 2026-03-25

### Fixed

- Resolve all pre-existing staticcheck lint errors blocking CI
- Replace deprecated Go 1.26 crypto/ecdsa field access with ecdh+x509 round-trip in wallet signing
- Remove unused functions and variables across CLI, runner, and probe-convert packages
- Fix error string style violations (punctuation, numeric HTTP status codes, nil context)

### Added

- Unit tests for 6 previously untested packages: config, plugin, visual, report, device, cloud/wallet
- Test coverage for config loading/validation, plugin registry, visual regression comparison, HTML report generation, permission resolution, and wallet operations

### Changed

- Bump GitHub Actions: actions/checkout v5→v6, actions/upload-artifact v4→v7, actions/setup-node v4→v6, actions/upload-pages-artifact v3→v4, codecov/codecov-action v4→v5

## [0.2.0] - 2026-03-22

### Added

- Cloud device farm integrations: BrowserStack, Sauce Labs, AWS Device Farm, Firebase Test Lab, LambdaTest
- WebSocket relay mode for cloud device farms with session TTL and auto-connect
- x402 payment protocol support for cloud API billing (EIP-712 wallet signing)
- VS Code extension: Session Manager sidebar for multi-device parallel testing
- VS Code extension: Test Explorer sidebar with workspace-wide test discovery
- VS Code extension: CodeLens inline Run/Debug buttons above tests
- VS Code extension: real-time diagnostics (lint-on-save) and IntelliSense completions
- VS Code extension: Run Profile webview panel for configuring test options
- Physical iOS device support via iproxy (libimobiledevice)
- `probe studio` command for interactive widget-tree inspection
- `probe generate` command for AI-assisted test generation (Claude API)
- `probe heal` command for self-healing selector repair with AI analysis
- `probe migrate` command for converting tests from other frameworks
- Landing page and Astro/Starlight documentation website
- Cloud relay configuration in probe.yaml (TTL, connect timeout, auto-enable)
- AI configuration in probe.yaml (API key, model selection)

## [0.1.0] - 2026-03-16

### Added

- ProbeScript language with indent-based natural language test syntax
- Go CLI with commands: test, lint, init, device, record, report, migrate, generate
- Dart ProbeAgent with WebSocket JSON-RPC 2.0 protocol and direct widget-tree access
- iOS simulator support with token file fast path and log stream fallback
- Android emulator support with ADB port forwarding and logcat token extraction
- Sub-50ms command round-trip execution
- Recipe system with parameterized reusable steps and `use` imports
- Data-driven tests with `Examples:` blocks and variable substitution
- `before each`, `after each`, and `on failure` hooks
- Conditional execution with `if`/`else` blocks
- `repeat N times` loops
- Visual regression testing with configurable threshold and pixel delta
- Test recording mode capturing taps, swipes, long presses, and text input
- Custom plugin system via YAML definitions
- probe-convert tool supporting 7 source formats at 100% construct coverage
- Supported formats: Maestro, Gherkin, Robot Framework, Detox, Appium (Python/Java/JS)
- VS Code extension with syntax highlighting, snippets, and commands
- HTML, JSON, and JUnit XML report generation with relative artifact paths
- Self-healing selectors via fuzzy matching (text, key, type, semantic strategies)
- HTTP mocking with `when ... respond with` syntax
- App lifecycle commands: `clear app data`, `restart the app`
- OS-level permission handling via ADB and simctl
- Configurable tool paths for ADB and Flutter binaries
- Parallel testing support with per-platform config files
- Video recording on iOS (H.264) and Android (screenrecord/scrcpy)
- Dart escape hatch with `dart:` blocks
- probe.yaml configuration with full resolution order (CLI flag > YAML > default)
