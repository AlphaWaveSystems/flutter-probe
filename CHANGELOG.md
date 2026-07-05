# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.10.2] - 2026-07-06

A small release with no functional changes — a reported regression was
re-investigated and re-closed, with a regression test added to lock in the
verified-correct behavior going forward.

### Testing
- **Locked in sequential tap+type behavior when one field auto-focuses on
  page load (PT-21, reopened and re-closed).** A report claimed sequential
  multi-field text entry breaks specifically when a field requests focus
  during `initState`/first frame (e.g. a "remember last email" convenience
  feature) — text supposedly keeps landing in that auto-focused field
  regardless of which field is tapped afterward. Re-investigated with the
  exact discriminator described: a widget test with one field auto-focusing
  via a post-frame `requestFocus()`, plus real-device verification against
  the actual login screen with the same pattern temporarily added, in both
  tap orders. Both pass cleanly — focus correctly shifts to whichever field
  is tapped, and each field retains only its own typed value. Added a
  regression test locking this in.

## [0.10.1] - 2026-07-05

A quick patch release for regressions surfaced during v0.10.0's own
verification pass, plus an unrelated CI-automation fix found along the way.

### Fixed
- **The daily Dependabot auto-merge workflow never actually merged anything,
  for months.** Its skip condition treated `mergeStateStatus == "UNKNOWN"`
  as "already in the merge queue" — but `UNKNOWN` just means GitHub hasn't
  finished computing mergeability yet (the common case on a fresh poll), not
  "already queued." Every eligible PR was silently skipped every single day
  since at least mid-May, despite the workflow itself reporting "success."
  Separately, even a PR that had passed that check would have hit a second,
  compounding bug: the merge command itself (`gh pr merge --squash`, no
  `--auto`) doesn't work for a merge-queue-gated branch — it just prints a
  warning and does nothing, the same issue found and fixed earlier this
  release for this repo's own CI (see PT-19-adjacent commit history). Fixed
  both: only skip on `DIRTY`/`BLOCKED` (states that definitively block a
  merge), and let a merge attempt happen and fail on its own terms
  otherwise; added `--auto` to the merge command. Verified by manually
  invoking the corrected logic against the real backlog — 9 previously
  stuck PRs (some open since May) are now genuinely in the merge queue.
- **`wait for network idle`/`wait for the page to load`/`wait for page to
  load` always misparsed, silently splitting into a no-op plus a stray,
  broken statement (PT-20).** `"for"` (and `"the"`) are both global filler
  words that `parseWait`'s initial filler-strip consumes immediately after
  `"wait"` — by the time the code checked for `"for"` specifically to
  distinguish this verb family, it was already gone, so that check could
  never match. The parser fell through to a default case that produced a
  `WaitPageLoad` step (the right kind, by coincidence) but never consumed
  "network idle"/"page to load" — those leftover words became a second,
  separate statement that then failed at runtime as an unknown recipe
  call. Invisible to `probe lint`, since both halves parse as individually
  valid syntax; only surfaces when the test actually runs. This bug has
  existed since the project's very first commit — not a regression from
  any recent release, despite surfacing during this release's own
  verification pass. Fixed by detecting the network/page-load cases
  directly (mirroring the pattern `wait for animations to end` already
  used for the same underlying issue) and consuming the full phrase before
  the next token is checked.
- **`.github/workflows/e2e.yml`'s CI E2E suite failed `flutter pub get` outright
  on every run** (`"name" field doesn't match expected name "probe_agent"`).
  The workflow's `dependency_overrides` block still referenced the package's
  old pre-rename name (`probe_agent`) instead of `flutter_probe_agent` — a
  stale reference to a rename that happened well before this release. Found
  when the `v0.10.0` tag push triggered this workflow directly (it isn't
  gated on regular PRs, only tag pushes, which is why it went unnoticed
  through several prior releases). Unrelated to this release's own changes;
  fixed by updating the override key to the current package name.

## [0.10.0] - 2026-07-05

A hardening release working through a backlog of real E2E test issues
surfaced by driver projects (`IMPROVEMENT_TASKS.md`, PT-01 through PT-19).
Every fix was reproduced and verified against a real device before shipping,
and several turned out to have a different (or larger) root cause than
originally reported — noted inline below.

### Added
- **`agent.launch_timeout` config option (and `--launch-timeout` flag)
  (PT-10).** `restart the app`/`clear app data` used to be bounded by a
  hardcoded, unconfigurable 90s step timeout — no amount of raising
  `dial_timeout`/`token_read_timeout` could help an app whose actual
  cold-launch path does non-trivial async work (e.g. Firebase App Check
  re-initialization costing 90-100s) before becoming interactive. Defaults
  to 120s; raise it for apps with an expensive startup path.
