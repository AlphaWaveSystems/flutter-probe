---
title: Timeouts, waiting and retries
description: How FlutterProbe waits for components, texts and backend responses, when it retries, and when it gives up - step timeouts, implicit waits, per-step budgets, optional steps and retried tests.
---

A test fails for two different reasons: the thing is **not there yet** (slow screen, slow backend) or it is **never going to be there**.
FlutterProbe gives you a few layers, from the loosest to the most explicit. Each layer is off or sane by default and none of them
changes a suite unless you ask.

## 1. The step timeout (always on)

Every step has a time limit: `defaults.timeout` in `probe.yaml` or `--timeout 45s` (default 30s). A step that is not done by then
fails with what it was waiting for, a screenshot, and (for backend steps) the last requests the app made. `restart the app` and
`clear app data` use `agent.launch_timeout` instead (default 120s).

`wait until "X" appears`, `wait for response ...` and the other `wait` steps poll until their target shows up or this limit is hit.

## 2. Implicit wait (opt in, whole run)

Without it, `tap "Save"` fails at once when the button is not on screen yet. With

```bash
probe test tests/ --implicit-wait 7s        # or defaults.implicit_wait: 7s
```

`tap`, `type`, `long press`, `double tap`, `clear`, `drag`, a plain `see`, and the response checks (`see response`, `store response`,
`if response`) retry for up to that long before failing, like Maestro. It is meant for navigations and loads that finish a moment
later. `don't see`, steps with `optional` or `if visible`, and the `wait` steps are never retried this way.

## 3. A budget for one step: `within`

```
tap "Export" within 5 seconds
wait until "Report ready" appears within 90 seconds
wait for response "/api/export" status 200 within 120 seconds
see "Done" within 500 ms
```

`within N seconds|ms` replaces both the step timeout and the implicit-wait window for that one step. Use it for the one slow
operation (an export, a payment, a cold backend) without raising the timeout of the whole suite, or to make a step fail fast.

## 4. Failing gracefully

When missing is acceptable, say so instead of catching the failure later:

```
tap "Skip tour" optional                  # try it; a failure is logged, the test goes on
tap "Allow" if visible                    # only when it is on screen right now
if "What's new" appears
  tap "Close"
if response "/api/me" json "data.plan" equals "pro"
  see "Premium"
otherwise
  see "Upgrade"
```

`if "X" appears` checks once (about a second) and takes the `otherwise` branch when absent; `if response` looks at the newest
recorded response and is false when there is none.

## 5. Retrying a block or a whole test

```
retry 3 times
  tap "Refresh"
  wait for response GET "/api/orders" status 200 within 5 seconds
  see "Order #1"
```

`retry N times` runs the block again from the top when a step in it fails (only the last attempt's error is reported). It is the
right tool for an action whose result is checked afterwards. For flaky tests as a whole, `defaults.retry_failed_tests: 2` (or `probe test --retry-failed 2`) runs a
failing test again up to that many extra times before reporting it failed. A test that passes on a retry counts as passed and is
marked (`passed on attempt 2` in the terminal, `"attempts": 2` in the JSON report), so flakiness stays visible. Off by default.

## Choosing

| The thing is... | Use |
|---|---|
| usually there, sometimes late | `--implicit-wait 7s` for the run |
| there only after one slow operation | `within 90 seconds` on the step that waits |
| optional (a banner, a tour) | `optional`, `if visible`, `if "X" appears` |
| the result of a flaky action | `retry 3 times` around action + check |
| a flaky test you cannot fix yet | `--retry-failed 2` / `defaults.retry_failed_tests` (and fix it) |
| a backend answer | `wait for response ... within N seconds`, or mock it to make it deterministic ([guide](/advanced/backend-data/)) |

Fail fast on purpose: a short `--timeout` with `within` on the few slow steps finds a missing element in seconds instead of
half a minute, and `--fail-on-warning` turns a tap that did nothing into a failure.
