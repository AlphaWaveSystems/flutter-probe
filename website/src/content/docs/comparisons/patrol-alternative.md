---
title: "FlutterProbe vs Patrol: Which Flutter E2E Framework to Choose"
description: "An honest comparison of FlutterProbe and Patrol for Flutter end-to-end testing. Architecture, syntax, performance, and CI/CD differences explained."
---

Patrol and FlutterProbe both solve problems that Flutter's built-in `integration_test` cannot. Both handle native OS dialogs. Both give you access to the Flutter widget tree. But they take fundamentally different architectural approaches, and that difference shapes everything from who writes the tests to how they run in CI.

This page is a technical comparison to help you decide which framework fits your team.

## What Patrol Offers

Patrol, developed by LeanCode, extends `integration_test` with two key additions:

1. **NativeAutomator.** A platform-specific automation layer that can tap native UI elements -- permission dialogs, system settings, the notification shade -- that `integration_test` cannot reach.
2. **Custom finders.** The `$('text')` syntax replaces verbose `find.text('text')` calls, making Dart test code shorter.

Patrol tests are Dart files that live inside your Flutter project. They compile and run as part of the app process, just like `integration_test`. This means you get direct widget-tree access with the addition of native automation.

```dart
patrolTest('user grants camera permission and takes photo', ($) async {
  await $.pumpWidgetAndSettle(MyApp());
  await $('Take Photo').tap();
  await $.native.grantPermission();
  await $.native.tap(NativeSelector(text: 'Allow'));
  expect($('Photo Preview'), findsOneWidget);
});
```

Patrol is a solid choice for Dart-native teams that need native dialog handling without leaving the Flutter test ecosystem.

## What FlutterProbe Offers

FlutterProbe uses a split architecture: a Go CLI runs on the host machine and sends commands over WebSocket to a Dart agent embedded in the app. Tests are written in [ProbeScript](/probescript/syntax/), a plain-English DSL stored in `.probe` files outside the app project.

```
test "user grants camera permission and takes photo"
  open the app
  tap "Take Photo"
  grant permission camera
  see "Photo Preview"
```

The Go CLI handles device management, test orchestration, reporting, and cloud-farm integration. The Dart agent handles widget-tree queries and actions. This separation means the test code never compiles into the app binary in release builds.

## Architecture Comparison

| Aspect | Patrol | FlutterProbe |
|---|---|---|
| Test language | Dart | ProbeScript (plain English) |
| Where tests live | Inside app project | Separate `.probe` files |
| Test runner | Flutter test runner | Go CLI (`probe run`) |
| Device communication | In-process | WebSocket + JSON-RPC |
| Command round-trip | In-process (nanoseconds) | Sub-50ms over WebSocket |
| App compilation | Tests compile with app | Agent compiles with app; tests do not |
| CLI dependency | Flutter CLI | FlutterProbe CLI + Flutter CLI |

Patrol's in-process model means zero network overhead per command. FlutterProbe's WebSocket round-trip adds roughly 10-40ms per command, but the total test duration is usually dominated by animations and network calls, not command overhead. In practice, both frameworks produce comparable end-to-end run times.

## Syntax and Readability

Patrol reduces Dart boilerplate compared to raw `integration_test`, but tests are still Dart code with imports, async/await, and programmatic control flow.

FlutterProbe's ProbeScript is intentionally constrained. Each line is a single command or assertion. There are no variables, loops, or conditionals in the base language. When you need programmatic logic, you use [hooks](/probescript/hooks/) (written in Dart or shell scripts) or [recipes](/probescript/recipes/) to encapsulate reusable flows.

This trade-off is deliberate. ProbeScript optimizes for readability by non-developers at the cost of expressiveness. If every person writing tests is a Dart developer, Patrol's syntax may feel more natural. If tests need to be reviewed or authored by QA engineers, product managers, or designers, ProbeScript removes the language barrier.

## Feature Comparison

