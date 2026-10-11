# Studio and physical devices: plan

Status: plan. Goal: a person with a physical iPhone (or Android phone) can connect Studio to it, run a test and see the result,
with no step that only works on a simulator. Today this is possible only through the CLI (`probe test --host <ip> --token <token>`),
because Studio's WiFi overlay lists only devices that advertise themselves over mDNS and has no manual entry. What must be true on
the device side is written down in the iOS guide ("Physical iPhone over WiFi: what must be true").

## Facts this plan rests on

- The agent listens only in a build with `--dart-define=PROBE_AGENT=true`, and on the network only with `PROBE_WIFI=true`; a release build
  refuses to start it. A production app therefore has nothing to connect to. (`probe_agent/lib/src/agent.dart`)
- The token is printed as `PROBE_TOKEN=` in the app log and is not advertised. (`probe_agent/lib/src/server.dart`)
- Discovery needs the app to pass a `ProbeAdvertiser` to the agent; the core agent has no native dependency. Studio browses
  `_flutterprobe._tcp`. (`probe_agent/lib/src/advertiser.dart`, `studio/wifi_discovery.go`)
- `App.ConnectWiFi(host, port, token)` already exists and needs no discovery; only the overlay lacks a way to enter the three values.
- iOS local network privacy, from Apple's technical note TN3179 (https://developer.apple.com/documentation/technotes/tn3179-understanding-local-network-privacy):
  "Listening for and accepting incoming TCP connections" does not require local network access; outgoing TCP connections, UDP multicast
  and broadcast do. So the agent's listener needs no permission prompt; **Bonjour advertising does** (it sends and receives multicast),
  which is why an advertiser build needs `NSLocalNetworkUsageDescription` and `NSBonjourServices` (`_flutterprobe._tcp`) in its `Info.plist`.
  Verified 2026-10-11.
- The USB path uses `iproxy`, which forwards to "a usbmux device" and is documented as `-l, --local` (USB, default) or `-n, --network`
  ("connect to network device"), i.e. devices that `usbmuxd` knows (`iproxy --help`; https://github.com/libimobiledevice/libusbmuxd/blob/master/tools/iproxy.c).
  It does not talk to CoreDevice, so the CoreDevice tunnel that `xcrun devicectl` uses cannot carry the `iproxy` path. `devicectl` is the way to
  list, install and launch, not to forward ports. Verified 2026-10-11.

## Phases

Each phase is its own pull request, with tests, docs on every surface and a CHANGELOG entry. Test apps are neutral fixtures, never a
real product.

### P1: manual entry in Studio's WiFi overlay (and the MCP path)

- Fields for host, port (default 48686) and token, next to the discovered-device list.
- Validation before anything is dialled: host is an IPv4 address, an IPv6 address or a hostname; port is 1 to 65535; token has the characters
  the agent can produce.
- A connection test (dial and ping) before the entry is kept; error messages say what to check: same network, the app in the foreground
  with the screen unlocked, a profile build with `PROBE_AGENT=true` and `PROBE_WIFI=true`, the port, a stale token after an app restart.
- The automation endpoint and the `studio_connect` MCP tool accept host, port and token, so an agent can drive the same path.
- Tests: Go tests for validation and the connect path, a headless browser test (Playwright) for the overlay, a native E2E addition.

### P2: a physical-device-ready fixture

- The Flutter twin fixture builds in profile mode with `PROBE_AGENT` and `PROBE_WIFI`, passes a `bonsoir`-based `ProbeAdvertiser`, and carries
  the `Info.plist` keys above, only in the fixture.
- `scripts/physical-ios-fixture.sh` prints the exact commands for the owner (`flutter run --profile -d <id>` with the dart-defines; the Apple
  signing team is the owner's step) and then the connect steps (CLI `--host/--token`, Studio overlay).

### P3: the device picker shows physical devices

- Physical iOS devices (via `devicectl`) and physical Android devices (via `adb`) are listed next to emulators and simulators.
- A device whose app is not running with the agent shows a clear "needs the app running with the agent" state with the checklist,
  instead of being absent or failing silently.

### P4: an opt-in physical-device E2E

- A suite gated by an environment variable, skipped in CI, plus a manual checklist with expected results, so a person with a phone can
  verify the whole path in about ten minutes.

## Out of scope

- Shipping the agent in production builds, or any unattended access to a phone.
- Changing the agent protocol: all phases use the existing connection path.
