---
title: "Flutter E2E Testing: A Complete Guide"
description: "Learn how to set up end-to-end testing for Flutter apps. Compare integration_test, Patrol, and FlutterProbe for real-device E2E testing."
---

End-to-end (E2E) testing verifies that your Flutter app works correctly from the user's perspective. Instead of testing a single widget or function in isolation, an E2E test launches the full application on a real device or emulator, walks through a user flow, and asserts that the expected outcome appears on screen.

This guide covers what E2E testing means for Flutter, the main approaches available today, and how to choose the right one for your project.

## Why E2E Testing Matters for Flutter

Unit tests and widget tests catch logic errors early, but they cannot verify that navigation, platform channels, network calls, and animations work together on a physical device. E2E tests fill that gap. They help you:

- Catch regressions in login flows, checkout screens, and other critical paths before users do.
- Validate platform-specific behavior on Android and iOS.
- Confirm that third-party plugins (camera, GPS, push notifications) integrate correctly.
- Gate CI/CD pipelines so broken builds never reach production.

Flutter's cross-platform promise means you ship to multiple targets from one codebase. E2E tests prove that promise holds on each target.

## The Three Main Approaches

### 1. integration_test (Flutter SDK)

`integration_test` is the official package bundled with the Flutter SDK. Tests are written in Dart using the `WidgetTester` API.

```dart
// integration_test/login_test.dart
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:my_app/main.dart' as app;

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('user can log in', (tester) async {
    app.main();
    await tester.pumpAndSettle();

    await tester.tap(find.text('Sign In'));
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(Key('email')), 'user@example.com');
    await tester.enterText(find.byKey(Key('password')), 'secret123');
    await tester.tap(find.text('Continue'));
    await tester.pumpAndSettle();

    expect(find.text('Welcome'), findsOneWidget);
  });
}
```

**Strengths:** Ships with Flutter, zero extra dependencies, direct access to the widget tree.

**Limitations:** Verbose Dart boilerplate, `pumpAndSettle` timing issues, no built-in reporting or CI dashboard, cannot interact with native system dialogs (permission prompts, keyboards).

### 2. Patrol

Patrol extends `integration_test` with native automation capabilities. It adds a `NativeAutomator` that can tap native OS dialogs, handle permissions, and interact with the notification shade.

```dart
patrolTest('user grants location permission', ($) async {
  await $.pumpWidgetAndSettle(MyApp());
  await $('Allow location').tap();
  await $.native.grantPermission();
  expect($('Map loaded'), findsOneWidget);
});
```

**Strengths:** Native dialog support, custom finder syntax (`$()`), built on top of the familiar Flutter test API.

**Limitations:** Still Dart-only, requires test code to live inside the app project, limited cloud-farm support, no plain-English syntax.

### 3. FlutterProbe

FlutterProbe takes a different approach. Tests are written in [ProbeScript](/probescript/syntax/), a plain-English language stored in `.probe` files. A Go CLI parses the scripts and sends commands to a lightweight Dart agent embedded in the app, communicating over WebSocket with sub-50ms round-trips.

```
test "user can log in"
  open the app
  wait until "Sign In" appears
  tap "Sign In"
  type "user@example.com" into "Email"
  type "secret123" into "Password"
  tap "Continue"
  see "Welcome"
```

**Strengths:** No Dart test code to maintain, human-readable scripts that non-developers can review, direct widget-tree access without a WebDriver layer, built-in [visual regression](/advanced/visual-regression/), [self-healing selectors](/advanced/self-healing/), support for five cloud farms, and parallel execution via `--parallel` and `--shard` flags.

**Limitations:** Requires installing a separate CLI and embedding the Dart agent. ProbeScript is purpose-built for E2E flows, so complex programmatic logic may need [hooks](/probescript/hooks/) or [recipes](/probescript/recipes/).

## Code Comparison at a Glance

| Aspect | integration_test | Patrol | FlutterProbe |
|---|---|---|---|
| Language | Dart | Dart | ProbeScript (plain English) |
| Native dialog support | No | Yes | Yes (permissions, GPS, clipboard) |
| Cloud farm support | Manual setup | Limited | 5 farms built-in |
| CI reporting | DIY | DIY | Built-in HTML/JUnit reports |
| Visual regression | No | No | Yes |
| Self-healing selectors | No | No | Yes |
| Migration tooling | N/A | N/A | Imports from 7 formats |

## When to Use Each

**Choose integration_test** when you need a quick smoke test on a single platform with no extra dependencies. It is the lowest-friction starting point and good enough for small projects with a single developer.

**Choose Patrol** when your tests must interact with native OS dialogs (permission prompts, system alerts) and your team is comfortable writing Dart test code. Patrol is a natural upgrade from `integration_test`.

**Choose FlutterProbe** when you want tests that scale across devices, run on cloud farms, and stay readable by QA engineers and product managers who do not write Dart. The [migration tool](/tools/probe-convert/) can convert existing Maestro, Gherkin, or Detox suites into ProbeScript automatically.

## Getting Started with FlutterProbe

1. [Install the CLI](/getting-started/installation/) -- a single binary for macOS, Linux, and Windows.
2. Add the Dart agent to your app (`pubspec.yaml` one-liner).
3. Write your first `.probe` file following the [Quick Start](/getting-started/quick-start/) guide.
4. Run it: `probe run tests/smoke/login.probe --device emulator-5554`.
5. View the HTML report or pipe JUnit XML into your CI pipeline. See [CI/CD with GitHub Actions](/ci-cd/github-actions/) for a working workflow.

The [VS Code extension](/tools/vscode/) adds syntax highlighting, CodeLens run buttons, and a test explorer so you can author and debug tests without leaving your editor.

## Further Reading

- [ProbeScript Syntax Reference](/probescript/syntax/) -- full language grammar and examples.
- [Data-Driven Tests](/probescript/data-driven/) -- parameterize flows with CSV or JSON data.
- [Architecture Deep Dive](/advanced/architecture/) -- how the Go CLI and Dart agent communicate.
- [Android Setup](/platform/android/) and [iOS Setup](/platform/ios/) -- platform-specific configuration.