- **Verbose connect diagnostics (PT-01).** `-v`/`--verbose` on `probe test` now
  traces every step of the connect handshake — ADB setup, port forward
  setup/teardown, each Android token-read source attempted (run-as,
  `/data/local/tmp`, logcat) with hit/miss detail, WebSocket dial attempts
  (including transient retries), and handshake accept/reject. Connect
  failures were previously a total black box with zero diagnostic output;
  see PT-01 in `IMPROVEMENT_TASKS.md`.
- **CLI↔agent version handshake (PT-07).** `probe.ping` now carries
  `client_version` (CLI → agent) and `agent_version` (agent → CLI) alongside
  the existing `{"ok":true}` response. Every connect path (WebSocket dial,
  HTTP dial, relay, and reconnect-after-restart) now logs a warning when the
  CLI and agent versions differ, and hard-fails with a clear error when they
  have different major versions. Older CLIs/agents that don't send or
  recognize these fields degrade gracefully — an empty/missing version is
  always treated as "unknown," never as a mismatch. Addresses PT-07 in
  `IMPROVEMENT_TASKS.md` — CLI/agent version drift was a standing suspect in
  unexplained connection failures with zero signal from the tool itself
  until now.

### Changed
- **ProbeScript now errors loudly instead of silently no-oping on several
  classes of malformed script (PT-02):**
  - An **unknown recipe call** (typo, or the recipe was never defined/isn't
    loaded from `recipes_folder`/`use`) is now a runtime error, not a silent
    skip. This was the single highest-leverage fix in `IMPROVEMENT_TASKS.md`
    — a silently-skipped call could mask a completely broken flow (e.g. a
    sign-in recipe that never actually signs anyone in) indefinitely, with
    every downstream test still reporting green.
  - An **unquoted `<placeholder>`** (e.g. `type <email>` instead of
    `type "<email>"`) is now a parse error. Angle brackets have no meaning
    outside a quoted string in ProbeScript's grammar; unquoted, both brackets
    were silently dropped by the lexer, leaving a bare identifier that gets
    typed/matched as literal text with no indication anything was wrong.
  - **`else` is now accepted as an alias for `otherwise`.** Previously `else`
    lexed as a plain identifier, was silently treated as an unknown recipe
    call (a sibling step of the `if`, not nested inside it), and its body ran
    **unconditionally** on every run regardless of the `if` condition —
    exactly the opposite of what the test author intended, with no error
    anywhere.
  - `resolve()`'s placeholder-substitution loop is now bounded. A variable
    bound to a value containing its own placeholder marker (e.g. passing the
    unquoted literal `<email>` as a recipe argument) previously looped
    forever substituting the same text for itself, hanging the CLI with no
    error; it now terminates, leaving the placeholder unresolved. (Full
    positional/named argument forwarding into nested recipe calls — a
    larger, separate redesign of how recipe calls are matched against
    definitions — is intentionally not part of this change; see PT-02's
    "Update" note in `IMPROVEMENT_TASKS.md`.)

### Fixed
- **`drag <selector> to <selector>` — the documented syntax — always failed
  to parse (PT-19).** `"to"` was lexed as its own token but was never
  actually consumable anywhere: it was missing from the parser's
  filler-word list and used nowhere else in the grammar. The parser choked
  on `to` where it expected the second selector to start, and the rest of
  the line got misparsed as an unrelated recipe call. Found during a final
  regression pass across the full e2e suite before this release — this bug
  has always existed; PT-02's error-loudly fix (above, same release) is what
  first surfaced it, since it previously failed silently instead.
