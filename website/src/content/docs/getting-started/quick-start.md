---
title: Quick Start
description: Write your first ProbeScript test, run it, and see results.
---

This guide walks you through writing and running your first FlutterProbe test. It assumes you have already [installed](/getting-started/installation/) the CLI and integrated the Dart agent.

The fastest path, if the CLI and agent are installed and your app is running with `--dart-define=PROBE_AGENT=true`:

```bash
probe init                                   # creates probe.yaml and tests/example.probe
probe doctor                                 # checks the project and this machine, and says how to fix what is missing
probe lint tests                             # checks the files parse
probe device list                            # find your device id
probe test tests/example.probe --device <id> -v
```

Open `probe.yaml` first and set `project.app` to your app's id. The sample test only opens the app, so it passes when the app starts with the agent enabled. Then add checks for your own screens. The rest of this page writes a real test.

## 1. Write a Test

Create a file `tests/smoke/login.probe`. The screen texts ("Sign In", "Email", "Dashboard") are examples: use the visible text or widget keys from your own app.

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
