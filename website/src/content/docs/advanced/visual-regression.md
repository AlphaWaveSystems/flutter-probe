---
title: Visual Regression Testing
description: Screenshot-based visual regression testing with configurable thresholds.
---

FlutterProbe includes screenshot-based visual regression testing that compares current screenshots against baseline images to detect unintended visual changes.

## How It Works

1. **Baseline capture** — On the first run, screenshots are saved as baseline images
2. **Comparison** — On subsequent runs, new screenshots are compared pixel-by-pixel against the baselines
3. **Diff reporting** — If the difference exceeds the configured threshold, the test fails with a visual diff

## Taking Screenshots

Use the `take screenshot` command in your tests:

```
test "checkout page looks correct"
  open the app
  tap "Cart"
  tap "Checkout"
  wait for the page to load
  take screenshot "checkout_page"
```

Screenshots are saved as PNG files in the `reports/screenshots/` directory.

## Configuration

### Threshold

The maximum allowed pixel difference as a percentage. If the diff exceeds this value, the test fails.

```yaml
# probe.yaml
visual:
  threshold: 0.5    # 0.5% — default
```

Or via CLI flag:

```bash
probe test tests/ --visual-threshold 0.5
```

### Pixel Delta

The per-pixel color delta tolerance. Small differences (e.g., anti-aliasing) below this value are ignored.

```yaml
visual:
  pixel_delta: 8    # default
```

Or via CLI flag:

```bash
probe test tests/ --visual-pixel-delta 8
```

## Typical Workflow

1. **Create baseline** — Run tests the first time to capture baseline screenshots
2. **Make changes** — Update your app UI
3. **Run tests** — Visual regression detects differences against the baseline
4. **Review diffs** — Check if changes are intentional
5. **Update baseline** — If changes are expected, re-run to update the baselines

## Tips

- Use a consistent device/simulator for baselines (different screen sizes produce different screenshots)
- Set a reasonable threshold — too low causes false positives from rendering differences, too high misses real regressions
- Run visual regression tests on a single platform first, then expand
- Screenshots are captured by the Dart agent directly (no external tools needed)
