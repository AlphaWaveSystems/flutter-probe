# Widget compatibility matrix

How FlutterProbe works with each widget family. **A cell stays "not assessed" until a test covers it**; it is changed only in the same pull request as the test.
`catalog.json` lists every widget; the families below are how they are grouped for testing. A widget FlutterProbe cannot work with is listed in the
"Unsupported" section, never left out.

Cells: *find* (select by text, id or type), *tap*, *type* (enter text), *scroll* (bring into view), *read* (its text or value), *fixture* (a screen in
`native-test-apps/flutter`), *device test* (a ProbeScript or Go test that ran on a device).

| Family | Examples | find | tap | type | scroll | read | fixture | device test |
|---|---|---|---|---|---|---|---|---|
| Text and display | `Text`, `RichText`, `Icon`, `Image` | see below (`Text`, `RichText` only) | not assessed | n/a | not assessed | see below | `display` screen | `probe-tests/widgets/display.probe` |
| Buttons | `ElevatedButton`, `TextButton`, `IconButton`, `CupertinoButton` | see below | see below | n/a | not assessed | see below | `buttons` screen | `probe-tests/widgets/buttons.probe` |
| Text input | `TextField`, `TextFormField`, `CupertinoTextField` | see below | see below | see below | not assessed | see below | `textinput` screen | `probe-tests/widgets/textinput.probe` |
| Selection controls | `Checkbox`, `Switch`, `Radio`, `Slider`, `CupertinoSwitch` | see below | see below | n/a | not assessed | see below | `selection` screen | `probe-tests/widgets/selection.probe` |
| Pickers and menus | `DropdownButton`, `PopupMenuButton`, `DatePickerDialog`, `CupertinoPicker` | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed |
| Lists and scrolling | `ListView`, `GridView`, `SingleChildScrollView`, `PageView` | see below (`PageView` not assessed) | see below | n/a | see below | see below | `lists` screen | `probe-tests/widgets/lists.probe` |
| Dialogs and sheets | `AlertDialog`, `BottomSheet`, `SnackBar`, `CupertinoAlertDialog` | see below | see below | n/a | n/a | see below | `dialogs` screen | `probe-tests/widgets/dialogs.probe` |
| Navigation | `AppBar`, `NavigationBar`, `TabBar`, `Drawer`, `Navigator` | see below (`AppBar`, `Navigator` only) | see below | n/a | not assessed | see below | `navigation` screen | `probe-tests/widgets/navigation.probe` |
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

### Navigation (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=navigation`, `probe-tests/widgets/navigation.probe`)

Measured on Flutter 3.44.8, on `Compat-Pixel-API35` (Android 15, API 35, `emulator-5590`) and `Compat-iPhone` (iPhone 17 Pro, iOS 26.5, simulator `A57AEE67-3473-4C20-BE25-8847C2A9D6BE`), 4 of 4 three times on each; the 5 variants that must fail (`probe-tests/negative/navigation.probe.expect-fail`) failed, 5 of 5 on each, each with the message its comment states.

