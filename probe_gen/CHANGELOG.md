# Changelog

## 0.9.9 - 2026-05-13

- **`DeliverSignal` emitter** — renders `deliver signal "name" ["value"]` for the new
  `DeliverSignal` annotation step. Omits the value argument when it equals the default
  `"true"`.
- New golden fixture `signal_delivery.probe.golden` verifying emitter output for both
  default-value and explicit-value signal delivery steps.

## 0.9.8 - 2026-05-12

- Version bump to match CLI v0.9.8. No builder or emitter changes in this release.

## 0.9.7 - 2026-05-12

### Added

- **3 new emitter cases** for `EnrollBiometric`, `BiometricMatch`,
  `BiometricNoMatch` — translate the corresponding Dart step classes
  to `enroll biometric`, `biometric match`, and `biometric no match`
  ProbeScript lines. New `biometric_auth` golden fixture exercises
  the happy-path (Face ID match) and unhappy-path (no-match) flows.

## 0.9.6 - 2026-05-12

### Fixed

- **`Mock` path is now quoted** in the emitted `when the app calls ...`
  line. Previously the path was unquoted, so the Go lexer split on `/`
  and the parser recorded only the first IDENT segment — `Mock(path:
  '/api/products')` silently became `/api`. Now emits `when the app
  calls GET "/api/products"` which round-trips correctly.
- **`See` state/containing/matching suffixes compose additively.**
  `See('x', state: SeeState.enabled, containing: 'y')` previously
  emitted only `see "x" contains "y"` (state silently dropped). Now
  emits `see "x" is enabled contains "y"` — all suffixes coexist as
  the parser supports.

### Added

- **`@ProbeCompositeTest` emission** — new `emitCompositeTest` walks
  `devices`, `OnDevice` groups, and `Sync` barriers, producing the
  standard `composite test` / `devices` / `<alias>:` / `sync` block
  layout.
- **`See.id` / `See.selector` rendering** — the emitter now reads the
  optional `id` and `selector` fields on `See`/`DontSee` and renders
  the appropriate target form. Text remains the default when no id
  or selector is provided.
- **`WaitUntil.idAppears` / `.idDisappears` rendering** — emits
  unquoted `#key` (selector form) when the DSL's `byId` flag is set.
- **6 new golden fixtures** in `probe_gen/test/fixtures/`:
  `mock_and_call`, `see_states`, `composite_chat`, `wait_variants`,
  `examples_inline`, `kitchen_sink`. The kitchen sink fixture
  exercises one of every step, selector kind, and control-flow
  construct. Every fixture round-trips through the Go-side parser
  via `internal/parser/golden_integration_test.go`.

### Changed

- **Emitter is no longer coupled to enum declaration order.**
  `_direction`, `_httpMethod`, and the `See` state name lookup now
  read the enum constant identifier (`_name` field) rather than
  indexing into a hard-coded array by `.index`. Reordering or
  inserting values in `Direction`, `HttpMethod`, or `SeeState` no
  longer silently corrupts emitted ProbeScript.

## 0.9.5 - 2026-05-12

- Version bump to match CLI v0.9.5. No annotation API changes.

## 0.9.4 - 2026-05-09

- Version bump to match CLI v0.9.4. No annotation API changes.

## 0.9.3 - 2026-05-09

- Initial release.
- `ProbeBuilder` reads `@ProbeSuite` / `@ProbeTest` / `@ProbeRecipe`
  annotations from the
  [flutter_probe_annotation](https://pub.dev/packages/flutter_probe_annotation)
  package and emits ProbeScript `.probe` files into `tests/generated/`.
- Cheap source-text pre-check skips files with no annotations so the
  builder is safe to apply to a whole `lib/` tree.
- Full coverage of all 31 ProbeScript action verbs, all 6 selector kinds,
  hooks (`beforeEach`, `afterEach`, `beforeAll`, `afterAll`, `onFailure`),
  loops (`Repeat`), conditionals (`If`/`otherwise`), recipes with named
  parameters, data-driven `Examples`, HTTP mocks, and inline Dart blocks.
- Cross-language verification: the generated `.probe` files are
  validated by FlutterProbe's Go-side parser via a CI integration test.
