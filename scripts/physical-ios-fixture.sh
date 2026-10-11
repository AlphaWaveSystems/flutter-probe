#!/usr/bin/env bash
# Prints the exact steps to run the Flutter fixture app on a PHYSICAL iPhone and connect FlutterProbe to it over WiFi,
# from the CLI and from Probe Studio. It changes nothing: it only inspects what is connected (and, with --check,
# probes the phone's address) and prints commands for you to run. Signing the app is your step (Apple developer team).
#
# Usage: scripts/physical-ios-fixture.sh [--device <flutter-device-id>] [--ip <phone-ip>] [--port 48686] [--check]
#   --device   the id `flutter devices` shows for the phone (default: the only connected iPhone)
#   --ip       the phone's IP address on your network (Settings > Wi-Fi > the network's (i) button)
#   --port     the agent port (default 48686)
#   --check    after the app is running, test reachability: ping, the agent's status endpoint, the Bonjour service
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_DIR="$ROOT/native-test-apps/flutter"
DEVICE=""; IP=""; PORT=48686; CHECK=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --device) DEVICE="${2:-}"; shift 2 ;;
    --ip) IP="${2:-}"; shift 2 ;;
    --port) PORT="${2:-}"; shift 2 ;;
    --check) CHECK=1; shift ;;
    -h|--help) sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1 (try --help)" >&2; exit 2 ;;
  esac
done
if ! [[ "$PORT" =~ ^[0-9]+$ ]] || (( PORT < 1 || PORT > 65535 )); then echo "--port must be 1 to 65535" >&2; exit 2; fi
if [[ -n "$IP" ]] && ! [[ "$IP" =~ ^[0-9A-Fa-f:.]+$|^[A-Za-z0-9.-]+$ ]]; then echo "--ip must be an address or a hostname" >&2; exit 2; fi

ok()   { printf '  [ok]      %s\n' "$*"; }
miss() { printf '  [missing] %s\n' "$*"; MISSING=1; }
MISSING=0

echo "== Physical iPhone: Flutter fixture, WiFi, FlutterProbe"
echo
echo "Prerequisites"
command -v flutter >/dev/null && ok "flutter ($(flutter --version 2>/dev/null | head -1))" || miss "flutter (https://docs.flutter.dev/get-started/install)"
command -v xcrun >/dev/null && xcrun --find devicectl >/dev/null 2>&1 && ok "xcrun devicectl (Xcode 15 or later)" || miss "Xcode with devicectl (install Xcode 15+, then: sudo xcode-select -s /Applications/Xcode.app)"
command -v probe >/dev/null && ok "probe ($(probe version 2>/dev/null | head -1); 0.24 or later is recommended: brew upgrade probe)" || miss "probe CLI (brew install AlphaWaveSystems/tap/probe)"
command -v python3 >/dev/null || miss "python3 (only used to read device lists)"
[[ -f "$APP_DIR/pubspec.yaml" ]] && ok "fixture app: native-test-apps/flutter" || miss "fixture app not found at $APP_DIR"

BUNDLE_ID="$(grep -m1 'PRODUCT_BUNDLE_IDENTIFIER = ' "$APP_DIR/ios/Runner.xcodeproj/project.pbxproj" 2>/dev/null | sed 's/.*= \(.*\);/\1/')"

echo
echo "Connected iPhones"
FLUTTER_IDS=""
if command -v flutter >/dev/null && command -v python3 >/dev/null; then
  FLUTTER_IDS="$(flutter devices --machine 2>/dev/null | python3 -c '
import json,sys
try: d=json.load(sys.stdin)
except Exception: d=[]
for x in d:
    if x.get("targetPlatform")=="ios" and not x.get("emulator"):
        print("%s\t%s" % (x.get("id"), x.get("name")))
' 2>/dev/null)"
fi
if [[ -z "$FLUTTER_IDS" ]]; then
  echo "  none found by flutter. Connect the phone (cable once, to trust the computer), unlock it, turn on"
  echo "  Developer Mode (Settings > Privacy & Security > Developer Mode), then run: flutter devices"