| Widget or behavior (fixture id) | find | tap | read | Result |
|---|---|---|---|---|
| `AppBar` title (`Inbox`) | pass (`see "Inbox"`) | n/a | pass (its text) | pass |
| `AppBar` action: an `IconButton` with the tooltip `Search` (`wf_search`) | pass (the tooltip text and `#wf_search`) | pass (both; the state text `Queries: N` counts) | n/a | pass |
| `Navigator.push` of a `MaterialPageRoute` (`wf_open_message`, the route titled `Message`) | pass (`see "Message"`, `see "From: Ada"` after `wait until ... appears`) | pass (`tap "Open message"`) | pass | pass |
| the route under an opaque route | **not matched** (the same test first sees `"Inbox"` and `"Queries: 0"`, then, while the message route is on top, `don't see "Inbox"`, `"Queries: 0"`, `"Open message"` all pass; `see "Inbox"` fails) | n/a | n/a | pass: the agent's route rule (PT-03) holds on a device |
| `go back` (the agent calls `Navigator.pop()` on the app's navigator; not a system back key) | pass (the first route is seen again) | n/a | n/a | pass, with the finding below |

**Findings.**
- **The route under an opaque route is not matched, on a device.** The agent's rule (`_isVisible` and `_routeOnScreen` in `probe_agent/lib/src/finder.dart`, unit test "ProbeFinder route-awareness (PT-03)") holds on both devices: while the message route is on top, the first route's title, texts and button are not found, and `see "Inbox"` fails.
- **During a pop, the leaving route is still matched.** Right after `go back`, `wait until "Inbox" appears` passes at once while the message route is still on screen in Material's page transition, and `don't see "From: Ada"` then finds it. Measured on both devices with those steps alone, 5 runs each: `Compat-Pixel-API35` 4 of 5 runs, `Compat-iPhone` 5 of 5. A test that checks a pop waits for the leaving route to go: `wait until "From: Ada" disappears`.

Not covered by the screen: `NavigationBar`, `TabBar`, `Drawer`, a route that is not opaque (a dialog, a popup menu, a bottom sheet: the page under it stays matched by design; item 8 of `NEXT.md`), named routes and `go_router`, the Android system back key and the iOS edge swipe (`go back` pops in the app, it does not press either), a nested `Navigator`, and Cupertino routes (item 10 of `NEXT.md`). Flutter 3.50.0 was not run for this family.
### Text and display: text (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=display`, `probe-tests/widgets/display.probe`)

Measured on Flutter 3.44.8, on `Compat-Pixel-API35` (Android 15, API 35, `emulator-5590`) and `Compat-iPhone` (iPhone 17 Pro, iOS 26.5, simulator `A57AEE67-3473-4C20-BE25-8847C2A9D6BE`): with the wait below, 5 of 5 tests in 10 of 10 runs on Android and in 5 of 5 runs on iOS (iOS also passed 3 of 3 before the wait was added); the 9 variants that must fail (`probe-tests/negative/display.probe.expect-fail`) failed, 9 of 9 on each, each with the message its comment states.

| Widget (fixture id) | find | read | Result |
|---|---|---|---|
| `Text` (`wf_text`, `Order total: 42.00`) | pass (the whole text, a part of it, `#wf_text`) | pass (`see "Order total" contains "42.00"`; a value the text does not hold fails with the text it has) | pass |
| `Text.rich` of two spans, `Hello ` and a bold `world` (`wf_rich`) | pass (`"Hello world"`: the spans are matched as one text; `"Hello"` and `"world"` as parts of it; `#wf_rich`) | pass (`see "Hello world" contains "world"`; the text read is the whole `Hello world`, as a failing `contains "earth"` shows) | pass |
| `RichText` of two spans, `Painted ` and `rich text` (`wf_richtext`) | pass (`"Painted rich text"`, `"rich text"`, `#wf_richtext`) | pass (`see "Painted rich text" contains "rich text"`; `contains "plain text"` fails) | pass |
| `Text` with `maxLines: 1` and `TextOverflow.ellipsis`, chosen longer than the screen is wide (an inference from its length; the agent does not report the painted width) (`wf_ellipsis`) | **the whole string is matched, the part that is not painted included** (`see "...United States of America"` passes; the painted, cut form with `…` does not); `#wf_ellipsis` | pass (`see "Shipping address" contains "United States of America"`: the text read is the whole string; `contains "Canada"` fails) | pass, with the finding below |

