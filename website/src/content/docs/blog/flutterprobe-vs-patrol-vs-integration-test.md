---
title: "FlutterProbe vs Patrol vs integration_test: An Honest Comparison"
description: "We compared FlutterProbe against Patrol and integration_test for real-world Flutter E2E testing. Here's what we found."
---

We tested FlutterProbe, Patrol, and `integration_test` against the same set of real-world scenarios to see how each tool performs in practice. This is not a marketing comparison — we document where FlutterProbe falls short alongside where it excels.

## Methodology

We used a sample Flutter e-commerce application with 28 screens, REST API integration, authentication, a shopping cart, checkout with payment, push notifications, and deep linking. The app was built with Flutter 3.27, using Riverpod for state management and GoRouter for navigation.

We wrote equivalent test suites in all three tools covering seven test scenarios:

1. **Sign up and sign in** — form entry, validation errors, successful authentication
2. **Product browsing** — list scrolling, filtering, search, pull-to-refresh
3. **Cart management** — add items, update quantity, remove items, verify totals
4. **Checkout flow** — address entry, payment method selection, order confirmation
5. **Push notification handling** — receive notification, tap to navigate to detail screen
6. **Deep link navigation** — open a product link, verify correct screen loads
7. **Permission flow** — camera permission request, grant, use camera feature

We ran each suite 10 times on the same hardware (M2 MacBook Pro, Android emulator API 34) and recorded build times, execution times, and failure rates.

## Results

### Build Time

| Tool | Initial build | Rebuild after test change |
|------|--------------|--------------------------|
| integration_test | 52s | 48s |
| Patrol | 68s | 61s |
| FlutterProbe | 52s | 0s |

`integration_test` and FlutterProbe have identical initial build times because the app compilation is the same. Patrol's initial build is longer due to the native test runner compilation on both Android and iOS.

The critical difference is rebuild time. `integration_test` and Patrol recompile the app for every test file change. FlutterProbe builds the app once — subsequent test changes modify `.probe` files that do not require recompilation. Over a development session with 20+ test iterations, this saves 15-20 minutes.

### Execution Time (7 Scenarios, Sequential)

| Tool | Mean | Median | Std Dev |
|------|------|--------|---------|
| integration_test | 4m 12s | 4m 08s | 11s |
| Patrol | 5m 31s | 5m 24s | 18s |
| FlutterProbe | 2m 47s | 2m 44s | 7s |

FlutterProbe's speed advantage comes from two factors: no recompilation between test files, and sub-50ms per-command latency versus 100-200ms for `pumpAndSettle`-based tools. The gap widens with more scenarios because the compilation overhead in `integration_test` and Patrol is paid per test file.

Patrol is slower than `integration_test` because the native automation layer adds overhead for every native interaction, and the communication between the Dart test and the native runner introduces latency.

### Scenario Coverage

| Scenario | integration_test | Patrol | FlutterProbe |
|----------|-----------------|--------|-------------|
| Sign up / sign in | Pass | Pass | Pass |
| Product browsing | Pass | Pass | Pass |
| Cart management | Pass | Pass | Pass |
| Checkout flow | Pass | Pass | Pass |
| Push notification | **Cannot test** | Pass | Pass |
| Deep link | Partial | Pass | Pass |
| Permission flow | **Cannot test** | Pass | Pass |

`integration_test` cannot interact with native OS elements. Push notification handling and permission dialogs are outside its scope. Deep link testing is possible only if the app handles the deep link internally without relying on the OS intent system.

Both Patrol and FlutterProbe handle all seven scenarios. Patrol uses its native automation layer (UIAutomator / XCUITest). FlutterProbe handles native interactions through its Dart agent's platform channel integration.

### Flakiness

Over 10 runs of the full suite:

| Tool | Flaky runs (at least 1 failure) |
|------|-------------------------------|
| integration_test | 1/10 |
| Patrol | 3/10 |
| FlutterProbe | 1/10 |

Patrol's higher flakiness rate was concentrated in the push notification and permission scenarios, where timing issues between the Dart test process and the native automation runner caused intermittent failures. Both `integration_test` and FlutterProbe had a single flaky failure each, both in the product browsing scenario (a scroll-related timing issue).

