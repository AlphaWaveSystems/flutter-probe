# WebView steps in ProbeScript (slice 3 scope note)

**Status:** Scope note for review. No code. It follows `docs/proposals/webview-content-automation.md`; the lookup, session and safety work is already in `internal/device` (`OpenWebView`, `Find`, `Tap`, `Type`, `WaitFor`, the `Selector` kinds, the `Redact` option).

## What this slice delivers

ProbeScript steps that reach content inside an app's WebView on Android, built on the session above, with the grammar, every dictionary surface, the user docs, redaction and evidence rules. iOS is not part of it.

## The steps (proposed syntax)

```
see "Getting Started" in webview
tap "Contact support" in webview
type "hello" into css:"#message" in webview
wait until "Message sent" appears in webview
see css:"#message" in webview
tap "Pay" in webview "checkout"          # pick one page when the app has several
```

- A **quoted string is text** (the visible text of an element). **`css:"..."` is a CSS selector.** The syntax decides the kind, so a text such as `#1 Item` or `.5 off` is text and a selector is never guessed from its first character. This maps onto the `Selector` kinds the session already takes (`Text`, `CSS`).
- `in webview` sends the step to the WebView page instead of the Flutter widget tree; with a following quoted string it names the page (a part of the title or the URL, case-insensitive). Without a name the app must have exactly one page.
- `see`, `tap`, `type` and `wait until ... appears` keep the meaning they have for widgets. `don't see ... in webview` is the negative of `see`.
- No step takes JavaScript. There is no raw-script form, now or in this slice (the owner question in the proposal stays "no").

## Rules for the runner glue

- **Android only.** On iOS the steps fail with a clear message, as `tap native` does today. In cloud mode (no adb) they print the same kind of skip line the native steps print.
- **The session is opened per step** in the first version and always closed (`WithWebView`): simple, fails closed, no state to leak between steps. A per-test session can come later if step time matters.
- **`Redact` is required.** The runner passes its secret masking (`sysdialog.ScrubKnown`) on every open; the session constructor will refuse a nil `Redact` rather than run unmasked.
- **Page text is untrusted.** Page-supplied text (titles, element text, page error messages) appears only in messages and reports, cut and masked; the runner never puts it in a ProbeScript, a shell command or a URL.
- **Typed text** never appears in a step line, an error, a log or a report beyond what the same text shows for an ordinary `type` step (a `${ENV}` value is masked there already).
- **The failure messages** are the ones in the proposal's table, from the session: no WebView, several pages, no match, not found, not visible, cannot take the focus, connection lost, page error, timed out.
- **Timeouts:** the step's own timeout is the call timeout; a `wait until` has its own total, as `WaitFor` enforces.
- **Evidence:** the step's screenshot is the device screenshot the run already takes; the page's HTML and the DevTools address are never written to a report. A failed WebView step records the failure message and, if the existing failure artifacts are on, the screenshot.

## What the user must know (user docs, stated plainly)

- The app's WebView must have debugging on (`WebView.setWebContentsDebuggingEnabled(true)`, usually a debug or profile build); a release build with it off has nothing to reach, and the step says so.
- Content inside **shadow DOM or an iframe is not searched.** Only the top document of a page is.
- A **read-only or disabled input ignores typed text**; the step refuses when the element does not take the focus.
- The focus check happens once, so a page that steals the focus again right after can still receive the text.
- Positions are in CSS pixels at a device pixel ratio of 1 in the tests so far; other ratios, scrolled pages and iframes are not yet tested.

## Surfaces to change (from the last two new verbs: set appearance and set airplane mode)

Parser and grammar: `internal/parser` (`token.go`, `lexer.go`, `ast.go`, `parser.go`, `grammar_test.go` and the EBNF in `website/src/content/docs/probescript/grammar.md`, with a conformance test). Runner: `internal/runner` (`executor.go`, `device_context.go`, step description with masking). Dictionary and docs: `website/src/content/docs/probescript/dictionary.md` and `syntax.md`, `platform/android.md` and `ios.md`, `tools/mcp.md`, `website/public/llms.txt`, `README.md`, `CHANGELOG.md`. Other surfaces: `internal/mcp/server.go` (the reference text), `vscode/syntaxes/probe.tmGrammar.json`, `vscode/snippets/probescript.json`, `vscode/src/providers/completions.ts`, `vscode/README.md`, and Studio's generated dictionary (`studio/frontend/src/probescript/dictionary.generated.ts`, regenerated, not edited by hand).

## Slices (each a separate PR)

- **V1. Grammar and parser:** the five forms, the kind from the syntax, the page name, the EBNF and its conformance test, hostile-input tests for the parser. No runner change: the steps parse and the runner reports "not implemented" until V2.
- **V2. Runner glue:** the steps over the session, `Redact` required, per-step open and close, the messages, the masked step line, tests with the fake DevTools server and a recording adb. One real run against a fixture app recorded in the PR.
- **V3. Dictionary and docs surfaces:** every file above, in one PR, with the user docs of the previous section.
- **V4. Fixture:** the one-WebView test app as an end-to-end fixture in the right place (the maintainers decide where it lives), with a run that taps, types and reads back.

## Not in this slice

iOS, shadow DOM and iframes, navigation steps, cookies and storage, file upload and download, a per-test session, raw JavaScript, and a Maestro-migration mapping.

## Questions for review

- Is `css:"..."` the right spelling for the selector kind, or would `selector "..."` read better next to the existing step names?
- Should the page name come after `in webview` (as above) or be a separate `use webview "checkout"` step that stays in effect for the test?
- Is a per-step session acceptable for the first release?