**Findings.**
- **A text selector matches the text the app gave the widget, not what is painted.** A `Text` cut with an ellipsis is found by its whole string, including the part a user cannot see, and not by the cut form shown on screen. A test that must show that a text is fully visible cannot rely on `see`.
- **A text selector matches the whole text or any part of it, exactly:** case counts (`"hello world"` fails) and spaces count (`"Hello  world"` fails); the spans of a `Text.rich` or a `RichText` are matched as one text, joined as written.
- **Right after `restart the app`, the screen can still be being built** (issue #793): on `Compat-Pixel-API35`, without a wait 3 of 10 runs failed, with a wait for the first item 1 of 10, and with a wait for the screen's last widget 10 of 10 passed. The suite waits for the last widget after the restart.

Not covered by the screen: `SelectableText`, `DefaultTextStyle` and `Text` inside other widgets (buttons are their own family), text scale and `textScaler`, right-to-left and other scripts, a `Text` with `semanticsLabel` (item 3 asks how a semantic label is matched), soft wrapping without `maxLines`, and `Icon`, `Image`, `FlutterLogo`, `CircleAvatar` (item 3). Flutter 3.50.0 was not run for this family.

### Lists and scrolling (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=lists`, `probe-tests/widgets/lists.probe`)

| Widget (fixture id) | find | tap | scroll | read | Result |
|---|---|---|---|---|---|
| `ListView.builder`, 200 rows (`wf_list`, rows `wf_row_N`) | pass (text; a row that is not built is absent) | pass (`tap "Row 150"` after a scroll; `tap` alone on an unbuilt row fails with "Widget not found") | pass (`scroll down until "Row 150" appears`, then `scroll up until "Row 0" appears`) | pass (state text `Tapped row: N`) | pass |
| `GridView.count`, 60 cells (`wf_grid`, cells `wf_cell_N`) | pass (an unbuilt cell is absent) | pass | pass (`Cell 55`, and back to `Cell 0`) | pass (`Tapped cell: N`) | pass |
| `CustomScrollView` with a `SliverList`, 200 items (`wf_sliver`, items `wf_item_N`) | pass (an unbuilt item is absent) | pass | pass (`Item 150`, and back to the header) | pass (`Tapped item: N`) | pass |
| `SingleChildScrollView`, 40 lines and a button (`wf_scrollview`, `wf_bottom`) | pass (the button) | pass (`tap "Bottom button"` scrolls it into view by itself: every child is built) | pass (`scroll down until "Bottom button" appears`) | pass (`Tapped bottom: yes`) | pass (`see` does not find a line below the fold, though it is built) |
| `Card` holding a `ListTile` with a title, a subtitle and a trailing icon (`wf_alice`, `wf_bob`) | pass (title and subtitle text) | pass (`tap` by the title; `long press #wf_alice`; `tap "Away" below "Bob"`) | n/a | pass (`Tapped: N`, `Long press: N`) | pass, with the findings below |
| `CupertinoListTile` in a `CupertinoListSection` (`wf_carol`) | pass (title and subtitle text) | pass | n/a | pass | pass |

**Findings.**
- **A lazy list, grid or sliver list builds the rows near the top only, and `see` does not find the others.** `see "Row 150"` fails before a scroll, and so does `tap "Row 150"` ("Widget not found", with the texts that are on screen). `scroll down until "Row 150" appears` builds it and the next `tap` works; `scroll up until "Row 0" appears` comes back. The same holds for the grid and the sliver list.
- **`scroll down until` a target that does not exist ends with a failure, it does not run on:** `scroll down until text("Row 999"): not found after scrolling` and the texts that are on screen, after about 3 seconds on a 200-row list.
- **A `SingleChildScrollView` builds every child, so `tap "Bottom button"` finds and scrolls to a button far below with no scroll step.** `scroll down until "Bottom button" appears` also works. `see` is stricter than `tap`: `see "Line 39"` and `see "Bottom button"` fail before a scroll, because a widget that is built but below the fold is not on screen.
- **A text selector takes the first widget whose text contains it, so a name that is also in a state text selects the state text.** After `tap "Alice"` the screen shows `Tapped: Alice`, and `long press "Alice"` lands on that text and not on the tile (`Long press: none`). The tile has to be named by id (`long press #wf_alice`), or the state text must not contain the name. The suite pins both.
- **A tile's title is not a container of its tile.** `see "Online" in "Alice"` fails (`positional("Online")` not found): `in "Alice"` looks inside the `Alice` text. `"Online" below "Alice"` works and so does `tap "Online" below "Alice"`; `see "Online" above "Alice"` fails, as it should. `in #wf_alice` is not accepted: the line is read as a call of a recipe named `#wf_alice`.
- **`see exactly 1 "Row 0"` fails with "found 2"** with `probe_agent` 0.27.0: `see exactly` reported 2 for a single visible `Text` (a row title, a state text, a button label: measured on all three) on Flutter 3.44.8 and 3.50.0, and `see exactly 2 "Row 0"` passed. The cause was the agent counting a `Text` and the `RichText` it builds (issue #800); it is fixed in the next `probe_agent` release, where one `Text` is 1. The negative variant N9 pins the 0.27.0 behaviour and changes with that release.

Not covered by the screen: `PageView` (a swipe), `Scrollbar`, a horizontal list, two scrollables on one screen (which one `scroll` moves), `ListView.separated`, pull to refresh, a list inside a dialog or a sheet, a list under an open keyboard, scroll by an amount, and the iOS behavior of any of this. The 9 variants of the suite that must fail
(`probe-tests/negative/lists.probe.expect-fail`) were run: all 9 fail at their own step with the message in their comment (a far row seen or tapped before a scroll, a scroll to a row that does not exist, a wrong tapped row, a far grid cell, a subtitle above its title, a subtitle `in` its title, a long press by a name that the state text also holds, and `see exactly 1`).

### Dialogs and sheets (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=dialogs`, `probe-tests/widgets/dialogs.probe`)

| Widget (fixture id) | find | tap | close | read | Result |
|---|---|---|---|---|---|
| `AlertDialog` "Delete item?" with the actions "Cancel" and "Delete" (`wf_open_dialog`, `wf_dialog_cancel`, `wf_dialog_delete`) | pass (title and text; `wait until "Delete item?" appears`) | pass (`tap "Cancel"`; `tap #wf_dialog_delete`); `tap "Delete"` taps the title, see below | pass (an action; `go back`) | pass (`Dialog: cancelled\|deleted\|none`) | pass, with the findings below |
| `CupertinoAlertDialog` "Discard changes?" (`wf_open_cupertino`, `wf_cupertino_keep`, `wf_cupertino_discard`) | pass (title and text) | pass (by id) | pass | pass (`Cupertino dialog: kept\|discarded`) | pass |
| `CupertinoActionSheet` "Share" with "Copy link", "Send" and the cancel button "Cancel" (`wf_open_sheet`) | pass | pass (`tap "Copy link"`, `tap "Cancel"` by text) | pass | pass (`Sheet: copy\|cancelled`) | pass |
| modal bottom sheet with two `ListTile`s (`wf_open_bottom`) | pass | pass (`tap "Option two"`) | pass (an option; `go back` without a choice) | pass (`Bottom sheet: two\|none`) | pass |
| `SnackBar` "Saved" with the action "Undo" (`wf_save`) | pass after `wait until "Saved" appears` | pass (`tap "Undo"`) | pass (it goes away by itself after 4 seconds, and `Undo` removes it) | pass (`Snack: saved\|undone`) | pass |

**Findings.**
- **A dialog, a sheet or a snack bar is still in the widget tree while it animates away, so a `don't see` right after the tap that closes it fails** ("Expected NOT to see ... but found 2 element(s)"). `wait until "Delete item?" disappears` first, then `don't see`. The same holds for appearing: `see "Saved"` right after `tap "Save"` fails, `wait until "Saved" appears` first. The suite waits at every such step.
- **`go back` closes a dialog and a bottom sheet** (the state text reads `none`: no choice was made). Whether the dialog is out of the tree when `go back` returns depends on the build: on Flutter 3.44.8 it was, on 3.50.0 it was not (a `don't see` right after `go back` failed there in 2 of 2 runs), so the test waits.
- **The page under an open dialog is still matched, and a tap on it does not reach it.** `see "Page taps: 0"` passes while the dialog is open, and `tap "Page button"` finds the button and passes, but the barrier takes the tap: the counter stays 0, and one tap after the dialog is closed makes it 1.
- **A text selector takes the first widget whose text contains it, so `tap "Delete"` taps the dialog's title "Delete item?" and not its button "Delete".** The dialog stays open (`Dialog: none`). The button has to be named by id (`tap #wf_dialog_delete`). `tap "Discard"` hit the title "Discard changes?" the same way, which is why the Cupertino actions have ids; `tap "Cancel"` and `tap "Copy link"` are not ambiguous and work by text.
- **A `SnackBar` that has an action stays on screen until it is dismissed, on the Flutter versions measured, unless it is built with `persist: false`.** The fixture sets it, so that "it goes away by itself" can be tested; without it, `wait until "Saved" disappears` timed out after 30 seconds. That is Flutter's own default, not the agent's.

Not covered by the screen: iOS (a Cupertino dialog and sheet were run on Android only), tapping the barrier to dismiss, a dialog with a text field, a scrolling dialog, `showDatePicker` and `showTimePicker`, a persistent bottom sheet, a floating or queued snack bar, `PopupMenuButton`, `Drawer`, and native system dialogs (see `tools/system-dialogs`). The 8 variants of the suite that must fail
(`probe-tests/negative/dialogs.probe.expect-fail`) were run: all 8 fail at their own step with the message in their comment (a snack bar and its action before `Save`, a dialog seen after Cancel, `tap "Delete"` taken for the button, a tap on the page under an open dialog counted, the wrong state, a sheet seen after an option, a state before its tap).

### WebView content (`native-test-apps/flutter-webview`, `probe-tests/webview.probe`)

An Android app with a `webview_flutter` 4.14.1 `WebViewWidget` that shows `assets/page.html` (the page of `native-test-apps/android-webview`, kept equal by a test) from the app's own assets, next to
Flutter widgets: a status text, a reload button and a text field with its echo. The agent runs in the app. WebView debugging is turned on with `AndroidWebViewController.enableDebugging(true)`.

| What | Step | Result |
|---|---|---|
| find by visible text | `see "Getting Started" in webview` | pass |
| find by CSS selector | `see css:"#result" in webview` | pass, but **it shows only that the element exists** (it is on the page before the tap too, empty); what it holds is read with `store text` (next rows) |
| absence | `don't see css:"#nope" in webview` | pass (and `see css:"#nope" in webview` fails) |
| tap by visible text | `tap "Contact support" in webview` | pass (the page then shows `Message sent`, found by `wait until "Message sent" appears in webview`, and the text of `#result` is **read back**: `store text of css:"#result" in webview`, typed into the Flutter field, the echo reads `Echo: Message sent`) |
| type | `type "hello" into css:"#message" in webview` | pass, **read back**: `store text of css:"#message" in webview as typed`, typed into the Flutter field, the echo reads `Echo: hello` |
| read | `store text of css:"#total" in webview as total` | pass: runs of spaces collapse (`Total: 42.00   USD` is stored as `Total: 42.00 USD`, 16 characters) and hidden characters are dropped (`css:"#hidden"` stores `abc d`, 5 characters) |
| reload from Flutter | `tap "Reload page"`, then `wait until "Page loaded (2)" appears` | pass: the page starts again (`Message sent` is gone) |
| scroll | there is no WebView scroll step | **not assessed** |

**The boundary.** The Dart agent does not see inside the page, and the WebView steps do not see the Flutter widgets: `see "Getting Started"` (no `in webview`) fails, and `see "Reload page" in webview` fails. A test uses
the Dart steps for the widgets and the `in webview` steps for the page, and both work in one test. The WebView steps are Android only; iOS (`webview_flutter_wkwebview`) was not assessed.

The variants of the suite whose steps assert something wrong or cross the boundary are in `native-test-apps/flutter-webview/probe-tests/negative/webview.probe.expect-fail` (the extension keeps them out of directory runs; the header says how to run them). All 9 fail with
the message that says why (a title the page does not have, the raw three spaces, the hidden characters kept, the message still there after a reload, an element that is not on the page, a Flutter widget looked for in the page, the page text looked for in the Flutter tree, a wrong typed value,
a wrong result text).

### How it was run

Flutter 3.50.0 (Dart 3.14.0) and Flutter 3.44.8 (Dart 3.12.2), each building the fixture with `--dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_WIDGETS=<family>`; the 3.50.0 builds used the Gradle, AGP and Kotlin versions of #711. Device: emulator-5600, AVD `ProbeWebViewSpike` (Android 15, API 35), one run per family
and build: 5 of 5 tests passed in each family on both builds (the radio and password tests as written above; the one radio check that fails is not in the test). On 3.50.0 the analyzer reports `package:flutter/material.dart` and `package:flutter/cupertino.dart` as deprecated ("deprecated after v3.47.0-0.0.pre", use `material_ui` and `cupertino_ui`): an info, the builds are unaffected, and the fixture and the agent still import them. Not run: iOS, a physical device, another Android version, TalkBack or other
accessibility services, other screen sizes or text scales.

**Buttons** were run on a build by each Flutter (`--dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_WIDGETS=buttons`; the 3.50.0 build in a scratch copy with a private SDK) on emulator-5600 (AVD `ProbeWebViewSpike`, Android 15, API 35): `buttons.probe` 7 of 7 and the negative file 0 passed, 8 failed, on each.
Not run: a physical device, another API level, TalkBack or other accessibility services, other screen sizes or text scales.

**Lists and scrolling** were run on a build by each Flutter (`--dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_WIDGETS=lists`; the 3.50.0 build in a scratch copy with a private SDK) on emulator-5600 (AVD `ProbeWebViewSpike`, Android 15, API 35): `lists.probe` 10 of 10 on each build and the negative file 0 passed, 9 failed, on each; after the test of a line below the fold was added (and main merged), 11 of 11 and 0 of 9 again on the 3.44.8 build.
In some earlier runs of small test files, the first test failed in `before each test` with "restart the app: reconnect with pre-shared token: agent not reachable within 20s" and the other tests of the file passed; it did not happen in the full-suite runs above. It is under investigation (a follow-up of issue #793). Not run: a physical device, another API level, iOS, TalkBack or other accessibility services, other screen sizes or text scales.

**Dialogs and sheets** were run on a build by each Flutter (`--dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_WIDGETS=dialogs`; the 3.50.0 build in a scratch copy with a private SDK) on emulator-5600 (AVD `ProbeWebViewSpike`, Android 15, API 35). Flutter 3.50.0: `dialogs.probe` 11 of 11 in each of 2 runs, and the negative file 0 passed, 8 failed. Flutter 3.44.8: 11 of 11 in one run of an earlier version of the suite (it differs from the final one in the `go back` test, which got its wait), and 10 passed in each of 2 runs of the final suite, the eleventh test not running because `before each test` failed to restart the app ("restart the app: reconnect with pre-shared token: agent not reachable within 20s"), a different test each time; so every test passed on 3.44.8 in at least one run, but not all 11 in one run of the final suite. The negative file: 0 passed, 8 failed. The reconnect failure is under investigation (a follow-up of issue #793) and tracks the load of the machine. Not run: a physical device, another API level, iOS, TalkBack or other accessibility services, other screen sizes or text scales.

**WebView content** was run separately: Flutter 3.44.8 and 3.50.0, each building `native-test-apps/flutter-webview` with `--dart-define=PROBE_AGENT=true`, `webview_flutter` and `webview_flutter_android` 4.14.1 (pinned; see
`RELEASES.md`), on emulator-5600 (AVD `ProbeWebViewSpike`, Android 15, API 35): `webview.probe` 5 of 5 on each build (6 of 6 after the result-text test was added, run on the 3.44.8 build), and the negative file 0 passed, 9 failed. The 3.50.0 build was made in a scratch copy with a private SDK; its `pubspec.lock` differs from the committed one only in
packages the Flutter SDK pins itself (`test_api`, `matcher` and others), not in the WebView packages. Not run: a physical device, another API level, the WebView component's version of the emulator (not recorded), iOS, any WebView steps beyond
the table.

## Unsupported

None recorded yet; none has been assessed.
