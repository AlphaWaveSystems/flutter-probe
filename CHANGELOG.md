# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
