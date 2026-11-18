---
title: When a Selector Breaks
description: What FlutterProbe does when a selector no longer matches. It lists what was on screen, can propose a replacement for you to apply, and never repairs a selector while a test runs.
---

FlutterProbe does **not** repair a selector while a test runs. A step such as `tap "Submit"` that finds no matching widget waits for its timeout and then fails. Nothing is substituted, retried with another selector or written back to your file. What FlutterProbe gives you is help to find the new selector quickly.

## What a failed step reports

When a step times out, the error ends with the texts and widget keys that were visible on screen at that moment:

```
line 42: wait until "Goal saved" appears timed out after 30s (context deadline exceeded) — visible texts: ["History", "Settings"], keys: ["tab_history"]
```

A failed `tap` or `see` carries the same ` — visible texts: [...], keys: [...]` ending. The lists are read from the app, so a typo, a changed label or a renamed key is usually visible in the message. An app built with an older agent may not report them.

## A proposed replacement in the report

```bash
probe test tests/ --ai-heal --format json -o reports/results.json
```

With an `ai:` block in `probe.yaml`, `--ai-heal` asks the configured model for one replacement selector for each failed selector step, chosen from the texts and keys that were on screen. The proposal is in the JSON report as `selector_proposal`. It is advisory: it is not applied to the test file, not verified against the app, and it changes no result and no exit code. See [Selector proposals for failed steps](/tools/ai/#selector-proposals-for-failed-steps) for the fields, the limits and what leaves your machine.

## Suggestions in Studio

Studio shows a suggestion under a failed step. Each one is a button you press; nothing is applied on its own, and none of them changes the result of the run.

- **Use the text on screen** (rule-based): when the screen shows exactly one text that is the same text written another way (a different case, spacing or Unicode form, such as `Sign in` against `Sign In`), the Results pane offers the step with that text.
- **Insert before the step** (rule-based): adds a `wait until "X" appears` line above the failed step.
- **Suggest a fix** (AI provider): one ProbeScript line for the failed step, on an advisory card, with **Replace the line**.

See [Studio](/tools/studio/) for the exact conditions and what Studio checks before it edits the buffer.

## What stays your job

Update the test. The proposals and suggestions point at a likely replacement; a flow that was restructured (two screens merged, a step removed) needs a person to rewrite it. For selectors that survive UI changes, prefer a `ValueKey` and the `#key` form over visible text.

## Not available

`probe heal` is not implemented. An earlier build printed suggestions that were not read from your files and changed nothing, so the command now refuses with an error that points to `--ai-heal`.
