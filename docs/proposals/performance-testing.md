# Performance testing and one UI (proposal, 2026-10-10)

Goal: measure CPU, memory, frames and network while tests run; assert, gate on baselines, show trends; and bring runs, devices,
performance, backend traffic and languages together in one UI (Probe Studio).

## Status

- Agent (0.23): `probe.perf_start` / `perf_snapshot` / `perf_stop` (frame timings via the engine's timings callback, resident memory
  sampled in-process). Implemented.
- CLI: `start measuring` / `stop measuring`, `see <metric> below N`, response time check; CPU sampler (Android /proc, iOS simulator
  host ps); `--perf-baseline` / `--perf-update-baseline` / `--perf-tolerance`, `reports/perf-history.jsonl`, `probe perf trend|compare`.
  Implemented, verified on an iOS simulator and an Android 14 emulator.
- MCP: `perf_trend`, `perf_compare`, guide section; `get_report` carries `perf`. Implemented.
- Studio: perf lines under each result. First step done.

## Not yet / ideas

- Physical iPhone CPU (needs `xctrace`/Instruments), GPU, battery, startup time (cold/warm launch), app size.
- Android `dumpsys gfxinfo` jank as a second opinion, `dumpsys meminfo` PSS alongside RSS.
- Percentile budgets per screen from `@perf` annotations.

## Studio as the one UI (roadmap)

Studio already has editor, device picker, live stream, widget tree and a results timeline (Wails, Go in-process runner). Planned panels,
each backed by an existing CLI/runner capability so Studio never grows logic of its own:

1. **Performance** - live gauges while a window is open (agent `perf_snapshot`), result charts and trend from `perf-history.jsonl`,
   baseline accept/reject.
2. **Backend** - the recorded HTTP exchanges (`probe.http_log`), mock editor, "wait for response" step builder.
3. **Languages** - locale switcher (`set language`), ARB key browser, per-locale run matrix.
4. **Runs** - history of reports, retries/attempts, device and language per result.
5. Housekeeping from the earlier Studio survey: CI build, keyword-drift test against the grammar page, run options (implicit wait,
   retry, loose matching, locale), version display.

Open question for the owner: which panel first.
