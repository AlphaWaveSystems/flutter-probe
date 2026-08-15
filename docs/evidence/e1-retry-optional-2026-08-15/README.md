# E-1 evidence — `retry N times` block + `optional` step modifier

Matches Maestro's `retry` block and `optional: true` step property (competitive roadmap Phase 1,
E-1).

## Automated regression coverage

`internal/parser/parser_test.go`: `TestParser_Retry`, `TestParser_Retry_DefaultCountIsOne`,
`TestParser_ActionOptional`, `TestParser_ActionWithoutOptional_DefaultsFalse`,
`TestParser_AssertOptional`.

`internal/runner/retry_optional_test.go`: `TestRunRetry_SucceedsAfterFailures` (a scripted client
that fails twice then succeeds — proves `retry` re-runs the whole block and stops at first
success, not "run every attempt regardless" like `repeat`), `TestRunRetry_ExhaustsAllAttempts`
(an always-failing body errors out after exactly `Count` attempts, not fewer/more/forever),
`TestRunStep_OptionalActionSwallowsFailure`, `TestRunStep_NonOptionalActionStillFails` (the
regression guard proving the swallow is actually gated on the flag), and
`TestRunStep_OptionalAssertSwallowsFailure`.

Full `go test ./...` passes with no regressions.

## Real-device evidence (water-sip, Android emulator)

`e1_retry_optional_evidence.probe`, 3 tests, run against a real emulator:

```
✓  retry N times executes its body end-to-end on a real device (6.292s)
✓  optional swallows a real, genuine failure instead of failing the test (5.371s)
   ⚠  optional step failed, continuing: rpc error -32001: Widget not found: text("...")
✗  non-optional identical failure still fails the test (regression guard) (5.259s)
   rpc error -32001: Widget not found: text("...")

2 passed, 1 failed  (16.922s)
```

All three outcomes are exactly as designed: `retry` executes its body cleanly end-to-end;
`optional` attempts an identical, genuinely-nonexistent selector, logs the warning shown above,
and the test still passes; the third test uses the **same failing selector without `optional`**
and correctly still fails — proving the swallow logic is gated on the modifier, not silently
hiding every failure.

Full output: `e1_evidence_output_clean.log` (ANSI codes stripped).
