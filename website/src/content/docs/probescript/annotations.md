---
title: Annotation-driven Tests
description: Declare ProbeScript tests as Dart annotations on your Flutter widget classes — type-checked by the compiler, generated to .probe at build time.
---

Annotations let you keep test definitions next to the widget code they exercise. Two Dart packages handle the loop:

- **[`flutter_probe_annotation`](https://pub.dev/packages/flutter_probe_annotation)** — `@ProbeSuite`, `@ProbeTest`, `@ProbeRecipe`, `@ProbeCompositeTest` decorators plus a fully type-checked step DSL.
- **[`flutter_probe_gen`](https://pub.dev/packages/flutter_probe_gen)** — `build_runner` builder that reads the annotations and emits matching `.probe` files into `tests/generated/`.

A renamed button or translated label no longer silently breaks tests — selectors live in the same file as the widget that owns them, and every step is type-checked by `flutter analyze` before it ever runs.

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

Run the builder once per change:

```bash
dart run build_runner build
```

A `.probe` file is written under `tests/generated/` for every annotated Dart file:

```
lib/screens/login_screen.dart   →   tests/generated/screens/login_screen.probe
```

Then run them with the regular CLI:

```bash
probe test tests/
```

## Annotations

### `@ProbeSuite`

Top-level annotation on any class. Groups tests, hooks, and recipes that share setup logic.

```dart
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
  ],
)
class LoginScreen extends StatelessWidget { /* … */ }
```

| Field | Emits |
|---|---|
| `tests` | one `test "name"` block per `ProbeTest` |
| `beforeEach` / `afterEach` | `before each test` / `after each test` |
| `beforeAll` / `afterAll` | `before all tests` / `after all tests` |
| `onFailure` | `on failure` hook |
| `recipes` | one `recipe "name"` block per `ProbeRecipe` |

### `@ProbeTest`

Single test — used inside `ProbeSuite.tests` or as a standalone top-level annotation.

| Field | Emits |
|---|---|
| `name` | `test "name"` |
| `tags: ['smoke']` | `@smoke` line |
| `steps` | indented test body |
| `examples` | `with examples:` table |

### `@ProbeRecipe`

Reusable recipe with named parameters. Reference parameters as `<paramName>` inside any string field.

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

### `@ProbeCompositeTest` (v0.9.6+)

Declares a multi-device composite test. Devices are listed by alias; per-device step groups use `OnDevice`, and `Sync` barriers force all devices to reach a checkpoint together.

```dart
@ProbeCompositeTest(
  name: 'alice sends bob a message',
  tags: ['composite', 'smoke'],
  devices: [
    Device('A', target: 'iPhone 15 Simulator'),
    Device('B', target: 'Pixel 9 Emulator'),
  ],
  body: [
    OnDevice('A', steps: [Open(), Tap(text: 'Sign in as Alice')]),
    OnDevice('B', steps: [Open(), Tap(text: 'Sign in as Bob')]),
    Sync('both signed in'),
    OnDevice('A', steps: [
      Tap(text: 'New message'),
      Type('hello bob'),
      Tap(text: 'Send'),
    ]),
    OnDevice('B', steps: [
      WaitUntil.appears('hello bob'),
      See('hello bob'),
    ]),
  ],
)
class ChatComposite {}
```

The emitted `.probe` block uses the standard `composite test` / `devices` / `sync` syntax. See the [composite test guide](/probescript/syntax/#composite-tests) for runtime details.

## Step DSL — full coverage

All 31 ProbeScript actions have a matching `const` Dart class. Common ones:

| Class | Emits |
|---|---|
| `Open()` / `OpenLink(url)` / `Close()` | `open the app` / `open link "url"` / `close the app` |
| `Restart()` / `Kill()` / `ClearAppData()` | corresponding lifecycle action |
| `Tap(id: 'login')` / `Tap(text: 'Sign In')` | `tap #login` / `tap "Sign In"` |
| `Tap(id: 'x', ifVisible: true)` | `tap #x if visible` |
| `DoubleTap` / `LongPress` / `GoBack()` | as named |
| `Type('hello', into: Field(id: 'msg'))` | `type "hello" into #msg` |
| `Clear(id: 'x')` | `clear #x` |
| `Swipe.up()` / `Scroll.down(on: …)` | `swipe up` / `scroll down …` |
| `Drag(from: …, to: …)` | `drag "from" to "to"` |
| `Rotate.landscape()` / `Toggle('switch')` / `Shake()` | as named |
| `AllowPermission('camera')` / `DenyPermission('mic')` | `allow permission "camera"` |
| `GrantAllPermissions()` / `RevokeAllPermissions()` | as named |
| `CopyToClipboard('x')` / `PasteFromClipboard()` | clipboard ops |
| `SetLocation(lat, lng)` / `VerifyExternalBrowser()` | as named |
| `TakeScreenshot('name')` / `CompareScreenshot('name')` | screenshot ops |
| `DumpWidgetTree()` / `SaveLogs()` / `Pause()` / `Log('msg')` | as named |
| `Store('value', as: 'var')` | `store "value" as var` |
| `See('X')` / `See('X', state: SeeState.enabled)` / `See('X', exactly: 2)` | `see "X"` and variants |
| `See.id('x', state: SeeState.focused)` (v0.9.6+) | `see #x is focused` |
| `See.selector(Ordinal(2, 'Item'))` (v0.9.6+) | `see 2nd "Item"` |
| `DontSee('X')` / `DontSee.id('x')` (v0.9.6+) | `don't see "X"` / `don't see #x` |
| `WaitFor.duration(N)` | `wait N seconds` |
| `WaitUntil.appears('X')` / `.disappears('X')` | `wait until "X" appears` etc. |
| `WaitUntil.idAppears('x')` (v0.9.6+) | `wait until #x appears` |
| `WaitForPageLoad()` / `WaitForNetworkIdle()` / `WaitForAnimations()` | as named |
| `If('cond', then: [...], otherwise: [...])` | `if "cond" appears` block |
| `Repeat(N, body: [...])` | `repeat N times` block |
| `RunDart('print("hi");')` | `run dart:` block |
| `Mock(method: HttpMethod.get, path: '/x', status: 200, body: '{…}')` | `when the app calls GET "/x"` block |
| `CallHttp(method: HttpMethod.post, url: '…', body: '…')` | `call POST "…" with body "…"` |
| `RecipeStep('name', args: [...])` | recipe invocation |

### Selectors

```dart
// Convenience (most common)
Tap(text: 'Sign In')
Tap(id: 'login_button')

// Explicit
Tap(selector: TextSel('Sign In'))
Tap(selector: IdSel('login_button'))
Tap(selector: TypeSel('ElevatedButton'))
Tap(selector: Ordinal(2, 'Item', container: 'List'))
Tap(selector: Below('Subtitle', anchor: 'Title'))
Tap(selector: Above('a', anchor: 'b'))
Tap(selector: LeftOf('a', anchor: 'b'))
Tap(selector: RightOf('a', anchor: 'b'))
Tap(selector: InContainer('Email', container: 'LoginForm'))
```

## See / DontSee — composable assertions (v0.9.6+)

`state`, `containing`, and `matching` can all coexist on a single `See`:

```dart
See('email field', state: SeeState.enabled, containing: 'email')
// → see "email field" is enabled contains "email"
```

`See.id` / `See.selector` target by `ValueKey` or rich selector:

```dart
See.id('password_field', state: SeeState.focused)
// → see #password_field is focused

See.selector(Below('Subtitle', anchor: 'Title'))
// → see "Subtitle" below "Title"
```

Same factories exist for `DontSee`.

## Output layout

Each annotated source file produces a single `.probe` in `tests/generated/`, preserving directory structure:

```
lib/screens/login.dart            →  tests/generated/screens/login.probe
lib/features/chat/chat.dart       →  tests/generated/features/chat/chat.probe
```

The generated file starts with a `do not edit` header that includes the source path. Run them like any other tests:

```bash
probe test tests/
```

## Should `tests/generated/` be committed?

Both workflows are valid:

- **Commit it** — review changes in PR like any other code, no need to run `build_runner` in CI before `probe test`. Add a CI step that fails if the builder produces diffs (catches forgotten regenerations).
- **Gitignore it** — single source of truth lives in the Dart code; CI runs `dart run build_runner build` before `probe test`.

Pick whichever fits your team.

## Cross-language validation

Every fixture in `flutter_probe_gen`'s test suite is round-tripped through the Go-side ProbeScript parser as part of CI (`internal/parser/golden_integration_test.go`). If the Dart emitter ever produces output the runtime can't parse, the Go test fails and the release is blocked — bugs are caught in CI, not at user runtime.

## Limitations

A few step types are exposed in the DSL but currently **not supported** by the runtime — using them in your tests will produce a "command not implemented" error at runtime:

- `Press('home')` / `Press('back')` — marked `@Deprecated` in v0.9.6. Will be enabled once the Go parser and Dart agent support platform key presses.
- `Pinch(zoomIn: true)` — same status.

Use `GoBack()` (which is fully supported) in place of `Press('back')`.