Flakiness is highly dependent on test implementation quality. These numbers reflect our test code, not inherent tool reliability.

## Where Each Tool Excels

### integration_test Strengths

- **Zero dependencies.** Ships with Flutter. No additional tools to install, no CLI to manage, no agent to embed.
- **Familiar API.** If you write Flutter widget tests, you already know the API. The `find`, `expect`, and `tester` patterns are identical.
- **Official support.** Maintained by the Flutter team. Guaranteed to stay compatible with new Flutter releases.
- **Simplicity.** For basic E2E smoke tests that do not need native interaction, `integration_test` is the fastest path from zero to a running test.

### Patrol Strengths

- **Native interaction.** The only Dart-based tool that can interact with permission dialogs, notifications, system settings, and other apps.
- **Dart ecosystem.** Tests are regular Dart code with access to the full pub.dev ecosystem for utilities, assertions, and data generation.
- **Custom selectors.** Patrol's `$` selector syntax is more concise than raw `find.byKey` calls.
- **Active community.** LeanCode maintains the project actively, and the community is responsive on GitHub and Discord.

### FlutterProbe Strengths

- **Build-once architecture.** Eliminates recompilation overhead, which compounds significantly in CI pipelines with large test suites.
- **ProbeScript readability.** Non-developers can read, write, and review tests. This changes team dynamics around test ownership.
- **Performance.** Sub-50ms command latency and no recompilation make FlutterProbe measurably faster for suites above 10 scenarios.
- **CI/CD tooling.** Built-in sharding, JUnit/HTML reporting, and [five cloud device farm integrations](/comparisons/flutter-test-automation/) reduce CI pipeline setup work.
- **Self-healing selectors.** Minor UI changes (reordering widgets, changing container types) do not break tests. This materially reduces maintenance effort.
- **Visual regression.** Built-in screenshot comparison catches unintended UI changes that functional tests miss. See [visual regression docs](/advanced/visual-regression/).

## Honest Limitations of FlutterProbe

No tool is perfect. Here is where FlutterProbe has real limitations:

**ProbeScript is not Dart.** For teams that want their E2E tests in the same language as their app, ProbeScript is an additional syntax to learn. Complex assertion logic that would be straightforward in Dart requires using [hooks](/advanced/hooks/) or [recipes](/advanced/recipes/), which adds indirection.

**Agent dependency.** The Dart agent must be initialized in the app. This is a code change to your `main()` function, guarded behind an `assert` block for debug-only inclusion. It is minimal, but it is a modification to the app under test.

**BSL license.** FlutterProbe uses the Business Source License 1.1, which converts to Apache 2.0 after the change date. For some organizations, any non-permissive license is a blocker regardless of practical implications. `integration_test` (BSD-3) and Patrol (Apache-2.0) have conventional open-source licenses.

**Smaller community.** `integration_test` has the entire Flutter community. Patrol has a dedicated and growing user base. FlutterProbe is newer and has fewer community resources, Stack Overflow answers, and third-party tutorials.

**Debugging experience.** When a Patrol or `integration_test` test fails, you can set breakpoints in your Dart IDE and step through both the test code and the app code in the same debug session. FlutterProbe tests run in a separate process, so debugging requires inspecting the agent logs and the CLI output rather than using a unified debugger. The [VS Code extension](/tools/vscode/) improves this but does not fully close the gap.

## Recommendation Matrix

| Your situation | Recommendation |
|---------------|---------------|
| Small app, Dart-only team, basic E2E needs | integration_test |
| Need native OS interaction, Dart-only team | Patrol |
| Large test suite, CI performance matters | FlutterProbe |
| QA engineers write tests, not just developers | FlutterProbe |
| Need cloud device farm testing beyond Firebase | FlutterProbe |
| Need visual regression testing | FlutterProbe |
| Want zero external dependencies | integration_test |
| Enterprise compliance requires permissive license | integration_test or Patrol |

These recommendations are not absolute. Evaluate based on your specific project size, team composition, CI infrastructure, and device coverage requirements. You can also combine tools — use `integration_test` for a small smoke suite and FlutterProbe for comprehensive regression testing.

For migration between tools, see the [migration guide](/getting-started/migration/). For a broader look at the testing landscape, see the [Flutter UI testing overview](/comparisons/flutter-ui-testing/).