- **`kill the app` followed by any step other than `open the app`/
  `restart the app` (a `wait`, `tap`, `see`, etc.) still hung and then
  permanently failed (PT-18, follow-on from PT-09's `open the app` fix).**
  The generic step-level auto-reconnect only re-dials, assuming the app
  process is already running — after a genuine `kill the app`, nothing is
  listening at all, so re-dialing could never succeed regardless of
  remaining attempts. Detects a connection-refused dial failure
  specifically (as opposed to a timeout/reset on a still-alive process,
  which a transient network drop would produce) and relaunches the app
  before retrying, the same way the `open the app` fix does, but from the
  generic reconnect path so it now covers every verb, not just `open`.
- **`open the app` after `kill the app` never actually relaunched the app
  (PT-09).** It always sent an RPC over the (now-closed) connection, which
  failed; the generic step-level auto-reconnect that kicked in afterward
  only re-dials assuming the app process is already running — it never
  relaunches one that was genuinely force-stopped. In practice this meant
  the documented `kill the app` → `open the app` pattern hung through the
  full reconnect-retry window and then failed with a connection-refused
  error. `open the app` now detects a dead connection and relaunches the
  app the same way `restart the app` does, before reconnecting. Found
  while documenting cross-`test`-block state behavior (see Documentation
  below) — not what that investigation originally set out to find, but
  blocking anyone who'd try to use `kill the app`/`open the app` for
  exactly the per-test isolation those docs describe as the supported way
  to opt out of the default shared-state behavior.
- **`take screenshot` could capture stale content from the previous route
  instead of the current screen (PT-16).** Found while investigating a
  scroll bug (PT-03): a screenshot taken right after navigating sometimes
  looked unchanged, even though `see`/`don't see` assertions confirmed real
  navigation had happened. Two independent causes:
  - Unlike every other verb, `screenshot` never called
    `_sync.waitForSettled()` before capturing — a capture taken right after
    navigation could land mid-route-transition instead of waiting for the
    push/pop animation to finish.
  - `_captureViaRepaintBoundary` picked the largest `RenderRepaintBoundary`
    in the *entire* element tree with no route-awareness — since `Navigator`
    keeps the previous route mounted underneath the current one, and both
    routes typically produce a same-size, screen-sized boundary, the strict
    `area > bestArea` comparison kept whichever one was visited first (the
    previous route, in `Overlay` insertion order), silently capturing the
    old screen instead. Same class of bug as PT-03/PT-15, just in the
    screenshot path instead of `ProbeFinder`/`scroll`. Fixed by skipping
    boundaries belonging to a non-current route, mirroring `ProbeFinder`'s
    existing fix.
- **`scroll` could lose the gesture arena to `Dismissible`-wrapped list rows
  and never actually scroll (PT-15).** `scroll` was a thin delegate to
  `swipe`'s pointer-gesture simulation, which has to *win* the gesture arena
  against any competing recognizer along the way — a `Dismissible` row's own
  `HorizontalDragGestureRecognizer` could still intercept it. Reproduced
  against a real iOS simulator: a 50-item list with `Dismissible` rows never
  scrolled past the first screen, while the identical verb worked fine on a
  plain list. `scroll`'s job is "reveal more content," unlike `swipe` (which
  tests a real gesture interaction like swipe-to-dismiss) — it doesn't need
  to enter the gesture arena at all, so it now drives the nearest
  `Scrollable`'s own `ScrollPosition` directly instead, sidestepping the
  competition entirely. When no selector is given, picks the Scrollable with
  the largest viewport (a `TextField`'s own internal cursor-scrolling
  `Scrollable` could otherwise be found first in tree order and silently
  "scroll" nothing visible).
- **`scroll`/`swipe` could report success while producing zero visible
  movement (PT-03).** Root-caused two independent bugs by reproducing
  against a real iOS simulator:
  - The synthetic drag gesture sent a single `PointerMoveEvent` covering the
    entire distance in one jump. Real touches (and Flutter's own gesture
    arena / scroll physics) expect a sequence of incremental moves — a
    single jump could fail to register as a scroll at all. Now split into
    10 incremental steps, matching a real drag.
  - The widget-tree finder had no concept of "which mounted route is
    actually the current one." Flutter's `Navigator` keeps previous routes
    mounted underneath the current one by default (no `Offstage` wrapper),
    so a screen reached via a stacked push could have several live
    `Scrollable`s (and other matching widgets) simultaneously — one per
    mounted route — and every selector-based verb (not just scroll/swipe)
    could silently resolve to a widget on a route the user can no longer
    see. Fixed by checking `ModalRoute.of(element)?.isCurrent` in the
    finder's core visibility check, so this also fixes false-positive
    `see`/`wait until` matches against stale content underneath the current
    screen — confirmed via a real-device repro where `don't see "<home page
    text>"` incorrectly found 2 elements after navigating away from Home,
    and now correctly finds none.

  A `scroll until #id visible` verb (combining corrected scroll targeting
  with polling) was in PT-03's original scope but is intentionally not part
  of this change — it's a larger, separate feature addition.
- **`close keyboard` and `close the app` were both complete no-ops (PT-12).**
  Both parse to the same `ActionStep` (`close`/`close keyboard`/`close the
  app` only differ in an argument name) and dispatch to
  `probe.device_action` with `action:"close"` — but the Dart agent's
  `_deviceAction` switch never had a `'close'` case at all, so neither did
  anything, silently. (The originally reported theory — an OS-level gesture
  colliding with iOS's Back-swipe — wasn't the actual cause: there was no
  gesture-based implementation in the first place.) Fixed by adding the
  missing case: `close keyboard` now calls
  `FocusManager.instance.primaryFocus?.unfocus()` directly in the Flutter
  widget tree (immune to any OS gesture collision, per the original PT-12
  suggestion), and `close the app` calls `SystemNavigator.pop()`.
