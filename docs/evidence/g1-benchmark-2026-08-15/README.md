# G-1 evidence — the public benchmark, re-run with current data + reversed-order replication

Two full `scripts/bench/run-comparison.sh` runs (N=10 per tool each — 40 total end-to-end test
suite executions), against water-sip's 9-flow suite (`tests/smoke.probe` vs
`.maestro/flows/smoke/`), same emulator (`emulator-5554`), same current probe build (`main` as of
this write-up — includes every Phase 1/2 feature shipped this session: E-1 through N-2). The
second run's whole purpose is B-3's own stated next step: replicate its device-connectivity
finding with the run order reversed, since a single probe-first run can't distinguish "Maestro's
own driver/device management is less stable under sustained runs" from "the emulator happened to
be the one dying regardless of which tool was running."

## Run 1 — probe-first (matches B-3's original order)

```
scripts/bench/run-comparison.sh --probe-bin <current probe> --probe-config water-sip/probe.yaml \
  --probe-target water-sip/tests/smoke.probe --maestro-target water-sip/.maestro/flows/smoke \
  --maestro-app-id com.alphawavesystems.watersip --device emulator-5554 --runs 10 \
  --order probe-first --android-adb-port 48686
```

| Tool | Runs | Median | P90 | Flake rate |
|---|---|---|---|---|
| FlutterProbe | 10 | 58.3s | 58.6s | 0% |
| Maestro | 10 | 123.0s | 123.6s | 100% |

FlutterProbe: 10/10 clean, 9/9 passed every run, tight 58.1–58.6s band. Maestro: 10/10 runs each
failed **the exact same one flow** — `undo-last-entry` — the same documented snackbar-timing flake
from B-1/B-3 (Maestro's out-of-process accessibility polling misses a UI element FlutterProbe's
in-process access catches reliably). No new failures, no device-connectivity issues across all 10
runs. **~2.11x slower on median wall-clock** (123.0s / 58.3s).

Raw data: `probe-first/probe/*.wallclock_ms` + `*.json`, `probe-first/maestro/*.wallclock_ms` +
`*.xml`.

## Run 2 — maestro-first (the reversed-order replication)

```
scripts/bench/run-comparison.sh --probe-bin <current probe> --probe-config water-sip/probe.yaml \
  --probe-target water-sip/tests/smoke.probe --maestro-target water-sip/.maestro/flows/smoke \
  --maestro-app-id com.alphawavesystems.watersip --device emulator-5554 --runs 10 \
  --order maestro-first --android-adb-port 48686
```

| Tool | Runs | Median | P90 | Flake rate |
|---|---|---|---|---|
| Maestro | 10 | 123.8s | 124.4s | 100% |
| FlutterProbe | 10 | 61.7s | 62.3s | 0% |

Same picture, order reversed: Maestro (now running **first**, fresh device) still fails
`undo-last-entry` on all 10 runs — nothing else, no new failures. FlutterProbe (now running
**second**, after 10 back-to-back Maestro runs on the same device) still passes 9/9 on all 10
runs — 0% flake, wallclock 58.1–65.3s (slightly wider band than run 1's, but no failures and no
connectivity issues). **~2.01x slower on median wall-clock** (123.8s / 61.7s).

Raw data: `maestro-first/probe/*.wallclock_ms` + `*.json`, `maestro-first/maestro/*.wallclock_ms`
+ `*.xml`.

## Reading this honestly

**B-3's device-connectivity finding does not replicate.** Across both orderings — 40 total suite
executions, ~70 minutes of sustained back-to-back runs on the same emulator instance — neither
tool showed the `adb devices` connectivity degradation B-3 observed starting at its run 7 (the
`AndroidDriver.uninstallMaestroDriverApp` `IOException: device offline`, followed by the device
staying unreachable for the rest of that session). Maestro ran first in one session and second in
the other; FlutterProbe likewise ran both positions. Device connectivity held in all four
tool/position combinations.

The most likely explanation, in hindsight: B-3's degradation was a one-off property of that
specific emulator instance/session, not a deterministic effect of which tool ran second or a
general Maestro-specific stability claim. This session's own harness development (see this
evidence folder's parent commit) independently ran into the same class of transient `adb`/agent
connectivity flake multiple times while debugging **this very re-run** — always recoverable with a
fresh `adb forward` and a relaunch, never fatal — which is consistent with "this emulator toolchain
has occasional connectivity hiccups under load, encountered by both tools, not just Maestro."

**What's now citable with confidence:**
- **FlutterProbe completed 20/20 real end-to-end runs of this suite across two independent
  sessions with 0% flake rate**, medians of 58.3s and 61.7s depending on position in the sequence.
- **Maestro completed 20/20 runs with exactly the same one documented flake in every single run**
  (the `undo-last-entry` snackbar-timing issue) — 100% flake rate on this specific suite, but a
  narrow, consistent, understood cause, not general instability.
- **The wall-clock gap is real and consistent across both orderings: ~2.0–2.1x slower for
  Maestro on median wall-clock**, not sensitive to run order.
- **The device-connectivity claim from B-3 should NOT be cited as a general Maestro stability
  finding** — it did not reproduce under a direct replication attempt with reversed order, and is
  better understood as a one-off from that specific session.

## Numbers for the blog post / comparison pages

- FlutterProbe: 0% flake across 20 runs, ~58–62s median depending on position, consistently faster
  than Maestro regardless of run order.
- Maestro: 100% flake across 20 runs on this suite (same single known flow every time), ~123–124s
  median.
- **~2.0–2.1x wall-clock speed advantage for FlutterProbe, reproduced across two independently
  ordered sessions.**
- The B-3 device-connectivity finding is retired as a citable claim — replicated and did not
  reproduce.
