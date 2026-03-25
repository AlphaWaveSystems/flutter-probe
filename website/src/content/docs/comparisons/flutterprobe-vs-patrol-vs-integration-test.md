---
title: "FlutterProbe vs Patrol vs integration_test: Complete Comparison"
description: "Side-by-side comparison of the three main Flutter E2E testing approaches. Architecture, syntax, performance, CI/CD, and ecosystem compared."
---

Choosing an E2E testing tool for Flutter means choosing between three primary options: Flutter's built-in `integration_test` package, Patrol by LeanCode, and FlutterProbe. Each takes a fundamentally different architectural approach, and those differences ripple through syntax, performance, CI/CD integration, and long-term maintenance.

This comparison is based on the state of each tool as of early 2026.

## Overview Table

| Feature | integration_test | Patrol | FlutterProbe |
|---------|-----------------|--------|-------------|
| **Maintainer** | Flutter team | LeanCode | Alpha Wave Systems |
| **License** | BSD-3 | Apache-2.0 | BSL 1.1 |
| **Language** | Dart | Dart | ProbeScript (.probe files) |
| **Architecture** | In-process | In-process + native | Out-of-process (Go CLI + Dart agent) |
| **Device requirement** | Yes | Yes | Yes |
| **Recompilation per test** | Yes | Yes | No |
| **Native OS interaction** | No | Yes | Yes |
| **Command latency** | ~100-200ms | ~100-200ms | <50ms |
| **Cloud device farms** | Firebase Test Lab | Firebase Test Lab | 5 providers |
| **CI/CD parallel execution** | Manual sharding | Manual sharding | Built-in --shard |
| **Self-healing selectors** | No | No | Yes |
| **Visual regression** | No | No | Yes |
| **Test recording** | No | No | Yes |
| **HTTP mocking** | Manual setup | Manual setup | Built-in |
| **VS Code extension** | No | No | Yes |

## Architecture Differences

The architectural differences are the most important factor in this comparison because they determine what is possible and what is not.

### integration_test

`integration_test` compiles your test code directly into the app binary. The test and the app run in the same Dart process. This gives tests direct access to the widget tree, finder APIs, and `WidgetTester`, but it means the test code is tightly coupled to the app. Every change to a test requires recompiling the app. There is no separation between the test runner and the application under test.

Native platform interactions — permission dialogs, system alerts, notifications — are out of scope. `integration_test` can only interact with widgets that Flutter renders.

### Patrol

Patrol extends `integration_test` by adding a native automation layer. It compiles a separate native test runner (using UIAutomator on Android and XCUITest on iOS) alongside the Flutter test. This lets Patrol interact with native OS elements: permission dialogs, system settings, notifications, and other apps.

The trade-off is complexity. Patrol tests still require recompilation, and the native layer adds build time and potential points of failure. The Dart test code communicates with the native runner across a process boundary, which introduces latency for native interactions.

### FlutterProbe

FlutterProbe takes a different approach entirely. The test runner is a Go CLI that communicates with a lightweight Dart agent embedded in the app via WebSocket JSON-RPC 2.0. Tests are written in `.probe` files using ProbeScript, not Dart.

Because the test runner is external to the app, the app binary is built once and reused across all test scenarios. There is no recompilation between tests. The Dart agent provides direct widget-tree access, so command execution is fast — under 50ms per action. Native OS interactions are handled through the agent's platform channel integration rather than a separate native test runner.

## Syntax Comparison

Here is the same test — verifying a login flow — in all three tools.

### integration_test

```dart
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:my_app/main.dart' as app;

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('login flow', (tester) async {
    app.main();
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(Key('email_field')), 'user@example.com');
    await tester.enterText(find.byKey(Key('password_field')), 's3cureP@ss');
    await tester.tap(find.byKey(Key('login_button')));
    await tester.pumpAndSettle();

    expect(find.text('Welcome back'), findsOneWidget);
  });
}
```

### Patrol

```dart
import 'package:patrol/patrol.dart';
import 'package:my_app/main.dart' as app;

void main() {
  patrolTest('login flow', ($) async {
    app.main();
    await $.pumpAndSettle();

    await $(#email_field).enterText('user@example.com');
    await $(#password_field).enterText('s3cureP@ss');
    await $(#login_button).tap();
    await $.pumpAndSettle();

    expect($(find.text('Welcome back')), findsOneWidget);
  });
}
```