- **The `focused` state check (`see`/`don't see #id is focused`) had a
  false-positive: it also matched *ancestors* of the selected element**, not
  just the element itself or its descendants. Once nothing more specific is
  focused (e.g. immediately after `FocusNode.unfocus()`), Flutter falls back
  to the enclosing `ModalRoute`'s own `FocusScopeNode` holding primary focus
  — and that scope is an ancestor of every widget on the current screen, so
  the old ancestor walk reported *all of them* as focused. Found while
  verifying the `close keyboard` fix above: `don't see #field is focused`
  kept failing right after a real, successful unfocus. Fixed by removing the
  ancestor walk, keeping only the direct match and the subtree walk (which
  already correctly handles a selector matching a composite widget like
  `TextField`, not the `EditableText` it builds internally).
- **`wait until #id appears`/`disappears` always searched for the literal
  text `"#my_button"` instead of resolving the id (PT-06).** `WaitStep`
  only carries a raw target string, not a selector kind (unlike
  `Selector`/`SelectorParam` used by `tap`/`type`), and the Dart agent's
  wait loop never checked for the `#` prefix before building a selector —
  it always built a *text* search, which can never match a non-text widget
  like an icon button. This meant `wait until #id appears` timed out on
  indisputably mounted, visible widgets every time, forcing real projects
  to fall back to hardcoded `wait N seconds` sleeps instead. Fixed by
  detecting the `#` prefix and dispatching an id selector, mirroring the
  same pattern `if`/`otherwise` conditionals already use. (The originally
  reported theory — that nesting inside `Material`/`Tooltip` breaks
  Semantics-based resolution — didn't independently reproduce: id
  resolution walks the full element tree unconditionally regardless of
  wrapper widgets, and a plain text search never touches Semantics at all,
  so nesting depth was never actually the cause.)
- **`tap #id`'s fast direct-invoke path only recognized `GestureDetector`/
  `InkWell`, missing `InkResponse`-based buttons (PT-05).** `InkWell` is
  just an `InkResponse` subclass with a fixed splash shape, and modern
  Material buttons (`IconButton`, `ElevatedButton`, etc.) commonly build an
  `InkResponse` directly — the old check missed them, always falling
  through to the slower synthetic-tap fallback. Verified (with new tests)
  that PT-05's actual reported symptom — `tap #id` on a button with no
  `onTap` `SemanticsAction`, or shadowed by an overlapping Semantics node —
  already worked correctly via that fallback: `Semantics` doesn't
  participate in hit-testing at all, so a real synthetic tap at the node's
  geometric center reaches whatever is actually rendered there regardless
  of Semantics-tree structure. No fix was needed for that path itself; this
  change only broadens the faster path to cover more cases before falling
  back to it.
- **`tap #id` could report success while leaving a text field genuinely
  unfocused (PT-04).** A real pointer tap on a text field requests focus as
  part of `EditableText`'s own internal tap handling; probe's tap paths
  (both the Semantics-direct-invoke fallback and the synthetic pointer tap)
  don't reliably reach that internal recognizer — a Semantics wrapper or a
  surrounding `GestureDetector`/`InkWell` can intercept the tap first. Now
  `tap #id` (and `type` when used without a preceding `tap`) explicitly
  requests focus on the field's real `FocusNode`, found by resolving down
  to the underlying `EditableText` the same way `type` already resolves its
  `TextEditingController`.
  - `see #id is focused` could never actually detect focus on a
    `TextField`/`TextFormField` for the same underlying reason: the check
    only walked *ancestors* looking for the focused widget, but the
    actually-focused widget (`EditableText`) is a *descendant*. Fixed to
    also walk down the subtree.
  - `don't see #id is focused` (and other negated state checks — `enabled`,
    `disabled`, `contains`) silently ignored the state entirely, reporting a
    failure as soon as the element existed at all. Now correctly passes when
    the element exists but isn't in that state.
- **Added regression test coverage confirming text selectors already resolve
  `ListTile` title/subtitle independently (PT-11).** The reported symptom —
  a title text selector failing on iOS 26.3+ because "the OS accessibility
  layer merges the title and subtitle into one combined node" — describes
  real platform (VoiceOver/XCUITest) behavior, but doesn't apply to probe:
  `_findByText` walks the live Flutter element tree directly and never
  touches the platform accessibility tree. `ListTile` builds title/subtitle
  as fully independent `Text` widgets with no merging anywhere in the
  element tree, so each already resolves correctly regardless of platform.
  No code fix was needed; added a test locking in this behavior since it
  wasn't previously covered.
