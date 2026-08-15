#!/usr/bin/env bash
set -euo pipefail

# run-comparison.sh — head-to-head wall-clock and flake-rate benchmark
# between a probe suite and a Maestro suite covering the same app flows.
#
# Part of the FlutterProbe/Maestro competitive roadmap
# (docs/roadmap/maestro-competitive-roadmap.md, B-2/B-3/G-1). Reusable
# across comparison apps — not specific to any one target repo.
#
# Usage:
#   scripts/bench/run-comparison.sh \
#     --probe-bin /path/to/probe \
#     --probe-config /path/to/probe.yaml \
#     --probe-target tests/smoke.probe \
#     --maestro-config /path/to/.maestro/config.yaml \
#     --maestro-target .maestro/flows/smoke \
#     --device emulator-5554 \
#     --runs 10 \
#     --out docs/evidence/benchmark-baseline-2026-08-14
#
# Requires: the `probe` binary, the `maestro` CLI, `python3` (for JUnit
# XML parsing and the summary table — no extra pip packages needed, only
# the standard library).
#
# What it does NOT do: boot devices, install apps, or manage app state
# between runs — point it at an already-running device with both apps
# already installed. Each probe run passes --config/--device/-y; each
# Maestro run passes --device and -e APP_ID from --maestro-app-id.

usage() {
  cat <<'EOF'
Usage: run-comparison.sh --probe-bin PATH --probe-config PATH --probe-target PATH \
         --maestro-target PATH --maestro-app-id ID --device DEVICE_ID \
         [--runs N] [--out DIR]

Required:
  --probe-bin PATH        Path to the probe CLI binary
  --probe-config PATH     Path to probe.yaml for the target app
  --probe-target PATH     .probe test file or directory to run
  --maestro-target PATH   Maestro flow file or directory to run
  --maestro-app-id ID     App ID to pass as -e APP_ID to maestro test
  --device DEVICE_ID      Device serial/UDID both tools will run against

Optional:
  --runs N                Number of repetitions per tool (default: 10)
  --out DIR                Output directory for raw logs + summary (default:
                           /tmp/probe-maestro-bench-<timestamp>)
EOF
}

RUNS=10
OUT_DIR=""
PROBE_BIN=""
PROBE_CONFIG=""
PROBE_TARGET=""
MAESTRO_TARGET=""
MAESTRO_APP_ID=""
DEVICE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --probe-bin) PROBE_BIN="$2"; shift 2 ;;
    --probe-config) PROBE_CONFIG="$2"; shift 2 ;;
    --probe-target) PROBE_TARGET="$2"; shift 2 ;;
    --maestro-target) MAESTRO_TARGET="$2"; shift 2 ;;
    --maestro-app-id) MAESTRO_APP_ID="$2"; shift 2 ;;
    --device) DEVICE="$2"; shift 2 ;;
    --runs) RUNS="$2"; shift 2 ;;
    --out) OUT_DIR="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

for required in PROBE_BIN PROBE_CONFIG PROBE_TARGET MAESTRO_TARGET MAESTRO_APP_ID DEVICE; do
  if [[ -z "${!required}" ]]; then
    echo "Missing required argument: --${required,,}" | tr '_' '-' >&2
    usage
    exit 1
  fi
done

if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="/tmp/probe-maestro-bench-$(date +%s 2>/dev/null || echo run)"
fi
mkdir -p "$OUT_DIR/probe" "$OUT_DIR/maestro"

echo "== FlutterProbe vs Maestro comparison =="
echo "Device:          $DEVICE"
echo "Runs per tool:   $RUNS"
echo "Probe target:    $PROBE_TARGET"
echo "Maestro target:  $MAESTRO_TARGET"
echo "Output dir:      $OUT_DIR"
echo

echo "--- Running probe suite ($RUNS runs) ---"
for i in $(seq 1 "$RUNS"); do
  echo "  probe run $i/$RUNS..."
  start_ns=$(date +%s%N 2>/dev/null || echo 0)
  set +e
  "$PROBE_BIN" test "$PROBE_TARGET" --config "$PROBE_CONFIG" --device "$DEVICE" \
    --format json -o "$OUT_DIR/probe/run-$i.json" -y >"$OUT_DIR/probe/run-$i.log" 2>&1
  probe_exit=$?
  set -e
  end_ns=$(date +%s%N 2>/dev/null || echo 0)
  elapsed_ms=$(( (end_ns - start_ns) / 1000000 ))
  echo "$elapsed_ms" > "$OUT_DIR/probe/run-$i.wallclock_ms"
  echo "$probe_exit" > "$OUT_DIR/probe/run-$i.exit_code"
done

echo
echo "--- Running Maestro suite ($RUNS runs) ---"
for i in $(seq 1 "$RUNS"); do
  echo "  maestro run $i/$RUNS..."
  start_ns=$(date +%s%N 2>/dev/null || echo 0)
  set +e
  maestro test --device "$DEVICE" -e "APP_ID=$MAESTRO_APP_ID" --format JUNIT \
    --output "$OUT_DIR/maestro/run-$i.xml" "$MAESTRO_TARGET" >"$OUT_DIR/maestro/run-$i.log" 2>&1
  maestro_exit=$?
  set -e
  end_ns=$(date +%s%N 2>/dev/null || echo 0)
  elapsed_ms=$(( (end_ns - start_ns) / 1000000 ))
  echo "$elapsed_ms" > "$OUT_DIR/maestro/run-$i.wallclock_ms"
  echo "$maestro_exit" > "$OUT_DIR/maestro/run-$i.exit_code"
done

echo
echo "--- Summary ---"
python3 "$(dirname "$0")/summarize.py" "$OUT_DIR" "$RUNS"
