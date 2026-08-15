# R-1 evidence — tap-family verbs resolve `<var>` placeholders

Real-device test against water-sip (Android emulator, `com.alphawavesystems.watersip`),
using `r1_var_tap_evidence.probe`: quick-adds 250ml (via `#today_quick_add_250`), stores the
visible "Undo" button's label in a variable, then taps `"<undo_label>"`.

## Pre-fix (built from `main` @ c640e06, before this branch's change)

```
✗  tap resolves a stored variable in its selector (6.357s)
   rpc error -32001: Widget not found: text("<undo_label>")
```

The unresolved literal `"<undo_label>"` was sent to the device — confirms the PT-02 addendum bug
exactly as described in IMPROVEMENT_TASKS.md.

## Post-fix (this branch)

```
✓  tap resolves a stored variable in its selector (8.991s)

1 passed  (8.991s)
```

Resolves to "Undo", taps the real button, undo succeeds (`0 ml` reappears).

## Automated regression coverage

`internal/runner/executor_test.go`:
- `TestResolveSelector_ResolvesVariablePlaceholder`
- `TestResolveSelector_LeavesPlainTextUnchanged`

Full package suite (`go test ./...`) passes with no regressions.