- **`probe test` could fail with zero diagnostic output (PT-14).** `testCmd`
  sets `SilenceErrors: true` so a failed *test* (already reported in detail
  by the runner) doesn't print a redundant generic line — but this silenced
  every other kind of error too (token read, connect, handshake failures),
  leaving nothing printed beyond whatever progress lines ran before the
  failure. Now only `errTestFailed` (and errors wrapping it) stay silent;
  every other error is printed. Also fixed a related cosmetic bug this made
  visible: the "is the app running with probe_agent?" suggestion was
  appearing twice in token-read failure messages (once from the low-level
  error, once from the wrapping call site).
- **Android token read could pick up a stale token from a dead process,
  causing an immediate non-retryable connection failure (PT-01).** `logcat -d`
  dumps the entire ring buffer since it was last cleared/the device booted;
  on any device that's run more than one probe session it can contain
  `PROBE_TOKEN=` lines from multiple app-process generations, including
  already-exited ones. The token-read fallback took the *first* matching
  line, which could belong to a dead process — the live agent then rejects
  that token with a non-retryable "bad handshake." Fixed to take the *last*
  (most recent) matching line instead, since the agent reprints its token
  every ~3s. Reproduced and confirmed fixed against a real Android emulator
  — a leftover process from an earlier test run was still present in the
  log buffer, and the CLI connected successfully once the fix was applied.
  This is a confirmed root cause for at least one of PT-01's two reported
  failure shapes ("instant exit ~0.5-0.6s"); the other shape (hangs the
  full configured timeout) was not reproduced in this environment and
  remains open — the tracing above should make it diagnosable if it recurs.
- **Android `probe test` never passed the app's bundle ID into token
  reading (PT-01)**, so the fastest, most reliable token source (`run-as
  <appID> cat cache/probe/token`) was silently dead code in the main
  `probe test` path — only the slower `/data/local/tmp` and `logcat`
  fallbacks ever ran. Now threads `project.app` through.
- **`flutter_probe_agent`'s reported version had drifted to 0.7.0** while
  `pubspec.yaml` moved on to 0.9.9 across several releases — the agent's
  mDNS advertisement and `GET /probe/status` endpoint were both silently
  reporting a stale version. Fixed to match `pubspec.yaml`, and moved into
  its own `agent_version.dart` file with a comment documenting that it must
  be bumped by hand alongside `pubspec.yaml` going forward.

### Documentation
- **Clarified that all `test` blocks (and hooks) in a `.probe` file share
  one continuous app instance and connection by default — nothing resets
  app/session state between them (PT-09).** Investigated the reported
  symptom (a later test failing as if session state had been silently
  reset) and it doesn't reproduce: there is no code path that resets
  anything between blocks. Documented this explicitly in `hooks.md`, along
  with how to opt into per-test isolation (`restart the app`/
  `clear app data`/`kill the app` in `before each`) for projects that want
  it. Also corrected an inaccurate claim in `app-lifecycle.md` that
  `open the app` always launches via ADB/simctl "not through the Dart
  agent" — true only when there's no live connection yet; otherwise it's a
  no-op RPC to the already-running agent.
- **Documented an API stability/deprecation policy for `flutter_probe_agent`'s
  public Dart API (PT-08)**, in `CONTRIBUTING.md`. Prompted by a past
  breaking change (a minor bump silently removed `ProbePlugin`/
  `ProbePluginRegistry`, which at least one downstream project had a
  load-bearing test feature built on). Future removals/breaking changes now
  require a `@Deprecated` window of at least one minor version plus an
  explicit CHANGELOG migration note before actual removal. Whether
  `ProbePlugin`/`ProbePluginRegistry` should be reintroduced under this
  policy, or that removal should stand as a permanent scope decision, is
  left as an open product question rather than resolved here.
- **Documented the native (non-Flutter) UI boundary explicitly (PT-13)**:
  image/file pickers, share sheets, and the handful of permission prompts
  that can't be bypassed by an OS-level grant are invisible to every
  selector-based verb, by design — probe drives the Flutter widget tree via
  an in-process Dart agent, and native OS UI never enters it. Noted that
  `take screenshot`/video recording already capture this content (the full
  physical screen, not just the Flutter view) even though no verb can
  select or tap inside it yet, and documented the current workarounds
  (design around it, or a test-only in-app bypass). A design proposal for a
  full native-UI bridging mode was written up separately, not implemented
  in this release.

## [0.9.9] - 2026-05-13

### Added
- **`deliver signal "name" ["value"]`** — new ProbeScript step that resolves
  a pending `awaitSignal(name)` call in the Flutter app. Use to unblock any
  OS-level interaction that isn't in the Flutter widget tree: push permission
  dialogs, payment sheets, App Tracking Transparency, deep-link handlers, etc.
  The value defaults to `"true"` when omitted.
