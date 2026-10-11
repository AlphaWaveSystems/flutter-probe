#!/usr/bin/env bash
# Browser-mode E2E for Studio: the real frontend served by Studio's headless mode and driven by Playwright.
# No native window, cursor or focus is involved, so it can run next to your own work (and, for the
# device-free half, on any CI runner).
#
# Usage: scripts/studio-e2e-web.sh [playwright args...]
#   STUDIO_E2E_NO_DEVICE=1  only the tests that need no device
#   SKIP_BUILD=1            reuse the Studio binary and the fixture app build
#   STUDIO_E2E_HEADED=1     show the browser (Chromium), not a Studio window
#   STUDIO_E2E_SIM_NAME     simulator to create/use (default StudioE2EWeb-iPhone17); macOS only
#   STUDIO_E2E_AGENT_PORT   agent port of the fixture app and the workspace (default 48690)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
OUT="${STUDIO_E2E_OUT:-$ROOT/reports/studio-e2e-web}"
mkdir -p "$OUT"
FIXTURE="$ROOT/native-test-apps/studio-fixture"
BUNDLE_ID="${STUDIO_E2E_BUNDLE_ID:-com.alphawavesystems.studioFixture}"
SIM_NAME="${STUDIO_E2E_SIM_NAME:-StudioE2EWeb-iPhone17}"
# Simulators share the host network: a dedicated agent port keeps this suite away from any other
# simulator that has a Probe-enabled app running on the default port.
AGENT_PORT="${STUDIO_E2E_AGENT_PORT:-48690}"
export STUDIO_E2E_SIM_NAME="$SIM_NAME" STUDIO_E2E_BUNDLE_ID="$BUNDLE_ID"

if [[ -z "${SKIP_BUILD:-}" ]]; then
  echo "== building Studio (headless)"
  (cd studio/frontend && npm ci --silent --no-audit --no-fund && npm run build --silent)
  (cd studio && go build -o "$OUT/studio-headless" .)
fi
export STUDIO_BIN="$OUT/studio-headless"

# A fresh copy of the fixture workspace: tests may change probe.yaml.
WS="$OUT/workspace"
mkdir -p "$WS/probe-tests"
sed "s/port: 48686/port: $AGENT_PORT/" "$FIXTURE/probe.yaml" > "$WS/probe.yaml"
cp "$FIXTURE"/probe-tests/*.probe "$WS/probe-tests/"
export STUDIO_WORKSPACE="$WS"

if [[ -z "${STUDIO_E2E_NO_DEVICE:-}" && "$(uname)" == "Darwin" ]]; then
  echo "== simulator $SIM_NAME with the fixture app"
  [[ -x bin/probe ]] || go build -o bin/probe ./cmd/probe
  bin/probe device start --platform ios --name "$SIM_NAME" >/dev/null
  UDID="$(xcrun simctl list devices | grep -F "$SIM_NAME (" | grep -o '[0-9A-F]\{8\}-[0-9A-F-]\{27\}' | head -1)"
  APP="$FIXTURE/build/ios/iphonesimulator/Runner.app"
  if [[ -z "${SKIP_BUILD:-}" && -z "${STUDIO_E2E_APP_BUNDLE:-}" ]]; then
    (cd "$FIXTURE" && flutter pub get >/dev/null && flutter build ios --simulator --debug --dart-define=PROBE_AGENT=true --dart-define=PROBE_PORT="$AGENT_PORT" >/dev/null)
  fi
  APP="${STUDIO_E2E_APP_BUNDLE:-$APP}"
  xcrun simctl install "$UDID" "$APP"
  xcrun simctl launch "$UDID" "$BUNDLE_ID" >/dev/null
  sleep 5
  export STUDIO_E2E_UDID="$UDID"
fi

cd studio/e2e-web
npm ci --silent --no-audit --no-fund
npx playwright install chromium >/dev/null
echo "== running"
npx playwright test "$@"
echo "report: $OUT/html/index.html"
