# Changelog

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
