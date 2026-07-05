# Changelog

## 0.9.10 - 2026-07-05

- `probe.ping` now returns `agent_version` (this package's version) alongside
  `ok`, and accepts an optional `client_version` field from the CLI. Part of
  the CLI↔agent version-compatibility handshake (see root CHANGELOG PT-07).
  Both fields are additive and ignored by older CLIs/agents that don't know
  about them.
- Fixed: the version reported in mDNS advertisements and `GET /probe/status`
  had drifted to `0.7.0` while this file's version moved on — corrected to
  match `pubspec.yaml` and moved into its own `agent_version.dart` file.

## 0.9.9 - 2026-05-13

- **`awaitSignal(String name)`** — new public function. Blocks until the CLI
  delivers `deliver signal "name"`. Returns the value string sent with the
  step (default `"true"`). Use to unblock any OS-level interaction not in
  the Flutter widget tree: push permission prompts, payment sheets, App
  Tracking Transparency, custom deep-link handlers, etc.
- New `probe.signal` JSON-RPC method handled by `ProbeExecutor`.

## 0.9.8 - 2026-05-12

- **`awaitBiometricResult()`** — new public function exported from
  `flutter_probe_agent`. Test apps in PROBE_AGENT builds call this instead
  of `local_auth.authenticate()` to receive the biometric match/no-match
  result from the CLI via the new `probe.biometric_signal` JSON-RPC command.
  Required on iOS 26+ simulator where `notifyutil` no-match notifications
  no longer resolve `LAContext.evaluatePolicy`.
- New `probe.biometric_signal` JSON-RPC method (`ProbeMethods.biometricSignal`)
  that delivers `true` (match) or `false` (no-match) to a pending
  `awaitBiometricResult()` Dart Completer.

## 0.9.7 - 2026-05-12

- Version bump to match CLI v0.9.7. No agent code changes — biometric
  authentication is driven via simctl/adb from the CLI, no on-device
  agent involvement needed.

## 0.9.6 - 2026-05-12

- Version bump to match CLI v0.9.6. No agent code changes — annotation DSL
  completeness work is in the flutter_probe_annotation &
  flutter_probe_gen packages.

## 0.9.5 - 2026-05-12

- **Fix: iOS/Impeller screenshots** — `take_screenshot` previously called
  `OffsetLayer.toImage()` on the root render view, which on iOS with the
  Impeller renderer returns a GPU-backed texture whose `toByteData(png)`
  is `null` — silently breaking screenshot capture. The agent now
  primarily captures via the largest visible `RenderRepaintBoundary` in
  the widget tree (Impeller-supported), and falls back to the old
  `OffsetLayer.toImage()` path only when no boundary is found (Skia).
  Awaits `WidgetsBinding.instance.endOfFrame` before capture so the
  latest frame is always in the image. Uses the actual view's
  `devicePixelRatio` rather than a hard-coded `2.0`.

## 0.9.4 - 2026-05-09

- Version bump to match CLI v0.9.4. No agent code changes — the .mcpb
  Claude Desktop Extension is a CLI/server-side packaging change.

## 0.9.3 - 2026-05-09

- Version bump to match CLI v0.9.3. No agent code changes in this release.
  Annotation-driven test generation is delivered by the new
  flutter_probe_annotation and flutter_probe_gen packages.

## 0.9.2 - 2026-05-09

- Version bump to match CLI v0.9.2. No agent code changes in this release.
  Step feedback improvements are CLI-side only.

## 0.9.1 - 2026-05-09

- Version bump to match CLI v0.9.1. No agent code changes in this release.
  MCP parity improvements are CLI/server-side only.

## 0.9.0 - 2026-05-09

- Version bump to match CLI v0.9.0. No agent code changes in this release.
  Composite tests are a CLI-only feature — the agent runs identically on each
  participating device and is unaware of the multi-device coordination layer.

## 0.7.0 - 2026-05-02

- **mDNS auto-discovery** — when running in WiFi mode (`PROBE_WIFI=true`), the
  agent now advertises itself over Bonjour/NSD as `_flutterprobe._tcp` so
  Studio (and any compatible client) can discover physical devices on the LAN
  without manual IP entry. The token is deliberately NOT included in TXT
  records — anyone on the same network would be able to read it. The agent
  still prints `PROBE_TOKEN=...` to logs as before.
- New dependency: `bonsoir: ^5.1.10`. Localhost-only deployments (no
  `PROBE_WIFI`) skip mDNS bring-up entirely so apps that only test on
  simulators pay zero overhead.

## 0.6.0 - 2026-04-26

- Version bump to keep in sync with CLI v0.6.0
- New RPCs: `probe.open_link`, `probe.set_time_dilation`, `probe.set_output`, `probe.drain_output`
- Relational selectors: `findRelational` resolves widgets by spatial relation (`below`, `above`, `left of`, `right of`) using `RenderBox` positions
- New asserts: `see "X" is focused` (FocusManager.primaryFocus check)
- New waits: `wait for animations to end` (polls `SchedulerBinding.hasScheduledFrame`)

## 0.5.7 - 2026-04-26

- No agent changes — version bump to keep in sync with CLI

## 0.5.6 - 2026-04-02

- Add Homebrew tap support (`brew tap AlphaWaveSystems/tap && brew install probe`)

## 0.5.5 - 2026-04-02

- License changed from BSL 1.1 to MIT — free to embed in any Flutter app, including commercial and proprietary

## 0.5.4

- Restructured README: clear two-part system explanation (CLI + agent)
- Added CLI installation instructions (go install, GitHub Releases)
- Step-by-step getting started guide (install CLI → add agent → write test → run)
- Architecture diagram showing CLI ↔ agent communication

## 0.5.3

- Automated publishing via GitHub Actions (OIDC, no secrets needed)
- Publish workflow chains after Release workflow success

## 0.5.2

- Fix pub.dev score: shorten description to under 180 chars
- Fix dartdoc angle bracket warning in plugin.dart
- Reduce public API to `ProbeAgent` and `isProbeEnabled` only

## 0.5.1

- HTTP POST endpoint (`POST /probe/rpc`) — stateless fallback transport for physical devices
- WiFi testing mode (`PROBE_WIFI=true`) — binds to `0.0.0.0` for network access
- Pre-shared restart token — enables `restart the app` over WiFi without USB
- Direct `onTap` fallback for `Semantics`-wrapped widgets on physical devices
- Unique pointer IDs for synthetic gestures (prevents collision with real touches)
- `sendFn` setter on `ProbeExecutor` for HTTP request routing

## 0.5.0

- Profile mode support — `ProbeAgent.start()` works in profile builds
- Release mode safeguards — blocked by default, opt-in via `allowReleaseBuild: true`
- WebSocket ping/pong keepalive (5s interval)
- Widget finder visibility filtering (Offstage, Visibility)
- Token file persistence for both iOS and Android

## 0.2.0

- Initial release with WebSocket server, JSON-RPC 2.0 protocol
- Widget finder: text, key, type, ordinal, positional selectors
- Touch gestures: tap, double tap, long press, swipe, scroll, drag
- Text input via TextEditingController
- Screenshot capture with base64 encoding
- Triple-signal UI synchronization
- Test recording engine
- Clipboard copy/paste
- URL launcher interception