- **`awaitSignal(String name)`** — new public function exported from
  `flutter_probe_agent`. Returns a `Future<String>` that resolves with the
  value sent by the CLI. Generalises the `awaitBiometricResult()` pattern to
  any named signal.
- **`DeliverSignal(String name, {String value})`** — new annotation step class
  in `flutter_probe_annotation`. Emits `deliver signal "name"` or
  `deliver signal "name" "value"`.
- Parser: `TOKEN_DELIVER`, `TOKEN_SIGNAL`, `VerbDeliverSignal`.
- Agent: `probe.signal` JSON-RPC method, `ProbeMethods.signal` constant.

## [0.9.8] - 2026-05-12

### Fixed
- **Biometric no-match on iOS 26+ simulator** — `notifyutil` no-match notifications
  no longer resolve `LAContext.evaluatePolicy` on iOS 26 / Xcode 26.5. The CLI now
  sends a `probe.biometric_signal {result: bool}` JSON-RPC command to the agent
  after firing platform-level notifications. Test apps call `awaitBiometricResult()`
  from `flutter_probe_agent` instead of `local_auth.authenticate()` in PROBE_AGENT
  builds; the agent resolves a Dart `Completer` with the CLI-delivered result, making
  biometric no-match reliable on all iOS simulator versions.

### Added
- **`awaitBiometricResult()`** — new public function in `flutter_probe_agent`.
  Returns a `Future<bool>` that resolves when the CLI delivers
  `probe.biometric_signal`. Usage pattern in test apps:
  ```dart
  final ok = const bool.fromEnvironment('PROBE_AGENT')
      ? await awaitBiometricResult()
      : await localAuth.authenticate(...);
  ```
- **`probe.biometric_signal`** — new JSON-RPC method. Sent by the CLI after
  the platform-level biometric simulation commands so the result is always
  delivered regardless of simulator version behavior.

## [0.9.7] - 2026-05-12

