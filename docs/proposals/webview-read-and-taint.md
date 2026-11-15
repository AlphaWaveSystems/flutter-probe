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

Classes, by the exact names in `internal/parser/ast.go` (the verb table; the structural test below keeps this table honest):

| Class | Steps |
|---|---|
| **sink** (refuse) | `VerbOpenLink`, `VerbAddMedia`, `VerbSetLanguage`, `VerbSetLocation`, `VerbSetAppearance`, `VerbSetAirplaneMode`, `VerbTypeNative` (reaches `adb shell input text`), `VerbTakeShot` and `VerbCompareShot` (names: file names and baseline keys), `VerbAllowPermission`, `VerbDenyPermission` (permission names reach `pm grant`), `VerbDeliverSignal` (names and values go to the agent), `HTTPCallStep` (URL, headers, body), `HTTPStep` mock rules (path, body), `DartBlock` (code), `PerfStep` start (the name keys a baseline) |
| **UI-only** (allowed) | `VerbType`, `VerbToggle`, `VerbTap` and the other selector steps (selector text, container, anchor), `VerbTapNative` (matched in process against the dump), `VerbTypeWebView` and the WebView targets, `AssertStep` and `WaitStep` texts, `VerbCopyClipboard`, `SystemDialogStep` (button and field names matched in the dialog), `VerbLog` (printed through the existing masking), the `see`/`wait` patterns |
| **propagates** | `VerbStore`, `RecipeCall` (parameters), `ConditionalStep` conditions (a check, not a sink) |
| **no string** | `VerbPause`, `VerbGoBack`, `VerbRestart`, `VerbKill`, `VerbClearAppData`, `VerbGrantAllPerms`, `VerbRevokeAllPerms`, biometric steps, `VerbDumpTree`, `VerbSaveLogs`, `LoopStep`, `RetryStep`, `TimedStep`, `TravelStep` (numbers) ... |

The entries marked as a sink above that I did not read in full yet (`VerbDeliverSignal`, `HTTPStep` mock rules, `DartBlock` expansion, `PerfStep`, `VerbSetLocation`) are verified when the guard is wired; the rule for a doubtful step is "sink": a false refusal is a clear error, a false allowance is the defect this design exists to prevent. `type` into a Flutter field is UI-only and allowed (the text is typed on the screen like any other); `log` is allowed and goes through `sysdialog.ScrubKnown`.

## Masking

A page value can be secret (a token shown on a page). Rules, in order of how much they rely on knowing:

1. The runner never prints a stored page value by itself: step lines show names, not values; the read step prints a length.
2. Wherever the value reaches a line or an error through a later step (`log "<total>"`, an error that quotes a selector), the existing `sysdialog.ScrubKnown` applies, as for any variable.
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

1. **Classification is complete.** The test lists every `ActionVerb` constant in `ast.go` and every step type that implements `Step`, and requires each to appear in exactly one of four tables in a `taint_classes.go` (`sinkSteps`, `uiOnlySteps`, `propagatingSteps`, `noStringSteps`). A new verb fails the test until someone classifies it.
2. **Sinks use the guard.** Using `go/parser`, the test finds the handler of each sink step (the `case` in `runAction`, or the named function) and requires that it takes strings through `forSink` and that it contains no direct call to `resolve`, `resolveVars` or `resolveSelector`.
3. **Outside the sinks nothing calls `forSink`'s opposite by accident:** UI-only handlers may call `resolve`; the test only reports a sink handler that does.
4. **Behaviour.** For every sink step, a table-driven test runs the step with a tainted variable against recording fakes (device context, agent client, HTTP) and requires the refusal, the exact message shape without the value, and that the fakes recorded no call.
5. **One-pass expansion:** a page value of `<token>` (with a `token` variable set) must not expand.

## Tests and mutants for the implementation

Taint set on read, on store, on recipe parameter use, through composition and through a copy; cleared by a script store; not set by data rows or `${ENV}`; one-pass; each sink refuses; each UI-only step allows; the read step: cap boundary (999, 1000, 1001), hidden characters dropped, the text form refused, the lookup modes (the proof re-run); masking by variable name; mutants one guard each (propagation points, each table entry, the one-pass rule, the guard's error, the cut).

## Slices (each a separate PR, one at a time)

1. **T1: taint without WebView.** `resolveT`, `forSink`, the tables, the structural test, the sink handlers switched to the guard, and the one-pass rule for tainted values; nothing is tainted yet (a test-only source marks values), so existing behaviour is unchanged and byte-identical.
2. **T2: the read step.** Grammar and parser (with the EBNF, conformance test and compatibility corpus), the `read` lookup mode and the proof re-run, the runner step, the cap and cleaning, masking by name, docs, and one real run on a named emulator.

## Open questions

- Should `store response ... as`, `read ... with ai into` and other stored values also be tainted? It is the same threat for a hostile server or screen, but it breaks the common `store` then `call` flow, so it needs its own decision and an opt-out (for example `store response "$.id" as id trusted`).
- Is `HTTPStep` (mock rules) a sink? Mock bodies are data for the app, but a page-chosen path or body changes what the app sees from the network; the design says sink until argued otherwise.
- Do we want a way to declare a read value safe, for example a validating form (`store text of ... as total matching "^[0-9.]+$"`) that clears taint when the value matches? It would make `call GET ".../<id>"` usable with a validated page value; it is left out of the first version.
