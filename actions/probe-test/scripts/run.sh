#!/usr/bin/env bash
# Runs `probe test` and records the exit code instead of failing at once, so that the reports can be uploaded first.
# Inputs (environment): PROBE_TESTS, PROBE_DEVICE, PROBE_JUNIT, PROBE_TIMEOUT, PROBE_UPLOAD (true|false), PROBE_EXTRA_ARGS.
set -uo pipefail

args=(test)
# shellcheck disable=SC2206
tests=(${PROBE_TESTS:-tests/})
args+=("${tests[@]}")
[ -n "${PROBE_DEVICE:-}" ] && args+=(--device "$PROBE_DEVICE")
[ -n "${PROBE_TIMEOUT:-}" ] && args+=(--timeout "$PROBE_TIMEOUT")
if [ -n "${PROBE_JUNIT:-}" ]; then
  mkdir -p "$(dirname "$PROBE_JUNIT")"
  args+=(--format junit -o "$PROBE_JUNIT")
fi
[ "${PROBE_UPLOAD:-false}" = "true" ] && args+=(--upload-report)
args+=(-y)
# shellcheck disable=SC2206
[ -n "${PROBE_EXTRA_ARGS:-}" ] && args+=(${PROBE_EXTRA_ARGS})

echo "probe ${args[*]}"
probe "${args[@]}"
code=$?
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "exit-code=$code" >> "$GITHUB_OUTPUT"
  echo "junit-path=${PROBE_JUNIT:-}" >> "$GITHUB_OUTPUT"
fi
# The action fails in its last step, after the reports are uploaded.
exit 0
