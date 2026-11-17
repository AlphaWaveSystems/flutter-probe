# Widget compatibility matrix

How FlutterProbe works with each widget family. **A cell stays "not assessed" until a test covers it**; it is changed only in the same pull request as the test.
`catalog.json` lists every widget; the families below are how they are grouped for testing. A widget FlutterProbe cannot work with is listed in the
"Unsupported" section, never left out.

Cells: *find* (select by text, id or type), *tap*, *type* (enter text), *scroll* (bring into view), *read* (its text or value), *fixture* (a screen in
`native-test-apps/flutter`), *device test* (a ProbeScript or Go test that ran on a device).

| Family | Examples | find | tap | type | scroll | read | fixture | device test |
|---|---|---|---|---|---|---|---|---|
| Text and display | `Text`, `RichText`, `Icon`, `Image` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Buttons | `ElevatedButton`, `TextButton`, `IconButton`, `CupertinoButton` | see below | see below | n/a | not assessed | see below | `buttons` screen | `probe-tests/widgets/buttons.probe` |
| Text input | `TextField`, `TextFormField`, `CupertinoTextField` | see below | see below | see below | not assessed | see below | `textinput` screen | `probe-tests/widgets/textinput.probe` |
| Selection controls | `Checkbox`, `Switch`, `Radio`, `Slider`, `CupertinoSwitch` | see below | see below | n/a | not assessed | see below | `selection` screen | `probe-tests/widgets/selection.probe` |
| Pickers and menus | `DropdownButton`, `PopupMenuButton`, `DatePickerDialog`, `CupertinoPicker` | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed |
| Lists and scrolling | `ListView`, `GridView`, `SingleChildScrollView`, `PageView` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Dialogs and sheets | `AlertDialog`, `BottomSheet`, `SnackBar`, `CupertinoAlertDialog` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Navigation | `AppBar`, `NavigationBar`, `TabBar`, `Drawer`, `Navigator` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Layout and containers | `Container`, `Row`, `Column`, `Stack`, `Card` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Gestures | `GestureDetector`, `Dismissible`, `Draggable`, `InkWell` | not assessed | not assessed | n/a | not assessed | n/a | not assessed | not assessed |
| WebView content | an embedded web page (`webview_flutter`) | see below | see below | see below | not assessed | see below | `native-test-apps/flutter-webview` | `native-test-apps/flutter-webview/probe-tests/webview.probe` |

## Assessed widgets

A cell says **pass** only where a step of the named test asserts it, **fail** where a step that should pass does not, and **not assessed** otherwise. A "read" that
the test does through a text that mirrors the widget (`Terms: on`) is a read of the mirror, not of the widget, and is said so.

How a result was obtained is under the tables. What the tests do **not** show is as much a part of the result as what they show.

### Selection controls (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=selection`, `probe-tests/widgets/selection.probe`)

