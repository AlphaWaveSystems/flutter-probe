#!/usr/bin/env bash
# Builds Studio and the fixture app, then runs the Studio native E2E suite (macOS).
# The default set is the smoke set; STUDIO_E2E_SET=full also runs the tests still
# to be ported to browser mode (#331).
# Usage: scripts/studio-e2e.sh [go test args...]
#   SKIP_BUILD=1       reuse existing Studio + fixture builds
#   STUDIO_E2E_*       see studio/e2e/studio_test.go (workspace, files, device)
#   STUDIO_E2E_LOCAL=path/to/dogfood.local.yaml  export its keys as STUDIO_E2E_* (see studio/e2e/DOGFOOD.md)
#   STUDIO_E2E_DISPLAY=n  REQUIRED: the second display (2 or more) the Studio and Simulator
#                         windows open on. Unset, the script refuses; it never runs on the main
#                         display. In CI (CI set) an unset value skips the run with exit 0.
#   STUDIO_E2E_SET=smoke|full  which native tests run (default smoke; see studio/e2e/README.md)
set -euo pipefail
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

# The display rule (studio/e2e/display.go): the suite runs only on a second
# display that STUDIO_E2E_DISPLAY names. Refuse before the build; there is no
# fallback to the main display.
attached_displays() {
  osascript -l JavaScript -e 'ObjC.import("AppKit"); $.NSScreen.screens.count'
}
refuse() {
  echo "studio-e2e: refusing to run: $1" >&2
  exit 2
}
if [[ -z "${STUDIO_E2E_DISPLAY:-}" ]]; then
  if [[ -n "${CI:-}" ]]; then
    echo "studio-e2e: skipped: no second display configured"
    exit 0
  fi
  attached=$(attached_displays 2>/dev/null) || attached="?"
  refuse "STUDIO_E2E_DISPLAY is not set. Name the second display (2 or more); the suite never runs on the main display. $attached display(s) attached."
fi
attached=$(attached_displays 2>/dev/null) || refuse "could not count the attached displays"
case "$STUDIO_E2E_DISPLAY" in
  ''|*[!0-9]*) refuse "STUDIO_E2E_DISPLAY=$STUDIO_E2E_DISPLAY must be a display number of 2 or more (1 is the main display). $attached display(s) attached." ;;
esac
if (( 10#$STUDIO_E2E_DISPLAY < 2 )); then
  refuse "STUDIO_E2E_DISPLAY=$STUDIO_E2E_DISPLAY must be 2 or more (1 is the main display). $attached display(s) attached."
fi
if (( 10#$STUDIO_E2E_DISPLAY > attached )); then
  refuse "STUDIO_E2E_DISPLAY=$STUDIO_E2E_DISPLAY but only $attached display(s) attached."
fi
case "${STUDIO_E2E_SET:-smoke}" in
  smoke|full) ;;
  *) refuse "STUDIO_E2E_SET=$STUDIO_E2E_SET is not smoke or full." ;;
esac

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
echo "== running suite (set: ${STUDIO_E2E_SET:-smoke}, display: ${STUDIO_E2E_DISPLAY})"
(cd studio && go test -tags studio_e2e ./e2e/ -count=1 -v -timeout 45m "$@") 2>&1 | tee reports/studio-e2e/go-test.log
echo "report: reports/studio-e2e/report.html"
