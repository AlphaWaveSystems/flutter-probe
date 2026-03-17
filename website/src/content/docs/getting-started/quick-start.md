---
title: Quick Start
description: Write your first ProbeScript test, run it, and see results.
---

This guide walks you through writing and running your first FlutterProbe test. It assumes you have already [installed](/getting-started/installation/) the CLI and integrated the Dart agent.

## 1. Write a Test

Create a file `tests/smoke/login.probe`:

```
test "user can log in"
  @smoke
  open the app
  wait until "Sign In" appears
  tap "Sign In"
  type "user@example.com" into "Email"
  type "secret123" into "Password"
  tap "Continue"
  see "Dashboard"
```

## 2. Validate Syntax

Before running, lint the file to catch syntax errors:

```bash
probe lint tests/smoke/login.probe
```

## 3. Run the Test

Make sure your app is running on a device or simulator with ProbeAgent enabled, then:

```bash
probe test tests/smoke/login.probe --device <UDID-or-serial> -v
```

Flags:
- `-v` enables verbose output
- `--device` targets a specific device (use `probe device list` to find available devices)
- `-y` auto-confirms destructive operations and auto-grants permissions

## 4. Run All Tests

```bash
probe test tests/ --device <UDID-or-serial> --timeout 60s -v -y
```

## 5. Filter by Tag

```bash
probe test tests/ --tag smoke
```

## 6. Generate Reports

```bash
# JSON output (for HTML report)
probe test tests/ --format json -o reports/results.json --video

# Generate HTML report
probe report --input reports/results.json -o reports/report.html --open

# JUnit XML (for CI)
probe test tests/ --format junit -o reports/results.xml
```

## 7. Watch Mode

Re-run tests automatically when `.probe` files change:

```bash
probe test tests/ --watch
```

## What to Read Next

- [ProbeScript Syntax](/probescript/syntax/) — full language reference
- [Recipes](/probescript/recipes/) — reusable step sequences
- [CLI Reference](/tools/cli-reference/) — all commands and flags
- [GitHub Actions](/ci-cd/github-actions/) — CI/CD setup
