---
title: When a Selector Breaks
description: What FlutterProbe does when a selector no longer matches. It lists what was on screen, can propose a replacement for you to apply, and never repairs a selector while a test runs.
---

FlutterProbe does **not** repair a selector while a test runs. A step such as `tap "Submit"` that finds no matching widget waits for its timeout and then fails. Nothing is substituted, retried with another selector or written back to your file. What FlutterProbe gives you is help to find the new selector quickly.

## What a failed step reports

A failed step ends with the texts and widget keys that were visible on screen at that moment. The agent adds the ending to a wait that times out, to `Widget not found`, and to `type` or `clear` with no focused field, and the CLI adds it when a step hits its own time limit:

```
Timed out waiting for "Goal saved" to appear — visible texts: ["History", "Settings"], keys: ["tab_history"]
```

The lists are read from the app, so a typo, a changed label or a renamed key is usually visible in the message. An app built with an older agent, or a screen with no visible text or keys, adds nothing.

## A proposed replacement in the report

```bash
probe test tests/ --ai-heal --format json -o reports/results.json
```

With an `ai:` block in `probe.yaml`, `--ai-heal` asks the configured model for one replacement selector for a step that ran into the CLI's step time limit (`test_timeout` or the step timeout) on a text or key selector, chosen from the texts and keys recorded on screen at that moment. A plain `Widget not found` from the agent, or a failure that recorded no lists, makes no model request and its entry in the report says the proposal is unavailable. A run makes at most 3 requests. The proposal is in the JSON report as `selector_proposal`. It is advisory: it is not applied to the test file, not verified against the app, and it changes no result and no exit code. See [Selector proposals for failed steps](/tools/ai/#selector-proposals-for-failed-steps) for the fields, the limits and what leaves your machine.

## Suggestions in Studio

Studio shows suggestions for a failed step as buttons. Each one is a click; nothing is applied on its own, and none of them changes the result of the run.

- **`Use "<text>"`** (rule-based): when a `see` or `tap` on a text timed out and the screen showed exactly one text that is the same text written another way (a different case, spacing or Unicode form, such as `Sign in` against `Sign In`), the "On screen" row under the step offers the same step with that text.
- **Insert before the step** (rule-based): under a failed `see` or `tap` whose element was not found, the "Suggested fix" adds a `wait until "X" appears` line above the failed step.
- **Suggest a fix** (AI provider): the button next to **Explain failures**. It asks for one ProbeScript line for the failed step of the last run and shows it on an advisory card with **Replace the line**.

See [Studio](/tools/studio/) for the exact conditions and what Studio checks before it edits the buffer.

## What stays your job

Update the test. The proposals and suggestions point at a likely replacement; a flow that was restructured (two screens merged, a step removed) needs a person to rewrite it. For selectors that survive UI changes, prefer a `ValueKey` and the `#key` form over visible text.

## Not available

`probe heal` is not implemented. An earlier build printed suggestions that were not read from your files and changed nothing, so the command now refuses with an error that points to `--ai-heal`.
