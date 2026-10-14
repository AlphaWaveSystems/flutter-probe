---
title: "Find and fix flaky Flutter end-to-end tests"
description: "How to tell a flaky Flutter end-to-end test from a real failure with FlutterProbe: see retried attempts in the reports, replace fixed sleeps with waits, and set per-step budgets."
---

A flaky test passes and fails on the same code. The fix depends on the cause, so the first job is to see which tests flake and how often. This page uses FlutterProbe's retry reporting, then the waiting tools that remove the usual causes.

## 1. Make flakiness visible

Re-run a failing test automatically, and keep the record of each attempt:

```bash
probe test tests/ --device <id> --retry-failed 2 --format junit --junit-attempts -o reports/results.xml
```

- `--retry-failed 2` re-runs a failing test up to two extra times. A test that passes on a retry counts as passed and is marked with its attempt count (`passed on attempt 2` in the terminal, `"attempts": 2` in the JSON report).
- `--junit-attempts` writes one test case per attempt, so tools that track flaky tests can see the failed attempts of a run that finally passed.

Retries hide flakiness from a green build, so treat every "passed on attempt 2" as a bug to fix, not a pass.

## 2. Find the cause

Look at the failure message and the screenshot of the failing attempt. The usual causes:

| Symptom | Likely cause | Fix |
|---|---|---|
| Tap did nothing, or "not found" right after a screen change | The next screen was not built yet | `wait until "X" appears` before the tap |
| Fails only on slow CI | A fixed delay that is too short | Replace delays with waits |
| Fails only after a backend call | The response arrives late | `wait for response "/api/x" status 200 within 10 seconds` |
| A zero-state check passes when it should not | Text selectors match substrings | `see "0 ml" matching "^0 ml$"` |
| First connect times out on a busy machine | The app took longer than 30s to start | Raise `agent.dial_timeout` |

## 3. Replace delays with waits

Wait for the thing you need, not for a number of seconds:

```
tap "Export"
wait until "Report ready" appears within 90 seconds
see "Download"
```

`within` gives one slow step its own budget without raising the timeout for the whole suite. To let every step retry briefly before failing, run with `--implicit-wait 7s`. See [Timeouts, waiting and retries](/advanced/timeouts-and-retries/) for the full set.

## 4. Treat warnings as failures

A tap that hit nothing is easy to miss. Fail on it:

```bash
probe test tests/ --device <id> --fail-on-warning
```

## 5. Prove the fix

Run the one test several times in a row on the machine where it flaked. If it passes every time with `--retry-failed 0`, the fix holds; keep `--retry-failed` for the safety net, not as the fix.

## Next

[Run the suite in GitHub Actions](/tutorials/flutter-e2e-github-actions/), or read about [timeouts and retries](/advanced/timeouts-and-retries/).

---

The CLI and everything on this page is free. Probe Cloud, hosted report storage, is a [design-partner preview](/probe-cloud/) and is not live yet.
