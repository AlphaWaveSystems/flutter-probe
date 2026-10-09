# Changelog

## [Unreleased]

## 0.20.0 - 2026-10-09

- Added: loose text matching (`loose` selector flag, `probe test --match-loose`): case, accents/diacritics, typographic apostrophes and dashes, full-width forms, invisible characters and whitespace fold on both sides (`lib/src/textfold.dart`, identical to the CLI via a shared fixture). Off by default.
- Fixed (regression in 0.19.2+): `scroll down in "<anchor>" until X appears` failed with `Widget not found: text("<anchor>")` once the anchor had scrolled off screen, because the scrollable was re-resolved from the anchor on every step. It is now resolved once.
- Fixed: `wait for idle` (and the settle after every action) returned before the UI reflected a tap. A `setState` schedules a frame, but the scheduler phase still reads idle until it starts, so the next `see` read the stale tree ("UI one tap behind", intermittent, seen on bottom sheets). The settle check now lets a requested frame run first (bounded to 250 ms, so a screen that animates forever does not stall actions).

## 0.19.5 - 2026-10-09

- No agent change (iOS driver and Android dialog fixes in the CLI); version aligned.

## 0.19.4 - 2026-10-08

- No agent change (iOS `clear app data` fix in the CLI); version aligned.

## 0.19.3 - 2026-10-08

- Fixed: the target of a bare `scroll` / `swipe` ranks scrollables that can scroll (extent > 0, physics accepting user input) before ones that cannot (a non-scrollable PageView).

## 0.19.2 - 2026-10-08

- Fixed: tap, long press, double tap, type and clear scroll a built but off-screen target into view (since 0.19.0 they failed on it).

## 0.19.1 - 2026-10-08

- No agent change (iOS `clear app data` fix in the CLI); version aligned.

## 0.19.0 - 2026-10-08

- Fixed: text/id lookups ignore widgets laid out entirely off screen and elements without a render object (mid-rebuild); `screenshot` retries while a boundary still needs paint.

## 0.18.1 - 2026-10-08

- Fixed: `probe.set_time_dilation` with a non-positive factor sets 0.001 instead of an invalid 0.

## 0.18.0 - 2026-10-08

- No agent change (implicit waiting is a CLI feature); version aligned.

## 0.17.2 - 2026-10-08

- No agent change (CLI migrate fixes); version aligned.

## 0.17.1 - 2026-10-08

- Fixed: the `checked` state check was missing and passed for every widget; it now reads Switch, SwitchListTile, CupertinoSwitch, Checkbox, CheckboxListTile, Radio, RadioListTile, FilterChip and ChoiceChip.

## 0.17.0 - 2026-10-08

- Fixed: `is disabled` / `is enabled` use the control enclosing the matched text (button, icon button, FAB, text field, switch, checkbox).
- Added: `wait` accepts a `pattern` for `appears` (`matching`); text selectors fall back to Tooltip messages and Semantics labels when no visible text matches.

## 0.16.9 - 2026-10-08

- Fixed: bare `type` / `clear` (empty selector) act on the focused text field (error when none is focused).
- Fixed: `see` / `wait until` find content of a page under a see-through route (dialog, bottom sheet, popup), for example a SnackBar.

## 0.16.8 - 2026-10-08

- Fixed: `probe.see` applies the `pattern` parameter (`matching`); it was received and ignored.

## 0.16.7 - 2026-10-08

- Added: device action `press_enter` (keyboard action key on the focused text field; warns when nothing has focus).

## 0.16.6 - 2026-10-07

- Fixed: a tap whose point is received by nothing (clipped, off screen, under the keyboard) produces the covered
  warning (when the screen also did not change) instead of a plain success; the warning mentions an open keyboard.

## 0.16.5 - 2026-10-06

- Changed: `go back` at the root route is a no-op with a warning instead of leaving the app (Android).
- Fixed: the "covered by another widget" tap warning is only reported when the tap also changed nothing on screen
  (no more false positives on SnackBar actions); `tap` prefers the reachable copy among several matches.

## 0.16.4 - 2026-10-06

- Fixed: `tap` prefers the reachable widget among several matches and warns about a covered target only after it stays
  covered for ~0.6 s (FP-21).

## 0.16.3 - 2026-10-06

- Fixed: `tap` waits (up to 2 s) until the target is hit-testable after a scroll, and only warns when the widget at the
  tap point is unrelated to the target (FP-20).

## 0.16.2 - 2026-10-06

- Changed: `tap` returns a `warning` in its result when another widget covers the tap point (the tap still happens), and
  waits up to 1.5 s while a scrollable around the target is still scrolling (Flutter ignores pointer events during
  scroll activity) (FP-19).

