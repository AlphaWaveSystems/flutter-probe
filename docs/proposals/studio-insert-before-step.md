# Design: "Insert before the step" for the rule-based fix in Studio

Status: proposal, no code. Revised after review; the code waits for acceptance.

## The problem
A failed `see "X"` or `tap "X"` that found no element gets a rule-based row, "Suggested fix: `wait until "X" appears`" (`studio/triage.go`; an id gets a reason only). The row has no button, unlike the AI card's **Replace the line**. That is by design today: the fix is a line **to add before** the failed step, not a replacement for it (replacing the failing `tap "X"` with a `wait until` would delete the step the test is about). A person types it. This proposal adds a button for it, with the safety rules of **Replace the line**, and one stronger anchor.

## What shows, and when
The triage rows are attached to every failed step the rules cover, whether the AI provider is on or off (`app.go` attaches `Triage`, `results.ts` shows the rows whenever it is there). The earlier wording "with the AI provider off" was copied from a code comment and is not the behaviour. So with AI on a failed step can show two things: the rule rows (with the new **Insert before the step**) and, after the person asks, an AI card (with **Replace the line**). They are different buttons with different labels and each acts on the failed line. Both are protected by the same "file unchanged since the run" rule below, so after either edit the other one refuses ("the file changed since the run: run again") instead of acting on a shifted line.

The button is under the rule's fix row only, only for a step whose rules gave a fix, only while the file the run failed in is the open file.

## What is inserted, and where
Exactly the rule's `Fix` text, one plain line (`plainSuggestionLine`), with the indentation of the failed step's own line, as a new line directly above it. Nothing else changes. Nothing is written to disk: the editor changes; **Save** stays the person's own action.

## The anchor: the file as the run read it
The line number of a failed step is a line of the file the **run** read, and a run reads the file on disk, while the editor may hold unsaved edits. Comparing only "line N still has the same text" does not prove it is the same step (two identical steps and an edit above can shift the other one into line N, and the insertion would land before the wrong step). So the check uses a stronger anchor:

- When a run starts, Go keeps the text of the file it parsed (the run's source; one run, the files of that run, each up to 1 MiB, dropped when the next run begins; it is not part of the 4-entry AI suggestion store, so rule fixes never evict AI suggestions, and a retried test, which fires one failed event per attempt, adds nothing to it).
- `CheckInsertBefore(file, line, fix, openFile, content)` answers ok only if the editor's text `content` is **exactly** the text the run read for that file. Any edit since the run, saved or not, makes it refuse: "the file changed since the run: run again". That settles which text the line number refers to (a dirty editor that differs from disk is refused), and it makes a wrong-step insertion impossible, because the whole file is the anchor, not one line.
- After an **Undo** the text is the run's text again, so the button works again; after an insertion it refuses (the file differs), and so do all other rows of that run. This is the same staleness the rows already have, made explicit: the Results rows and step marks of a run describe the file as it was, any manual edit already makes their line numbers unreliable, and now the offers say so instead of acting.

## The other checks, in Go
`CheckInsertBefore` reads nothing from disk and writes nothing (the run's text is in memory); it answers `{ok, line, text}` or a one-line reason. It refuses when:
1. the run's text for the file is not held (a new run began, or the file was not part of the run);
2. the open file is not the file the step ran from (`sameFile`);
3. the file changed since the run (above);
4. the fix is not one plain line;
5. the line directly above the failed step already is that wait (a second press cannot stack two);
6. line N does not hold exactly one step, or that step is not of an allowed profile (`stepsOf`, `allowed`, `profile` as Replace does: a step on the closed list of `see`/`tap`), so a composite or wrapper line, or a step the rules do not cover, is refused;
7. the text after the insertion does not parse into the old steps plus exactly one new `wait` step at line N, with every other step unchanged and only moved down one line. `sameSteps` cannot be reused (it compares line by line and an insertion shifts every following line), so a second comparison is written for this: the old step list with lines from N on increased by one, plus the new step, must equal the new list.

The window applies the edit only when the check said ok **and** the editor text is still the text that was checked.

## Undo, line endings
One editor edit in one undo stop (`pushUndoStop` before and after, as Replace does): **Undo** once removes the line and nothing else; the caret goes to the inserted line. The new line takes the line ending of the line it sits above (a CRLF file stays CRLF). A failed step on the last line of a file with no line ending at the end is the one case with no ending to copy: the inserted line gets the file's own ending and the last line stays as it was, without one. Both cases need editor-level tests (a Playwright spec with a CRLF file and with a file with no final newline), not only Go tests.

## Results on screen
After an insertion every line below it has moved by one, and the step marks and row line numbers of that run no longer match. The marks are cleared, a note says "The file changed: run again", and a click on a row of that run says why it does not jump, until the next run (an Undo does not bring them back: they are not recomputed). The existing **Re-run** card after a Replace is offered only when the line count is unchanged (`lineCount` in `selector-fix.ts`); an insertion changes it, so no Re-run card is offered after an insertion, by that same rule.

## The method
A new bound method, `CheckInsertBefore`: pure like `CheckReplaceLine`, with the same entry shape in `studio/bound_methods.json` (category `ui`, callers `window` and `bridge`, touches none, confirm none; the guard test derives the touches and fails if it reaches a file, the keychain, the network or a device). The automation endpoint and the MCP server cannot call it. No AI, no provider, no network.

## Tests to write with the code
- Go: a table for each refusal above (no run text, another file, a changed file by one character and by an edit above that shifts an identical step, an unsaved edit, an empty or multi-line fix, a wait already above, a line with two steps, a step not on the closed list, a result that does not parse into old plus one) and the accepted case, with a mutant per refusal; two identical steps with an edit above (the wrong-step case) must be refused.
- Playwright (no device, a seeded failed run): the button appears under the rule row only, with AI on and off; one press inserts and one undo removes and the button works again; a second press is refused; an edit first (above or on the line) is refused; another open file is refused; the rows of the old run no longer jump after an insertion; with AI on, after Insert the AI card's Replace refuses, and the other way round; a CRLF file; a failed step on a last line with no final newline.

## Open question for the owner
Should the same click also re-run the file? This design says no (the person looks at the new line first, then presses Run), which also keeps every press one visible change.
