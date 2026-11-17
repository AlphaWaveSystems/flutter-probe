# Design: "Insert before the step" for the rule-based fix in Studio

Status: proposal, no code. Written before the code, as asked, so the rules can be argued first.

## The problem
With the AI provider off, a failed `see "X"` or `tap "X"` that found no element gets a rule-based row, "Suggested fix: `wait until "X" appears`" (`studio/triage.go`). The window check noticed it has no button, unlike the AI card's **Replace the line**. That is by design today: the fix is a line **to add before** the failed step, not a replacement for it (replacing the failing `tap "X"` with a `wait until` would delete the step the test is about). A person has to type it. This proposal adds a button for it, with the safety rules of **Replace the line**.

## What is inserted
Exactly the rule's `Fix` text, one plain line (`wait until "X" appears`), with the indentation of the failed step's own line, as a new line directly above it. Nothing else changes: not the failed step, not any other line. Nothing is written to disk: the editor changes, and **Save** stays the person's own action.

It is offered only where the rules gave a fix (a text selector; an id gets a reason only, as today), only while the file the run failed in is the file open in the editor, and only under the failed step's own rows (the row, its error, the suggested fix).

## The checks, in Go, before the editor changes
A new bound method, `CheckInsertBefore(id, openFile, content)`, shaped like `CheckReplaceLine`: it reads nothing from disk and writes nothing, and answers `{ok, line, text}` or a one-line reason. It needs what Replace needs, so the failed step's result must be remembered the way the AI suggestions are: when the rules give a fix, Studio keeps `{id, file, line, original line text, fix, created}` for a short time (the same lifetime as a suggestion's) and the row carries the `id`. It refuses, with its reason, when:
1. the id is unknown or expired ("run again");
2. the open file is not the file the step ran from (`sameFile`);
3. **the step moved**: line N of the current text is not exactly the text it had when it failed. It never searches for the step by its text, because two steps can say the same thing;
4. the fix is not one plain line (`plainSuggestionLine`);
5. the line directly above the failed step already is that wait (a second press, or a wait the person typed): it brings nothing new, so pressing twice cannot stack two;
6. the file does not parse as it is, or after the insertion does not parse into exactly the old steps plus one new `wait` step directly before the failed one, with every other step unchanged and only moved down one line (`stepsOf` on both texts, as `sameSteps` does for Replace).

The window applies the edit only when the check said ok **and** the editor text is still the text that was checked (the same recheck as Replace: a change in between asks to press again).

## Undo
One editor edit in one undo step: **Undo** once removes the line and nothing else. The caret goes to the inserted line. A CRLF file stays a CRLF file (the new line takes the line ending of the line it sits above).

## What happens to the results on screen
The rows of the run name line numbers, and every line below the insertion has moved down by one, so the old numbers would point at the wrong lines. After an insertion the step marks of that run in the editor are cleared and a note says "The file changed: run again", and a click on a row of that run no longer jumps (it says why), until the next run. The button is then gone for that step. This is the one place the design differs from Replace, which keeps the line count.

## What does not change
No AI, no provider, no network. The method is pure (it touches nothing, like `CheckReplaceLine`) and gets its entry in `studio/bound_methods.json`; the automation endpoint and the MCP server cannot call it.

## Tests to write with the code
- Go: a table for each refusal above (unknown id, expired, another file, the line moved, two identical steps, an empty or multi-line fix, a wait already above, a file that does not parse, CRLF) and for the accepted case, with a mutant per refusal.
- Playwright (no device, a seeded failed run): the button appears under the rule row only; one press inserts and one undo removes; a second press is refused; an edit to the failed line first is refused with the reason; another open file is refused; the rows of the old run no longer jump.

## Open question for the owner
Should the same click also re-run the file? This design says no (the person looks at the new line first, then presses Run), to keep every press one visible change.
