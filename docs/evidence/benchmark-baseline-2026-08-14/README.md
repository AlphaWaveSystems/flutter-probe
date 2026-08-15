# B-3 — baseline benchmark: FlutterProbe vs Maestro on water-sip

10 runs per tool, `scripts/bench/run-comparison.sh`, same booted Android emulator
(`emulator-5554`), same 9-flow suite (water-sip's `tests/smoke.probe` vs the new
`.maestro/flows/smoke/` suite from B-1). Probe ran first (all 10 runs), then Maestro (all 10 runs)
against the same still-running emulator. Raw per-run logs, JSON reports, and JUnit XML are all
included in `probe/` and `maestro/` alongside this file.

## FlutterProbe: 10/10 clean

| Run | Wall-clock | Result |
|---|---|---|
| 1 | 60.8s | 9/9 passed |
| 2 | 59.7s | 9/9 passed |
| 3 | 59.3s | 9/9 passed |
| 4 | 59.6s | 9/9 passed |
| 5 | 59.6s | 9/9 passed |
| 6 | 59.8s | 9/9 passed |
| 7 | 59.6s | 9/9 passed |
| 8 | 59.8s | 9/9 passed |
| 9 | 59.6s | 9/9 passed |
| 10 | 59.4s | 9/9 passed |

**Median 59.6s, P90 59.9s, 0% flake rate.** Tight variance (59.3–60.8s across all 10) — no
degradation over the full 10-run session.

## Maestro: 0/10 fully clean

| Run | Wall-clock | Result |
|---|---|---|
| 1 | 120.7s | 8/9 passed — `undo-last-entry` failed (known, documented finding, see B-1 evidence) |
| 2 | 122.3s | 8/9 passed — `undo-last-entry` failed |
| 3 | 123.5s | 8/9 passed — `undo-last-entry` failed |
| 4 | 123.3s | 8/9 passed — `undo-last-entry` failed |
| 5 | 122.7s | 8/9 passed — `undo-last-entry` failed |
| 6 | 124.4s | 8/9 passed — `undo-last-entry` failed |
| 7 | 109.4s | 6/9 passed — `undo-last-entry`, `navigate-settings-tab`, `quick-add-250` all failed |
| 8 | 9.8s | 0/9 ran — `Device emulator-5554 was requested, but it is not connected` |
| 9 | 6.6s | 0/9 ran — device still not connected |
| 10 | 6.3s | 0/9 ran — device still not connected |

**Two distinct findings, not one:**

1. **Runs 1–6 (the "clean" 60%): exactly the one known flake.** `undo-last-entry`'s failure is
   the same narrow-window snackbar-timing issue documented in `docs/evidence/b1-flow-mapping-2026-08-14/`
   — Maestro's out-of-process accessibility polling misses a UI element FlutterProbe's in-process
   access catches reliably. Consistent, understood, not new information from this run.

2. **Runs 7–10: the emulator's ADB connection degraded and then died outright, and never
   recovered.** Run 7's log shows `java.io.IOException: ... device offline` thrown from *Maestro's
   own* post-test driver-uninstall cleanup step (`AndroidDriver.uninstallMaestroDriverApp`), and
   two additional, previously-never-failing flows (`navigate-settings-tab`, `quick-add-250`) failed
   in that same run — the first sign of instability spreading beyond the one known flake. By run 8,
   `adb` could no longer see the device at all, and it stayed that way for runs 9–10 *and remains
   offline right now, after the whole benchmark session ended* (`adb devices -l` returns nothing;
   confirmed independently of the benchmark script itself).

## Reading this honestly — what this does and doesn't prove

**Solid:** FlutterProbe completed all 10 runs of a real, live-verified suite in a tight ~59.6s
band with zero failures. That's real, repeated, first-hand data, not a one-off.

**Real but needs a caveat:** Maestro never completed a fully clean run across all 10 attempts on
this suite in this session, and by the second half of the session couldn't connect to the device
at all. This is genuine, observed behavior — not fabricated or cherry-picked — but this run alone
can't cleanly separate "Maestro's own device/driver management is less stable under sustained
back-to-back runs" from "this specific emulator was already trending toward instability for
unrelated reasons, and Maestro's run just happened to be the one running when it gave out."
FlutterProbe's 10 runs happened first, back-to-back, on the identical emulator instance with no
issues — which weighs against "the emulator was already dying" as the full explanation, but
proper attribution needs a second run with the order reversed (Maestro first) before this becomes
a citable general claim rather than "here's exactly what happened once."

**Not claimed:** a general "Maestro crashes emulators" statement. What's claimed is narrower and
fully evidenced above: in this specific 20-run session, probe had zero connectivity issues across
its 10 runs, and Maestro's device connection degraded partway through its 10 runs and never
recovered, with driver-uninstall-time `IOException`s in Maestro's own log at the point of onset.

## Numbers for the roadmap / comparison pages

Citable now: **FlutterProbe completed 10/10 real end-to-end runs of this suite with a 59.6s
median and 0% flake rate.** Maestro's comparable number, restricted to its runs that actually
executed the suite (1–7), is a 122.6s median with a 100% flake rate (all 7 had at least one
failure) — roughly **2.1x slower on median wall-clock**, with materially different reliability.
The device-connectivity finding (runs 8–10) should be reproduced with reversed run order before
it's cited publicly as a Maestro-specific stability claim; flag it as a replication candidate for
G-1's public benchmark rather than a settled number.
