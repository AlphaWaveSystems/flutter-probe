# R-5 evidence — `dump tree` / `dump the widget tree` / `save device logs` misparse

## Root cause

`parseActionDumpTree`/`parseActionSaveLogs` (`internal/parser/parser.go`) advanced past the verb
keyword, called `skipFillers()`, then `consumeNewline()` — never explicitly consuming the
non-filler words that follow ("tree", "widget tree", "device logs"). `consumeNewline()` silently
no-ops when the current token isn't a newline, so those words were left dangling on the same
line, and the top-level statement loop tried to parse them as a second step — an unknown recipe
call.

## Automated regression coverage

`internal/parser/parser_test.go`: `TestParser_DumpTree_Short`, `TestParser_DumpTree_Long`,
`TestParser_SaveDeviceLogs` — all assert exactly 1 step in the test body.

**Pre-fix** (`git stash` on `parser.go`):
```
--- FAIL: TestParser_DumpTree_Short (step count: got 2, want 1)
--- FAIL: TestParser_DumpTree_Long (step count: got 2, want 1)
--- FAIL: TestParser_SaveDeviceLogs (step count: got 2, want 1)
```

**Post-fix**: all 3 pass. Full `go test ./...` passes with no regressions.

## Real-device evidence (water-sip, Android emulator)

**Pre-fix** (found while gathering V-1/PT-03 evidence, see `docs/evidence/v1-scroll-pt03-2026-08-14/`):
```
✗  dump tree canonical form
   line 4: unknown recipe call "tree" — no recipe with that name is defined...
✗  save device logs canonical form
   line 4: unknown recipe call "device logs" — no recipe with that name is defined...
```

**Post-fix** (`dump_tree_check.probe`, `save_logs_check.probe`, both included in this folder):
```
✓  dump tree canonical form (5.032s)
✓  save device logs canonical form (4.878s)
```
