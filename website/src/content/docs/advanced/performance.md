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

## Measure every test without editing them

```bash
probe test tests/ --measure-all --perf-baseline perf-baseline.json --perf-update-baseline
```

`--measure-all` (or `perf: {measure_all: true}` in `probe.yaml`) opens a measuring window named after the test around each test body, so an existing suite gets baselines with no script edits. A test that opens its own `start measuring` window is left alone, so you can mix both: measure everything by default and add precise windows where it matters. Measurements are keyed `file::test::test`. Memory and frames need agent 0.23+; on an older agent the run continues with a warning and no numbers.

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

Every measurement is stored under `file::test::name`, with the file relative to the current directory (or to `--perf-key-base DIR`), so the same tests give the same keys from another checkout. Baselines written by earlier versions (the path as passed) are still found. A measurement fails its test when a number exceeds the baseline by more than
the tolerance (default 20%) **and** by more than a noise floor (5 MB memory, 5 points CPU, 2 points slow frames, 3 ms frame time,
20 KB data), so a big percentage of a tiny number does not break the build. When the baseline is zero or negative (memory growth can be) only the absolute change against the floor counts, and the message shows the real delta. A measurement without a baseline entry is new and passes; the run prints how many there were, so keys that did not match (another directory) do not go unnoticed.
`--retry-failed N` re-runs a test that failed this way, which smooths a noisy device.

### Which metrics are safe to gate on

Two runs of the same build differ a lot in single-sample numbers. By default a comparison fails only on the stable ones:

| Gated by default | Why |
|---|---|
| `memory` (peak RSS) | stable between runs of one build |
| `slow_frames`, `frame_time` (p95) | percentages and percentiles over many frames |
| `data` | deterministic for a given flow |
| `cpu` (average) | only for windows of 5 seconds or more, over a shorter window a few busy milliseconds swing the average by tens of points |

These are recorded and shown, but **not** gated unless you ask: `memory_growth` (end minus start of a noisy RSS, 3 to 50 MB between identical runs), `frame_max` (one slowest frame) and `cpu_peak` (the busiest second). Choose the set with `--perf-metrics memory,cpu,frame_max` (or `all`) and drop some with `--perf-ignore cpu`; `probe perf compare --metrics` takes the same list. Debug builds and simulators are noisier than profile builds on a device: prefer a wider `--perf-tolerance` there.

**On emulators and simulators the default gate is `memory`, `data` and `cpu` (windows of 5 seconds or more); `frame_time` and `slow_frames` gate only on physical devices.** Even with the stable default above, the frame metrics (`frame_time`, `slow_frames`) vary a lot between two runs of the same build on an emulator or a simulator (software rendering, shared host, debug builds), and a rerun against a fresh baseline can report regressions that are noise. Name the frame metrics explicitly (`--perf-metrics memory,data,frame_time`) if you want them gated there anyway; otherwise keep them for physical devices in profile mode. A configurable frame budget and a median over several runs are planned.

### Slow-frame budget

A frame is "slow" when its build+raster time exceeds 16.7 ms (one 60 Hz interval). On a 120 Hz display, or in a debug build where every frame is above that, set another budget in `probe.yaml`:

```yaml
perf:
  frame_budget_ms: 8.3   # 120 Hz; use a looser value (for example 33) for debug simulator builds
```

The budget is sent with `probe.perf_start` (agent 0.25 or later; an older agent keeps 16.7 ms and the run prints a note). The report records it as `frame_budget_ms`, and `slow_frames` is not compared between windows that used different budgets.

### Restarts and unavailable numbers

- A window that contains `restart the app` (or any step that relaunches the process, for example `clear app data`) spans two processes; `open the app` is not treated as a restart, it only counts when it actually has to launch the app again. The measuring window is re-opened on the new process, so memory and frames cover the time since the relaunch; the run prints a warning, the report marks the window `window_spans_restart`, CPU is not reported, the comparison skips it (in compare mode the window is reported as skipped) and `--perf-update-baseline` leaves it out (a baseline never gets zeros or numbers of a half-window). Put the window around the steps after the restart if you want it in a baseline.
- A `start measuring` without a name is reported as `measurement`.
- CPU is reported only when the sampler read the app at least twice in the same process. Otherwise it is absent (not `0.0`) and skipped by comparisons.
- `start measuring` while the app is not running fails with "no app is connected ... open the app first".

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

## What the numbers mean (and do not)

- **Simulators and emulators:** an iOS simulator can only build debug, where build plus raster is usually above the 16.7 ms slow-frame threshold, so `slow_frames` sits near 100% in every test and says nothing; `frame_time` (p95) and the slowest frame are usable. An Android emulator that renders in software shows the same in quiet scenes. Numbers from simulators and emulators are only comparable from run to run on the same host; take absolute budgets from a profile build on a real device.
- **Memory is the app process's resident set size.** On an iOS simulator in debug it includes the whole debug engine (several hundred MB), on an Android profile build about half of that. Compare it with a baseline from the same kind of build, not with a budget from another.
- **Network assertions see `dart:io` traffic only** (`see response`, `see no requests`, `see data transferred`, the HTTP log). Requests made by native SDKs (Firebase, Crashlytics, Remote Config, fonts fetched natively) do not appear, so an "offline-first" assertion can only be about the Dart HTTP client.
