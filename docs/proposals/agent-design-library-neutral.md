# The agent and the standalone design libraries (`material_ui`, `cupertino_ui`)

Status: proposal, for review. No code in this note. Issue: #852. Stopgap: #855 (merged). The wrapper fix of #860 is #861 (merged).

## The problem

The agent decides some things from the classes of `package:flutter/material.dart` and `package:flutter/cupertino.dart`: whether a control
is enabled or checked, where a tooltip is, which widgets are route bodies, which widgets are interactive in the device view. Flutter 3.50
deprecates those two libraries in favour of the standalone packages `material_ui` and `cupertino_ui`. A widget of those packages has the same
name as the framework's, but it is another class, so the agent does not recognise it.

Measured in #852 (Flutter 3.50.0, `material_ui` 1.11.1, `cupertino_ui` 1.1.7, a widget test running the agent's own request handler; one run,
no device):

- `see #off is enabled` on a **disabled** `material_ui` `ElevatedButton` **passed**: a silent false pass;
- `is disabled` on it failed with "is enabled";
- `is checked` on a `material_ui` `Checkbox` or `Switch`, or a `cupertino_ui` `CupertinoSwitch`, failed with "not a checkable control";
- a tooltip selector (`tap "Add item"` on an `IconButton`) found nothing;
- text, tap, type, `contains`, `is focused`, `is filled` and the obscured-field protection worked, since they go through the widgets library
  (`EditableText`, hit testing, focus).

## Facts about the packages

These are from the package pages (the pub.dev API) and the Flutter docs, read on 2026-11-17 and 2026-11-18. The migration guide was read in full
on 2026-11-18 (the page says it was last updated on 2026-11-09 and reflects Flutter 3.50).

- `material_ui` 1.11.1 and `cupertino_ui` 1.1.7 are the latest versions (both published 2026-11-16). Both declare `environment: sdk ^3.13.0`
  and `flutter >=3.47.0` (the pub.dev API, `pubspec.environment` of the latest version, read 2026-11-18). The first versions, 0.0.1
  (2026-02-18), declared `flutter >=3.35.0`. The migration guide itself states no minimum version.
- The classes are **new types copied from the framework**, not re-exports: "values from `package:flutter/material.dart` cannot be assigned
  to parameters expecting types from `package:material_ui`" (migration guide). In the API docs, `material_ui`'s `ElevatedButton` extends
  `material_ui`'s own `ButtonStyleButton`. The widgets-layer classes (`Widget`, `Text`, `Semantics`, `GestureDetector`, `EditableText`) are
  the framework's.
- Timeline (migration guide): contributions to the in-framework libraries were frozen in 3.44, the packages could be adopted from 3.47, and
  the in-framework libraries are formally deprecated in 3.50. **No removal version is stated.**
- An app can mix both libraries through `MaterialUiCompatibilityBridge`, which the guide describes: it injects theme and localization data
  downward, and it cannot resolve type mismatches when a dependency's public API uses in-framework types. The guide names no Cupertino bridge
  class (it says `material_ui` and `cupertino_ui` "provide compatibility bridge utilities"), and it does not call the bridge deprecated or
  temporary.
- Package authors are told to treat the migration as a major version bump. Testing tools are not mentioned.
- The agent today declares `flutter: ">=3.19.0"` and `sdk: ">=3.3.0 <4.0.0"` (`probe_agent/pubspec.yaml`). Its CI runs Flutter 3.29.x.

## Where the agent names a design-library class

Every `is`/`is!` test on a class of `material.dart` or `cupertino.dart` in `probe_agent/lib/src` (line numbers are at main `75e89a41`, read on
2026-11-18; they drift with every agent change, so the symbol names are the stable reference):

