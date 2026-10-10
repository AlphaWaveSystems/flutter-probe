#!/usr/bin/env bash
# Builds Studio and the fixture app, then runs the Studio E2E suite.
# Usage: scripts/studio-e2e.sh [go test args...]
#   SKIP_BUILD=1       reuse existing Studio + fixture builds
#   STUDIO_E2E_*       see studio/e2e/studio_test.go (workspace, files, device)
#   STUDIO_E2E_LOCAL=path/to/dogfood.local.yaml  export its keys as STUDIO_E2E_* (see studio/e2e/DOGFOOD.md)
#   STUDIO_E2E_DISPLAY=n  open Studio/Simulator windows on display n (1 = main, default)
set -euo pipefail
export STUDIO_E2E_DISPLAY="${STUDIO_E2E_DISPLAY:-1}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ "$(uname)" != "Darwin" ]]; then
  echo "studio-e2e: macOS only (drives the real Studio window)" >&2
  exit 2
fi

if [[ -n "${STUDIO_E2E_LOCAL:-}" ]]; then
  # Flat key: value YAML → STUDIO_E2E_<KEY> environment.
  while IFS=: read -r k v; do
    [[ -z "$k" || "$k" =~ ^# ]] && continue
    v="${v#"${v%%[![:space:]]*}"}"; v="${v%\"}"; v="${v#\"}"
    export "STUDIO_E2E_$(echo "$k" | tr '[:lower:]' '[:upper:]')=$v"
  done < "$STUDIO_E2E_LOCAL"
fi

if [[ -z "${SKIP_BUILD:-}" ]]; then
  echo "== building Studio"
  (cd studio/frontend && npm install --silent)
  (cd studio && wails build >/dev/null)
  if [[ -z "${STUDIO_E2E_APP_BUNDLE:-}" ]]; then
    echo "== building fixture app (iOS simulator, agent enabled)"
    (cd native-test-apps/studio-fixture && flutter pub get >/dev/null && \
      flutter build ios --simulator --debug --dart-define=PROBE_AGENT=true >/dev/null)
  fi
fi

mkdir -p reports/studio-e2e
echo "== running suite"
(cd studio && go test -tags studio_e2e ./e2e/ -count=1 -v -timeout 45m "$@") 2>&1 | tee reports/studio-e2e/go-test.log
echo "report: reports/studio-e2e/report.html"
