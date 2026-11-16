# Widget compatibility matrix

How FlutterProbe works with each widget family. **A cell stays "not assessed" until a test covers it**; it is changed only in the same pull request as the test.
`catalog.json` lists every widget; the families below are how they are grouped for testing. A widget FlutterProbe cannot work with is listed in the
"Unsupported" section, never left out.

Cells: *find* (select by text, id or type), *tap*, *type* (enter text), *scroll* (bring into view), *read* (its text or value), *fixture* (a screen in
`native-test-apps/flutter`), *device test* (a ProbeScript or Go test that ran on a device).

| Family | Examples | find | tap | type | scroll | read | fixture | device test |
|---|---|---|---|---|---|---|---|---|
| Text and display | `Text`, `RichText`, `Icon`, `Image` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Buttons | `ElevatedButton`, `TextButton`, `IconButton`, `CupertinoButton` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Text input | `TextField`, `TextFormField`, `CupertinoTextField` | see below | see below | see below | not assessed | see below | `textinput` screen | `probe-tests/widgets/textinput.probe` |
| Selection controls | `Checkbox`, `Switch`, `Radio`, `Slider`, `CupertinoSwitch` | see below | see below | n/a | not assessed | see below | `selection` screen | `probe-tests/widgets/selection.probe` |
| Pickers and menus | `DropdownButton`, `PopupMenuButton`, `DatePickerDialog`, `CupertinoPicker` | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed |
| Lists and scrolling | `ListView`, `GridView`, `SingleChildScrollView`, `PageView` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Dialogs and sheets | `AlertDialog`, `BottomSheet`, `SnackBar`, `CupertinoAlertDialog` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Navigation | `AppBar`, `NavigationBar`, `TabBar`, `Drawer`, `Navigator` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Layout and containers | `Container`, `Row`, `Column`, `Stack`, `Card` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Gestures | `GestureDetector`, `Dismissible`, `Draggable`, `InkWell` | not assessed | not assessed | n/a | not assessed | n/a | not assessed | not assessed |
| WebView content | an embedded web page (`webview_flutter`) | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed |

## Assessed widgets

A cell says **pass** only where a step of the named test asserts it, **fail** where a step that should pass does not, and **not assessed** otherwise. A "read" that
the test does through a text that mirrors the widget (`Terms: on`) is a read of the mirror, not of the widget, and is said so.

How a result was obtained is under the tables. What the tests do **not** show is as much a part of the result as what they show.

### Selection controls (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=selection`, `probe-tests/widgets/selection.probe`)

| Widget (fixture id) | find (text, `#id`) | tap | read | Result |
|---|---|---|---|---|
| `Checkbox` in a `CheckboxListTile` (`wf_checkbox`) | pass (text) | pass (`toggle`) | pass (`see ... is checked`, on and off) | pass |
| `Switch` in a `SwitchListTile` (`wf_switch`) | pass (text) | pass (`toggle`) | pass (`see ... is checked`) | pass |
| `Radio` in a `RadioListTile` under a `RadioGroup` (`wf_radio_small`, `_medium`, `_large`) | pass (text) | pass (`tap`; the group's state text changes) | **fail**: `see "Medium" is checked` is refused ("not checked") for a radio that is on | **fail** (finding, below) |
| `Slider` (`wf_slider`) | pass (`#id`) | pass (`swipe right on` moves the value) | not assessed (the value is read through its mirror text) | partly |
| `CupertinoSwitch` (`wf_cupertino_switch`) | pass (`#id`) | pass (`toggle`) | not assessed (mirror text only; `is checked` was not tried) | partly |

**Finding: a radio under `RadioGroup` is never "checked" to the agent.** Since the `RadioGroup` API the group value is kept by the `RadioGroup`, not by each `Radio`; the agent
decides `is checked` for a radio by comparing the radio's `value` with its own `groupValue` (`probe_agent/lib/src/executor.dart`, `_checkedState`), so it answers "not checked" for a radio that is on. The tap itself works
(the fixture's state text moves to `Size: medium`). Reproduced on a build by 3.50.0 and on a build by 3.44.8. The agent's own unit test uses the older `groupValue:` form of `Radio`, which is why it did not show. A fix is a separate change.

### Text input (`native-test-apps/flutter`, build flag `PROBE_FIXTURE_WIDGETS=textinput`, `probe-tests/widgets/textinput.probe`)

| Widget (fixture id) | find (`#id`) | type | clear | read | Result |
|---|---|---|---|---|---|
| `TextField` (`wf_textfield`) | pass | pass | pass via the mirror (`Plain: (empty)` after `clear`; the field's own text was not read) | not assessed (read through a mirror text) | partly |
| `TextFormField` with a validator (`wf_formfield`, `wf_validate`) | pass | pass | not assessed | mirror only (the test asserts `Form error: Required` and `Form error: none`, the mirror of the validator's result; the error text the field itself shows is not asserted) | partly |
| `TextField` with `obscureText` (`wf_password`) | pass | pass (the mirror shows 6 characters) | not assessed | not assessed (see the observation) | partly |
| `TextField` with `maxLines: 3` (`wf_multiline`) | pass | pass for one line; **a line break was not typed** | not assessed | not assessed | partly |
| `CupertinoTextField` (`wf_cupertino_field`) | pass | pass | not assessed | not assessed (mirror text only) | partly |

**Observation, not a verdict:** after typing into an obscured field, `see "<the typed text>"` finds it. The agent's own tests promise that an obscured value is never *echoed* in its output; they do not say a check
for the value must fail, so this was left out of the test rather than asserted either way.

### How it was run

Flutter 3.50.0 (Dart 3.14.0) and Flutter 3.44.8 (Dart 3.12.2), each building the fixture with `--dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_WIDGETS=<family>`; the 3.50.0 builds used the Gradle, AGP and Kotlin versions of #711. Device: emulator-5600, AVD `ProbeWebViewSpike` (Android 15, API 35), one run per family
and build: 5 of 5 tests passed in each family on both builds (the radio and password tests as written above; the one radio check that fails is not in the test). On 3.50.0 the analyzer reports `package:flutter/material.dart` and `package:flutter/cupertino.dart` as deprecated ("deprecated after v3.47.0-0.0.pre", use `material_ui` and `cupertino_ui`): an info, the builds are unaffected, and the fixture and the agent still import them. Not run: iOS, a physical device, another Android version, TalkBack or other
accessibility services, other screen sizes or text scales.

## Unsupported

None recorded yet; none has been assessed.
