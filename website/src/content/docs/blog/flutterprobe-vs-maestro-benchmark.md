---
title: "FlutterProbe vs Maestro: A Benchmark, Replicated"
description: "We ran FlutterProbe and Maestro head-to-head, 20 runs each, twice — including a second run with the tool order reversed specifically to test a finding from our own earlier session that we weren't ready to stand behind yet."
---

We benchmarked FlutterProbe against Maestro on the same real Flutter app, the same 9-flow suite,
10 runs per tool. Then we did it again with the run order reversed — not because we doubted the
speed numbers, but because an earlier internal session had turned up a device-connectivity finding
we explicitly flagged as unproven and said we'd replicate before citing it publicly. This is that
replication, and the honest result: **that specific finding didn't hold up, so we're retiring it.**
The speed numbers did hold up, twice.

## Methodology

The target app is [water-sip](https://github.com/AlphaWaveSystems), a real Flutter habit-tracking
app with authentication, local persistence, and a Riverpod state layer — not a synthetic demo. We
wrote a 9-flow suite covering the app's core paths (launch, quick-add at three quantities, undo,
goal completion, settings navigation, unit toggling, history) once in ProbeScript and once as an
equivalent Maestro flow suite, flow-for-flow.

Each run: a booted Android emulator, both tools targeting the same already-installed app, the same
device instance across all 20 runs in a session. Wall-clock time measured from process start to
process exit, using [our benchmark harness script](https://github.com/AlphaWaveSystems/flutter-probe/blob/main/scripts/bench/run-comparison.sh)
— open source, reusable against any app with equivalent probe/Maestro suites, not a one-off script
we ran and threw away.

We ran the full 20-run comparison twice: once with FlutterProbe's 10 runs first, then Maestro's 10
— and once with the order fully reversed. The second run exists for one reason: an earlier internal
benchmark session (documented in our own roadmap and evidence trail) saw the emulator's ADB
connection degrade partway through Maestro's runs, in a session where FlutterProbe had already run
first with zero issues. That's suggestive, but a single ordered run genuinely can't separate "one
tool is less stable under sustained load" from "the device happened to be the one dying regardless
of which tool was running at the time." We said so at the time and flagged it as a replication
candidate rather than a citable claim. This is that replication.

## Results

### Run 1 — FlutterProbe first

| Tool | Runs | Median | P90 | Flake rate |
|---|---|---|---|---|
| FlutterProbe | 10 | 58.3s | 58.6s | 0% |
| Maestro | 10 | 123.0s | 123.6s | 100% |

### Run 2 — Maestro first (reversed order)

| Tool | Runs | Median | P90 | Flake rate |
|---|---|---|---|---|
| Maestro | 10 | 123.8s | 124.4s | 100% |
| FlutterProbe | 10 | 61.7s | 62.3s | 0% |

The speed gap held in both directions: **~2.0–2.1x faster median wall-clock for FlutterProbe**,
whether it ran first or second in the session.

### About that 100% Maestro flake rate

It's one specific, understood cause, not general instability. All 20 Maestro runs — across both
orderings — failed exactly the same flow: `undo-last-entry`, a snackbar-timing race where Maestro's
out-of-process accessibility polling misses a UI element that appears and disappears in a narrow
window. FlutterProbe's in-process access to the widget tree doesn't have this timing gap, which is
exactly why the difference shows up as a flake-rate gap and not just a speed gap. Every other flow
passed, every run, both orderings — the number isn't "Maestro is unreliable," it's "this one
interaction pattern is a real gap for out-of-process accessibility polling," which is worth knowing
regardless of which tool you pick.

### The device-connectivity finding: retired

Across both 20-run sessions — 40 total end-to-end suite executions, roughly 70 minutes of sustained
back-to-back runs on the same emulator — neither tool's device connection degraded, in either
position. Maestro ran first in one session and second in the other. FlutterProbe did the same.
Connectivity held in all four combinations.

We're not going to spin this as a win we almost had. The honest read is that the original finding
was very likely a one-off property of that specific emulator session — normal toolchain flakiness
that could have hit either tool, not a deterministic effect of run order or a real Maestro-specific
stability issue. We're retiring it as a citable claim. The [full evidence, raw per-run
logs, and reasoning are public](https://github.com/AlphaWaveSystems/flutter-probe/tree/main/docs/evidence/g1-benchmark-2026-08-15) —
we'd rather publish a benchmark that walks back its own earlier finding than publish one that
quietly drops the inconvenient part.

## Where each tool stands today

**Speed and reliability on this suite:** FlutterProbe, by a reproducible ~2x margin, with zero
flakes across 20 runs. Maestro's one flake is narrow and understood, not a broad reliability
problem.

**Native UI (pickers, share sheets):** until recently this was a real gap for FlutterProbe — Maestro
has always been able to drive native, non-Flutter UI directly. We closed the Android side of that
gap this cycle (`tap native` / `see native` / `type native`, dispatched via `uiautomator`, no new
dependencies) and have a scoped, evidence-grounded proposal for iOS using WebDriverAgent — the same
mechanism Appium has used for iOS automation for close to a decade. iOS support isn't shipped yet;
Android's gap is closed.

**Language and ecosystem:** Maestro's YAML flows and FlutterProbe's ProbeScript are both
deliberately non-Dart, readable-by-non-developers formats — closer to each other here than either
is to `integration_test` or Patrol. Maestro has a larger existing community and broader
Android/iOS-native ecosystem outside of Flutter specifically; FlutterProbe is Flutter-first and
newer.

## Recommendation

If you're choosing between the two for a Flutter app today: the speed and flake-rate gap in this
benchmark is real and reproducible, and worth weighing seriously for CI-heavy suites where wall-clock
time compounds. If your suite leans heavily on native pickers/share sheets on iOS specifically,
that's still a real, open gap for FlutterProbe as of this writing — track the proposal, or use
Maestro for that slice of coverage in the meantime.

Full raw data, both orderings, every run: [`docs/evidence/g1-benchmark-2026-08-15/`](https://github.com/AlphaWaveSystems/flutter-probe/tree/main/docs/evidence/g1-benchmark-2026-08-15) —
not summarized, not cherry-picked, the same JSON reports and JUnit XML the harness itself produced.
