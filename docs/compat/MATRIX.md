# Widget compatibility matrix

How FlutterProbe works with each widget family. **A cell stays "not assessed" until a test covers it**; it is changed only in the same pull request as the test.
`catalog.json` lists every widget; the families below are how they are grouped for testing. A widget FlutterProbe cannot work with is listed in the
"Unsupported" section, never left out.

Cells: *find* (select by text, id or type), *tap*, *type* (enter text), *scroll* (bring into view), *read* (its text or value), *fixture* (a screen in
`native-test-apps/flutter`), *device test* (a ProbeScript or Go test that ran on a device).

| Family | Examples | find | tap | type | scroll | read | fixture | device test |
|---|---|---|---|---|---|---|---|---|
| Text and display | `Text`, `RichText`, `Icon`, `Image`, `CircleAvatar`, `FlutterLogo` | see below | see below (`FlutterLogo` only) | n/a | not assessed | see below | `display` screen | `probe-tests/widgets/display.probe` |
| Buttons | `ElevatedButton`, `TextButton`, `IconButton`, `CupertinoButton` | see below | see below | n/a | not assessed | see below | `buttons` screen | `probe-tests/widgets/buttons.probe` |
| Text input | `TextField`, `TextFormField`, `CupertinoTextField` | see below | see below | see below | not assessed | see below | `textinput` screen | `probe-tests/widgets/textinput.probe` |
| Selection controls | `Checkbox`, `Switch`, `Radio`, `Slider`, `CupertinoSwitch` | see below | see below | n/a | not assessed | see below | `selection` screen | `probe-tests/widgets/selection.probe` |
| Pickers and menus | `DropdownButton`, `PopupMenuButton`, `DatePickerDialog`, `CupertinoPicker` | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed |
| Lists and scrolling | `ListView`, `GridView`, `SingleChildScrollView`, `Scrollbar`, `PageView` | see below (`PageView` not assessed) | see below | n/a | see below | see below | `lists` screen | `probe-tests/widgets/lists.probe` |
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
| `IconButton` with a tooltip (`wf_icon_button`) | pass (`#id` and the tooltip text `Add item`; an icon glyph as a selector was not tried) | pass (both) | pass (found by its tooltip, it reads as the tooltip: `see "Add item" contains "Add"`, since the fix for issue #818; found by `#wf_icon_button` it still reads as `""`) | pass |
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

Not covered by the screen: `SelectableText`, `DefaultTextStyle` and `Text` inside other widgets (buttons are their own family), text scale and `textScaler`, right-to-left and other scripts, a `Text` with `semanticsLabel`, soft wrapping without `maxLines`. Flutter 3.50.0 was not run for this family.

### Text and display: widgets without text (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=display`, `probe-tests/widgets/display.probe`)

Measured on Flutter 3.44.8 with the agent from main at `edb0fce` (it waits for a laid-out view, issue #793), on `Compat-Pixel-API35` (Android 15, API 35, `emulator-5590`) and `Compat-iPhone` (iPhone 17 Pro, iOS 26.5, simulator `A57AEE67-3473-4C20-BE25-8847C2A9D6BE`): the suite (8 tests, the text tests above included) passed 3 of 3 runs on each device, and the 12 variants that must fail (`probe-tests/negative/display.probe.expect-fail`) failed on both, each with the message in its comment. An exploratory run of 16 single steps gave the same result on both devices. No text on the screen holds "Favorite", "Logo" or "AB" (the logo's state text is `Taps: N`), so each match below comes from the widget itself. With the fix for issue #818 the suite has 9 tests (the labels are read) and 13 variants that must fail.

| Widget (fixture id) | find | tap | read | Result |
|---|---|---|---|---|
| `Icon(Icons.favorite, semanticLabel: "Favorite")` (`wf_icon`) | pass (`#wf_icon`; the semantic label `"Favorite"`; `"Favourite"` fails) | not assessed (`tap "Favorite"` completes, but the icon has no handler to show it arrived) | pass (`see "Favorite" contains "Favorite"`; the label is its text, since the fix for issue #818; it read as `""` before) | partly |
| `Image.memory` with `semanticLabel: "Logo"` (`wf_image`; an inline PNG, so the fixture needs no asset) | pass (`#wf_image`; the semantic label `"Logo"`) | not assessed | pass (`see "Logo" contains "Logo"`; `contains "Banner"` fails with the label; it read as `""` before the fix for issue #818) | partly |
| `CircleAvatar` with the child `Text("AB")` (`wf_avatar`) | pass (`#wf_avatar`; `"AB"`, its `Text`) | not assessed | pass (`see "AB" contains "AB"`) | pass |
| `FlutterLogo` inside a `GestureDetector` that counts taps (`wf_logo` on the logo) | pass (`#wf_logo`) | pass (`tap #wf_logo` makes `Taps: 1`: the tap lands on the logo's center and reaches the detector around it) | n/a | pass |

**Findings.**
- **A semantic label is a text selector.** When no visible text matches, the agent falls back to a `Semantics` label (an `Icon` and an `Image` put their `semanticLabel` in one) or a `Tooltip` message, so an icon or an image with a label is found by it, as an icon button is by its tooltip. The label is matched exactly, like a text (`"Favourite"` fails).
- **A widget found by its semantic label or tooltip reads as that label** (issue #818, fixed). Before the fix the agent's text read (`_textOf` in `probe_agent/lib/src/executor.dart`) knew only `Text`, `RichText` and text fields, while the match is the `Semantics` or `Tooltip` element, so `see "Logo" contains "Logo"` failed with `contains "", not "Logo"`; measured the same for the icon button found by its tooltip (`see "Add item" contains "Add"` read `""` on `Compat-Pixel-API35`). With the fix all three read their label, on both devices. Still `""`: a widget found by `#id` whose only text is a label or tooltip (`see #wf_icon_button contains "Add"`), since the id match is the button, not its `Tooltip`. The label is read only for a widget with no text field below it: a labelled wrapper around a text field reads as the field's text, and around an obscured field as nothing.
- **A tap by id on a widget with no handler of its own reaches a detector around it**: the tap is delivered at the widget's center, and the hit test finds the `GestureDetector` above.

Not covered by the screen: `Image.network` and `Image.asset` (loading, an error image), an `Icon` with no label (found by `#id` only; an icon glyph as a selector was not tried), `ImageIcon`, `Ink.image`, `CircleAvatar` with a `backgroundImage` and no child, and `ExcludeSemantics` or `Semantics(excludeSemantics:)` around a labelled widget. Flutter 3.50.0 was not run for this part.

### Lists and scrolling (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=lists`, `probe-tests/widgets/lists.probe`)

Measured on Flutter 3.44.8 with the agent from main (it waits for a laid-out view, issue #793, and counts a `Text` once, issue #800), on `Compat-Pixel-API35` (Android 15, API 35, `emulator-5590`) and `Compat-iPhone` (iPhone 17 Pro, iOS 26.5, simulator `A57AEE67-3473-4C20-BE25-8847C2A9D6BE`): the suite (13 tests) passed 3 of 3 runs on `Compat-iPhone`; on `Compat-Pixel-API35`, 3 of the 4 runs whose output was kept passed 13 of 13 and the 4th failed one test at `restart the app` with `agent not reachable within 30s` before the test ran (issue #823); three earlier runs whose output was not captured are not counted. The 10 variants that must fail failed on both devices, each with the message in its comment. Some runs before this one failed a test at `restart the app` with `agent not reachable within <token_read_timeout>` (issue #823: about 1 restart in 25 overall, on builds by both Flutter versions, cause unknown, load not measured); in one such run on `Compat-Pixel-API35` I observed a load average of about 49 at the end of the run, an observation, not a measured cause.

| Widget (fixture id) | find | tap | scroll | read | Result |
|---|---|---|---|---|---|
| `ListView.builder`, 200 rows (`wf_list`, rows `wf_row_N`) | pass (text; a row that is not built is absent) | pass (`tap "Row 150"` after a scroll; `tap` alone on an unbuilt row fails with "Widget not found") | pass (`scroll down until "Row 150" appears`, then `scroll up until "Row 0" appears`) | pass (state text `Tapped row: N`) | pass |
| `GridView.count`, 60 cells (`wf_grid`, cells `wf_cell_N`) | pass (an unbuilt cell is absent) | pass | pass (`Cell 55`, and back to `Cell 0`) | pass (`Tapped cell: N`) | pass |
| `CustomScrollView` with a `SliverList`, 200 items (`wf_sliver`, items `wf_item_N`) | pass (an unbuilt item is absent) | pass | pass (`Item 150`, and back to the header) | pass (`Tapped item: N`) | pass |
| `SingleChildScrollView`, 40 lines and a button (`wf_scrollview`, `wf_bottom`) | pass (the button) | pass (`tap "Bottom button"` scrolls it into view by itself: every child is built) | pass (`scroll down until "Bottom button" appears`) | pass (`Tapped bottom: yes`) | pass (`see` does not find a line below the fold, though it is built) |
| `Card` holding a `ListTile` with a title, a subtitle and a trailing icon (`wf_alice`, `wf_bob`) | pass (title and subtitle text) | pass (`tap` by the title; `long press #wf_alice`; `tap "Away" below "Bob"`) | n/a | pass (`Tapped: N`, `Long press: N`) | pass, with the findings below |
| `CupertinoListTile` in a `CupertinoListSection` (`wf_carol`) | pass (title and subtitle text) | pass | n/a | pass | pass |
| `Scrollbar` (thumb always shown) around a `ListView.builder` of 100 rows, one `ScrollController` for both (`wf_scrollbar`, rows `wf_bar_row_N`) | transparent: rows are found as in a list without one (an unbuilt row is absent) | transparent (`tap "Bar row 90"` after a scroll) | transparent: `scroll` moves the list's `ScrollPosition` directly (`jumpTo` on the `Scrollable`), so the `Scrollbar` takes no part; the same steps pass with no `Scrollbar` or with its thumb hidden | transparent (`Tapped bar row: N`) | transparent, not a test of a `Scrollbar` behaviour: removing the `Scrollbar` from the page fails no test. Its thumb is not a selector target, so dragging or tapping it cannot be assessed |

**Findings.**
- **A lazy list, grid or sliver list builds the rows near the top only, and `see` does not find the others.** `see "Row 150"` fails before a scroll, and so does `tap "Row 150"` ("Widget not found", with the texts that are on screen). `scroll down until "Row 150" appears` builds it and the next `tap` works; `scroll up until "Row 0" appears` comes back. The same holds for the grid and the sliver list.
- **`scroll down until` a target that does not exist ends with a failure, it does not run on:** `scroll down until text("Row 999"): not found after scrolling` and the texts that are on screen, after about 3 seconds on a 200-row list.
- **A `SingleChildScrollView` builds every child, so `tap "Bottom button"` finds and scrolls to a button far below with no scroll step.** `scroll down until "Bottom button" appears` also works. `see` is stricter than `tap`: `see "Line 39"` and `see "Bottom button"` fail before a scroll, because a widget that is built but below the fold is not on screen.
- **A text selector takes the first widget whose text contains it, so a name that is also in a state text selects the state text.** After `tap "Alice"` the screen shows `Tapped: Alice`, and `long press "Alice"` lands on that text and not on the tile (`Long press: none`). The tile has to be named by id (`long press #wf_alice`), or the state text must not contain the name. The suite pins both.
- **A tile's title is not a container of its tile.** `see "Online" in "Alice"` fails (`positional("Online")` not found): `in "Alice"` looks inside the `Alice` text. `"Online" below "Alice"` works and so does `tap "Online" below "Alice"`; `see "Online" above "Alice"` fails, as it should. `in #wf_alice` is not accepted: the line is read as a call of a recipe named `#wf_alice`.
- **`see exactly 1 "Row 0"` failed with "found 2"** with `probe_agent` 0.27.0: `see exactly` reported 2 for a single visible `Text` (a row title, a state text, a button label: measured on all three) on Flutter 3.44.8 and 3.50.0, and `see exactly 2 "Row 0"` passed. The cause was the agent counting a `Text` and the `RichText` it builds (issue #800). It is fixed in the agent of main (#806): `see exactly 1` of a single `Text` passes and `see exactly 2` fails with "found 1", on builds by both Flutter versions (the fixture uses the agent of the repository). The suite has the passing case, and the negative variant N9 asserts the wrong count 2.

Not covered by the screen: `PageView` (a swipe), dragging a `Scrollbar` thumb, a horizontal list, two scrollables on one screen (which one `scroll` moves), `ListView.separated`, pull to refresh, a list inside a dialog or a sheet, a list under an open keyboard, and scroll by an amount. The 10 variants of the suite that must fail
(`probe-tests/negative/lists.probe.expect-fail`) were run: all 10 fail at their own step with the message in their comment (a far row seen or tapped before a scroll, a scroll to a row that does not exist, a wrong tapped row, a far grid cell, a subtitle above its title, a subtitle `in` its title, a long press by a name that the state text also holds, `see exactly 2` of a single `Text`, and a far row of the `Scrollbar` list seen before a scroll).

### Dialogs and sheets (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=dialogs`, `probe-tests/widgets/dialogs.probe`)

| Widget (fixture id) | find | tap | close | read | Result |
|---|---|---|---|---|---|
| `AlertDialog` "Delete item?" with the actions "Cancel" and "Delete" (`wf_open_dialog`, `wf_dialog_cancel`, `wf_dialog_delete`) | pass (title and text; `wait until "Delete item?" appears`) | pass (`tap "Cancel"`; `tap #wf_dialog_delete`); `tap "Delete"` taps the title, see below | pass (an action; `go back`) | pass (`Dialog: cancelled\|deleted\|none`) | pass, with the findings below |
| `CupertinoAlertDialog` "Discard changes?" (`wf_open_cupertino`, `wf_cupertino_keep`, `wf_cupertino_discard`) | pass (title and text) | pass (by id) | pass | pass (`Cupertino dialog: kept\|discarded`) | pass |
| `CupertinoActionSheet` "Share" with "Copy link", "Send" and the cancel button "Cancel" (`wf_open_sheet`) | pass | pass (`tap "Copy link"`, `tap "Cancel"` by text) | pass | pass (`Sheet: copy\|cancelled`) | pass |
| `AlertDialog` with a tall body on the Cover page (`wf_open_cover`, `wf_open_tall`, `wf_tall_close`, a "Page button" and a "Top button") | pass | pass (`tap "Close"`; a button under the dialog body; a button outside it) | pass (the action; a tap on the barrier) | pass (`Cover dialog: closed\|none`, `Page taps`, `Top taps`) | pass, with the findings below |
| modal bottom sheet with two `ListTile`s (`wf_open_bottom`) | pass | pass (`tap "Option two"`) | pass (an option; `go back` without a choice) | pass (`Bottom sheet: two\|none`) | pass |
| `SnackBar` "Saved" with the action "Undo" (`wf_save`) | pass after `wait until "Saved" appears` | pass (`tap "Undo"`) | pass (it goes away by itself after 4 seconds, and `Undo` removes it) | pass (`Snack: saved\|undone`) | pass |

**Findings.**
- **A dialog, a sheet or a snack bar is still in the widget tree while it animates away, so a `don't see` right after the tap that closes it fails** ("Expected NOT to see ... but found 2 element(s)"). `wait until "Delete item?" disappears` first, then `don't see`. The same holds for appearing: `see "Saved"` right after `tap "Save"` fails, `wait until "Saved" appears` first. The suite waits at every such step.
- **`go back` closes a dialog and a bottom sheet** (the state text reads `none`: no choice was made). Whether the dialog is already out of the tree when `go back` returns is an observation of mine on one device and not a rule: in my runs it was gone on 3.44.8 and still there on 3.50.0 (a `don't see` right after `go back` failed in 2 of 2 runs), and a reviewer's widget test on 3.44.8 found it still in the tree; `go back` only waits for animation controllers and pending frames, not for the route animation. The test waits.
- **The page under an open dialog is still matched** (`see "Dialog: none"` passes while the dialog is open), **and where a tap on one of its widgets lands depends on what covers the widget.** `tap` finds the widget either way. On the Cover page, whose geometry is fixed (a "Page button" at the centre of the page, a centred "Tall dialog" with a 240 dp non-interactive body, a "Top button" above the dialog):
  - A button that the dialog's **body** covers: the step passes with the agent's warning `tap target text("Page button") is covered by another widget ... the tap lands on whatever is on top and the screen did not change (topmost hit: ...)`, the tap lands on the dialog, the counter stays 0 and the dialog stays open (the later `tap "Close"` finds its button). The suite does not assert the warning (a `.probe` file has no step for one); `--fail-on-warning` would fail the step; a widget test asserts it (see below).
  - A button **outside** the body: the tap lands on the barrier and dismisses the dialog (`showDialog` default), the button's own handler does not run (`Top taps: 0`), and a following `tap "Close"` fails with "Widget not found".
  - **My first reading was wrong twice.** I first wrote "the barrier takes the tap" for a button that was in fact behind the dialog's body (the warning was in my output), and then put the button at a place in a list that gave a different answer on other screens (a reviewer found three of four screen geometries tapping the dialog's Cancel button instead). The Cover page is built so that the answer does not depend on the screen, and `native-test-apps/flutter/test/dialogs_cover_test.dart` (`flutter test` in that folder, no device) runs the agent's `tap` on four screens (1080x2400 at 2.625, 1080x2340 at 2.75, 1080x2400 at 2.8 and 1080x1920 at 2.625) and checks where each tap lands; a mutant that moves the button off the centre fails all 12 of its tests. It is run by hand, not by CI (CI runs the tests of `probe_agent` only), and the Cover page is built for a portrait screen of at least about 400x800 dp, the size of the four surfaces: on a 360x640 dp screen the Tall dialog does not open the way the suite expects, and in landscape the dialog covers the Top button. The suite itself was run on one device, the emulator below.
- **A text selector takes the first widget whose text contains it, so `tap "Delete"` taps the dialog's title "Delete item?" and not its button "Delete".** The dialog stays open (`Dialog: none`). The button has to be named by id (`tap #wf_dialog_delete`). `tap "Discard"` hit the title "Discard changes?" the same way, which is why the Cupertino actions have ids; `tap "Cancel"` and `tap "Copy link"` are not ambiguous and work by text.
- **A `SnackBar` that has an action stays on screen until it is dismissed, on the Flutter versions measured, unless it is built with `persist: false`.** The fixture sets it, so that "it goes away by itself" can be tested; without it, `wait until "Saved" disappears` timed out after 30 seconds. That is Flutter's own default, not the agent's.

Not covered by the screen: iOS (a Cupertino dialog and sheet were run on Android only), a dialog with `barrierDismissible: false`, a dialog with a text field, a scrolling dialog, `showDatePicker` and `showTimePicker`, a persistent bottom sheet, a floating or queued snack bar, `PopupMenuButton`, `Drawer`, and native system dialogs (see `tools/system-dialogs`). The 9 variants of the suite that must fail
(`probe-tests/negative/dialogs.probe.expect-fail`) were run: all 9 fail at their own step with the message in their comment (a snack bar and its action before `Save`, a dialog seen after Cancel, `tap "Delete"` taken for the button, a tap on a button that the dialog body covers counted, the wrong state, a sheet seen after an option, a state before its tap, the Cancel button after a tap on the barrier).

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

**Lists and scrolling** were run on a build by each Flutter (`--dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_WIDGETS=lists`; the 3.50.0 build in a scratch copy with a private SDK) on emulator-5600 (AVD `ProbeWebViewSpike`, Android 15, API 35): `lists.probe` 10 of 10 on each build and the negative file 0 passed, 9 failed, on each; after the test of a line below the fold was added (and main merged), 11 of 11 and 0 of 9 again on the 3.44.8 build. After the agent fix of issue #800 (#806) and the test `see exactly counts a visible Text once`, 12 tests: **Flutter 3.50.0**: the suite 12 of 12 in 1 run, and the negative file in 1 run 0 passed, 9 failed, every test at its own step, N9 ending with "found 1". **Flutter 3.44.8**: the suite in 3 runs, 11 of 12 in each, the twelfth test not running because `before each test` failed to restart the app (the reconnect failure below), a different test each time, so every test passed in at least one run but not all 12 in one run; the negative file in 3 runs, 0 passed and 9 failed in each, but in 2 of the 3 runs one test failed in `before each test` (the same restart failure) instead of at its own step, and in the third run every test failed at its own step, N9 ending with "found 1".
In some earlier runs of small test files, the first test failed in `before each test` with "restart the app: reconnect with pre-shared token: agent not reachable within 20s" and the other tests of the file passed; it also happened in each of the 3 full-suite runs on 3.44.8 described above (once per run), and in none of the 2 runs on 3.50.0 listed here. It is tracked in issue #823 (measurements per Flutter build and the `probe.yaml` of the runs): it happened on builds by both Flutter versions, about 1 restart in 25 over all my runs, and the machine load was not measured. Not run: a physical device, another API level, iOS, TalkBack or other accessibility services, other screen sizes or text scales.

**Dialogs and sheets** were run on a build by each Flutter (`--dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_WIDGETS=dialogs`; the 3.50.0 build in a scratch copy with a private SDK) on emulator-5600 (AVD `ProbeWebViewSpike`, Android 15, API 35). The final suite has 12 tests and 9 negative variants, run on an emulator whose screen is 1080x2400 at 420 dpi (`wm size`, `wm density`; 411x914 dp, a ratio of 2.625). **Flutter 3.50.0**: the suite 12 of 12 in each of 3 runs, and the negative file 0 passed, 9 failed in each of 3 runs, every test at its own step. **Flutter 3.44.8**: the suite 12 of 12 in each of 3 runs, and the negative file 0 passed, 9 failed in each of 3 runs, every test at its own step. None of these 12 runs had a restart failure. Earlier versions of the suite and of the screen (before the Cover page replaced the covered-tap test) did: `before each test` failed to restart the app ("restart the app: reconnect with pre-shared token: agent not reachable within 20s") in a different test of most runs, on both builds, about 1 restart in 25 over all my runs; machine load was not measured and nothing re-ran a test (no `retry_failed_tests`, `agent.token_read_timeout` 20 s, `agent.dial_timeout` 30 s). It is tracked in issue #823. The geometry of the Cover page is checked separately, without a device, by `test/dialogs_cover_test.dart` on four screens (12 of 12). Not run: a physical device, another API level, iOS, TalkBack or other accessibility services, other screen sizes or text scales.

**WebView content** was run separately: Flutter 3.44.8 and 3.50.0, each building `native-test-apps/flutter-webview` with `--dart-define=PROBE_AGENT=true`, `webview_flutter` and `webview_flutter_android` 4.14.1 (pinned; see
`RELEASES.md`), on emulator-5600 (AVD `ProbeWebViewSpike`, Android 15, API 35): `webview.probe` 5 of 5 on each build (6 of 6 after the result-text test was added, run on the 3.44.8 build), and the negative file 0 passed, 9 failed. The 3.50.0 build was made in a scratch copy with a private SDK; its `pubspec.lock` differs from the committed one only in
packages the Flutter SDK pins itself (`test_api`, `matcher` and others), not in the WebView packages. Not run: a physical device, another API level, the WebView component's version of the emulator (not recorded), iOS, any WebView steps beyond
the table.

## Unsupported

None recorded yet; none has been assessed.