### FlutterProbe

```
tap "Email" text field
type "user@example.com"
tap "Password" text field
type "s3cureP@ss"
tap "Sign In" button
see "Welcome back"
```

The ProbeScript version is shorter and does not require Dart knowledge. It uses widget labels and types rather than keys, which means tests do not break when developers refactor widget keys. The trade-off is that ProbeScript is less expressive than Dart for complex assertions or custom logic — though [recipes](/advanced/recipes/) and [hooks](/advanced/hooks/) extend its capabilities.

## Performance

Performance differences stem directly from architecture.

**Build time.** `integration_test` and Patrol require compiling the app for each test file (or group of tests in a single file). FlutterProbe builds the app once. For a suite of 30 test scenarios across 10 test files, this can save 5-15 minutes of CI time.

**Command latency.** FlutterProbe's direct widget-tree access via the Dart agent yields sub-50ms command execution. `integration_test` and Patrol use `pumpAndSettle`, which waits for animations and frame rendering. In practice, `pumpAndSettle` adds 100-200ms per action, sometimes more if animations are long-running.

**Suite execution time.** For a 30-scenario suite, typical wall-clock times in CI:

| Tool | Sequential | 4-way parallel |
|------|-----------|----------------|
| integration_test | 25-35 min | 8-12 min |
| Patrol | 30-40 min | 10-14 min |
| FlutterProbe | 10-15 min | 3-5 min |

FlutterProbe's advantage grows with suite size because the one-time build cost is amortized and per-command latency compounds across hundreds of actions.

## CI/CD Integration

All three tools work in CI/CD pipelines, but the integration depth varies.

`integration_test` produces basic pass/fail output. JUnit XML reporting requires additional packages. Firebase Test Lab integration is straightforward because Google maintains both tools.

Patrol inherits `integration_test`'s CI story and adds nothing specific. Its native layer can introduce CI-specific issues — particularly on Linux CI runners where Android emulator configuration is sensitive to the native test runner setup.

FlutterProbe includes built-in JUnit XML and HTML reporting, built-in `--shard` for parallel execution without CI matrix configuration, and tested integrations with five cloud device farm providers. See the [CI/CD guide](/ci-cd/github-actions/) for setup details.

## Cloud Device Farm Support

| Provider | integration_test | Patrol | FlutterProbe |
|----------|-----------------|--------|-------------|
| Firebase Test Lab | Yes | Yes | Yes |
| AWS Device Farm | Community scripts | No | Yes |
| BrowserStack | No | No | Yes |
| Sauce Labs | No | No | Yes |
| LambdaTest | No | No | Yes |

If your testing requirements include running on physical devices across multiple manufacturers and OS versions, FlutterProbe's broader cloud support simplifies the setup.

## Migration

If you have an existing `integration_test` or Patrol suite, FlutterProbe includes a migration tool that converts Dart test files to ProbeScript:

```bash
flutterprobe migrate --from integration_test --input test/integration/ --output tests/
```

The migration tool handles the seven most common test framework patterns. Complex custom logic in test files may require manual adjustment. See the [migration guide](/getting-started/migration/) for details.

## When to Use Each Tool

**Choose `integration_test` if** you want zero external dependencies, your team writes Dart exclusively, and you need only basic E2E coverage without native interaction. It is built into Flutter and will always be maintained by the Flutter team.

**Choose Patrol if** you need native OS interaction (permissions, notifications, system dialogs) and want to stay in the Dart ecosystem. Patrol is well-maintained and has a growing community.

**Choose FlutterProbe if** you need fast CI execution, non-Dart test syntax for cross-functional teams, cloud device farm support beyond Firebase, or features like visual regression testing, self-healing selectors, and [test recording](/tools/recorder/). FlutterProbe's out-of-process architecture provides structural advantages for larger test suites and CI-heavy workflows.

The tools are not mutually exclusive. Some teams use `integration_test` for a small set of smoke tests that live alongside the app code and FlutterProbe for the broader E2E regression suite. Choose based on your team's specific needs, CI infrastructure, and scale.