| Class | Where | What it decides | On a `material_ui` app |
|---|---|---|---|
| `ButtonStyleButton`, `IconButton`, `FloatingActionButton`, `TextField`, `Switch`, `Checkbox` | `executor.dart` `_controlDisabled` (2294-2300) | `is enabled` / `is disabled` | **read as enabled (false pass)**; refused since #855 |
| `Switch`, `Checkbox`, `Radio`, the three list tiles, `FilterChip`, `ChoiceChip`, `CupertinoSwitch`, `CupertinoRadio` | `executor.dart` `_checkedState` (2065-2104), the message at 1348 | `is checked` | "not a checkable control"; refused since #855 |
| `Scaffold`, `Dialog`, `BottomSheet`, `CupertinoPageScaffold` | `executor.dart` `_routeTransitionInFlight` (1438) | wait for a route transition before a tap (FP-13) | **not measured.** Expected: no route body is recognised, so the wait never happens. A tap right after a dialog closes can land on its barrier: a flaky failure, or a tap that does nothing (a silent risk) |
| `InkResponse` | `executor.dart` `_findDirectTapTarget` (710; the `InkResponse` tests at 720 and 739) | direct `onTap` on a matched `Semantics` wrapper | falls back to a real pointer tap, which works (#852: `tap "Submit"` worked) |
| `Tooltip` | `finder.dart` (252), `executor.dart` `_textOf` (2308; the `Tooltip` test at 2326) | the tooltip fallback of a text selector, and its read (#818) | **found nothing** (#852) |
| the names of the controls the agent reads, with the framework type test for each | `executor.dart` `_enabledReadNames` (2233), `_checkedReadNames` (2244), `_isLookalike`, `_lookalikeBelow`, `_refuseUnreadControl` (the #855 stopgap) | recognises a control of the same name that is not the framework class, to refuse instead of answering | exists so that a `material_ui` control is refused; it must be kept in step with whatever option is chosen |
| `TextField`, `CupertinoTextField` (`FormField` is widgets) | `finder.dart` `_fieldKeyOf` (107) | the key of a field around a match | not measured. `FormField` still covers `TextFormField` |
| `ButtonStyleButton`, `IconButton`, `FloatingActionButton`, `CupertinoButton`, `Checkbox`, `Radio`, `Switch`, `CupertinoSwitch`, `InkResponse`, `Tooltip` | `device_view.dart` `_isInteractive` (215-252) | which widgets `probe device view` and Studio's inspector mark as interactive | not measured. Expected: these controls are not marked |

The obscured-field protection, `EditableText`, focus, hit testing, `Text`, `RichText`, `Semantics` and `GestureDetector` are all
widgets-layer classes and are unaffected.

## Risk ranking

1. **`is enabled` on a disabled button passes.** A wrong verdict that no one sees. Closed by the #855 stopgap (it refuses).
2. **Route transitions are not waited for.** Not measured. Taps right after a dialog, sheet or page transition may be lost or land on a
   barrier: flaky failures, and possibly a passing step that did nothing.
3. **`is checked` is refused.** A visible failure, not a wrong verdict.
4. **The tooltip selector finds nothing.** A visible failure.
5. **The device view and Studio's inspector do not mark the controls.** A tooling gap, not a test verdict.
6. **The direct-tap shortcut for a `Semantics`-wrapped `InkResponse`.** The real tap still works.

## What the stopgap (#855) does, and why it is only a stopgap

`is enabled`, `is disabled` and `is checked` look, on the target and its ancestors up to 40, for a **lookalike**: a widget whose runtime
type name (type arguments dropped) is the name of a control the agent reads, but which is not that framework class (each name is paired
with the framework type test). On a lookalike, the step throws "cannot be evaluated", negated steps too. #852's test was rerun against #855: the false pass is closed and the framework controls are unchanged.

Its limits:

- **It refuses, it does not read.** On a migrated app every `is enabled`, `is disabled` and `is checked` step fails, an enabled button
  included. A test author loses those steps until the real fix.
- **It depends on runtime type names.** In a release build with `--obfuscate` (the agent can run in release with `allowReleaseBuild` or
  `PROBE_AGENT_FORCE`), names are mangled. The lookalike is then not seen, and the old guess (enabled) comes back.
- **An app's own class with a framework name** (`class Checkbox extends StatelessWidget`) is refused too.
- **It covers state checks only.** Routes, tooltips and the device view are unchanged.

## Options

### (a) Import both families

The agent depends on `material_ui` and `cupertino_ui` and tests both classes.

- Dart has no import that depends on the Flutter version, so the dependency is unconditional. That raises the agent's floor to what the
  packages need: Dart 3.13 and Flutter 3.47, both declared by the latest versions of the packages (`environment` on pub.dev; see the facts above).
  Which older package versions an agent could still allow, and what that would do to the floor, is not examined here. The floor today is
  Flutter 3.19.
- Every app that adds the agent then also resolves `material_ui` and `cupertino_ui`, even if it does not use them: a larger dependency
  graph, and version conflicts when an app pins another version.
- **Cost to users:** apps on Flutter 3.19 to 3.46 can no longer take a new agent. How many there are is unknown: the agent sends no
  telemetry. pub.dev shows only download counts, not the Flutter versions behind them.
- This would be a breaking agent release (a major version).

### (b) Names and dynamic access

Recognise controls by runtime type name, and read `onPressed`, `onChanged`, `value` and `enabled` through `dynamic`.

- No new dependency, and no floor change.
- Breaks under `--obfuscate`: names are mangled, and whether dynamic getters survive tree shaking and obfuscation is **not verified**.
- Fragile: a field's name or meaning can change between package versions, the packages now move independently of Flutter, and nothing
  type-checks the reads.

### (c) The `Semantics` the controls build

Material and Cupertino controls describe their state to accessibility tools through `Semantics` widgets in their own subtree. `Semantics`
and `SemanticsProperties` are widgets-layer classes, so they are the same in both families. The properties are on the widget, so the
**semantics tree does not need to be enabled**.

Measured on the framework controls (Flutter 3.44.8, a widget test that walks each control's subtree for `Semantics` properties; **reported, not reproducible from this repository**: the test is not committed):

| Control | Properties found |
|---|---|
| `ElevatedButton` (enabled, disabled) | `enabled=true` / `false`, `button=true` |
| `IconButton`, `FloatingActionButton` (disabled) | `enabled=false`, `button=true` |
| `TextField` (`enabled: false` / default) | `enabled=false` / `true` |
| `Checkbox` | `checked`, `enabled` |
| `Switch`, `CupertinoSwitch`, `SwitchListTile` | `toggled`, `enabled` |
| `Radio` in a `RadioGroup` | `checked`, `inMutuallyExclusiveGroup`, `enabled` |
| `FilterChip`, `ChoiceChip` | `selected`, `enabled`, `button` |
| `Tooltip`, and an `IconButton`'s tooltip | `tooltip` |
| `CupertinoButton` (enabled or disabled) | `button=true` only: **no enabled state** |

- No new dependency, no floor change, and no runtime type names, so it works under `--obfuscate`.
- It reads what assistive technology reads, which is closer to what the person sees than a widget field.
- **Measured on `material_ui` and `cupertino_ui` too.** The same walk on Flutter 3.50.0 with `material_ui` 1.11.1 and `cupertino_ui` 1.1.7
  (a widget test on a scratch app, one pumped state per control) gives the same properties for every row of the table. **Reported, not reproducible from this repository:** the scratch app and its test are not committed, so this walk and the one above are measurements to repeat, not evidence to cite:
  `enabled` on the buttons, text fields and chips, `checked` on `Checkbox` and `Radio`, `toggled` on the switches, `selected` on the
  chips, `tooltip` on `Tooltip` and an `IconButton`'s tooltip, and the same gap, `CupertinoButton` with `button=true` and no enabled value.
  Not yet run: the agent reading these values, and any device.
- **Gaps:** `CupertinoButton` carries no enabled state, in either library; an app can override a control's semantics (`ExcludeSemantics`, its own `Semantics`
  around it, `MergeSemantics`). Then there is nothing to read, and the step should refuse, not guess.
- **Routes:** route bodies can be found without design classes through `ModalRoute.of(element)` on the elements of the tree (widgets
  layer), instead of looking for `Scaffold`, `Dialog`, `BottomSheet` or `CupertinoPageScaffold`.

### (d) Hybrid: recommended

1. **Read state from `Semantics` first** (option c): `enabled`, `checked`, `toggled`, `selected`, `mixed`, from the nearest `Semantics` with
   that property inside the matched control's subtree, or on the way up to it.
2. **Keep the framework class reads as a cross-check** where they apply. If the class and the `Semantics` disagree, refuse, because one of
   them is not what the app shows.
3. **Refuse when there is nothing to read.** (This also covers a related gap on main, issue #860: the enabled reader did not look below
   the target, so a disabled button behind an id wrapper read as enabled; #861, merged, fixes it with the framework classes.) No class the agent knows, and no `Semantics` with the property: the step fails with "cannot be
   evaluated", as the stopgap does now. Never fall back to a default.
4. **Routes through `ModalRoute`**, the tooltip selector through `SemanticsProperties.tooltip`, and the device view's interactive marking
   through `Semantics` `button`/`textField`/`toggled`/`checked`, plus `EditableText`.
5. **Names only in messages.** The runtime type name stays out of every decision, and is used only to say "a `material_ui` control".

No new dependency, no floor change, no breaking change for apps on the in-framework libraries. The stopgap's lookalike refusal becomes
unnecessary for enabled and checked, and can be removed once (1) to (3) land.

## Test plan

- **Unit tests in `probe_agent`** (no `material_ui` dependency):
  - every framework control in the table above, enabled and disabled, checked and not;
  - stand-in lookalike classes (as in #855) that build the same `Semantics`, which must be read;
  - stand-ins that build none, which must refuse;
  - a control wrapped in `ExcludeSemantics` and in an app's own `Semantics(enabled: true)` around a disabled button (the disagreement
    rule);
  - mutants per rule.
- **A scratch app on Flutter 3.50** with `material_ui` and `cupertino_ui`, in its own throwaway folder. Not the e2e-test-workspace: adding
  packages there upgrades unrelated dependencies. It reruns #852's measurement test and extends it to the rows #852 did not measure:
  - routes: a dialog, a sheet and a page transition followed by a tap;
  - `TextField(enabled: false)`, chips, list tiles, `TextFormField`, `DropdownButton`;
  - the tooltip selector;
  - `probe device view`.
- **On devices:** a throwaway `material_ui` twin of the fixture's `selection` and `buttons` screens, built with the 3.50 SDK and run on the
  two named devices (`Compat-Pixel-API35`, `Compat-iPhone`). The same suites must pass on both the framework fixture and the twin.
- **A regression guard:** a source test that no new `is` test on a `material.dart` or `cupertino.dart` class is added to a state decision
  without a `Semantics` path.

## Effect on the agent release

- Options (c) and (d): no floor change. A minor agent release: a fix plus new behaviour. A step that was refused by the stopgap now gets an
  answer.
- Option (a): Flutter 3.47 / Dart 3.13 floor, a major release, and a dependency every app pulls in.
- The stopgap (#855) can ship in the next agent release on its own.

## Implementation, in small pull requests

1. `is enabled` / `is disabled` from `Semantics`, with the cross-check and the refusal; unit tests; the 3.50 scratch measurement.
2. `is checked` from `Semantics` (`checked`, `toggled`, `selected`).
3. Route discovery through `ModalRoute`; a measurement of the dialog and sheet rows on 3.50.
4. The tooltip selector and read through `SemanticsProperties.tooltip`.
5. The device view's interactive marking.
6. The `material_ui` twin fixture on the two named devices; the compatibility matrix gets a `material_ui` column.
7. Remove the stopgap's lookalike refusal for the checks that read `Semantics`.

## Questions for the owner

1. Is option (d) acceptable, or should the agent take the `material_ui`/`cupertino_ui` dependency (option a) and the Flutter 3.47 floor?
2. A control that builds no `Semantics` with the state (`CupertinoButton`): should it refuse (proposed), or keep the old guess for the
   framework class only?
3. When the class and the `Semantics` disagree (an app's own `Semantics` over a control): refuse (proposed), or trust the `Semantics`,
   which is what assistive technology reads?
4. Should the matrix and the docs advertise `material_ui` support only after the device runs of step 6, or as soon as steps 1 and 2 pass on
   the 3.50 scratch app?
5. How long to keep the in-framework class reads, given that no removal version is announced?
