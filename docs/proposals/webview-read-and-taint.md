# Reading text from a WebView, and keeping page text out of the runner's hands

**Status:** Design for review. No code. It builds on `docs/proposals/webview-content-automation.md` and `webview-verbs-slice3.md` (the WebView steps now run on Android).

## Why

The WebView steps can act on a page and assert on it (`see`, `wait until`), but a test cannot use what the page shows: a total, an order number, a code. A read step fixes that, and it introduces the first variable whose value is chosen by a page rather than by the script author. A page (or anyone who can put text in it) must not be able to steer the runner through that value, so the read comes with a rule about where such a value may go.

## The step

```
store text of css:"#total" in webview "checkout" as total
```

- `css:"..."` only. With a text operand the stored value would be the operand itself, so the parser refuses the text form with a message that says so.
- The stored value is the element's visible text (`innerText`, or the `value` of an input), with runs of whitespace collapsed, as the lookup already computes it for matching.
- An absent or hidden element, several pages, no WebView and the other lookup failures fail the step with the existing messages. An empty text is stored as an empty string.
- **Size:** at most 1000 characters. A longer text is cut at 1000 and the step prints `stored "total": 1000 characters (cut from 4312)`, never the text.
- **Hidden characters** (control and format characters, line separators, bidi controls, zero-width characters, U+FFFE/FFFF, invalid UTF-8) are dropped before the value is stored, by the same cleaning the report writers use, so a value cannot hide text from a reader of a log or a report.
- The lookup gets a third mode, `read`, in the same fixed template (the selector still travels only as data). The template changes, so the token-level proof test is re-run, and the reply gains the element's text up to the cap. The step line shows no value.

## Taint

A **tainted** value is one that came from a page. Taint is a property of a variable's *value*, tracked by one mechanism, not a check in each step.

**Sources.** Only the new read step. (`store response ... as`, `read ... with ai into` and the HTTP store are untrusted in principle too, but marking them would break existing tests that store a response id and then `call` with it; they are an open question below, not part of this change.)

**Representation.** The executor keeps, next to `vars map[string]string`, a set of tainted variable names. Variables are expanded where they are used, so the unit of tracking is the *expansion*:

```go
// internal/runner/taint.go
// resolveT expands variables, ${ENV} values and generators like resolve, and reports the names of the tainted variables whose value ended up in the result.
func (e *Executor) resolveT(s string) (out string, taintedBy []string)

// forSink is the only way a string that leaves the UI may be taken from the script: it expands s and refuses it when a tainted value is part of it.
func (e *Executor) forSink(step, s string) (string, error)
```

