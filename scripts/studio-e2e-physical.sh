#!/usr/bin/env bash
# Opt-in Studio E2E against a PHYSICAL phone (Probe Studio driven in a browser, no native window). It never runs in CI.
# Prepare the phone first: scripts/physical-ios-fixture.sh --ip <phone-ip>  (prints the steps), and keep the fixture app
# in the foreground with the screen unlocked. Checklist and expected results: studio/e2e-web/PHYSICAL.md
#
# Usage: scripts/studio-e2e-physical.sh --host <phone-ip> --token <PROBE_TOKEN value> [--port 48686]
#   (or STUDIO_E2E_PHONE_HOST / STUDIO_E2E_PHONE_TOKEN / STUDIO_E2E_PHONE_PORT in the environment)
# The token is passed to the test through the environment of this process only and is never printed.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [[ -n "${CI:-}" ]]; then echo "studio-e2e-physical: skipped (never runs in CI)"; exit 0; fi

HOST="${STUDIO_E2E_PHONE_HOST:-}"; TOKEN="${STUDIO_E2E_PHONE_TOKEN:-}"; PORT="${STUDIO_E2E_PHONE_PORT:-48686}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --host) HOST="${2:-}"; shift 2 ;;
    --token) TOKEN="${2:-}"; shift 2 ;;
    --port) PORT="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1 (try --help)" >&2; exit 2 ;;
  esac
done
if [[ -z "$HOST" || -z "$TOKEN" ]]; then
  echo "need the phone's address and token: --host <phone-ip> --token <value after PROBE_TOKEN=>" >&2
  echo "(steps to get them: scripts/physical-ios-fixture.sh --ip <phone-ip>)" >&2
  exit 2
fi

# A quick reachability check first, so a wrong address fails in seconds with a clear message.
if ! curl -s --max-time 5 "http://$HOST:$PORT/probe/status" | grep -q '"agent":"flutter_probe"'; then
  echo "nothing answers as a FlutterProbe agent on $HOST:$PORT. Check: profile build with PROBE_AGENT=true and PROBE_WIFI=true," >&2
  echo "app in the foreground with the screen unlocked, phone and this computer on the same network." >&2
  echo "Run: scripts/physical-ios-fixture.sh --ip $HOST --port $PORT --check" >&2
  exit 1
fi

export STUDIO_E2E_PHYSICAL=1 STUDIO_E2E_PHONE_HOST="$HOST" STUDIO_E2E_PHONE_TOKEN="$TOKEN" STUDIO_E2E_PHONE_PORT="$PORT"
export STUDIO_E2E_NO_DEVICE=1 # no simulator is needed: the phone is the device
exec "$ROOT/scripts/studio-e2e-web.sh" -g "physical phone"