## 0.16.1 - 2026-10-06

- No agent change. Version bump to match the CLI release 0.16.1 (the 0.16.0 CLI release failed to build on Windows; see the root
  changelog).

## 0.16.0 - 2026-10-06

- Fixed: a bare `scroll` chose the largest scrollable even when it was a hidden `IndexedStack` tab; it now prefers a
  scrollable that receives touches at its center (FP-16).
- Fixed: id selectors in error messages read `id("#key")`; they now read `#key`.
- Docs: full agent reference in the README (build flags, ports and tokens, modes, troubleshooting) and a documentation
  link on pub.dev. The agent still has no native plugin dependency.

## 0.15.0 - 2026-10-06

- Fixed: `agent_version.dart` reported 0.13.0 in 0.14.0, causing a bogus CLI/agent version-mismatch warning.
- **BREAKING (WiFi auto-discovery only):** the core agent no longer depends on `bonsoir`. A native plugin in an
  app's dependencies is linked into every build of that app, release included (size, privacy, a possible
  local-network prompt) regardless of `--dart-define=PROBE_AGENT`. mDNS advertising is now an optional hook:
  implement `ProbeAdvertiser` in your own app (copy-paste `bonsoir` example in the Studio docs) and pass it to
  `ProbeAgent.start(advertiser: ...)` in the build flavor that wants it. No separate package is published. Without
  it WiFi testing is unchanged (`--host <ip> --token <token>`) and the agent logs `PROBE_MDNS=off`. New public
  API: `ProbeAdvertiser`, `mdnsServiceType`. The advertised port is now the actually bound port (it used to be
  the preferred one even after a fallback) (FP-15).
- Fixed: the finder/executor resolved routes with `ModalRoute.of(element)`, subscribing queried elements to a
  route scope from outside a build; since 0.10.0 that could trip a framework assertion (red screen) after
  closing a sheet or dialog. Routes are now read without subscribing (`probeRouteOf`) (FP-13).

- Added: `probe.scroll` accepts `until` (selector) — scroll until the target is on screen, then
  `ensureVisible` it; stops early at the end of the list (FP-13).
- Added: `probe.wait` kind `idle` (route transitions + frames/animations/HTTP settled); `tap` waits up
  to 2 s for an in-flight route transition (FP-13).
- Added: `probe.visible_summary` RPC; `Widget not found` / `Timed out waiting for` errors now list the
  visible texts and keys (FP-13).
- Added: `--dart-define=PROBE_PORT=<n>` moves the agent off 48686 (pairs with `probe test --agent-port`).
- Fixed: `type`/`clear` assigned `controller.text`, which never reached `onChanged`/`inputFormatters`;
  they now use `EditableTextState.userUpdateTextEditingValue` like real keystrokes (FP-13).

## 0.14.0 - 2026-08-31

- Fixed: `tap #id` could invoke a Semantics-wrapped button's `onTap` directly even when something
  else (a modal barrier, a loading overlay, an unrelated `Stack` sibling) covered it on screen —
  `_tryDirectTap`'s Element-tree walk has no relationship to paint order. Now gated behind a
  read-only hit test (the same `hitTestInView` call Flutter's own pointer dispatch uses
  internally); only takes the direct-invoke fast path when the target is genuinely the topmost
  thing at its own screen position, otherwise falls through to the existing real hit-tested
  pointer tap (FP-10).

## 0.13.0 - 2026-08-15

- No agent-side changes — version kept in lockstep with the CLI's 0.13.0 release.

## 0.12.1 - 2026-08-15

- No agent-side changes — version kept in lockstep with the CLI's 0.12.1 release.

## 0.12.0 - 2026-08-15

- Fixed: the app-cache-dir copy of the ProbeAgent's reconnection token was written once, at
  server startup, to a directory Android documents as clearable by the OS at any time (confirmed
  reproducible: the file disappeared permanently, mid-session, after visiting a screen that
  touches `ImagePicker` — PT-27). The existing every-3-second `PROBE_TOKEN=` log re-print now also
  re-attempts the file write, so a cleared cache dir gets a fresh copy back within seconds instead
  of staying gone — and permanently breaking reconnection — for the rest of the session.
- Fixed: `_textOf` only read `Text`/`RichText` widgets, so `see #field contains "..."` silently
  returned an empty string for any `TextField`/`TextFormField` selector (the check always failed,
  reporting `contains "", not "<expected>"`). Now reuses the existing `_findTextController`
  up/down search (already used for tap-to-focus and `clear()`) to find the underlying
  `EditableText`'s controller when the matched widget isn't `Text`/`RichText`/`EditableText`
  itself.