Composition is covered because taint is decided when the expansion happens: `"x <total> y"` is tainted because `total` was substituted into it; `store "<total>" as copy` marks `copy` tainted (the store expands its text with `resolveT` and records the result's taint); a value inserted into another variable carries over the same way; a recipe parameter is bound to its argument text (`e.vars[param] = args[i]`) and is expanded when used, so a parameter whose argument mentions `<total>` is tainted at the point of use, **including inside the recipe at a sink**, with nothing extra to do. `${ENV}` values and data-driven row values are not tainted.

**One-pass expansion for tainted values.** Today the expansion loop re-scans the text after each substitution, so a page text such as `<token>` would itself be expanded into the `token` variable. A tainted value is therefore inserted verbatim and never re-scanned (the substitution for tainted variables is a single pass over the original text). This also stops a page from reading other variables.

**The guard.** `forSink` returns an error and the step sends nothing:

```
open link: <total> holds text read from a WebView page, so it cannot be used to choose a link, a file, a command or an address; tainted values can be typed, looked for (see, wait until), logged and used in selectors
```

The message names the variable (script text), never the value. Retries and `optional` do not turn it into success: it is a script error, not a missing element.

## Where a tainted value can and cannot go

A step is a **sink** when a string from the script leaves the UI interaction: a shell or process argument, a file name or path, a URL or network request, code, or something the report or a baseline keys on. These steps take their strings from `forSink`; everything else keeps `resolve`.

Classes, by the exact names in `internal/parser/ast.go`. Each entry below was **checked by reading its handler in `internal/runner` (at main `ae6b5a7`)** and, where the answer was not obvious, by running the step; the structural test below makes the table binding.

| Class | Steps | What the handler does with its strings |
|---|---|---|
| **sink** (takes strings through `forSink`) | `VerbOpenLink` | `e.resolve(a.Name)` goes to `OpenDeepLink` (`adb shell am start` / `simctl openurl`) or the agent |
| | `VerbAddMedia` | `e.resolve(a.Name)` is a host file path for `adb push` / `simctl addmedia` |
| | `VerbSetLanguage` | `e.resolve(a.Name)` goes to the device settings commands |
| | `VerbTypeNative` | `e.resolve` of the query and the text; the text reaches `adb shell input text` |
| | `VerbTakeShot`, `VerbCompareShot` | the name goes through `e.shotName` (`e.resolve`) and becomes a file name and, for `compare`, a baseline key |
| | `HTTPCallStep` (`call`) | `e.resolve` of the URL and body; a network request |
| | `MockBlock` (`mock`, `runMock`) | `e.resolve` of the path and the body, registered with the agent and remembered across restarts: a page value would choose what the app receives from the network |
| | `PerfStep` start (`start measuring "<name>"`) | `e.resolve` of the name; it is the label in the report and a key into the performance baseline (`Baseline.Lookup(base, file, test, name)`, and `Save` writes it to the baseline file) |
| | `SystemDialogStep` (`type ... into system field`, `tap` in a system dialog) | `e.resolve` of the button, the text and the field. The text goes to `sysdialog.ResolveSecret`, which **reads the environment variable a `$NAME` text names** and types its value: a page value that spelled `$SOME_ENV` would pick which runner variable is typed (found in review of the first version), so every string of the step takes the guard and `ResolveSecret` never sees a tainted text |
| **literal only** (no expansion today; must stay that way or take the guard) | `VerbLog` | prints `a.Name` **as written**: `log "hello <x>"` prints `<x>`, it does not expand variables (checked by running it) |
| | `VerbSetLocation` | splits `a.Name` as written into lat and lng for the device; no expansion (checked by running it) |
| | `VerbSetAppearance`, `VerbSetAirplaneMode` | the value is a fixed word checked by the parser; no expansion |
| | `VerbAllowPermission`, `VerbDenyPermission` | the permission name is used as written; no expansion |
| | `VerbDeliverSignal` | `a.Name` and `a.Text` are sent to the agent (`MethodSignal`) as written; no expansion |
| | `DartBlock` | `runDart` sends `d.Code` to the agent as written; variables are not expanded in it |
| | `TravelStep` | waypoints are numbers parsed by the parser |
| | `VerbOpen`, `VerbClose`, `VerbRotate`, `VerbPressKey` | take their word ("the app", "keyboard", an orientation, a key) as written; no expansion |
| **UI-only** (allowed to use `resolve`) | `VerbType`, `VerbToggle`, `VerbTap`, `VerbLongPress`, `VerbDoubleTap`, `VerbClear`, `VerbDrag`, `VerbSwipe`, `VerbScroll` and the scroll-until target, selector text, container and anchor, `VerbTapNative` (matched in process against the dump), `VerbTypeWebView` and the WebView targets, `AssertStep` and `WaitStep` texts and patterns, `HTTPStep` (`store response` and `see requests` matching), `ConditionalStep` conditions (`if "..." appears` is matched as written; `if response ...` expands its reference and check, which are matched, not sent), `VerbReadWithAI` (the query is sent to the configured AI provider with a screenshot, masked before it leaves; a page value in it is data, not an address, and the value stored by `read ... with ai` is a known limit, see below) | the text is matched on the screen, typed, or compared |
| **propagates** | `VerbStore`, `RecipeCall` (parameters), `VerbCopyClipboard`, `VerbPasteClipboard` | `store` expands its text with `resolveT` and records the result's taint; a recipe call binds its argument text to the parameter and is expanded where it is used; `copy` records whether the copied text held a tainted value and a `paste` sets the `clipboard` variable's taint to that, until a clean `copy` (a paste does not clear it). The flag belongs to the clipboard, so it lives as long as the device's state does: on the `DeviceContext` of the run (cloud runs, which have none, keep it on the `Runner`), not on the `Executor`, which is rebuilt for every test attempt. A page value that the script copies cannot be washed through the clipboard, in the same test, in a later test or in a retry. The flag is set **after** the copy returns: a copy that fails leaves the clipboard as it was, so the flag never goes down on a failure (a failed tainted copy marks it up, since what the clipboard holds is then unknown). A composite device that has no device context keeps one flag per device alias for the run. A **new run** (a new `probe test` process, or a new device context such as one per `--locales` entry) starts with a clean flag even if the physical clipboard still holds a page value from the run before; Studio keeps the context of its connection |
| **no string** | `VerbPause`, `VerbGoBack`, `VerbPressEnter`, `VerbShake`, `VerbRestart`, `VerbKill`, `VerbClearAppData`, `VerbGrantAllPerms`, `VerbRevokeAllPerms`, `VerbVerifyBrowser`, biometric steps, `VerbDumpTree`, `VerbSaveLogs`, `LoopStep`, `RetryStep`, `TimedStep`, `PerfStep` stop and `see` thresholds, `SyncStep` | no script string is used, or only a number |

Two facts from reading the handlers change the earlier draft of this design. First, `log` does not expand variables today, so a tainted value cannot reach a log line through `log` (it prints `<total>` as text); if `log` ever expands variables, it must mask what it prints like the other lines, and it is classed literal-only until then. Second, five steps I had listed as sinks do not expand at all today (`VerbDeliverSignal`, `DartBlock`, `VerbSetLocation`, the permission steps, appearance and airplane mode): their class is "literal only", and the structural test requires that they contain no call to a variable resolver, so adding expansion later forces a decision (move the step to the sink class and use the guard). A false refusal of a sink is a clear error; a false allowance is the defect this design exists to prevent, so a step whose handler is unclear is a sink.

`type` into a Flutter field is UI-only and allowed (the text is typed on the screen like any other). Tainted values are also allowed in `see` and `wait until` texts, in selectors, in conditions and in the clipboard.

### Values that are clean by design, and strings that are interpreted

**Clean by design** (not tainted, whatever they hold): data-driven row values, `${ENV}` values, the response variables of `call` (`response.status`, `response.body`) and of `store response`, the text read by `read ... with ai`, and a variable set by a script `store` of clean text. They are a stated limit of this design (see the decisions below). The clipboard is not on this list when the script copied the text: it propagates. A clipboard written by the app or the page itself (a "copy link" button), and then pasted by the script, is clean by design: the script's `copy` never saw it, and nothing in the runner can tell where the text came from.

**Strings that are interpreted** (read in the audit for the review of the first version): `sysdialog.ResolveSecret` (a `$NAME` text typed into a system dialog, now behind the guard); `expandEnv` (`${NAME}`, applied after variables and never to a tainted value, which is a token at that point); the l10n keys (`\x1el10n:` markers made when the script is parsed, never from a variable); `use` and CSV paths (taken from the script text when the file is loaded, never from a variable); recipe names (matched as written); and the `probe test` flags (command line, not script). No other function of the runner reads the environment, a file or the clipboard from a script string.

**Selector kind.** A selector's kind is decided by its syntax in the script (the WebView operands) or, for the older steps, from the text: `see any of`, `wait until any of` and `if "..." appears` pick the `id` kind from a leading `#` of the **expanded** text, so a page value `#x` can switch such a selector from text to id. That is a choice between two ways of matching on the screen, and neither sends anything out; it is documented here rather than changed, because the existing steps work this way for every variable.

## Masking

A page value can be secret (a token shown on a page). Rules, in order of how much they rely on knowing:

1. The runner never prints a stored page value by itself: step lines show names, not values; the read step prints a length.
2. Wherever the value reaches a line or an error through a later step (an error that quotes a selector or a typed text), the existing `sysdialog.ScrubKnown` applies, as for any variable. (`log` does not expand variables today, see above.)
3. The variable's name goes through `sysdialog.NoteEnvValue(name, value)`, so a variable named like a secret (`token`, `password`, `code`, ...) registers its value as known and it is masked everywhere, including values that reach a report.
4. A value is not registered as secret otherwise: a page text that is a total or an order number is not a secret, and masking every digit run it contains would damage unrelated output.

## Interaction with the existing variable rules

- **Per test, like every variable.** Reading into a name that exists replaces its value and its taint (a test cannot clear taint by reading into a name from a clean source: the only clean assignment is a script `store`, which clears it).
- **Data-driven rows** set variables before the test runs and are clean. A read into a row column's name replaces it and taints it.
- **`${ENV}`** is untouched and untainted.
- **Recipes:** parameters follow the argument as above. A recipe defined in another file behaves the same, since the guard is in the executor, not in the file.
- **Composite tests (several devices):** each device's executor has its own variables; a value read on one device is not visible to another, so nothing crosses.
- **`if` conditions and `see`/`wait` patterns** may use tainted values: they compare text.
- **Selectors may use tainted values** (`tap "<total>"`): the text is matched on the screen; the page value cannot be used as a selector *kind* switch because the syntax decides kinds.

## The structural test

The point of one mechanism is that a new step cannot forget it. A test in `internal/runner` (like `device_context_builders_test.go`, which scans the source for builders of `DeviceContext`) makes that mechanical:

1. **Classification is complete.** The test lists every `ActionVerb` constant in `ast.go` and every step type that implements `Step`, and requires each to appear in exactly one of five classes: sink, literal only, UI only, propagates, no string (`taint_classes.go` is one map from name to class). A new verb fails the test until someone classifies it.
2. **Sinks use the guard.** Using `go/parser`, the test finds the handler of each sink step (the `case` in `runAction`, or the named function) and requires that it takes strings through `forSink` and that it reaches no `resolve`, `resolveVars`, `resolveSelector` or `expandEnv`, **directly or through any helper of the package** (a small call graph followed by name; the scan stops at the guard, at the expansion itself and at the points where the executor re-enters its dispatcher for a nested step, which has its own handler). A second string that goes through a helper that resolves it is a negative test of the scan.
3. **Literal-only handlers stay literal:** the handlers of the literal-only class reach no `resolve`, `resolveVars`, `resolveSelector` or `forSink`, also through helpers; adding expansion to one fails the test until the step is reclassified as a sink and takes the guard. UI-only handlers may call `resolve`.
4. **Behaviour.** For every sink step, a table-driven test runs the step with a tainted variable against recording fakes (device context, agent client, HTTP) and requires the refusal, the exact message shape without the value, and that the fakes recorded no call.
5. **One-pass expansion:** a page value of `<token>` (with a `token` variable set) must not expand.

## Tests and mutants for the implementation

Taint set on read, on store, on recipe parameter use, through composition and through a copy; cleared by a script store; not set by data rows or `${ENV}`; one-pass; each sink refuses; each UI-only step allows; the read step: cap boundary (999, 1000, 1001), hidden characters dropped, the text form refused, the lookup modes (the proof re-run); masking by variable name; mutants one guard each (propagation points, each table entry, the one-pass rule, the guard's error, the cut).

## Slices (each a separate PR, one at a time)

1. **T1: taint without WebView.** `resolveT`, `forSink`, the tables, the structural test, the sink handlers switched to the guard, and the one-pass rule for tainted values; nothing is tainted yet (a test-only source marks values), so existing behaviour is unchanged and byte-identical.
2. **T2: the read step.** Grammar and parser (with the EBNF, conformance test and compatibility corpus), the `read` lookup mode and the proof re-run, the runner step, the cap and cleaning, masking by name, docs, and one real run on a named emulator.

## Decisions taken in review, known limits, and future work

- **Known limit (decided):** `store response ... as`, `read ... with ai into` and the other stored values are **not** tainted. A hostile server or screen is the same threat, but marking them would break the common `store` then `call` flow; it needs its own decision and an opt-out. This change protects against values read from a WebView only.
- **`read ... with ai` takes tainted values (decided):** its query is resolved and sent, with a screenshot, to the AI provider the user configured. A tainted value in it sends page text to that provider: data leaving to a configured service, not an action on the device, a file or an address, so it is allowed. The masking on that path (`sysdialog.ScrubKnown` of the query before it leaves) stays, and the class table keeps the step as UI only so that a later change to its handler is reviewed against this decision.
- **Mock rules are a sink (decided):** a page-chosen path or body would change what the app sees from the network.
- **T3, future, with its own review:** a validating form, `store text of ... as total matching "<RE2 regex>"`, that clears taint when the value matches. RE2 only, a length cap on the pattern, an anchored match, and the validated value must still pass the guard's other checks (for example, no newline).