else
  while IFS=$'\t' read -r id name; do echo "  $id   $name"; done <<<"$FLUTTER_IDS"
  if [[ -z "$DEVICE" ]] && [[ "$(wc -l <<<"$FLUTTER_IDS" | tr -d ' ')" == "1" ]]; then DEVICE="$(cut -f1 <<<"$FLUTTER_IDS")"; fi
fi
DEV="${DEVICE:-<flutter-device-id>}"
HOST="${IP:-<phone-ip>}"

cat <<EOS

Steps (run them in this order)

1. Signing (your step, once). Open the project and choose your Apple developer team:
     open "$APP_DIR/ios/Runner.xcworkspace"
   Runner target > Signing & Capabilities > Team. If the bundle id ${BUNDLE_ID:-com.example.probeFlutterFixture} is taken
   for your team, change it there. On the phone, the first launch asks you to trust the developer:
   Settings > General > VPN & Device Management.

2. Build, install and start the fixture in PROFILE mode with the agent listening on the network:
     cd "$APP_DIR"
     flutter run --profile -d $DEV \\
       --dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true
   Keep this terminal open and the phone unlocked with the app in the foreground (iOS suspends background apps and
   the agent stops listening). When iOS asks "Allow ... to find and connect to devices on your local network", choose
   Allow: that prompt is for the Bonjour advertisement.
   The console prints:   PROBE_TOKEN=<token>   (again every 3 seconds). Copy that value, without "PROBE_TOKEN=".
   It also prints PROBE_MDNS=off only if the advertiser did not start.

3. Check that the Mac can reach the agent (add --check to this script to run these for you):
     ping -c 1 $HOST
     curl -s http://$HOST:$PORT/probe/status        # expect {"agent":"flutter_probe",...}
     dns-sd -B _flutterprobe._tcp local.             # the phone should appear (stop with Ctrl-C)

4a. Connect from the CLI and run the fixture's tests:
     probe test native-test-apps/flutter/probe-tests/flutter_suite.probe --host $HOST --agent-port $PORT --token <token>

4b. Connect from Probe Studio: open Studio, click the antenna (WiFi) button. If the phone is listed, pick it and paste the
   token. If not: "Connect manually", host $HOST, port $PORT, token <token>, "Test connection", then "Connect".
   Studio only dials addresses on your local network (private ranges, link-local, .local names).

If something fails, the order of suspects is: not a profile build or no PROBE_WIFI=true (nothing listens) > the app is not in
the foreground > the Mac and the phone are not on the same network (a guest or IoT network, client isolation, a VPN) >
a new token after an app restart. Full list: https://flutterprobe.dev/platform/ios/ ("Physical iPhone over WiFi").
EOS

if (( CHECK )); then
  echo
  echo "Checks against ${IP:-<no --ip given>}"
  if [[ -z "$IP" ]]; then echo "  give --ip <phone-ip> to run the checks"; exit 2; fi
  FAIL=0
  if ping -c 1 -t 3 "$IP" >/dev/null 2>&1; then ok "ping $IP answers"; else printf '  [fail]    ping %s: no answer (different network, or the phone is asleep)\n' "$IP"; FAIL=1; fi
  BODY="$(curl -s --max-time 5 "http://$IP:$PORT/probe/status" 2>/dev/null || true)"
  if [[ "$BODY" == *'"agent":"flutter_probe"'* ]]; then ok "the agent answers on $IP:$PORT ($BODY)"; else printf '  [fail]    nothing answers on %s:%s: no agent (not a profile build with PROBE_AGENT/PROBE_WIFI, not running, or the app is in the background)\n' "$IP" "$PORT"; FAIL=1; fi
  if command -v dns-sd >/dev/null; then
    SEEN="$(perl -e 'alarm 4; exec @ARGV' dns-sd -B _flutterprobe._tcp local. 2>/dev/null | grep -c 'flutterprobe' || true)"
    if [[ "${SEEN:-0}" -gt 0 ]]; then ok "the Bonjour service _flutterprobe._tcp is advertised"; else printf '  [info]    no _flutterprobe._tcp advertisement seen in 4 s: Studio will not list the phone, use "Connect manually"\n'; fi
  fi
  exit $FAIL
fi
exit $MISSING
