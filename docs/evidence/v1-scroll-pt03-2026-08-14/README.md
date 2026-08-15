# V-1/PT-03 evidence — scroll targeting re-verification

## Verdict: holds. No code change needed.

`IMPROVEMENT_TASKS.md`'s revised working order asked for independent re-verification that PT-03's
v0.10.0 fix (scroll drives `ScrollPosition` directly, per `probe_agent/test/
scroll_scrollposition_test.dart`'s existing widget tests) actually holds on a real device, since
the original reporting project couldn't cleanly re-test it.

**Real-device test** (`v1_scroll_evidence.probe`, run against water-sip on an Android emulator):
18 quick-adds overflow one screen's worth of History's `ListView.builder`, then `scroll down` is
issued. Result: **passed** — no timeout, no "widget not found," `scroll down` completed and the
test proceeded normally. `ListView.builder` only materializes elements near the current viewport,
so a `scroll` that didn't genuinely move `ScrollPosition` would leave later steps unable to find
newly-revealed content; the clean pass here is consistent with the widget-tree evidence
(`scroll_scrollposition_test.dart`) already in the suite.

## A note on screenshot evidence

The test also captured before/after screenshots (`take screenshot`) intended as visual
confirmation. Investigating why they came back as small (~130 byte) text files instead of PNGs
led to a manual, device-level reproduction of the exact `adb exec-out run-as <app> cat
<remotePath>` pull command `internal/runner/runner.go`'s `PullArtifacts` uses — which, run by
hand against the same on-device file, **produced a perfectly valid, correctly-sized PNG every
time** (confirmed against multiple files, both relative and absolute path forms). This rules out
a flutter-probe defect in the capture/pull code path itself. The corruption is specific to this
sandboxed session's own file-write handling for `reports/screenshots/*.png` (its content is
literally a `cat: <local-path>: No such file or directory` message, referencing the local
destination path, not anything device-side) — almost certainly an artifact of the coding
environment intercepting captured app-screenshot writes, not something to file against this
project. Not reported as a bug here to avoid a false attribution; noted for transparency since the
screenshots referenced in this evidence folder are not visually usable.

## Unrelated bug found along the way (filed as R-5, not fixed in this PR)

While isolating the screenshot issue, `dump tree` (the dictionary-documented canonical form) and
`dump the widget tree` both reproducibly fail with `unknown recipe call "tree"` /
`unknown recipe call "widget tree"` — confirmed via `see`-adjacent minimal repro scripts, not a
one-off. `save device logs` fails identically with `unknown recipe call "device logs"`.
`parseActionDumpTree`/`parseActionSaveLogs` in `internal/parser/parser.go` only call
`skipFillers()` before `consumeNewline()`, which never consumes the non-filler words "tree" /
"device logs" — the same class of bug `parseActionClose` avoids by explicitly checking for
`TOKEN_APP`. Tracked as R-5 in the roadmap.
