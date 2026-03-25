---
title: "Why We Built FlutterProbe"
description: "The story behind FlutterProbe — why we created a new Flutter E2E testing framework instead of using integration_test or Patrol."
---

We did not set out to build a testing framework. We set out to ship a Flutter app reliably, and the existing tools kept getting in the way. This is the story of why FlutterProbe exists, the design decisions we made, and what we learned along the way.

## The Problems We Kept Hitting

In 2024, our team was building a mid-complexity Flutter application — around 40 screens, a REST API backend, real-time updates via WebSocket, and platform-specific features on both Android and iOS. We had solid unit test coverage and decent widget tests. But bugs kept reaching production. Not logic bugs — integration bugs.

A navigation change would break a deep link. A state management refactor would cause data to disappear on one screen but not another. An Android 14 behavioral change would break our permission flow while everything passed on Android 13. These are the kinds of bugs that only surface when the full app runs on a real device.

So we adopted `integration_test`. It worked, initially. But three pain points became impossible to ignore.

### Pain Point 1: Recompilation

Every change to a test required recompiling the entire app. For our project, that was 45-90 seconds per compilation. With 25 test scenarios across 8 test files, a full CI run spent more time compiling than testing. During development, the feedback loop was brutal: change one assertion, wait a minute, see if it passes, repeat.

We tried grouping tests into fewer files to reduce compilations, but this introduced test ordering dependencies and made failures harder to isolate.

### Pain Point 2: No Native Interaction

Our app required camera permissions, location access, and push notification handling. `integration_test` cannot interact with native OS dialogs. We evaluated Patrol, which solved the native interaction problem, but inherited the recompilation issue and added its own: the native test runner (UIAutomator on Android, XCUITest on iOS) introduced build complexity and CI configuration overhead that our team struggled to maintain.

### Pain Point 3: The Dart Requirement

Our QA engineer did not write Dart. She could describe test scenarios precisely and had excellent instincts for what to test, but translating those scenarios into Dart test code with `find.byKey`, `pumpAndSettle`, and `expect` was a bottleneck. Every test had to go through a developer, which meant tests were written late and updated rarely.

## The Design Philosophy

We started prototyping an alternative in late 2024 with three core principles.

### Principle 1: Separate the Test Runner from the App

If the test code is compiled into the app, every test change requires a rebuild. The solution is to move the test runner outside the app entirely. FlutterProbe's architecture puts the test runner in a Go CLI process that communicates with a lightweight Dart agent inside the app via WebSocket JSON-RPC 2.0.

The app binary is built once. The Dart agent starts a WebSocket server when initialized. The Go CLI connects, sends commands (tap, type, assert, scroll), and receives results. When you change a test, you change a `.probe` file — no recompilation.

We chose Go for the CLI because it compiles to a single static binary with no runtime dependencies. No JVM, no Node.js, no Python environment to manage. Download the binary and run it.

### Principle 2: Plain English Test Syntax

Tests should be readable by anyone on the team — developers, QA engineers, product managers. Not as documentation, but as the actual executable test.

ProbeScript is a structured, line-based syntax where each line is a command:

```
tap "Add to Cart" button
see "1 item in cart"
tap "Checkout" button
wait for "Order Summary" screen
see "$49.99"
```

There is no boilerplate, no imports, no class definitions. The syntax is constrained by design — it does not try to be a general-purpose programming language. For complex logic (conditional flows, custom data generation, API setup), FlutterProbe provides [hooks](/advanced/hooks/) and [recipes](/advanced/recipes/) that run arbitrary Dart or shell code before and after tests.

The constraint is intentional. When a test is a 6-line `.probe` file, anyone can read it, anyone can review it in a PR, and anyone can spot when a test is missing or incorrect.

### Principle 3: Direct Widget-Tree Access, Not UI Automation

Most mobile E2E testing tools work through platform-level UI automation frameworks: UIAutomator, XCUITest, Espresso, Appium. These frameworks interact with the accessibility layer or the view hierarchy. For Flutter apps, this means going through Flutter's platform rendering bridge, which adds latency and loses information about the widget tree.

FlutterProbe's Dart agent runs inside the Flutter process and has direct access to the widget tree. When the Go CLI sends a "tap 'Submit' button" command, the agent traverses the widget tree, finds the matching element, and executes the gesture directly. No platform bridge, no accessibility layer translation.

This is why per-command latency is under 50ms. It is also why FlutterProbe can implement features like self-healing selectors — the agent has full visibility into widget types, keys, labels, and tree structure, so it can find elements even when the tree changes slightly between app versions.

## Architecture Decisions and Trade-offs

Every architecture involves trade-offs. Here are the ones we made consciously.

**WebSocket instead of gRPC or HTTP.** WebSocket gives us a persistent bidirectional connection with low overhead. gRPC would have been more structured but adds a protobuf compilation step and larger agent binary. HTTP request-response would add latency for every command. WebSocket JSON-RPC 2.0 gives us structured messaging over a fast transport with wide tooling support for debugging.

**Go instead of Dart for the CLI.** A Dart CLI would let us share code between the agent and the runner. We chose Go because it produces zero-dependency static binaries, has excellent concurrency primitives for parallel test execution, and avoids coupling the test runner to the Flutter SDK version. The CLI works with any Flutter version — only the Dart agent needs to be compatible.

**ProbeScript instead of YAML or Gherkin.** YAML is verbose for sequential actions. Gherkin (Given/When/Then) adds ceremony that obscures the actual test steps. ProbeScript is closer to a shell script in structure — imperative, line-by-line, with minimal syntax. We found this the easiest format for both writing and reading tests quickly.

**BSL 1.1 instead of MIT or Apache.** We chose the Business Source License to sustain development. The source code is open and readable. After the change date, the license converts to Apache 2.0. Individual developers and small teams can use FlutterProbe freely. This lets us invest in the project full-time without relying solely on consulting or donations.

## What We Learned

**QA engineers write more tests when the barrier is lower.** After switching to ProbeScript, our QA engineer went from writing zero tests to maintaining 30+ scenarios independently. The volume of test coverage increased, and the time from bug report to regression test decreased from days to hours.

**Build-once architecture changes CI economics.** Our CI E2E time dropped from 35 minutes to 8 minutes. That changed developer behavior — they started actually waiting for E2E results before merging, instead of merging and hoping.

**Self-healing selectors reduce maintenance but do not eliminate it.** When a UI redesign moves a button from the app bar to a floating action button, the selector might still find it by label. But when a flow is restructured — two screens merged into one, or a step removed — the test needs manual updating. Self-healing handles the small changes; humans handle the structural ones.

## What Is Next

FlutterProbe is under active development. Current priorities include expanding [AI test generation](/tools/ai-generation/) capabilities, deepening [visual regression testing](/advanced/visual-regression/), and improving the [VS Code extension](/tools/vscode/) for test authoring and debugging. The [ProbeScript language](/probescript/syntax/) continues to evolve based on real-world usage patterns.

If you want to try it, start with the [installation guide](/getting-started/installation/). If you have an existing test suite, the [migration tool](/getting-started/migration/) converts from seven common test frameworks including `integration_test` and Patrol.
