# E-2 evidence — element-scoped visual regression

`compare screenshot "x" of "Widget"` — matches Maestro's `assertScreenshot` + `cropOn` (competitive
roadmap Phase 1, E-2).

## Automated regression coverage

`internal/visual/regression_test.go`: `TestCropToBounds_ExtractsCorrectRegion` (crops a 4-quadrant
test image to the top-right quadrant, decodes the result, and asserts every pixel is the expected
green — proves the crop pulls the *right* pixels, not just a rectangle of the right size),
`TestCropToBounds_ClampsPartiallyOutOfBoundsRegion` (a region extending past the image edges is
clamped instead of erroring — tolerates slightly-stale `RenderBox` geometry), and
`TestCropToBounds_ErrorsWhenRegionIsFullyOutOfBounds` (zero-overlap fails loudly instead of
silently producing an empty image that would always "pass").

`internal/parser/parser_test.go`: `TestParser_CompareShot_NoScope`, `TestParser_CompareShot_ElementScoped`
(the "of" suffix is normally a filler word silently stripped everywhere else — this proves it's
correctly captured as a scope selector here), `TestParser_CompareShot_ElementScopedByID`.

Full `go test ./...` passes with no regressions.

## Real-device evidence (water-sip, Android emulator)

`e2_visual_regression_evidence.probe`, 2 tests. Required rebuilding water-sip with a current
`flutter_probe_agent` (the installed build predated the `probe.selector_bounds` RPC this feature
calls — already used in production for AI-redaction, just not present in this app's pinned
version) and the `--dart-define=PROBE_AGENT=true` flag main.dart requires — both reverted after,
water-sip's repo is unmodified.

```
✓  element-scoped baseline established and matches an unchanged widget (5.262s)
✗  element-scoped comparison detects a real content change in that widget (6.079s)
   visual regression: "e2_ml_text_changing" differs by 37.61% (threshold 0.50%),
   diff: reports/visual-diff/e2_ml_text_changing_diff_20260815_061103.png
```

Both outcomes are exactly as designed:
- Test 1 establishes a baseline crop of `#today_current_ml_text` at "0 ml", immediately re-compares
  against itself (same state) — passes with 0% diff.
- Test 2 establishes a baseline at "0 ml", taps quick-add 250, waits for "250 ml" to appear, then
  re-compares the same crop — **fails with a real, precise 37.61% diff** (not near-0%, not 100%),
  proving the crop is scoped to just the text region and genuinely detects content changes there.

Baseline dimensions confirmed **68×44 pixels** (checked via the PNG header) — a small, precise
widget crop, not a full-screen screenshot (this device's screen is ~1000+ px wide).
