---
title: "Looking for an integration_test Alternative? Try FlutterProbe"
description: "Why developers switch from Flutter's integration_test to FlutterProbe — human-readable tests, faster execution, built-in CI/CD, and no WidgetTester boilerplate."
---

Flutter's `integration_test` package is the default starting point for end-to-end testing. It ships with the SDK, requires no extra dependencies, and gives you direct access to the widget tree through `WidgetTester`. For small projects, that is often enough.

But as your app and test suite grow, `integration_test` starts to show its limits. This page walks through the specific pain points developers hit and explains how FlutterProbe addresses each one.

## What integration_test Does Well

Credit where it is due. `integration_test` has real strengths:

- **Zero setup cost.** It is already in your Flutter SDK. Add a dependency, create a test file, and run `flutter test integration_test/`.
- **Direct widget-tree access.** `WidgetTester` interacts with the render tree, so assertions are fast and deterministic when the app is simple.
- **Single language.** Tests are Dart, the same language as your app code.
- **First-party maintenance.** The Flutter team maintains it alongside the framework itself.

If your project has fewer than ten E2E tests and a single developer, `integration_test` may be all you need. The pain points below tend to emerge once a team or test suite crosses a certain threshold.

## Where integration_test Falls Short

### 1. Verbose boilerplate

Every test requires importing bindings, calling `ensureInitialized`, wrapping logic in `testWidgets`, and manually pumping frames. A simple login test is 20-30 lines of Dart before you reach the first assertion.

```dart
// integration_test with WidgetTester
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
```

Compare the same flow in [ProbeScript](/probescript/syntax/):

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

Eight lines, no imports, no pump calls. The ProbeScript version is readable by anyone on the team, including QA engineers and product managers who do not write Dart.

### 2. pumpAndSettle fragility

`pumpAndSettle` waits for all animations to complete, but it has a hardcoded timeout and fails silently when animations loop (loading spinners, shimmer effects). Developers end up writing custom pump loops or arbitrary `Future.delayed` calls, both of which are fragile.

FlutterProbe's `wait until` command polls the widget tree directly with configurable timeouts and clear error messages when a target does not appear. There is no frame-pumping to manage.

### 3. No native dialog support

`integration_test` operates inside the Flutter engine. It cannot tap native permission prompts, system keyboards, or OS-level alerts. If your app requests camera access or location permissions, `integration_test` cannot automate that step.

FlutterProbe handles permissions, clipboard access, and GPS location natively. Commands like `set location 37.7749, -122.4194` and `grant permission camera` work across Android and iOS without platform-specific workarounds. See the [Android](/platform/android/) and [iOS](/platform/ios/) platform guides for details.

### 4. No built-in reporting

After a test run, `integration_test` prints pass/fail to the console. There is no HTML report, no JUnit XML for CI, no screenshots on failure. Teams end up building custom reporting pipelines from scratch.

FlutterProbe generates HTML reports and JUnit XML out of the box. Screenshots are captured automatically on failure and embedded in the report. The [CI/CD integration guide](/ci-cd/github-actions/) shows how to publish reports as GitHub Actions artifacts.

### 5. No parallel execution

`integration_test` runs tests sequentially on a single device. For a suite of 50 tests, this means long CI times with no way to shard across multiple devices.

FlutterProbe supports `--parallel` to run tests concurrently on multiple connected devices and `--shard` to split a suite across CI runners. A 30-minute sequential run can drop to under 5 minutes with six parallel devices.

### 6. No visual regression

`integration_test` can take screenshots, but there is no built-in diffing or baseline management. Catching a misaligned button or a wrong color requires manual inspection.

FlutterProbe's [visual regression](/advanced/visual-regression/) feature captures baseline screenshots, diffs them against subsequent runs pixel-by-pixel, and reports differences with highlighted overlays. Thresholds are configurable per test.

### 7. Fragile selectors

When widget keys change or text is localized, `find.byKey` and `find.text` break. There is no fallback mechanism.

FlutterProbe's [self-healing selectors](/advanced/self-healing/) try multiple strategies (text, key, semantic label, widget type) and automatically recover when one strategy fails. The healed selector is logged so you can update the test at your convenience.

## Migration Path

Switching does not require rewriting everything at once. Here is a practical approach:

1. **Install FlutterProbe** alongside your existing tests. Follow the [installation guide](/getting-started/installation/) -- it takes under two minutes.
2. **Convert a few tests.** Pick three to five critical flows and rewrite them as `.probe` files. Use the [Quick Start](/getting-started/quick-start/) as a template.
3. **Run both suites in CI.** Keep `integration_test` for coverage while you validate the FlutterProbe results.
4. **Expand gradually.** As confidence grows, migrate more tests. If you have tests in other formats (Maestro YAML, Gherkin feature files, Detox JavaScript), the [probe-convert](/tools/probe-convert/) tool can automate the translation.
5. **Retire integration_test.** Once all critical paths are covered by ProbeScript, remove the Dart test files and simplify your CI pipeline.

## What You Gain After Migrating

| Capability | integration_test | FlutterProbe |
|---|---|---|
| Lines of code per test | 20-30 (Dart) | 5-10 (ProbeScript) |
| Native dialog handling | No | Yes |
| CI reporting | Manual | HTML + JUnit built-in |
| Visual regression | No | Yes |
| Self-healing selectors | No | Yes |
| Parallel execution | No | `--parallel`, `--shard` |
| Cloud farm support | Manual | 5 farms built-in |
| Test recording | No | [Yes](/tools/recording/) |
| HTTP mocking | Manual setup | Built-in |

## Getting Started

1. [Install the CLI](/getting-started/installation/).
2. Walk through the [Quick Start](/getting-started/quick-start/).
3. Explore [hooks](/probescript/hooks/) for setup/teardown logic (before all, before each, after each, after all, on failure).
4. Set up [data-driven tests](/probescript/data-driven/) to parameterize flows across user roles or locales.
5. Connect to your CI with [GitHub Actions](/ci-cd/github-actions/).
