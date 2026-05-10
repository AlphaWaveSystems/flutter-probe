# flutter_probe_annotation

Dart annotations for [FlutterProbe](https://flutterprobe.dev) — declare end-to-end
tests as decorators on your Flutter screen classes. Pair with
[`flutter_probe_gen`](https://pub.dev/packages/flutter_probe_gen) to generate
`.probe` test files at build time.

## Why

Without annotations, ProbeScript test files live in `tests/` divorced from the
widget code they exercise. Renaming a button silently breaks tests that reference
its old label. Annotations co-locate test intent with the screen that owns it,
so the test definition lives next to the widget tree it walks — and the Dart
compiler type-checks every step before any test ever runs.

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

```dart
import 'package:flutter/material.dart';
import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  name: 'Login',
  beforeEach: [Open()],
  tests: [
    ProbeTest('user can log in', tags: ['smoke'], steps: [
      Tap(id: 'email_field'),
      Type('alice@example.com'),
      Tap(id: 'password_field'),
      Type('hunter2'),
      Tap(text: 'Sign In'),
      WaitUntil.appears('Dashboard'),
      See('Dashboard'),
    ]),
    ProbeTest('shows error on bad password', steps: [
      Tap(id: 'email_field'),
      Type('alice@example.com'),
      Tap(text: 'Sign In'),
      See('Invalid credentials'),
    ]),
  ],
)
class LoginScreen extends StatelessWidget {
  const LoginScreen({super.key});

  @override
  Widget build(BuildContext context) => const Scaffold(/* … */);
}
```

Then run:

```bash
dart run build_runner build
probe test tests/        # picks up tests/generated/login_screen.probe
```

## Available steps

All 31 ProbeScript actions are supported. Common ones:

| Step | Emits |
|---|---|
| `Open()` | `open the app` |
| `Tap(id: 'login')` | `tap #login` |
| `Tap(text: 'Sign In')` | `tap "Sign In"` |
| `Type('hello', into: Field(id: 'msg'))` | `type "hello" into #msg` |
| `See('Welcome')` | `see "Welcome"` |
| `DontSee('Error')` | `don't see "Error"` |
| `WaitUntil.appears('X')` | `wait until "X" appears` |
| `WaitFor.duration(2)` | `wait 2 seconds` |
| `Swipe.up()` | `swipe up` |
| `Repeat(3, body: [...])` | `repeat 3 times` block |
| `If(condition: 'X', then: [...])` | `if "X" appears` block |
| `TakeScreenshot('login')` | `take screenshot "login"` |

See the full step reference in the package source under `lib/src/steps.dart`.

## License

MIT — © 2026 Alpha Wave Systems S.A. de C.V.