| Widget (fixture id) | find (text, `#id`) | tap | read | Result |
|---|---|---|---|---|
| `Checkbox` in a `CheckboxListTile` (`wf_checkbox`) | pass (text) | pass (`toggle`) | pass (`see ... is checked`, on and off) | pass |
| `Switch` in a `SwitchListTile` (`wf_switch`) | pass (text) | pass (`toggle`) | pass (`see ... is checked`) | pass |
| `Radio` in a `RadioListTile` under a `RadioGroup` (`wf_radio_small`, `_medium`, `_large`) | pass (text) | pass (`tap`; the group's state text changes) | pass with the fixed agent (`see ... is checked` on the chosen radio, `don't see ... is checked` on the others); **fail** with an agent from before the fix (finding, below) | pass (needs `probe_agent` with the fix) |
| `Slider` (`wf_slider`) | pass (`#id`) | pass (`swipe right on` moves the value) | not assessed (the value is read through its mirror text) | partly |
| `CupertinoSwitch` (`wf_cupertino_switch`) | pass (`#id`) | pass (`toggle`) | not assessed (mirror text only; `is checked` was not tried) | partly |

**Finding, fixed: a radio under `RadioGroup` was never "checked" to the agent.** Since the `RadioGroup` API the group value is kept by the `RadioGroup`, not by each `Radio`; the agent
decided `is checked` for a radio by comparing the radio's `value` with its own `groupValue` (`probe_agent/lib/src/executor.dart`), so it answered "not checked" for a radio that was on. The tap itself worked
(the fixture's state text moves to `Size: medium`). Reproduced on a build by 3.50.0 and on a build by 3.44.8; the agent's own unit test used the older `groupValue:` form of `Radio`, which is why it did not show. The agent now reads
the radio's own group value when it has one and otherwise the nearest `RadioGroup` above it (`Radio`, `RadioListTile`, `CupertinoRadio`). The row above is the result with the fixed agent on a build by each Flutter version
(`selection.probe`, 5 of 5 on emulator-5600); an app that still runs an agent from before the fix keeps the old answer until it takes a `probe_agent` release with the fix.

### Text input (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=textinput`, `probe-tests/widgets/textinput.probe`)

| Widget (fixture id) | find (`#id`) | type | clear | read | Result |
|---|---|---|---|---|---|
| `TextField` (`wf_textfield`) | pass | pass | pass via the mirror (`Plain: (empty)` after `clear`; the field's own text was not read) | not assessed (read through a mirror text) | partly |
| `TextFormField` with a validator (`wf_formfield`, `wf_validate`) | pass | pass | not assessed | mirror only (the test asserts `Form error: Required` and `Form error: none`, the mirror of the validator's result; the error text the field itself shows is not asserted) | partly |
| `TextField` with `obscureText` (`wf_password`) | pass | pass (the mirror shows 6 characters) | not assessed | the value is not readable by design: `don't see "secret"` and `don't see "sec"` pass (see below) | pass for what is asserted |
| `TextField` with `maxLines: 3` (`wf_multiline`) | pass | pass for one line; **a line break was not typed** | not assessed | not assessed | partly |
| `CupertinoTextField` (`wf_cupertino_field`) | pass | pass | not assessed | not assessed (mirror text only) | partly |

**The value of an obscured field is never searchable (agent rule).** The agent used to match a text selector against an obscured field's value, so `see "<typed text>"` (or a prefix of it), `wait until ... appears`, `tap` and `selector_bounds` by that text found the field: a test could learn the value one prefix at a time. Its responses never carried the value (`dump_tree`, `visible_summary`, `hit_test`, the failure messages: measured, none did). The value is now never a match target; the field is still found by its id, key, label or hint. The test asserts it (`don't see "secret"`, `don't see "sec"`): with an agent from before the rule it fails at `don't see "secret"`. To assert on such a field, assert what the screen shows about it: a `Text` with its length (`see "Password length: 6"`, as `wf_password` does), the field by its id (`see #wf_password`), or what the app does after the value is accepted; `see #pw is filled` and `is empty` are planned. A suite that taps "show password" and then asserts the value now fails, because a field seen obscured stays secret once the app shows it. Limits: a field that was never seen obscured is ordinary text, two fields that share a key share the state, a `Text` that mirrors the value is just text, and the value is still readable through `run_dart`, screenshots and the app's own logs, which this rule does not cover.

### Buttons (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=buttons`, `probe-tests/widgets/buttons.probe`)

| Widget (fixture id) | find | tap | read (`is enabled` / `is disabled`) | Result |
|---|---|---|---|---|
| `ElevatedButton` (`wf_elevated`) | pass (text) | pass (`tap`; the state text `Elevated: N` counts) | pass (`is enabled`) | pass |
| `FilledButton` (`wf_filled`) | pass (text) | pass | not assessed | partly |
| `OutlinedButton` (`wf_outlined`) | pass (text) | pass | not assessed | partly |
| `TextButton` (`wf_text_button`) | pass (text) | pass | not assessed | partly |
| `CupertinoButton` (`wf_cupertino_button`) | pass (text) | pass | not assessed | partly |
| `ElevatedButton.icon` (`wf_with_icon`) | pass (the label's text) | pass | not assessed | partly |
| `IconButton` with a tooltip (`wf_icon_button`) | pass (`#id` and the tooltip text `Add item`; an icon glyph as a selector was not tried) | pass (both) | not assessed | pass |
| `FloatingActionButton` with a tooltip (`wf_fab`) | pass (`#id` and the tooltip text `Compose`) | pass (both) | not assessed | pass |
| disabled `ElevatedButton` (`wf_disabled`) | pass (text) | **a tap raises no error** (the fixture's count stays 0, but it has no handler, so that is true by construction: it does not show that the tap did nothing) | pass (`is disabled`; `is enabled` fails) | pass, with the finding below |
| `ElevatedButton` with `onPressed` and `onLongPress` (`wf_long_press`) | pass (text) | pass (`tap` runs only the tap handler; `long press` runs only the long-press handler) | not assessed | pass |

**Findings.**
- **A tap on a disabled button is not an error.** The step passes. Whether the tap reached the button is not measured: the fixture's disabled button has no handler, so its count cannot go up and "stays 0" proves nothing about the tap. A test that wants to know a button is disabled must say `see "..." is disabled`; `tap` will not tell it.
- **A held press on a button with no long-press handler ends as a tap** (`long press "Press elevated"` makes `Elevated: 1`; measured on Flutter 3.44.8 and 3.50.0). This is Flutter's own behavior (the tap recogniser has no timeout), not a fault of the step; the suite pins it so a change in either shows up.
- **Tooltips are selectors**: an icon button or a floating action button, which show no text, are found by their tooltip text, the same way they are by `#id`.

Not covered by the screen: `ButtonBar` (deprecated in the catalog), `ToggleButtons` and `SegmentedButton` (selection), `PopupMenuButton` and `DropdownButton` (pickers and menus), a button inside a dialog or a list, a double tap, a button that scrolls into view, and a button under an overlay. The 8 variants of the suite that must fail
(`probe-tests/negative/buttons.probe.expect-fail`) were run: all 8 fail with the message that says why (a count one too many, `is disabled` on an enabled button and `is enabled` on a disabled one, the disabled button's counter expected above 0, a held press taken for no tap, a label that is not on the screen, a tap taken for a long press, one button's tooltip finding another).

### WebView content (`native-test-apps/flutter-webview`, `probe-tests/webview.probe`)

An Android app with a `webview_flutter` 4.14.1 `WebViewWidget` that shows `assets/page.html` (the page of `native-test-apps/android-webview`, kept equal by a test) from the app's own assets, next to
Flutter widgets: a status text, a reload button and a text field with its echo. The agent runs in the app. WebView debugging is turned on with `AndroidWebViewController.enableDebugging(true)`.

| What | Step | Result |
|---|---|---|
| find by visible text | `see "Getting Started" in webview` | pass |
| find by CSS selector | `see css:"#result" in webview` | pass |
| absence | `don't see css:"#nope" in webview` | pass (and `see css:"#nope" in webview` fails) |
| tap by visible text | `tap "Contact support" in webview` | pass (the page then shows `Message sent`, found by `wait until "Message sent" appears in webview`) |
| type | `type "hello" into css:"#message" in webview` | pass, **read back**: `store text of css:"#message" in webview as typed`, typed into the Flutter field, the echo reads `Echo: hello` |
| read | `store text of css:"#total" in webview as total` | pass: runs of spaces collapse (`Total: 42.00   USD` is stored as `Total: 42.00 USD`, 16 characters) and hidden characters are dropped (`css:"#hidden"` stores `abc d`, 5 characters) |
| reload from Flutter | `tap "Reload page"`, then `wait until "Page loaded (2)" appears` | pass: the page starts again (`Message sent` is gone) |
| scroll | there is no WebView scroll step | **not assessed** |

**The boundary.** The Dart agent does not see inside the page, and the WebView steps do not see the Flutter widgets: `see "Getting Started"` (no `in webview`) fails, and `see "Reload page" in webview` fails. A test uses
the Dart steps for the widgets and the `in webview` steps for the page, and both work in one test. The WebView steps are Android only; iOS (`webview_flutter_wkwebview`) was not assessed.

Each failure above was shown by a variant of the suite whose steps assert something wrong or cross the boundary: all 8 variants fail with the message that says why (a title the page does not have, the raw three spaces,
the hidden characters kept, the message still there after a reload, an element that is not on the page, a Flutter widget looked for in the page, the page text looked for in the Flutter tree, a wrong typed value). The variant file is not
kept in the repository.

### How it was run

Flutter 3.50.0 (Dart 3.14.0) and Flutter 3.44.8 (Dart 3.12.2), each building the fixture with `--dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_WIDGETS=<family>`; the 3.50.0 builds used the Gradle, AGP and Kotlin versions of #711. Device: emulator-5600, AVD `ProbeWebViewSpike` (Android 15, API 35), one run per family
and build: 5 of 5 tests passed in each family on both builds (the radio and password tests as written above; the one radio check that fails is not in the test). On 3.50.0 the analyzer reports `package:flutter/material.dart` and `package:flutter/cupertino.dart` as deprecated ("deprecated after v3.47.0-0.0.pre", use `material_ui` and `cupertino_ui`): an info, the builds are unaffected, and the fixture and the agent still import them. Not run: iOS, a physical device, another Android version, TalkBack or other
accessibility services, other screen sizes or text scales.

**Buttons** were run on a build by each Flutter (`--dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_WIDGETS=buttons`; the 3.50.0 build in a scratch copy with a private SDK) on emulator-5600 (AVD `ProbeWebViewSpike`, Android 15, API 35): `buttons.probe` 7 of 7 and the negative file 0 passed, 8 failed, on each.
Not run: a physical device, another API level, TalkBack or other accessibility services, other screen sizes or text scales.

**WebView content** was run separately: Flutter 3.44.8 and 3.50.0, each building `native-test-apps/flutter-webview` with `--dart-define=PROBE_AGENT=true`, `webview_flutter` and `webview_flutter_android` 4.14.1 (pinned; see
`RELEASES.md`), on emulator-5600 (AVD `ProbeWebViewSpike`, Android 15, API 35): `webview.probe` 5 of 5 on each build. The 3.50.0 build was made in a scratch copy with a private SDK; its `pubspec.lock` differs from the committed one only in
packages the Flutter SDK pins itself (`test_api`, `matcher` and others), not in the WebView packages. Not run: a physical device, another API level, the WebView component's version of the emulator (not recorded), iOS, any WebView steps beyond
the table.

## Unsupported

None recorded yet; none has been assessed.
