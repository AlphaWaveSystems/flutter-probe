# Flutter and Dart releases

One entry per stable release series, newest first. "Assessed" means the release notes and the catalog diff were read and the fixture suite was run on that version; an
entry says "not assessed" for what was not done. The newest `## Flutter <version>` heading must be the newest stable release: the weekly check
(`.github/workflows/compat-drift.yml`) compares it with Flutter's release manifest.

How the facts below were gathered, so they can be checked: **release date, Dart version and revision** come from Flutter's official release manifest
(`https://storage.googleapis.com/flutter_infra_release/releases/releases_macos.json`); **release-note items** were read in the release-notes pages themselves
(`https://docs.flutter.dev/release/release-notes/release-notes-<version>`), which are lists of pull-request titles and carry **no release date and no
minimum-requirements section**; **Dart changes** were read in the Dart SDK's own changelog at the release tag (`https://github.com/dart-lang/sdk/blob/3.14.0/CHANGELOG.md`, and `.../blob/3.13.7/CHANGELOG.md` for 3.13); **tool requirements** were read in the Flutter tool's source at each release's tag commit (`packages/flutter_tools`), not in prose; the **catalog
diff** is `docs/compat/catalog.json` between the two releases. Anything not in one of those is not claimed.

## Flutter 3.50.0 (stable) with Dart 3.14.0

- **Released:** 2026-11-11 (manifest). Framework revision `ab2912035e160d9cad6306df307576da6128ebdb`, checked: a clone of tag `3.50.0` of `https://github.com/flutter/flutter`
  has exactly that commit.
