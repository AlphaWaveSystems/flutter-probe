# Changelog

## 0.9.9 - 2026-05-13

- **`DeliverSignal(name, {value})` Step class** — emits `deliver signal "name" ["value"]`.
  Use to unblock any OS-level interaction not in the Flutter widget tree: push permission
  prompts, payment sheets, App Tracking Transparency, custom deep-link handlers, etc.

## 0.9.8 - 2026-05-12

- Version bump to match CLI v0.9.8. No annotation changes — biometric signal delivery is
  handled by the agent (`awaitBiometricResult`) and CLI; no new annotation steps needed.

## 0.9.7 - 2026-05-12

### Added

- **`EnrollBiometric`, `BiometricMatch`, `BiometricNoMatch` Step classes** —
  drive Face ID / Touch ID / fingerprint flows on iOS Simulator and
  Android emulator from annotation-authored tests. Skipped on physical
  devices with a warning. See https://flutterprobe.dev/probescript/annotations/#biometric-authentication-v097.

## 0.9.6 - 2026-05-12

### Added

- **`@ProbeCompositeTest`** — declare multi-device composite tests as
  annotations. Pair with `Device(alias, target: ...)` declarations,
  `OnDevice(alias, steps: [...])` per-device step groups, and
  `Sync(label)` cross-device barriers. The generated `.probe` block
  uses the standard `composite test` / `devices` / `sync` syntax that
  the CLI runner already understands.
- **`See.id(key)` / `See.selector(Selector)`** — target assertions by
  `ValueKey` or by rich selector (Ordinal, Below/Above/LeftOf/RightOf,
  InContainer, TypeSel). Same factories on `DontSee`. Previously,
  `See`/`DontSee` only accepted a literal text string — the parser
  always supported every selector kind but the DSL didn't expose it.
- **`WaitUntil.idAppears(key)` / `.idDisappears(key)`** — emit
  unquoted `wait until #key appears`, exercising the Go parser's
  WaitSelector branch. More reliable than text matching for widgets
  with stable `ValueKey`s.
- **Composable `See` suffixes** — `See('x', state: SeeState.enabled,
  containing: 'y')` now emits `see "x" is enabled contains "y"`
  (both suffixes present). Previously the second suffix silently
  overwrote the first.

### Changed

- **`Press` and `Pinch` are now `@Deprecated`** with a clear note —
  the Go-side parser has no `press` or `pinch` case and emitted text
  would be misparsed as a recipe call. They'll be re-enabled when
  runtime support lands. Use `GoBack()` in place of `Press('back')`.

## 0.9.5 - 2026-05-12

- Version bump to match CLI v0.9.5. No annotation API changes.

## 0.9.4 - 2026-05-09

- Version bump to match CLI v0.9.4. No annotation API changes.

## 0.9.3 - 2026-05-09

- Initial release.
- `@ProbeSuite`, `@ProbeTest`, `@ProbeRecipe` annotations for declaring
  ProbeScript tests directly on Flutter screen classes.
- Full step DSL covering all 31 ProbeScript action verbs (Tap, Type, See,
  Wait, Swipe, Scroll, Drag, Restart, Kill, ClearAppData, permissions,
  clipboard, location, screenshots, etc.).
- All 6 selector kinds (text, id, type, ordinal, positional, relational).
- Hooks: `beforeEach`, `afterEach`, `beforeAll`, `afterAll`, `onFailure`.
- Loops via `Repeat(times, body)`, conditionals via `If(condition, then, otherwise)`.
- Recipes with named parameters via `@ProbeRecipe`.
- Data-driven tests via `Examples` with column headers and rows.
- HTTP calls (`CallHttp`) and mocks (`Mock`).
- Inline Dart blocks via `RunDart(code)`.

Pair with `flutter_probe_gen` to emit `.probe` files at build time.
