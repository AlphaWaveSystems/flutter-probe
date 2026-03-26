# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