| Feature | Patrol | FlutterProbe |
|---|---|---|
| Native dialog handling | Yes (NativeAutomator) | Yes (built-in commands) |
| Widget-tree access | Yes (in-process) | Yes (via Dart agent) |
| Custom finders | `$('text')` syntax | Text, key, semantic label, type |
| Self-healing selectors | No | [Yes](/advanced/self-healing/) |
| Visual regression | No | [Yes](/advanced/visual-regression/) |
| Test recording | No | [Yes](/tools/recording/) |
| HTTP mocking | Manual setup | Built-in |
| GPS / location mocking | Manual | `set location lat, lng` |
| Clipboard control | Manual | `set clipboard "text"` |
| Parallel execution | No | `--parallel`, `--shard` |
| Cloud farm support | Limited | 5 farms (BrowserStack, Firebase Test Lab, AWS Device Farm, Sauce Labs, LambdaTest) |
| CI reporting | Manual | HTML + JUnit built-in |
| Migration tooling | No | [7 formats](/tools/probe-convert/) (Maestro, Gherkin, Detox, Appium, Robot, integration_test, Patrol) |
| VS Code extension | No | [Yes](/tools/vscode/) (syntax highlighting, CodeLens, test explorer) |
| Data-driven tests | Manual (Dart loops) | [Built-in](/probescript/data-driven/) (CSV, JSON) |
| Hooks | Dart setUp/tearDown | [before all/each, after all/each, on failure](/probescript/hooks/) |

## Performance

Both frameworks access the widget tree directly, so element lookups are fast on both sides. The main performance differences come from test orchestration:

- **Patrol** compiles tests into the app binary. Changing a test requires recompiling the app, which can take 30-90 seconds on large projects.
- **FlutterProbe** compiles only the Dart agent into the app. ProbeScript files are interpreted at runtime by the Go CLI. Changing a test requires no recompilation -- just re-run the command. This makes the edit-run cycle significantly faster during test development.

For CI runs where the app is compiled once and all tests execute sequentially, compilation overhead is amortized and both frameworks are comparable. For parallel runs, FlutterProbe's `--shard` flag distributes tests across devices, which Patrol does not support natively.

## CI/CD Integration

Patrol relies on the Flutter test runner and outputs results to stdout. Getting JUnit XML or HTML reports requires third-party packages or custom scripts.

FlutterProbe's CLI generates reports natively. A typical [GitHub Actions workflow](/ci-cd/github-actions/) looks like:

```yaml
- name: Run E2E tests
  run: probe run tests/ --device emulator-5554 --report junit --report html

- name: Upload report
  uses: actions/upload-artifact@v4
  with:
    name: test-report
    path: probe-reports/
```

Reports include screenshots on failure, step-by-step timing, and pass/fail summaries. The [reports documentation](/ci-cd/reports/) covers all output formats.

## Who Each Framework Is Best For

**Choose Patrol if:**

- Your entire team writes Dart and prefers staying in one language.
- You need native automation but want to stay close to the standard Flutter test API.
- Your test suite is small to medium (under 50 tests) and runs on local devices.
- You do not need visual regression, self-healing selectors, or cloud-farm execution.

**Choose FlutterProbe if:**

- Non-developers (QA, product, design) need to read or write tests.
- You run tests on cloud device farms or need parallel execution across multiple devices.
- You want visual regression testing and self-healing selectors to reduce maintenance.
- You are migrating from another framework (Maestro, Detox, Appium, Gherkin) and want automated conversion.
- Fast iteration matters -- no recompilation when tests change.

## Migrating from Patrol to FlutterProbe

If you have existing Patrol tests, FlutterProbe's [probe-convert](/tools/probe-convert/) tool can translate them into ProbeScript:

```bash
probe convert --from patrol --input tests/patrol/ --output tests/probe/
```

The converter handles `patrolTest` blocks, `$()` finders, `NativeAutomator` calls, and common assertions. Review the output for edge cases, then run the converted tests with `probe run`.

## Getting Started with FlutterProbe

1. [Install the CLI](/getting-started/installation/) -- a single binary, no Node.js or Gradle plugins required.
2. Follow the [Quick Start](/getting-started/quick-start/) to write your first `.probe` test.
3. Explore the [ProbeScript syntax reference](/probescript/syntax/) for the full command set.
4. Set up the [VS Code extension](/tools/vscode/) for syntax highlighting and one-click test runs.
