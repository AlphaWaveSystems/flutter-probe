# Changelog

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
