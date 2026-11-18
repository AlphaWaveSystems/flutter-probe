# What to assess next

A ranked list of the widgets that apps use most and that `MATRIX.md` has not assessed, with the exact test to write for each. It is a plan, not a result: no cell of the matrix changes because of this file, and nothing here says how FlutterProbe behaves with a widget. Each item ends in a test, and the matrix changes in the pull request that adds that test.

## How the ranking was made

- **Source.** [`flutter/samples`](https://github.com/flutter/samples) at `12a191622909214b516b0626fc1413be1000d12c` (2026-11-11). This is the Flutter team's own collection of sample apps, and is public and current. It has 36 packages with Dart code (a package is the nearest folder with a `pubspec.yaml`).
- **Count.** For each widget in `catalog.json` (Flutter 3.50.0, abstract widgets left out), the number of packages that construct it at least once (`Name(` or `Name.named(`), and the number of such calls. Tests (`test/`, `integration_test/`, `test_driver/`), generated files and comments are left out. The script is `widget-usage.py` in this folder: `python3 -I docs/compat/widget-usage.py docs/compat/catalog.json <samples checkout> out.json`.
- **Rank.** By packages, then calls. Layout-only widgets (`Column`, `Center`, `Padding`, `SizedBox`, `Theme` and others) are left out of the main list: a test does not find, tap or read them. The ones that change what `see` or `tap` mean (overlap, opacity) are a separate item.
- **Bias.** The sample apps are demos. Pickers, menus, tabs, drawers and sheets are rare in them (`DropdownButton` 0 packages, `TabBar` 2, `Drawer` 1), so the source cannot rank these families. They are listed after the main ranking without a claim about how often production apps use them.

## Ranked list

"Packages" is how many of the 36 packages use the widget. The matrix state is that of `MATRIX.md` today.

| # | Widget(s) | Packages | Family in the matrix | State today | What the test must settle |
|---|---|---|---|---|---|
| 1 | `Text`, `RichText` | 32, 3 | Text and display | assessed: `display` screen, `Text` and `RichText` (find and read) on two named devices (see `MATRIX.md`); `Icon` and `Image` stay item 3 | how `see` matches the text a user sees |
| 2 | `Scaffold` + `AppBar`, `Navigator` (push, pop, back) | 26 + 22, 9 | Navigation | assessed: `navigation` screen, `AppBar` and `Navigator` on two named devices (see `MATRIX.md`); back: in-app pop only (not the system back key or iOS edge swipe) | the title and actions of a bar, and what `see` matches after a push, on a device |
| 3 | `Icon`, `Image`, `FlutterLogo`, `CircleAvatar` | 19, 7, 7, 5 | Text and display | assessed: `display` screen on two named devices (see `MATRIX.md`); a semantic label is found and, since the fix for issue #818, read | how a widget with no text is found |
| 4 | `ListView` (and `.builder`), `SingleChildScrollView`, `CustomScrollView` + `SliverList`, `GridView`, `Scrollbar` | 12, 7, 5 + 4, 3, 3 | Lists and scrolling | assessed: `lists` screen, `ListView.builder`, `GridView.count`, `SliverList`, `SingleChildScrollView`, and a `Scrollbar` around a list (transparent to `scroll`, `tap` and `see`; its thumb cannot be selected), on two named devices (Android and iOS) with Flutter 3.44.8, and earlier on a build by each Flutter on one emulator (see `MATRIX.md`) | scrolling to an item that is not built yet |
| 5 | `Card` + `ListTile`, `CupertinoListTile` | 9 + 8, 2 | Layout and containers (`Card`); Lists and scrolling (the tiles) | assessed: the tiles on the `lists` screen, on a build by each Flutter on one emulator (see `MATRIX.md`); `Card` alone stays in Layout and containers | tapping a row and reading its subtitle |
| 6 | `CircularProgressIndicator`, `LinearProgressIndicator` | 8, 1 | Text and display | assessed: `progress` screen (its own family), a spinner waited for until it goes away, one that never does, a progress bar, on two named devices (see `MATRIX.md`) | waiting for a spinner to go away |
| 7 | `GestureDetector`, `InkWell` | 7, 5 | Gestures | assessed: `gestures` screen, tap, double tap and long press on a text inside a detector, nested detectors and a `Semantics`-wrapped detector, on two named devices (see `MATRIX.md`); a detector with tap and double tap runs `onTap` 300 ms late | a tap on a text inside a gesture widget |
| 8 | `AlertDialog`, `Dialog`, `CupertinoAlertDialog`, `CupertinoActionSheet` | 5, 1, 1, 2 | Dialogs and sheets | assessed: `dialogs` screen, `AlertDialog`, `CupertinoAlertDialog`, `CupertinoActionSheet` and a bottom sheet, on a build by each Flutter on one emulator and on two named devices (Android and iOS, Flutter 3.44.8) (see `MATRIX.md`); `Dialog` alone not run | a dialog's text, its actions, and closing it |
| 9 | `SnackBar` (through `ScaffoldMessenger`) | 4 | Dialogs and sheets | assessed: `dialogs` screen, a `SnackBar` with an action, on a build by each Flutter on one emulator and on two named devices (Android and iOS, Flutter 3.44.8) (see `MATRIX.md`) | a message that appears and goes away by itself |
| 10 | `CupertinoPageScaffold` + `CupertinoNavigationBar`, `CupertinoSliverNavigationBar`, `CupertinoTabScaffold` + `CupertinoTabBar` | 4, 3, 2 | Navigation | not assessed | the Cupertino bars: title, back, tabs |
| 11 | `Stack` + `Positioned`, `Opacity`, `Visibility` | 9 + 8, 4, 1 | Layout and containers | assessed: `layout` screen, `Opacity(0)`, `Visibility`, `Offstage`, `IgnorePointer`, `AbsorbPointer` and a covered `Stack` child, on two named devices (see `MATRIX.md`); found issue #842 (a tap inside `IgnorePointer` passed silently) | whether a covered or invisible widget counts as seen or tappable |

### The tests

Each item is a new screen of the Flutter fixture app (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=<family>`, as for `selection`, `textinput` and `buttons`) with a suite in `probe-tests/widgets/`, and a file of variants that must fail in `probe-tests/negative/` (`.expect-fail`). Ids are `ValueKey`s with the `wf_` prefix the existing screens use. Every step below exists in the grammar today; the one follow-up that needs a missing verb says so.

1. **`display` screen: text.** Widgets:
   - `Text("Order total: 42.00")` (`wf_text`)
   - a `Text` with `maxLines: 1` and `overflow: TextOverflow.ellipsis` whose string is longer than the screen is wide (`wf_ellipsis`)
   - `Text.rich` with two spans, "Hello " and a bold "world" (`wf_rich`). The source has no `Text.rich` call; it is here because it builds the same `RichText` that the 3 packages construct.
   - a `RichText` (`wf_richtext`)

   Steps:
   - `see "Order total: 42.00"`
   - `see "Order total" contains "42.00"`
   - `see "Hello world"`, which settles whether spans are matched as one text
   - `see "world"`, which settles whether a single span matches
   - for the ellipsis text: `see` the whole string, which settles whether the text or the painted text is matched

   Must fail: `see "Order total: 42.01"`, and `see` a text that is not on the screen.
2. **`navigation` screen: app bar and routes.** Widgets: a `Scaffold` with an `AppBar` titled "Inbox", an action `IconButton` with the tooltip "Search" (`wf_search`), and a button that pushes a second route titled "Message".

   Steps:
   - `see "Inbox"`, then `tap "Search"`
   - `tap "Open message"`, then `see "Message"`
   - `don't see "Inbox"` while the second route is on top. The agent excludes a route under the current one when an opaque route covers it (`_isVisible` and `_routeOnScreen` in `probe_agent/lib/src/finder.dart`), and a unit test covers it (`probe_agent/test/finder_test.dart`, group "ProbeFinder route-awareness (PT-03)"). What is open is the device run: this step on a built app on a device.
   - `go back`, then `see "Inbox"`

   Must fail: `see "Inbox"` while the message route is on top.

   A route under one that is not opaque (a dialog, a popup menu, a bottom sheet) stays on screen, and the agent keeps matching it by design: under an open dialog, `see` still finds the page's text. Item 8 depends on this.
3. **`display` screen: widgets without text.** Widgets:
   - an `Icon` with `semanticLabel: "Favorite"` (`wf_icon`)
   - an `Image` with `semanticLabel: "Logo"` (`wf_image`; built with `Image.memory` from an inline PNG, so the fixture needs no asset)
   - a `CircleAvatar` showing "AB" (`wf_avatar`)
   - a `FlutterLogo` (`wf_logo`)

   Steps:
   - `see #wf_icon` and `see "Favorite"`, which settles whether a semantic label is a selector, as a tooltip is for buttons
   - `see #wf_image` and `see "Logo"`
   - `see "AB"`
   - `tap #wf_logo`, with a counter to show the tap arrived

   Must fail: `see "Favourite"` (another spelling), and `see #wf_missing`.
4. **`lists` screen: scrolling.** Widgets: a `ListView.builder` of 200 rows "Row 0" to "Row 199" (`wf_list`), a `GridView.count` of 60 cells, and a `CustomScrollView` with a `SliverList`.

   Steps:
   - `see "Row 0"`, then `don't see "Row 150"`. The row is not built yet, so this settles whether `see` reports a lazy list's unbuilt rows as absent.
   - `scroll down until "Row 150" appears`, then `tap "Row 150"`; a state text names the tapped row
   - `scroll up until "Row 0" appears`
   - the same for the grid ("Cell 55") and the sliver list

   Must fail: `see "Row 150"` before any scroll, and `scroll down until "Row 999" appears`, which must end with a clear failure rather than run on.
5. **`lists` screen: tiles.** Widgets: `Card`s holding `ListTile`s with title "Alice", subtitle "Online" and a trailing `Icon`, and `CupertinoListTile`s.

   Steps:
   - `tap "Alice"`, with a state text
   - `see "Online"`
   - `see "Online" in "Alice"`, which settles whether a positional selector matches inside a tile
   - `long press "Alice"`, if the tile has `onLongPress`

   Must fail: `see "Offline" in "Alice"`.
6. **`display` screen: progress.** Widgets: a `CircularProgressIndicator` (`wf_spinner`) shown for 3 seconds after a tap on "Load", then replaced by "Loaded", and a `LinearProgressIndicator` with a value.

   Steps:
   - `tap "Load"`, then `see #wf_spinner`
   - `wait until #wf_spinner disappears`
   - `see "Loaded"`

   Must fail: `see "Loaded"` straight after the tap, and `wait until #wf_spinner disappears` against a spinner that never goes away, which must end with a clear timeout.
7. **`gestures` screen.** Widgets: a `GestureDetector` with `onTap` around a `Text("Tap me")` (`wf_gesture`), an `InkWell` around a `Text("Ink me")`, and a `GestureDetector` with `onDoubleTap`.

   Steps:
   - `tap "Tap me"` and `tap #wf_gesture`, each moving a counter
   - `tap "Ink me"`
   - `double tap "Twice"`

   Must fail: a count one too many. This item matters for devices: an earlier project note says a tap on a `Semantics`-wrapped `GestureDetector` failed on physical devices. The fixture should have that case (`Semantics(label: ...)` around the detector), and the suite should run on a physical device, not only an emulator.
8. **`dialogs` screen: dialogs.** Widgets: a button that opens an `AlertDialog` titled "Delete item?" with the actions "Cancel" and "Delete", a `CupertinoAlertDialog`, and a `CupertinoActionSheet`.

   Steps:
   - `tap "Open dialog"`, then `see "Delete item?"`
   - `tap "Cancel"`, then `don't see "Delete item?"`
   - open the dialog again, `tap "Delete"`, then a state text
   - open it again and `go back`. This settles whether back closes a dialog.
   - the same for the Cupertino dialog and sheet

   Must fail: `see "Delete item?"` after Cancel.

   Under the open dialog the page is still matched (item 2), so `see` a text of the page passes while the dialog is open, and `don't see` it fails. The suite asserts that as the rule. A `tap` on a widget of the page under the open dialog finds the widget, so what the test settles is where the tap lands: on a button behind the dialog's body it lands on the dialog (the agent warns that the target is covered by another widget), on a button outside the body it lands on the barrier, which dismisses the dialog; either way the button's own counter must not move. Which one a given widget gets depends on the screen, so the fixture fixes the geometry (the Cover page of the `dialogs` screen: a button at the centre, a tall centred dialog, a button above it) and a widget test checks it on four screen sizes.
9. **`dialogs` screen: snack bar.** Widgets: a button that shows `SnackBar(content: Text("Saved"), action: SnackBarAction(label: "Undo"))` for 4 seconds.

   Steps:
   - `tap "Save"`, then `see "Saved"`
   - `tap "Undo"`, then a state text
   - `tap "Save"` again, then `wait until "Saved" disappears`

   Must fail: `see "Saved"` before the tap.
10. **`cupertino` screen.** Widgets: a `CupertinoTabScaffold` with tabs "Home" and "Settings", a `CupertinoNavigationBar` with a back button to a pushed page, and a `CupertinoSliverNavigationBar` with a large title.

    Steps:
    - `tap "Settings"`, then `see` the tab's content
    - push a page, then `go back`, and also `tap` the back control by its text
    - `see` the large title after a `scroll down`, when it has collapsed into the bar

    Must fail: the content of the other tab is seen.
11. **`overlap` screen.** Widgets:
    - a `Stack` where a `Positioned` banner covers the button "Hidden under banner"
    - an `Opacity(opacity: 0)` around a `Text("Ghost")`
    - a `Visibility(visible: false)` around a `Text("Gone")`

    Steps:
    - `tap "Hidden under banner"`, with a counter. This settles whether the tap reaches the covered button or the banner.
    - `see "Ghost"`. This settles whether a fully transparent text counts as seen.
    - `don't see "Gone"`. The agent docs say `Visibility(false)` is not matched.

    The first two are open questions, so the suite asserts what the run shows and the matrix records it as a finding.

### Families the source cannot rank

These matrix families are "not assessed", and the sample apps hardly use them: `PopupMenuButton` and `MenuAnchor` (1 package each), `DropdownButton` and `DropdownMenu` (0), `CupertinoPicker` (2), `CupertinoDatePicker` (1), `DatePickerDialog` (0), `TabBar` (2), `NavigationBar` and `NavigationRail` (1 each), `Drawer` and `NavigationDrawer` (1 each), `BottomSheet` (0), `Dismissible` (1), `Draggable` (0), `PageView` (1), `RefreshIndicator` (1). A test of each belongs on a `pickers` screen and on the `navigation` and `gestures` screens above. Their order is the owner's call, since this source says nothing about how often apps use them.

## What is still not assessed, in the order to do it (status 2026-11-18)

Counted from the family table of `MATRIX.md` at `f6e68a2e`: **9 cells say "not assessed"**: the whole Pickers and menus row (7), and the `scroll` cell of Layout and containers and of WebView content. (At `20187dcd` there were 24: the `scroll` cells of five families (#863), the Layout row (#847) and the Gestures row (#851) have been assessed on named devices since.) The list below adds what the matrix says about iOS, other libraries and other devices. Cost is the size of one pull request in this folder's style (a screen, a suite, negative variants, matrix and docs); nothing here is a result.

| # | What is open | Cells | Cost | State |
|---|---|---|---|---|
| 1 | **The `scroll` cell.** Done for Text and display, Buttons, Text input, Selection controls and Navigation: one widget below the fold on each screen (#863, Android, both Flutter versions; iOS not run). Open: **Layout and containers** (its screen has no widget below the fold yet: the same one widget and one test) and **WebView content** (item 11: no step scrolls a page in a WebView). Pickers and menus is item 2; the Gestures family has no `scroll` cell to set in this table today. | 2 | one small pull request for Layout; item 11 for WebView | five of the seven families that had one are done (Text and display, Buttons, Text input, Selection controls, Navigation); Layout and WebView are open |
| 2 | **Pickers and menus, the whole row**: `DropdownButton`, `PopupMenuButton`, `DatePickerDialog`, `CupertinoPicker`. The source of this file cannot rank them (0 to 2 packages each), but they are routes and overlays, where the Dialogs family found that a closing overlay is in the tree until it finishes, that a covered tap lands on the overlay, and that a text selector takes the first widget that holds the text. | 7 | one pull request per two widgets | open; some local work exists that is not in the repository, so nothing about it is claimed here |
| 3 | **Layout and containers**: `Stack`, `Opacity`, `Visibility` (what a person cannot see or tap). | 1 (`scroll`, item 1) | one pull request | assessed (#847) |
| 4 | **Gestures**: `GestureDetector`, `InkWell`, a text inside a gesture widget. | 0 | one pull request | assessed (#851) |
| 5 | **Navigation, the rest of the row**: `NavigationBar`, `TabBar`, `Drawer`, and a route that is not opaque (the Dialogs family covers dialogs and sheets). | in the row's `find` and `tap` cells (partly) | one pull request | open |
| 6 | **iOS.** Navigation, Text and display, Lists, Dialogs and sheets, Progress indicators, Layout and Gestures have a run on the iPhone 17 Pro simulator (`Compat-iPhone`) in `MATRIX.md`; Selection controls, Text input, Buttons and the Scroll section were Android only; WebView steps are Android only (iOS not assessed). | 4 | a re-run of the existing suites, no new screens | open |
| 7 | **Follow-ups inside assessed families** (the next section): a line break and `clear` in text fields, `see #slider` read, `is enabled` on the other button kinds, a button in a dialog or a list, `double tap`, `scroll` in a WebView page. | about 10 steps | small | open |
| 8 | **Lists, what the screen did not cover**: `PageView` (a swipe), a horizontal list, two scrollables on one screen, `ListView.separated`, pull to refresh. **Dialogs, what the screen did not cover**: `barrierDismissible: false`, a dialog with a text field, a persistent bottom sheet. | 8 widgets | two small pull requests | open |
| 9 | **The standalone `material_ui` and `cupertino_ui` packages (issue #852).** Every result of the matrix holds for widgets of `package:flutter/material.dart` and `cupertino.dart`. Measured on 2026-11-17 on the real packages (a widget test): before #855 a disabled button was read as enabled and `is checked` could not read a Checkbox or Switch; since #855 and #861 the three state steps refuse on a control of another library, and a tooltip selector still finds nothing. The real fix is designed (`docs/proposals/agent-design-library-neutral.md`, #857) and not built. The matrix has no cell for these packages. | every family | blocked on building the fix | stopgap merged, fix designed |
| 10 | **Other surfaces and devices**: other API levels, a physical device, other text scales, TalkBack and other accessibility services, screen sizes below about 400x800 dp (the Cover page of the Dialogs screen needs a portrait screen at least that big). | all | a measurement plan, not a screen | open |
| 11 | **WebView scroll: a step that does not exist yet.** The `scroll` cell of WebView content is "not assessed" because no step scrolls a page inside a WebView. This needs a small design note first (which verb, for example `scroll down in webview`; and how the agent scrolls: a script run through the WebView controller such as `window.scrollBy`, or the platform view's own scrolling), then a runner and agent step, then a fixture page that is taller than the screen and a test. Do not start before item 2. | 1 | a design note, then a runner and agent change | open, depends on a decision |

Why this order: items 1 and 7 are the cheapest and close the most cells; item 2 is the largest family that has nothing, and it is made of the cases where results so far were wrong or surprising; 3 and 4 are done; 6 turns Android-only results into results for both platforms; 9 is last because the stopgap already makes the wrong answers refusals, and a test can say more only once the real fix is built. The restart failure of `before each test` (issue #823) is not a widget cell, but it decides how many runs of any suite are complete, so a suite's result should be read with it in mind.

## Partly assessed: follow-ups from the matrix

These cells are "partly" or "not assessed" inside families that already have a screen. Each is one or two steps on the existing screen:

- **Text input:**
  - read a field's own text rather than its mirror. **This needs a verb that does not exist:** `store text of` takes only a CSS selector in a WebView (`store text of css:"..." in webview as name`), so a Flutter field's text cannot be stored today. Until there is one, the field is read through a mirror, as now;
  - type a line break into the `maxLines: 3` field (the matrix says it was not typed);
  - `clear` on the form field and the Cupertino field.
- **Selection controls:** `see #wf_slider` reads its value (only the mirror is read today), and `see "..." is checked` on the `CupertinoSwitch`.
- **Buttons:**
  - `is enabled` and `is disabled` on the other button kinds (only `ElevatedButton` is read);
  - a button inside a dialog (item 8) and inside a list (item 4);
  - a `double tap`.
- **Scroll:** settled for lists (item 4) and, with one widget below the fold on each screen, for text and display, buttons, text input, selection controls and navigation (Android, both Flutter versions). It stays "not assessed" for WebView content (there is no scroll step for page content), and on iOS for every family but lists.
- **WebView content:** scrolling in the page (there is no WebView scroll step), and iOS (`webview_flutter_wkwebview`).

## Limits

- One source: 36 demo packages are not a census of production apps. A second source, for example the top packages on pub.dev by likes, would test the order but was not used.
- The count is textual: a widget built through a helper (a `showDialog` builder, a `const` list) is counted only where its constructor appears, and a name used for another class of the same name would be counted. `AlertDialog` built inside `showDialog` is counted, because its constructor is written there.
- Only constructors count: how often a widget is shown at run time, or how often a test would touch it, is not measured.
