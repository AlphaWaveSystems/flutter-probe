---
title: Self-Healing Selectors
description: AI-powered selector recovery using fuzzy matching against the widget tree.
---

FlutterProbe includes a self-healing system that can recover from broken selectors by fuzzy-matching against the live widget tree. When a selector fails to find its target, the AI module tries alternative strategies before giving up.

## How It Works

When a selector like `tap "Submit"` fails to find a matching widget, the self-healing module:

1. Dumps the current widget tree
2. Runs multiple fuzzy matching strategies against the tree
3. If a confident match is found, the command proceeds with the recovered selector
4. The recovery is logged so you can update the test later

## Matching Strategies

The system uses four strategies, evaluated in order of confidence:

### 1. Text Fuzzy Match

Finds widgets with similar text content using string similarity scoring.

- `"Submitt"` matches `"Submit"` (typo recovery)
- `"Sign in"` matches `"Sign In"` (case variation)

### 2. Key Partial Match

Finds widgets whose `ValueKey` partially matches the selector.

- `#login` matches a widget with key `loginButton`
- `#submit` matches a widget with key `submitForm`

### 3. Type + Position Match

Finds widgets by type and position when text/key matching fails.

- Matches the Nth widget of a given type in the tree
- Useful when widget text has changed but the structure remains the same

### 4. Semantic Match

Uses semantic labels and accessibility information to find widgets.

- Matches against `Semantics` labels in the widget tree
- Falls back to tooltip and hint text

## Configuration

Self-healing is enabled by default. The system only attempts recovery when a standard selector lookup fails — there is no performance overhead for passing selectors.

## Logging

When a selector is healed, the test output includes a warning:

```
  ⚠ Selector "Submitt" healed → matched "Submit" (text_fuzzy, confidence: 0.92)
```

This tells you which strategy was used and the confidence score, so you can update the test to use the correct selector.

## Limitations

- Self-healing adds latency when a selector fails (widget tree dump + matching)
- Low-confidence matches are rejected to prevent false positives
- The system cannot recover from structural changes where the target widget has been removed entirely
- Works best when selectors are close to correct (typos, minor text changes, key renames)
