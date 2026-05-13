# Annotation-driven test generation

Co-locate ProbeScript tests with the Flutter widgets they exercise. Two
small Dart packages — `flutter_probe_annotation` and `flutter_probe_gen` —
let you declare tests as decorators on screen classes; a `build_runner`
builder emits `.probe` files into `tests/generated/` at build time.

---

## Why

Without annotations, `.probe` files live in their own directory, divorced
from widget code. A renamed button or translated label silently breaks
tests. Annotations:

- Co-locate test intent with the widget that owns it.
- Are **type-checked by Dart** — a misspelt step name fails at
  `flutter analyze`, not at runtime.
- Generate stable `#id` selectors that the test runner already prefers.
- Don't change the rest of the FlutterProbe pipeline — the generated
  `.probe` file goes through the same parser, the same agent, the same
  reporter.

---

## Install

```yaml
# pubspec.yaml
dependencies:
  flutter_probe_annotation: ^0.9.6
  flutter_probe_agent: ^0.9.6

dev_dependencies:
  flutter_probe_gen: ^0.9.6
  build_runner: ^2.15.0
```

---

## Annotate a screen

```dart
import 'package:flutter/material.dart';
import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  name: 'Login',
  beforeEach: [Open()],
  tests: [
    ProbeTest('user can log in', tags: ['smoke', 'critical'], steps: [
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

Run the builder:

```bash
dart run build_runner build
probe test tests/        # picks up tests/generated/login_screen.probe
```

---

## Annotation reference

### `@ProbeSuite`

Top-level annotation on any Dart class. Holds tests, hooks, and recipes.

| Field | Emits |
|---|---|
| `tests: [ProbeTest(...)]` | one `test "..."` block per entry |
| `beforeEach: [Step]` | `before each test` |
| `afterEach: [Step]` | `after each test` |
| `beforeAll: [Step]` | `before all tests` |
| `afterAll: [Step]` | `after all tests` |
| `onFailure: [Step]` | `on failure` |
| `recipes: [ProbeRecipe(...)]` | one `recipe "..."` block per entry |

### `@ProbeTest`

A single test — usable inside `@ProbeSuite.tests` or as a standalone
top-level annotation.

| Field | Emits |
|---|---|
| `name` | `test "name"` |
| `tags: ['smoke']` | `@smoke` line under the test |
| `steps: [Step]` | the indented test body |
| `examples: Examples(...)` | a `with examples:` table |

### `@ProbeRecipe`

A reusable recipe with named parameters. Reference parameters as
`<paramName>` inside any string field of a step.

```dart
ProbeRecipe('sign in', params: ['email', 'password'], steps: [
  Tap(id: 'email_field'),
  Type('<email>'),
  Tap(id: 'password_field'),
  Type('<password>'),
  Tap(text: 'Sign In'),
])
```

Invoke from a test with `RecipeStep('sign in', args: ['a@b.com', 'pw'])`.

---

## Step DSL — full reference

All 31 ProbeScript verbs are covered. Every step class is `const`
constructible.

| Class | Emits |
|---|---|
| `Open()` | `open the app` |
| `OpenLink(url)` | `open link "url"` |
| `Close()` | `close the app` |
| `Restart()` | `restart the app` |
| `Kill()` | `kill the app` |
| `ClearAppData()` | `clear app data` |
| `Tap(id: 'x')` / `Tap(text: 'X')` | `tap #x` / `tap "X"` |
| `Tap(id: 'x', ifVisible: true)` | `tap #x if visible` |
| `DoubleTap` / `LongPress` | `double tap "..."` / `long press "..."` |
| `Press('home')` | `press home` |
| `GoBack()` | `go back` |
| `Type('text', into: Field(id: 'x'))` | `type "text" into #x` |
| `Clear(id: 'x')` | `clear #x` |
| `Swipe.up()` / `Swipe.down(on: ...)` | `swipe up` |
| `Scroll.down()` | `scroll down` |
| `Drag(from: ..., to: ...)` | `drag "from" to "to"` |
| `Pinch(zoomIn: true)` | `pinch out` |
| `Rotate.landscape()` | `rotate landscape` |
| `Toggle('switch')` | `toggle "switch"` |
| `Shake()` | `shake` |
| `AllowPermission('camera')` | `allow permission "camera"` |
| `DenyPermission('mic')` | `deny permission "mic"` |
| `GrantAllPermissions()` / `RevokeAllPermissions()` | `grant all permissions` |
| `CopyToClipboard('x')` / `PasteFromClipboard()` | `copy "x" to clipboard` |
| `SetLocation(lat, lng)` | `set location lat, lng` |
| `VerifyExternalBrowser()` | `verify external browser opened` |
| `TakeScreenshot('name')` / `CompareScreenshot('name')` | `take screenshot "name"` |
| `DumpWidgetTree()` / `SaveLogs()` / `Pause()` / `Log('msg')` | as named |
| `Store('value', as: 'var')` | `store "value" as var` |
| `See('X')` / `See('X', state: SeeState.enabled)` / `See('X', exactly: 2)` | `see "X"` etc. |
| `DontSee('X')` | `don't see "X"` |
| `WaitFor.duration(N)` | `wait N seconds` |
| `WaitUntil.appears('X')` / `WaitUntil.disappears('X')` | `wait until "X" appears` |
| `WaitForPageLoad()` / `WaitForNetworkIdle()` / `WaitForAnimations()` | as named |
| `If('cond', then: [...], otherwise: [...])` | `if "cond" appears` block |
| `Repeat(N, body: [...])` | `repeat N times` block |
| `RunDart('print("hi");')` | `run dart:` block |
| `Mock(method: HttpMethod.get, path: '/x', status: 200, body: '{}')` | `when the app calls GET /x` block |
| `CallHttp(method: HttpMethod.post, url: 'https://x', body: '{}')` | `call POST "https://x" with body "{}"` |
| `RecipeStep('name', args: [...])` | recipe invocation with quoted args |

---

## Selectors

```dart
// Convenience (most common)
Tap(text: 'Sign In')
Tap(id: 'login_button')

// Explicit
Tap(selector: TextSel('Sign In'))
Tap(selector: IdSel('login_button'))
Tap(selector: TypeSel('ElevatedButton'))
Tap(selector: Ordinal(2, 'Item', container: 'List'))
Tap(selector: Below('Email', anchor: 'Sign Up'))
Tap(selector: InContainer('Email', container: 'LoginForm'))
```

---

## Output layout

A file at `lib/screens/login.dart` with annotations becomes:

```
tests/generated/screens/login.probe
```

Directory structure under `lib/` is preserved so generated files are
traceable back to their source.

The generated file always starts with a `do not edit` header that includes
the source path:

```
# Generated by flutter_probe_gen — do not edit by hand.
# Source: lib/screens/login.dart

test "user can log in"
  …
```

---

## Should I commit `tests/generated/`?

Two valid workflows:

- **Commit it** — review tests as part of the PR, no need to run the
  builder in CI before `probe test`. Add a CI check that fails if the
  builder produces diffs (catches forgotten regenerations).
- **Gitignore it** — single source of truth lives in the Dart code; the
  CI pipeline runs `dart run build_runner build` before `probe test`.

Both work; pick the one that fits your team's review style.

---

## Cross-language validation

Every fixture used by `flutter_probe_gen`'s test suite is also parsed by
the Go-side ProbeScript parser in CI
(`internal/parser/golden_integration_test.go`). If the Dart emitter
produces output the Go parser can't accept, the Go test fails — bugs in
the emitter are caught immediately rather than at user runtime.
