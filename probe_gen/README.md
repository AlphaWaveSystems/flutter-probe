# flutter_probe_gen

`build_runner` code generator for [FlutterProbe](https://flutterprobe.dev). Reads
`@ProbeSuite`, `@ProbeTest`, and `@ProbeRecipe` annotations from
[`flutter_probe_annotation`](https://pub.dev/packages/flutter_probe_annotation)
and emits matching `.probe` test files into `tests/generated/`.

## Install

```yaml
dependencies:
  flutter_probe_annotation: ^0.9.3
  flutter_probe_agent: ^0.9.3

dev_dependencies:
  flutter_probe_gen: ^0.9.3
  build_runner: ^2.15.0
```

## Use

Annotate any Flutter class with `@ProbeSuite`:

```dart
import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  beforeEach: [Open()],
  tests: [
    ProbeTest('user can log in', steps: [
      Tap(text: 'Sign In'),
      See('Dashboard'),
    ]),
  ],
)
class LoginScreen extends StatelessWidget { /* … */ }
```

Then run:

```bash
dart run build_runner build
```

A `.probe` file is written next to each annotated source file:

```
lib/screens/login_screen.dart   →  tests/generated/screens/login_screen.probe
```

Run them with the FlutterProbe CLI:

```bash
probe test tests/
```

## How it works

The builder declares `lib/{{}}.dart` → `tests/generated/{{}}.probe`. Files
that don't mention `@ProbeSuite`, `@ProbeTest`, or `@ProbeRecipe` (cheap
text pre-check) are skipped without invoking the analyzer, so it's safe
to enable on a whole `lib/` tree.

For each annotated class, the builder reads the constant value of the
annotation, walks the nested `Step`/`Selector` graph, and emits one block
of ProbeScript per test, recipe, or hook list.

## License

MIT — © 2026 Alpha Wave Systems S.A. de C.V.
