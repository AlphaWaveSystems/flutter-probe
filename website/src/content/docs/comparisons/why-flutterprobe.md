---
title: "Why FlutterProbe: Compared with Maestro, Patrol and Appium for Flutter"
description: "How FlutterProbe differs from Maestro, Patrol and the Appium Flutter driver for Flutter end-to-end testing, with sources for every claim about the other tools and a plain list of when each one fits."
---

Four tools come up most often for Flutter end-to-end tests: Maestro, Patrol, the Appium Flutter driver and FlutterProbe. They make different trade-offs. This page states those trade-offs plainly. Every statement about another tool links to its own documentation, read on 2026-10-12. If one of them has changed since, tell us in a [GitHub issue](https://github.com/AlphaWaveSystems/flutter-probe/issues) and we will correct it.

## In one table

| | Maestro | Patrol | Appium Flutter driver | FlutterProbe |
|---|---|---|---|---|
| Test language | YAML flows | Dart | JavaScript, Python, Ruby and others | ProbeScript (plain English) |
| How it sees the app | The accessibility (semantics) tree | Inside the Flutter test process, plus a native automation layer | The Flutter driver extension | An agent in your app, driven over WebSocket |
| Changes to your app | None (tests the built APK or IPA) | Tests are Dart files in the Flutter project | `flutter_driver` and `enableFlutterDriverExtension` in a debug or profile build | `probe_agent` package, enabled with a build flag |
| Flutter widget keys | Not visible to it (per its docs) | see its docs | see its docs | Yes |

Sources: [Maestro Flutter docs](https://docs.maestro.dev/get-started/supported-platform/flutter), [Patrol](https://patrol.leancode.co/), [Appium Flutter driver](https://github.com/appium/appium-flutter-driver). The FlutterProbe column describes this project; see the [architecture](/advanced/architecture/) page.

## What each tool says about itself

**Maestro** finds Flutter widgets through the semantics tree: by text, by `semanticLabel`, or by a semantic `identifier` (Flutter 3.19 and later). Its docs say it cannot see Flutter `Key`s and recommend Semantics instead, and that it does not yet support Flutter desktop. Its upside is that it needs no changes to the app: it tests the compiled APK or IPA directly. Source: [Maestro Flutter docs](https://docs.maestro.dev/get-started/supported-platform/flutter).

**Patrol** is an open-source E2E framework from LeanCode for Flutter. Tests are written in Dart with its own finder syntax, and it handles permission dialogs, notifications, WebViews and device settings from those tests. It lists compatibility with device farms such as Firebase Test Lab, BrowserStack and AWS Device Farm. Source: [patrol.leancode.co](https://patrol.leancode.co/).

**Appium Flutter driver** lets Appium drive a Flutter app. The app must be built in debug or profile mode with the `flutter_driver` package and `enableFlutterDriverExtension`. It supports Android and iOS, the client tests can be written in several languages, and the project describes itself as maintained by the community. Source: [appium-flutter-driver](https://github.com/appium/appium-flutter-driver).

## Where FlutterProbe differs

- **Plain-English tests that live outside the app.** A `.probe` file reads like a manual test script, so people who do not write Dart can read and review it. Tests are not compiled into your app.
- **It reads the Flutter widget tree, not only the accessibility tree.** Tapping by widget key works without adding Semantics labels just for tests. This is the main reason the agent exists.
- **One CLI for the whole loop.** Device handling for Android and iOS (simulators, emulators, physical devices), a lint step, JUnit and JSON reports, retries, and performance baselines.
- **A path in from other tools.** `probe-convert` translates Maestro, Gherkin, Detox, Appium, Robot, `integration_test` and Patrol suites into ProbeScript. See [probe-convert](/tools/probe-convert/).
- **Local first.** The CLI and agent run on your machine and in your CI. Uploading reports to Probe Cloud is optional and still a [preview](/probe-cloud/).

## What it costs you

- **The agent must be in the app build.** You add the `probe_agent` package and build with a flag. Maestro needs nothing like this.
- **A new test language.** ProbeScript is small, but it is not Dart and not YAML. The [syntax reference](/probescript/syntax/) is the whole language.
- **License.** The CLI is under the Business Source License 1.1, which allows any use except offering a competing hosted testing service. The Dart agent is MIT. If you require an OSI-approved license for every tool in your pipeline, Patrol and Appium fit that better. See the [LICENSE](https://github.com/AlphaWaveSystems/flutter-probe/blob/main/LICENSE).
- **A younger project** with a smaller community than Maestro, Patrol or Appium.

## When each one is the better choice

- **Pick Maestro** if you want zero changes to the app, cover several app types (not only Flutter), and your widgets already carry text or semantic labels.
- **Pick Patrol** if your team writes Dart, wants tests next to the app code, and prefers a fully open-source license.
- **Pick the Appium Flutter driver** if you already run Appium across other platforms and want Flutter in the same grid and language.
- **Pick FlutterProbe** if you want readable plain-English tests, widget-key level access without extra Semantics work, and one CLI for devices, reports and CI.

## Try it

Install and run a first test in a few minutes: [Quick start](/getting-started/quick-start/). Want to try Probe Cloud as it takes shape? See the [design-partner offer](/pricing/#design-partners).
