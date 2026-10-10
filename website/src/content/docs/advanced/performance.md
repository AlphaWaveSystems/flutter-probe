---
title: Performance testing
description: Measure CPU, memory, frame timings and network of your app while a test runs - assertions, report sections, baselines that fail a run on regressions, and trends across runs.
---

A test that passes can still be slow, leak memory or hammer the network. FlutterProbe measures the app **while the test runs** and
lets you assert on it, compare it with a baseline, and watch it over time.

## Measuring a window

```
test "checkout stays light"
  open the app
  start measuring "checkout"
  tap "Pay"
  wait for response POST "/api/pay" status 200 within 10 seconds
  stop measuring
  see memory below 300 MB
  see memory growth below 20 MB
  see cpu below 60 percent
  see slow frames below 5 percent
  see data transferred below 500 KB
  see response POST "/api/pay" below 2000 ms
```

`start measuring "name"` opens a window; `stop measuring` closes it and prints one line with the numbers. `see ...` steps check the
window that is open (the numbers so far) or the last one closed. A window left open at the end of the test is closed for you.
`stop measuring` waits about a second, because Flutter hands frame timings over in batches.

| Assertion | Measures |
|---|---|
| `see memory below 300 MB` | peak memory of the app process (resident set) |
| `see memory growth below 20 MB` | memory at the end minus at the start: a leak check |
| `see cpu below 60 percent` | average CPU, in percent of one core (100 = one core fully busy) |
| `see cpu peak below 90 percent` | the busiest second |
| `see slow frames below 5 percent` | frames whose build + raster took more than 16.7 ms |
| `see frame time below 16 ms` | 95th percentile frame time |
| `see slowest frame below 100 ms` | the single slowest frame |
| `see data transferred below 500 KB` | request + response bytes of the app's HTTP traffic in the window |
| `see response GET "/api/x" below 800 ms` | how long the newest matching response took |

## Where the numbers come from

| Number | Source | Android | iOS simulator | Physical iPhone |
|---|---|---|---|---|
| Memory, frames | the agent, inside the app | yes | yes | yes |
| Network | the agent's HTTP recording (`dart:io` only) | yes | yes | yes |
| CPU | read by the CLI from the device | yes (`/proc`) | yes (the app is a Mac process) | no |

On a physical iPhone, CPU needs Xcode Instruments; a `see cpu ...` step there fails with a message saying so, and the other numbers
still work. Memory is the process's resident set as the OS reports it, not the Dart heap.

## Measure the right build

Debug builds are slow by design: on a debug build most frames are "slow", CPU is high and memory includes debug bookkeeping.
Run performance tests against a **profile** build (`flutter build apk --profile`, `flutter build ios --profile`; physical iPhones
need profile or release anyway) and start the agent with `--dart-define=PROBE_AGENT=true`. Emulator and simulator numbers also
move with the host load, so use thresholds with headroom and the baseline comparison below instead of absolute limits for CPU.

## Baselines: fail a run that got worse

```bash
# on main: record what "good" looks like (only passing tests are written)
probe test tests/perf/ --yes --perf-baseline perf-baseline.json --perf-update-baseline

# on every change: fail tests whose numbers got clearly worse
probe test tests/perf/ --yes --perf-baseline perf-baseline.json --perf-tolerance 20
```

Every measurement is stored under `file::test::name`. A measurement fails its test when a number exceeds the baseline by more than
the tolerance (default 20%) **and** by more than a noise floor (5 MB memory, 5 points CPU, 2 points slow frames, 3 ms frame time,
20 KB data), so a big percentage of a tiny number does not break the build. A measurement without a baseline entry is new and passes.
`--retry-failed N` re-runs a test that failed this way, which smooths a noisy device.

To compare two baselines (a branch against main): `probe perf compare perf-baseline.main.json perf-baseline.branch.json`.

## Trends

Every run that measured something appends its numbers to `reports/perf-history.jsonl` (`--perf-history FILE` to move it,
`--no-perf-history` to skip it):

```bash
probe perf trend --metric memory
probe perf trend --metric slow_frames --filter checkout --last 20
```

prints a sparkline per measurement with the latest value and the change since the first run. Metrics: `memory`, `memory_growth`,
`cpu`, `cpu_peak`, `slow_frames`, `frame_time`, `frame_max`, `data`.

## Reports

`--format json` adds a `perf` array to every test; the HTML report shows a **Performance** block in each test's details; Probe
Studio lists the measurements under each result. The MCP tools `get_report`, `perf_trend` and `perf_compare` give AI agents the
same data.

## Ideas that work well

- **Leak check:** repeat an action inside one window (`repeat 10 times` around quick add, open history, switch language) and assert
  `see memory growth below 15 MB`.
- **Scroll budget:** measure a long list scrolled to the end and assert `see slow frames below 10 percent`.
- **Chatty screens:** measure a screen load and assert `see exactly 3 requests` and `see data transferred below 200 KB`.
- **Slow backend:** mock a 3-second answer (`respond with 200 ... after 3 seconds`) and assert the app stays smooth meanwhile.

See also: [Testing against backend data](/advanced/backend-data/) and [Timeouts, waiting and retries](/advanced/timeouts-and-retries/).