### Added
- **Biometric authentication testing** — three new ProbeScript steps that drive Face ID / Touch ID / fingerprint flows on iOS Simulator and Android emulator without real hardware. Skipped on physical devices with a warning (same pattern as `set location` and other simulator-only ops).
  - `enroll biometric` — marks the simulator/emulator as having an enrolled face or finger. iOS posts the `com.apple.BiometricKit.enrollmentChanged` Darwin notification via `xcrun simctl spawn booted notifyutil`. Android requires the fingerprint to be pre-enrolled in Settings.
  - `biometric match` — simulates a successful capture, satisfying any pending biometric prompt. iOS posts `*_Sim.faceCapture.match` AND `*_Sim.fingerTouch.match` so the same step works on Face ID and Touch ID devices. Android runs `adb -s <serial> emu finger touch 1`.
  - `biometric no match` — simulates a failed capture so the app's "authentication failed" path can be tested. iOS posts the `.no-match` variants; Android runs `adb emu finger touch 9999` (an unregistered id).
  - **Annotation DSL**: matching `EnrollBiometric()`, `BiometricMatch()`, `BiometricNoMatch()` const Step classes in `flutter_probe_annotation`, with a new `biometric_auth` golden fixture in `flutter_probe_gen/test/fixtures/` that round-trips through the Go parser via the cross-language integration test.
  - **Parser**: 2 new tokens (`TOKEN_BIOMETRIC`, `TOKEN_ENROLL`), 3 new `ActionVerb` constants, 2 new parser dispatch cases. 3 new unit tests in `parser_test.go`.
  - **Runner**: `EnrollBiometric` / `BiometricMatch` / `BiometricNoMatch` methods on `DeviceContext`, dispatch cases in `Executor.runAction`, and human-readable strings in `stepDescription`.
  - **Docs**: new section in [annotations.md](https://flutterprobe.dev/probescript/annotations/#biometric-authentication-v097) and [syntax.md](https://flutterprobe.dev/probescript/syntax/#biometric-authentication) on the website. Per-package CHANGELOGs updated.

## [0.9.6] - 2026-05-12

### Fixed
- **`flutter_probe_gen`: `Mock` path silently truncated.** The emitter wrote the path unquoted (`when the app calls GET /api/products`), so the Go lexer split on `/` and the parser only recorded the first IDENT segment. Now emits the canonical quoted form. Caught by a new `mock_and_call` golden + the existing cross-language integration test.
- **`flutter_probe_gen`: `See` suffixes silently dropped.** When `state`, `containing`, and `matching` were all set on a single assertion, only the last branch's text reached the output. Now composes all three suffixes additively: `see "x" is enabled contains "y" matching "z"`. Caught by a new `see_states` golden covering the matrix.

### Added
- **`flutter_probe_annotation`: `@ProbeCompositeTest` annotation.** The flagship multi-device composite testing feature finally has a DSL surface. Pair with `Device(alias, target: …)`, `OnDevice(alias, steps: […])` per-device groups, and `Sync(label)` cross-device barriers. Emitter generates standard `composite test` / `devices` / `<alias>:` / `sync` blocks that the existing CLI runner picks up unchanged.
- **`flutter_probe_annotation`: `See.id` / `See.selector` factories** — assertions can now target by `ValueKey` or any rich selector (Ordinal, Below/Above/LeftOf/RightOf, InContainer, TypeSel) rather than only by literal visible text. Same factories on `DontSee`. The Go parser always supported this; the DSL just didn't expose it.
- **`flutter_probe_annotation`: `WaitUntil.idAppears` / `.idDisappears`** — emits unquoted `#key` selector form (Go parser's WaitSelector branch), which is more reliable than text matching for stable `ValueKey`-tagged widgets.
- **`flutter_probe_gen`: 6 new golden fixtures.** `mock_and_call`, `see_states`, `composite_chat`, `wait_variants`, `examples_inline`, `kitchen_sink`. The kitchen sink fixture exercises one of every step, selector kind, and control-flow construct. Every fixture round-trips through `internal/parser/golden_integration_test.go`. Total golden coverage went from 4 → 10 fixtures, builder tests from 5 → 11.

### Changed
- **`flutter_probe_annotation`: `Press` and `Pinch` are now `@Deprecated`.** The Go parser has no `press` or `pinch` case, so emitted text fell through to `parseRecipeCall` and was misinterpreted. Marked deprecated until runtime support lands. Use `GoBack()` in place of `Press('back')`.
- **`flutter_probe_gen`: emitter no longer coupled to enum declaration order.** `_direction`, `_httpMethod`, and the `See` state lookup now read the enum constant identifier (`_name` field) instead of indexing a hard-coded array by `.index`. Reordering `Direction`, `HttpMethod`, or `SeeState` no longer silently corrupts emitted ProbeScript.

### Docs
- New website page: [Annotation-driven Tests](https://flutterprobe.dev/probescript/annotations/) — full reference for the annotation DSL with every step class, selector kind, and the new composite test syntax.

## [0.9.5] - 2026-05-12

### Fixed
- **Dart agent: iOS / Impeller screenshots** — `take_screenshot` previously called `OffsetLayer.toImage()` on the root render view. On iOS with the Impeller renderer (Flutter's default on iOS 17+), that returns a GPU-backed texture whose `toByteData(ImageByteFormat.png)` is `null`, so capture silently produced nothing. The agent now primarily captures via the largest visible `RenderRepaintBoundary` in the widget tree (Impeller-supported); the legacy `OffsetLayer` path is only used as a fallback when no boundary is found (Skia). Also awaits `WidgetsBinding.instance.endOfFrame` before capture so the latest frame is always in the image, and uses the actual `View.devicePixelRatio` rather than a hard-coded `2.0`.

## [0.9.4] - 2026-05-09

### Added
- **Claude Desktop Extension (`.mcpb`)** — `probe-mcp` is now packaged as a one-click Claude Desktop Extension. Users drag `flutter-probe-<platform>-<arch>.mcpb` onto Claude Desktop's Extensions settings, pick their Flutter project directory, and all 18 MCP tools are available. No `brew install`, no JSON config editing, no PATH setup. CI builds and attaches `.mcpb` artifacts for darwin-arm64, darwin-amd64, linux-amd64, and windows-amd64 to every release.
  - Manifest declares `server.type: "binary"` and bundles the platform-specific `probe-mcp` executable.
  - `user_config.projectRoot` is a directory picker surfaced as the `PROBE_PROJECT_DIR` env var.
  - `probe-mcp` reads `PROBE_PROJECT_DIR` at startup and `os.Chdir`s into it so `run_tests`, `list_files`, `get_report`, and `init_project` resolve paths against the user's Flutter project rather than Claude Desktop's working directory.
  - `scripts/build-mcpb.sh` produces a bundle locally from any built `probe-mcp` binary; uses `npx @anthropic-ai/mcpb` for schema validation and packing.
  - New `build-mcpb` job in `.github/workflows/release.yml` runs after `build`, downloads the per-platform `probe-mcp` artifacts, packs them into `.mcpb` bundles, and attaches them to the GitHub release.

## [0.9.3] - 2026-05-09

### Added
- **Annotation-driven test generation** — two new Dart packages, `flutter_probe_annotation` and `flutter_probe_gen`, let you declare ProbeScript end-to-end tests as decorators on your Flutter screen classes. A `build_runner` builder reads the annotations at build time and emits matching `.probe` files into `tests/generated/`, picked up unchanged by `probe test`.
  - **Annotation API**: `@ProbeSuite`, `@ProbeTest`, `@ProbeRecipe` plus a fully type-checked step DSL covering all 31 ProbeScript action verbs (Tap, Type, See, Wait, Swipe, Scroll, Drag, Restart, Kill, ClearAppData, permissions, clipboard, location, screenshots, etc.), all 6 selector kinds (text, id, type, ordinal, positional, relational), hooks (`beforeEach`, `afterEach`, `beforeAll`, `afterAll`, `onFailure`), loops (`Repeat`), conditionals (`If`/`otherwise`), recipes with named parameters, data-driven `Examples`, HTTP mocks (`Mock`), and inline Dart blocks (`RunDart`).
  - **Builder**: declares `lib/{{}}.dart` → `tests/generated/{{}}.probe`. Cheap text pre-check skips files without annotations so it's safe to enable on a whole `lib/` tree. Each annotated class becomes a top-level `test "..."` block with optional tags and step body.
  - **Cross-language validation**: every Dart-emitted golden is parsed by the Go-side parser in CI (`internal/parser/golden_integration_test.go`), so a malformed emitter line is caught immediately rather than at user runtime.
  - **Docs**: see [`docs/wiki/Annotations.md`](docs/wiki/Annotations.md) for the full reference.

## [0.9.2] - 2026-05-09

### Added
- **Real-time step feedback** — the runner now emits progress during test execution instead of staying silent until a step completes:
  - **Pre-step indicator** (verbose mode): prints `→ step description` immediately before each step runs. On a TTY the line is overwritten in place by the `✓/✗` result when the step finishes (clean single-line-per-step output). On non-TTY (CI) both lines are appended.
  - **Progress ticker** (all modes): a goroutine fires every 5 seconds while a step is still running and prints `⏱ step... (Ns)`. Stops immediately when the step completes — fast steps produce no ticker output.
  - **Timeout warning** (all modes): when a step has consumed ≥ 80% of its context deadline, a one-time `⚠ step still running — Ns elapsed, Ns timeout` warning is printed. Gives time to react before the step times out.
  - **Non-verbose TTY status line**: even without `--verbose`, a faint `\r`-overwriting status line shows the current step name while it runs and is cleared when it finishes. No output on non-TTY so CI logs stay clean.

## [0.9.1] - 2026-05-09

### Added
- **MCP: 3 new tools** — `init_project` (scaffold `probe.yaml` + `tests/`), `generate_report` (HTML from JSON results), `record` (record interactions → `.probe` file with configurable timeout)
- **MCP: Android emulator shutdown** — `shutdown_device` now accepts `serial` for Android emulators in addition to `udid` for iOS simulators
- **MCP: composite test support** — `run_tests` exposes a `composite_devices` parameter (`"A=host:port/token B=udid"`) that maps directly to `--composite-device` flags; `write_test` description documents the full `composite test` syntax
- **MCP: `run_tests` flags documented** — tool description now enumerates key flags (`--timeout`, `--format`, `--parallel`, `--shard`, `--host/--token`, `--disable-animations`, `--video`, `--stream`)
- **MCP: `write_test` validates before writing** — content is parsed in-process before the file is created; syntax errors are returned immediately without touching the filesystem

### Fixed
- **MCP: `get_report` used alphabetical order instead of modification time** — `get_report` and `generate_report` now find the most recently *modified* JSON file in `reports/`, not the lexicographically last one

## [0.9.0] - 2026-05-09

### Added
- **Composite tests** — a new `composite test` keyword lets a single `.probe` file orchestrate multiple devices simultaneously. Declare device aliases (`A`, `B`, `C`, …), write per-device step blocks (`A: tap "Login"`), and use `sync "label"` as a cross-device barrier that all goroutines must reach before any proceeds. Supports N devices. Files with composite tests coexist with regular `test` blocks. Configure devices via `--composite-device A=host:port/token` (WiFi), `--composite-device B=<udid>` (iOS simulator), or `--composite-device C=emulator-5554` (Android). Probe.yaml can also set `composite.devices` for project-level defaults. If composite devices are not configured at runtime, composite tests are reported as SKIPPED.
- **Failure semantics**: if one device fails mid-test, the shared context is cancelled immediately; all other devices are unblocked from any `sync` barrier they are waiting at and their next step returns a cancelled error. The final result marks the root cause device as FAIL and secondary devices as CANCELLED.

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
