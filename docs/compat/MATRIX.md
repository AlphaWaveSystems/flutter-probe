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
| Text input | `TextField`, `TextFormField`, `CupertinoTextField` | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed |
| Selection controls | `Checkbox`, `Switch`, `Radio`, `Slider`, `CupertinoSwitch` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Pickers and menus | `DropdownButton`, `PopupMenuButton`, `DatePickerDialog`, `CupertinoPicker` | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed |
| Lists and scrolling | `ListView`, `GridView`, `SingleChildScrollView`, `PageView` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Dialogs and sheets | `AlertDialog`, `BottomSheet`, `SnackBar`, `CupertinoAlertDialog` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Navigation | `AppBar`, `NavigationBar`, `TabBar`, `Drawer`, `Navigator` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Layout and containers | `Container`, `Row`, `Column`, `Stack`, `Card` | not assessed | not assessed | n/a | not assessed | not assessed | not assessed | not assessed |
| Gestures | `GestureDetector`, `Dismissible`, `Draggable`, `InkWell` | not assessed | not assessed | n/a | not assessed | n/a | not assessed | not assessed |
| WebView content | an embedded web page (`webview_flutter`) | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed | not assessed |

## Unsupported

None recorded yet; none has been assessed.