## 0.11.0 - 2026-08-14

- Added: `ProbeFinder.boundsFor` and a new `probe.selector_bounds` RPC method —
  resolves a widget's full on-screen pixel bounding box (origin + size), not
  just the center point `dump_tree`/`_elementInfo` already expose. Used by the
  CLI to black out configured regions of a screenshot before it's sent to an
  AI provider for `see "..." with ai` assertions (FP-1). No behavior change
  for any existing selector/finder call.

## 0.10.4 - 2026-07-06

- Fixed: the `ordinal` selector kind (`tap 1st ...`, `tap 2nd ...`, etc.) always matched
  by displayed text only, even when combined with an `#id` selector — `1st #card_id`
  silently matched nothing, since `#card_id` isn't real on-screen text. Now checks for
  the `#` prefix and matches by key instead when present, mirroring the plain `id`
  selector kind (PT-26).

## 0.10.3 - 2026-07-06

No changes to this package's own code — bumped to stay in lockstep with the CLI's
version. This release's fixes (PT-23: recipe calls starting with "open" misparsed;
PT-24: hyphenated recipe names misparsed) are entirely in the Go CLI's parser/lexer.
Full detail in the root CHANGELOG's `[0.10.3]` section.

## 0.10.2 - 2026-07-06

No functional changes — a test-only addition (`test/sequential_focus_autofocus_test.dart`)
locking in correct sequential tap+type behavior when a field auto-focuses on page load,
after a reported regression (PT-21, reopened) was re-investigated and found not to
reproduce. Full detail in the root CHANGELOG's `[0.10.2]` section. Version bumped purely
to stay in lockstep with the CLI's version.

## 0.10.1 - 2026-07-05

No changes to this package's own code — bumped to stay in lockstep with the
CLI's version (used for the `probe.ping` version-mismatch check). This patch
release's fixes (ProbeScript's `wait for ...` parsing, plus two CI-only
automation fixes) are entirely on the Go CLI/tooling side. Full detail in the
root CHANGELOG's `[0.10.1]` section.

## 0.10.0 - 2026-07-05

A hardening release — every fix below was found and verified against a real
device, working through a backlog of real E2E test issues surfaced by driver
projects. Full detail on each is in the root CHANGELOG's `[Unreleased]`
section (also being cut as part of this release); this is the summary
relevant to apps embedding this package directly.

- `probe.ping` now returns `agent_version` (this package's version) alongside
  `ok`, and accepts an optional `client_version` field from the CLI. Part of
  the CLI↔agent version-compatibility handshake. Both fields are additive and
  ignored by older CLIs/agents that don't know about them.
- Fixed: the version reported in mDNS advertisements and `GET /probe/status`
  had drifted to `0.7.0` while this file's version moved on — corrected to
  match `pubspec.yaml` and moved into its own `agent_version.dart` file.
- Fixed: `scroll`/`swipe` could report success while producing zero visible
  movement (a single-jump synthetic drag, and stale content from a route
  mounted underneath the current one both resolving as visible).
- Fixed: `tap #id` could leave a text field genuinely unfocused, and the
  `focused` state check (`see`/`don't see #id is focused`) had a false
  positive matching ancestors of the selected element.
- Fixed: `tap #id`'s fast direct-invoke path now recognizes `InkResponse`
  (not just `InkWell`), covering more Material buttons before falling back
  to a real hit-tested pointer tap.
- Fixed: `wait until #id appears`/`disappears` always searched for the
  literal text `"#my_button"` instead of resolving the id — timed out on
  every non-text widget (icon buttons, etc.) regardless of visibility.
- Fixed: `close keyboard` and `close the app` were both complete no-ops.
  `close keyboard` now calls `FocusManager.instance.primaryFocus?.unfocus()`
  directly; `close the app` calls `SystemNavigator.pop()`.
- Fixed: `don't see #id is <state>` (negated state checks — `focused`,
  `enabled`, `disabled`, `contains`) silently ignored the checked state,
  degrading to a bare existence check.
- Fixed: `scroll` could lose the gesture arena to `Dismissible`-wrapped list
  rows and never actually scroll — now drives the nearest `Scrollable`'s own
  `ScrollPosition` directly instead of simulating a pointer gesture.
- Fixed: `take screenshot` could capture stale content from a previous route
  instead of the current screen (missing `waitForSettled()` call, and
  `_captureViaRepaintBoundary` had no route-awareness).

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
