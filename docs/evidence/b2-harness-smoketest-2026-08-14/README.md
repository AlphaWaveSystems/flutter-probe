# B-2 evidence — comparison harness smoke test

Ran `scripts/bench/run-comparison.sh` end-to-end with `--runs 2` against water-sip's new 9-flow
suite (both tools, per B-1's evidence) to prove the harness itself works before committing to a
full N=10 baseline run (B-3).

```
scripts/bench/run-comparison.sh \
  --probe-bin <probe binary> \
  --probe-config water-sip/probe.yaml \
  --probe-target water-sip/tests/smoke.probe \
  --maestro-target water-sip/.maestro/flows/smoke \
  --maestro-app-id com.alphawavesystems.watersip \
  --device emulator-5554 \
  --runs 2
```

```
Tool          Runs    Median    P90       Flake rate
----------    ------  --------  --------  ------------
FlutterProbe  2       60.3s     60.7s     0%
Maestro       2       121.2s    121.9s    100%
```

Sample raw outputs included: `run-1.json` (probe) and `run-1.xml` (Maestro, confirms the
`<testsuites>` wrapper parsing path, not just bare `<testsuite>`).

## Reading these numbers honestly

This is a **harness smoke test, not a benchmark result** — N=2 is too small to draw a real
performance conclusion, and Maestro's 100% flake rate here is entirely the known,
already-documented `undo-last-entry` timing issue (see `docs/evidence/b1-flow-mapping-2026-08-14/`),
not a general Maestro reliability claim. What this run *does* prove: the harness correctly
executes both tools, correctly parses both output formats (including catching the real flake
rather than reporting a false 100% pass), and produces a usable comparison table. A real N=10
baseline is B-3.

The ~2x wall-clock gap (60s vs 121s) is directionally consistent with the competitive roadmap's
speed thesis (in-process element-tree access vs. out-of-process accessibility polling) but should
not be cited as a validated number until B-3's larger sample.