- **Catalog** (`catalog.json`, made from that clone): 572 widgets (widgets 323, material 191, cupertino 58), against 570 on 3.44.8.
  - Added: `SliverClipRect`, `SliverClipRRect` (release notes: "Add SliverClipRect and SliverClipRRect", #179003).
  - Removed: none. Renamed: none. Changed (library, base class, abstract or deprecated flag): none. Deprecated: the same three (`ButtonBar`, `RawKeyboardListener`, `WillPopScope`).
  - `material` and `cupertino` are still in `packages/flutter` in 3.50.0, although the breaking-change page "Migrate to standalone `material_ui` and `cupertino_ui` packages" is listed
    under 3.50 (`https://docs.flutter.dev/release/breaking-changes/material-ui-and-cupertino-ui`; read in full on 2026-11-17: see "Deprecations that reach the agent" below). A later release that moves them would change the catalog's source:
    the weekly check would show it as a large diff.
- **Release-note items that touch what FlutterProbe drives** (title and PR number as printed; a title is not a statement of behaviour, and none has been tested here):
  - Semantics: "Removes deprecated ignoreSemantics parementers" (#191493); "Add a semantics role for slider" (#193324, after a revert, #193288); "Fix sibling nodes crash under
    MergeSemantics" (#191587); "Add service extension getSemanticsTree" (#189635); "Rebuild a semantics fragment when its sibling conflict changes" (#190431).
  - Gestures and hit testing: "Fix lower DragTarget not being recognized in overlapping targets" (#188979); "Fix stale eagerWinner reference in GestureArenaManager when rejected before arena close" (#187008).
  - Text input: "[iOS] TextField should support typing accented words with external keyboard" (#191062); "Add numeric-password variation to TextInputType.numberWithOptions" (#190637);
    "Fix multiline EditableText selection highlighting width" (#185239).
  - Platform views (what a WebView is): "Don't size or offset the Android platform view before it is laid out" (#190895); "Fix pointer id/coordinate mismatch in Android platform view
    multi-touch" (#191631); "Fix 1px edge clipping on Platform Views" (#190311).
  - Dialogs, navigation, lists: "Add barrierBuilder support to showDialog and showGeneralDialog" (#187992); "Fix Hero size changes during Navigator resize" (#190025); "Fix OverlayPortal layout in
    shrink-wrapped overlays" (#193079); "Add itemExtentBuilder support to ListView.separated" (#191157).
  - Test and tool APIs: "[flutter_test] Ensure LiveTestWidgetsFlutterBinding executes frame and pointer callbacks in the test zone" (#191595); "Pin androidx.test dependencies in integration_test" (#193316);
    "[flutter_tools] Support UTF-16 and UTF-8 BOM decoding in --dart-define-from-file" (#190547).
- **Breaking-change pages listed for 3.50** (`https://docs.flutter.dev/release/breaking-changes`; titles from the index, pages not read in full except the migration to `material_ui` and `cupertino_ui`, below): `DropdownButtonFormField` gets `enabled` and an optional `onChanged`;
  migration to the standalone `material_ui` and `cupertino_ui` packages; restricted Android engine flags in release mode; soft hyphens render at line breaks; stricter Android `Intent` verification for app
  entrypoints; web `viewport-fit=cover` `SafeArea` support; `WidgetSpan` order in right-to-left text.
- **Deprecations that reach the agent** (read 2026-11-17; the two breaking-change pages were fetched as HTML and their text read, not summarized; neither page states a removal version or date: the word "remov" is not in either):
  - **`package:flutter/material.dart` and `package:flutter/cupertino.dart` are formally deprecated in 3.50, in favour of the standalone packages `material_ui` and `cupertino_ui`.** Page "Migrate to standalone material_ui and cupertino_ui
    packages" (`https://docs.flutter.dev/release/breaking-changes/material-ui-and-cupertino-ui`, last updated 2026-11-09, landed in 3.49.0-1.0.pre-295, stable in 3.50, flutter/flutter #192957 and #191448): contributions to the in-framework
    libraries were frozen from 3.44; version 1.0 of the packages was published from 3.47 and developers could opt in from then; migration is `dart fix --apply --code=migrate_design_widgets` (imports become `package:material_ui/material_ui.dart`
    and `package:cupertino_ui/cupertino_ui.dart`) or by hand; for dependencies that still import the legacy libraries there is a `MaterialUiCompatibilityBridge` (it passes theme and localization data down, and cannot fix type mismatches in a
    dependency's public API); the Material and Cupertino localization delegates move into the new packages. The in-framework libraries are still in `packages/flutter` on 3.50.0 (the catalog is made from them).
  - **What the agent imports** (counted on `main` at 4c934793): `package:flutter/material.dart` and `package:flutter/cupertino.dart`, each with a `show` list, in three files of `probe_agent/lib/src` (`executor.dart`, `finder.dart`,
    `device_view.dart`); 54 files of `probe_agent/test`; and the Flutter fixtures (`native-test-apps/flutter`, `flutter-webview`, `studio-fixture`). The agent decides what a widget is by those classes.
  - **Measured: on widgets of `material_ui` the agent gives wrong answers for some steps** (a scratch app on Flutter 3.50.0 with `material_ui` 1.11.1 and `cupertino_ui` 1.1.7, the agent of `main`, a widget test, no device; issue #852): text,
    `tap` by text, `type`, `contains`, `is focused`, `is filled` and the protection of an obscured field's value work; `see #off is disabled` on a button with `onPressed: null` **fails ("is enabled") and `see #off is enabled` passes on it**; `is checked`
    on a `Checkbox`, a `Switch` and a `CupertinoSwitch` fails with "is not a checkable control"; a tooltip selector finds nothing. Not measured: routes, dialogs and sheets, disabled `TextField`, chips, list tiles, `TextFormField`, pickers, the device
    view, a device.
  - **`cacheExtent` and `cacheExtentStyle` are deprecated in 3.44** (page "Deprecated `cacheExtent` and `cacheExtentStyle`", `https://docs.flutter.dev/release/breaking-changes/scroll-cache-extent`, last updated 2026-05-20, landed in 3.41.0-0.0.pre, stable in
    3.44, flutter/flutter #181092): replaced by `scrollCacheExtent` with a `ScrollCacheExtent` (`ScrollCacheExtent.pixels(500.0)` or `.viewport(0.5)`) on `ListView`, `GridView`, `CustomScrollView`, `Viewport` and render objects such as `RenderViewport`.
  - **The analyzer on the agent** (`flutter analyze` of `probe_agent` with Flutter 3.44.8, all in `test/`): **25 issues**, of which **22 are `Radio` deprecations** (11 `groupValue`, 11 `onChanged`, "deprecated after v3.32.0-0.0.pre", in `checked_state_radio_test.dart` and
    `checked_state_radio_userwidget_test.dart`, which use the old API on purpose so that they compile on every Flutter the agent supports), **2 are `cacheExtent`** ("deprecated after v3.41.0-0.0.pre": `scroll_into_view_test.dart:31`, `scroll_until_anchor_test.dart:27`)
    and 1 is an unused local variable (`recorder_test.dart:374`, a warning). So the infos are mostly not `cacheExtent`. The CI step `dart analyze lib/ --fatal-infos` covers `lib/` only and is clean; the tests' infos do not fail it.
  - **What we do:** (1) **`cacheExtent` and the `Radio` API in the agent's tests: no change now.** The agent supports Flutter `>=3.19.0` (`probe_agent/pubspec.yaml`) and CI runs it on 3.29.x; `ScrollCacheExtent` lands in 3.41.0-0.0.pre, so the two tests would not compile
    there. The infos stay until the supported floor passes 3.44. (2) **The imports of `package:flutter/material.dart` and `cupertino.dart` stay:** they work for every app on the supported Flutter versions, including every app that has not migrated, and the libraries are not removed in 3.50.0. A switch to the standalone packages
    would raise the floor to at least 3.47 (the page says developers could opt in from 3.47; the packages' own minimum Flutter version was not read). (3) **The gap for apps that migrate to `material_ui` is a bug, not a note: issue #852**, open, with the measurement and no fix chosen. Until it is fixed, the compatibility matrix does not claim any Material or Cupertino result for an app that uses the standalone packages.
- **Tool requirements, read in the tool source** (3.44.8 in brackets). The Gradle plugin refuses a build below the "error" value and warns below the "warn" value:

  | | error below | warn below | template default |
  |---|---|---|---|
  | Gradle | 9.3.1 [8.7.0] | 9.5.0 [8.14.0] | 9.5.0 [9.1.0] |
  | Android Gradle Plugin | 9.1.1 [8.6.0] | 9.3.1 [8.11.1] | 9.3.1 [9.0.1] |
  | Kotlin Gradle plugin | 2.4.10 [2.0.0] | 2.4.20 [2.2.20] | 2.4.20 [2.3.20] |
  | Java | 17 [17] | 17 [17] | |
  | Android `minSdk` | 23 [23] | 24 [24] | |

  Xcode required by the tool: 15 [15]. Highest Gradle, AGP and KGP the tool knows: 9.5.0, 9.4, 2.4.20 [9.3.1, 9.1, 2.3.20].
- **What this means for FlutterProbe**
  - **The Android fixture apps were below the new minimums, and the build proved it.** `native-test-apps/flutter` and `native-test-apps/studio-fixture` used Gradle 9.1.0, AGP 9.0.1 and Kotlin 2.3.20: under 3.50.0's
    error values (9.3.1, 9.1.1, 2.4.10). **Measured (2026-11-16):** `flutter build apk --debug` of the unchanged `native-test-apps/flutter` on 3.50.0 stops with "Your project's Gradle version (9.1.0) is lower than Flutter's
    minimum supported version of 9.3.1". After raising both fixtures to Gradle 9.5.0, AGP 9.3.1 and Kotlin 2.4.20 (the 3.50.0 template values), both fixtures **build on 3.50.0 and on 3.44.8** (debug APKs, each in a private
    SDK folder and scratch Gradle home); the only warning is the existing one that the `bonsoir_android` plugin applies the Kotlin Gradle plugin.
  - **Nothing the agent uses is removed or renamed in these notes:** `probe_agent/lib` and the Flutter fixture use none of `ignoreSemantics`, `describeEnum`, `WillPopScope`, `ButtonBar`, `RawKeyboardListener`,
    `FlutterVersion` or `DropdownButtonFormField`. The agent's only `cacheExtent` use is in two of its own tests (`test/scroll_until_anchor_test.dart`, `test/scroll_into_view_test.dart`); `cacheExtent` is deprecated
    ("deprecated after v3.41.0-0.0.pre", use `scrollCacheExtent`) and is still present in 3.50.0, so it is a note, not a removal risk.
  - **CI pins:** `.github/workflows/ci.yml` uses Flutter `3.29.x` and `.github/workflows/e2e.yml` `3.41.4`; both predate AGP 9 and are not 3.50.0. Neither workflow builds the fixtures in this repository: `ci.yml` runs the agent's own tests, and `e2e.yml` builds the app it checks out into `e2e-app` from
    another repository, whose Gradle, AGP and Kotlin versions are not these fixtures' and were not read here; it will need 3.50.0's minimums when its pin moves. Aligning the two pins to one recorded version is a separate
    change.
- **Fixture suite on 3.50.0:** run (2026-11-16) on emulator-5600, AVD `ProbeWebViewSpike` (Android 15, API 35): `native-test-apps/flutter/probe-tests/flutter_suite.probe`, **5 of 5 passed** with the app built by Flutter 3.50.0 and the
  raised Gradle, AGP and Kotlin; the same suite with the app built by 3.44.8 and the same fixture files also passed 5 of 5. Only that suite was run: not the list, crash, load or WebView scenarios, and not iOS. The CLI was a
  development build, which prints "version mismatch" against the agent.
- **Release notes and breaking-change pages:** read as above; of the individual breaking-change pages only the two named under "Deprecations that reach the agent" were read (2026-11-17); the blog post was not.
- **Dart 3.14.0 changes** (`https://github.com/dart-lang/sdk/blob/3.14.0/CHANGELOG.md`, the section for 3.14.0; the file at that tag still marks it "Unreleased", so the date is the manifest's): `dart:isolate` unships the experimental synchronous
  isolate APIs that 3.13 exposed by accident (`Isolate.runSync` and related; breaking); `Float32x4`/`Int32x4`/`Float64x2` `toString()` returns `'V128'` and `min`/`max` follow IEEE 754 (breaking);
  `dart:mirrors` is marked `@deprecated`; `dart:io` Mac process spawning uses `posix_spawn`; new `dart migrate` command. Nothing in it concerns the agent: it uses none of those APIs.

## Flutter 3.47.0 to 3.47.9 (stable) with Dart 3.13.0 to 3.13.7

| Release | Date | Dart | Revision |
|---|---|---|---|
| 3.47.0 | 2026-08-12 | 3.13.0 | `4cf2416426` |
| 3.47.1 | 2026-08-19 | 3.13.1 | `6655482ec0` |
| 3.47.2 | 2026-08-27 | 3.13.2 | `d3b14c8769` |
| 3.47.3 | 2026-09-09 | 3.13.3 | `e8113bf456` |
| 3.47.4 | 2026-09-11 | 3.13.3 | `9584c6713b` |
| 3.47.5 | 2026-09-18 | 3.13.4 | `6a19cca564` |
| 3.47.6 | 2026-10-01 | 3.13.5 | `5fc346839b` |
| 3.47.7 | 2026-10-08 | 3.13.5 | `abaf9c5237` |
| 3.47.8 | 2026-10-23 | 3.13.7 | `737ba8f885` |
| 3.47.9 | 2026-10-29 | 3.13.7 | `c197ca5199` |

(Date, Dart and the first ten characters of the revision, from the release manifest. There was no stable 3.45, 3.46, 3.48 or 3.49 release.)

- **Catalog:** none was made for 3.47; the next catalog is 3.50.0's. The change from 3.44.8 to 3.50.0 is above.
- **Release notes:** the index has a page for 3.47.0 only (`https://docs.flutter.dev/release/release-notes/release-notes-3.47.0`); there are no pages for 3.47.1 to 3.47.9, and nothing is claimed about the patch releases except their manifest
  entries. The 3.47.0 page lists, among others (titles as printed): "Update semantics accessibility block to also block keyboard focusability" (#186206); "iOS a11y sets header trait based on heading level" (#186916); "Update merge
  semantics logic to merge sibling nodes" (#183745); "Adds semantics role check to isSemantics and matchesSemantics" (#188825); "Stricten isSemantics and matchesSemantics children mismatch check" (#188827);
  "Add `clipBehavior` parameter to `AnimatedCrossFade`" (#184545); "Replace IndexedStack visibility children with _VisibilityScope + ExcludeFocus" (#184884); "Fix owner-mismatch crash for OverlayPortal inside a deferred-mount
  parent" (#185793); "[ios][pv] Reland platform view hitTest approach (again)" (#185126); "Expose the app's build name and number as compile-time constants" (#187935). It carries no release date and no requirements section.
- **Breaking-change pages listed for 3.47** (titles; not read in full): OpenGL ES render-to-texture content stored top-down; update of the semantics header and `headingLevel` behaviour on iOS and Android
  (`.../breaking-changes/semantics-header-heading-level`); removal of `describeEnum` (`.../breaking-changes/remove-describeEnum`). The index also links a page about deprecating the `value` of `DropdownButtonFormField`
  (`.../breaking-changes/deprecate-dropdownbuttonformfield-value`), whose release group was not read.
- **Dart 3.13** (`https://github.com/dart-lang/sdk/blob/3.13.7/CHANGELOG.md`): 3.13.0 (2026-08-12) adds primary constructors (needs `sdk: '^3.13.0'`) and has breaking changes (no `final`/`var` on non-declaring parameters; a type-promotion change; `dart:io`
  `NetworkInterface.addresses` returns `List<InterfaceAddress>`); 3.13.1 to 3.13.7 are fixes (3.13.2 "Deprecates the legacy analyzer plugin system"; 3.13.7 (2026-10-22) uses the Android 14+ Conscrypt root certificate store).
  Dates of the 3.13.x patches are as that file prints them, not the manifest's.
- **What this means for FlutterProbe:** the agent uses none of the removed or deprecated APIs named above. The tool requirements of 3.47 were not read. **Fixture suite on 3.47.x:** not run. **Assessed:** no.

## Flutter 3.44.8 (stable) with Dart 3.12.2

- **Source:** the SDK installed on the machine that made the first `catalog.json` (framework commit dated 2026-07-23, revision `058e0af2c2b57e369d905a03ac9748b0ebf543c6`). 3.44.9 (2026-08-06, Dart 3.12.2, `6b182d2c75`) followed it; not assessed.
- **Catalog:** 570 widgets (widgets 321, material 191, cupertino 58); 30 abstract; 3 marked deprecated (`ButtonBar`, `RawKeyboardListener`, `WillPopScope`).
- **Tool requirements (tool source at the 3.44.8 tag commit, `058e0af2c2b57e369d905a03ac9748b0ebf543c6`, which a scratch clone of the tag has):** Gradle 8.7.0 error / 8.14.0 warn / 9.1.0 template; AGP 8.6.0 / 8.11.1 / 9.0.1; Kotlin Gradle plugin 2.0.0 / 2.2.20 / 2.3.20; Java 17; `minSdk` 23 / 24; Xcode 15.
- **Release notes and breaking changes:** not assessed, except that the breaking-change page for the `cacheExtent` deprecation, listed under 3.44, was read on 2026-11-17 (see "Deprecations that reach the agent" in the 3.50.0 entry). **Fixture suite on this version:** run on the fixtures as raised for 3.50.0 (Gradle 9.5.0, AGP 9.3.1, Kotlin 2.4.20): 5 of 5 on emulator-5600 (see the 3.50.0 entry). The fixtures as they were before the raise (the template defaults of this release) were not built here.
- **Dart 3.12.2 changes:** not assessed.

## Packages the fixture apps depend on

A package a fixture app is built from, pinned, with what was read in its release notes. These are not Flutter releases: the weekly check does not read this section.

### `webview_flutter` 4.14.1 and `webview_flutter_android` 4.14.1 (`native-test-apps/flutter-webview`, pinned exactly in `pubspec.yaml` and locked in `pubspec.lock`; a test keeps them equal)

- **Source:** the `CHANGELOG.md` of each package as published (read from the downloaded packages), which give no release dates. `webview_flutter_platform_interface` 2.15.1 and `webview_flutter_wkwebview` 3.26.2 are resolved with them
  (3.27.2 of the latter is newer; it is the iOS implementation, which the fixture does not use).
- **What matters for FlutterProbe:** `webview_flutter_android` 4.12.1 "migrates to Built-in Kotlin to support AGP 9" and "updates the minimum supported SDK version to Flutter 3.44 / Dart 3.12", so the fixture needs **Flutter 3.44 or newer**; its
  `minSdk` is 24. 4.14.0 adds `setWebAuthenticationSupport`, 4.13.0 a way to reach the native `WebView` from a plugin binding, 4.12.0 cookie retrieval, 4.11.0 `setInsetsForWebContentToIgnore`: none is used. `webview_flutter` 4.14.0 raises its minimum to
  Flutter 3.38 / Dart 3.10. The API the fixture needs is `AndroidWebViewController.enableDebugging(bool)`, a static method that turns on WebView debugging, which the WebView steps need (they reach the page through its DevTools socket).
- **Done:** both packages pinned exactly; the fixture builds on Flutter 3.44.8 and 3.50.0 (the 3.50.0 build with the Gradle, AGP and Kotlin versions of the Flutter fixture) and `webview.probe` passes 5 of 5 on each (see `MATRIX.md`, "WebView content").
- **Not assessed:** the iOS implementation, any version after 4.14.1, the Dart changes of these packages beyond their changelogs.
