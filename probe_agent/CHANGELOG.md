# Changelog

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
