# Changelog

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
