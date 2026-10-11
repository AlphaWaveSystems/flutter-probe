---
title: iOS
description: iOS simulator setup, token reading, simctl integration, and platform-specific notes.
---

## Requirements

- macOS with Xcode installed
- `xcrun simctl` command-line tools
- Flutter app built for simulator with `--dart-define=PROBE_AGENT=true`
- **Physical devices only:** `iproxy` from `libimobiledevice` for USB port forwarding (see [Physical Device Setup](#physical-device-setup) below)

## Simulator Setup

### List available simulators

```bash
probe device list
xcrun simctl list devices available
```

### Boot a simulator

```bash
probe device start --platform ios
probe device start --platform ios --name my-sim   # named; created if it does not exist
```

Every simulator or emulator probe opens has a name, and every test result records the device it ran on (name and id/serial; physical devices by model or serial).

Or manually:

```bash
xcrun simctl boot <UDID>
```

### Build and install your app

```bash
flutter build ios --debug --simulator --dart-define=PROBE_AGENT=true
xcrun simctl install <UDID> build/ios/iphonesimulator/YourApp.app
xcrun simctl launch <UDID> com.example.myapp
```

For cases where Xcode SDK and simulator iOS versions differ, use `xcodebuild` directly:

```bash
xcodebuild -workspace ios/Runner.xcworkspace \
  -scheme Runner \
  -sdk iphonesimulator \
  -configuration Debug \
  -destination 'platform=iOS Simulator,id=<UDID>' \
  DART_DEFINES=$(echo 'PROBE_AGENT=true' | base64) \
  -derivedDataPath build/ios \
  build
```

## Physical Device Setup

Testing on a physical iOS device requires USB port forwarding because, unlike simulators, physical devices do not share the host's loopback network. FlutterProbe uses `iproxy` (from the `libimobiledevice` suite) to forward the agent's WebSocket port over USB.

### Install iproxy

```bash
brew install libimobiledevice  # macOS
```

Verify the installation:

```bash
iproxy --help
```

### How it works

When targeting a physical device, the CLI runs `iproxy <host-port> <device-port> -u <UDID>` to create a TCP tunnel over USB. The CLI then connects to `ws://127.0.0.1:<host-port>/probe?token=...` just like it would for a simulator.

:::note
`iproxy` is **not** needed for iOS simulators (they share the host loopback network) or Android devices (which use `adb forward` for port forwarding).
:::

### Physical iPhone over WiFi (CLI and Studio): what must be true

WiFi avoids the USB tunnel and its cable mode-flipping, and it is the path Studio's 📡 button uses. Everything below must hold; the
checks at the end tell you which one does not.

1. **The phone runs a build of your app that contains the agent.** The App Store or production build has none: it does not listen on
   port 48686 and never will. `ProbeAgent.start()` is a no-op unless the app was built with `--dart-define=PROBE_AGENT=true`, and the
   agent refuses to start in a release build unless the app passes `allowReleaseBuild: true`.
2. **Build mode is profile, with your flavor if the project has one, and WiFi is switched on at build time:**
   ```bash
   flutter build ios --profile --flavor <your-flavor> \
     --dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true
   ```
   Without `PROBE_WIFI=true` the agent listens on localhost only and nothing on the network can reach it. Debug builds cannot be launched
   from the home screen of a physical iPhone, so use profile. Install with `xcrun devicectl device install app --device <UDID> <Runner.app>`.
3. **The app is running in the foreground with the screen unlocked.** iOS suspends background apps and the listener stops with them.
   The agent binds `0.0.0.0` on port **48686** (`--dart-define=PROBE_PORT=<n>` moves it; if the port is busy it takes the next one, up to
   48695, and prints `PROBE_PORT=<n>`).
4. **You have the session token.** The agent prints `PROBE_TOKEN=<token>` to the app's console every 3 seconds. It is never advertised on
   the network. Read it with `xcrun devicectl device process launch --console --device <UDID> <bundle-id>`, or from Xcode's console.
5. **The Mac can reach the phone's IP directly.** Both must be on the same network segment. A Mac on Ethernet and a phone on WiFi work
   only if the router bridges them (no client isolation, no separate guest or IoT network, and for discovery no mDNS filtering between
   the wired and wireless side). The Mac's own WiFi being off is fine.
6. **How the Mac finds the phone:**
   - **CLI, always works:** `probe test tests/ --host <phone-ip> --token <token>` (add `--agent-port <n>` if you moved the port).
   - **Studio's 📡 overlay, manual entry:** open **Connect manually**, type the phone's IP address or hostname, the port (48686) and the
     token from the app's `PROBE_TOKEN=` line, press **Test connection**, then **Connect**. This needs no advertiser. See
     [Studio: Manual WiFi entry](/tools/studio/#manual-wifi-entry).
   - **Studio's 📡 overlay, discovery:** it browses the Bonjour service `_flutterprobe._tcp` and lists what answers; you pick the
     device and paste the token. A device only appears when the agent advertises: it only does when the app passes a `ProbeAdvertiser`
     advertises when the app passes a `ProbeAdvertiser` (for example one built on `bonsoir`) to `ProbeAgent.start(advertiser: ...)`,
     which needs the `_flutterprobe._tcp` entry in `NSBonjourServices` and a `NSLocalNetworkUsageDescription` in that build's `Info.plist`
     (see [Studio: WiFi auto-discovery](/tools/studio/#wifi-auto-discovery)). Without an advertiser the app logs `PROBE_MDNS=off`.
   - **Studio's device picker** lists the phone even when it is only paired over the network (as "WiFi only") and says what it needs; see [Studio: the device picker and physical phones](/tools/studio/#the-device-picker-and-physical-phones).
   - **Studio over USB** (not WiFi): the device picker lists the phone as `physical`, uses an `iproxy` tunnel and reads the token from the
     device log with `idevicesyslog` (`brew install libimobiledevice`); the build still has to be profile with `PROBE_AGENT=true`.

**Check from the Mac, in this order:**

```bash
xcrun devicectl list devices                      # the phone is paired, developer mode on, state "connected"
dns-sd -B _flutterprobe._tcp local.               # does the app advertise? (only with a ProbeAdvertiser)
curl -s http://<phone-ip>:48686/probe/status      # {"agent":"flutter_probe","version":"..."}: the agent is reachable
```

The status endpoint needs no token. If `curl` is refused or times out: the build has no agent or no `PROBE_WIFI=true`, the app is not in the
foreground, or the network is blocking the Mac from the phone. If `curl` answers but `dns-sd` shows nothing: the app has no
`ProbeAdvertiser` or mDNS is filtered; use Studio's manual entry or the CLI `--host/--token` path.

:::note
Two things that are easy to get wrong, both checked against official sources (2026-10-11):
- **No Local Network prompt for the listener.** Apple's technical note [TN3179](https://developer.apple.com/documentation/technotes/tn3179-understanding-local-network-privacy)
  lists "listening for and accepting incoming TCP connections" as *not* needing local network access, so the agent's server needs no
  permission. Bonjour advertising does (it uses multicast), which is why only a build with an advertiser needs
  `NSLocalNetworkUsageDescription` and `NSBonjourServices`.
- **`devicectl` is not a port forwarder.** `iproxy` forwards to a "usbmux device" (USB, or network devices known to `usbmuxd` with `-n`; see `iproxy --help`);
  it does not use the CoreDevice tunnel that `xcrun devicectl` uses. Use `devicectl` to list, install and launch, and WiFi or `iproxy` to connect.
:::

## Connection Flow (Simulator)

The iOS simulator shares the host's `localhost` network. No port forwarding is needed.

1. The CLI connects directly to `ws://127.0.0.1:<port>/probe?token=...`
2. Token is read via **fast path**: file at `~/Library/Developer/CoreSimulator/Devices/<UDID>/data/tmp/probe/token`
3. If the file is not available, the **fallback** streams simulator logs with `xcrun simctl spawn <UDID> log stream` and parses `PROBE_TOKEN=` lines
4. The ProbeAgent prints the token every 3 seconds to handle late-connecting CLI instances

## Permissions

iOS simulator permissions are managed via `xcrun simctl privacy`:

```
allow permission "camera"          # simctl privacy grant <UDID> camera <bundleID>
deny permission "location"         # simctl privacy revoke <UDID> location <bundleID>
grant all permissions
revoke all permissions
```

This works on iOS 14+ simulators for camera, location, microphone, photos, contacts, calendar and sms.
`probe test --grant camera,location` applies the same grants once before the first test.

**Notifications have no simctl service**, so they cannot be granted by name. Since 0.16.0 probe answers the system
alert instead: `allow permission "notifications"` and `probe test --grant notifications` tap **Allow** when the alert
appears, through the iOS system-dialog driver (an XCUITest runner; needs Xcode, simulators only). See
[System dialogs](/tools/system-dialogs/). Android is unaffected: `POST_NOTIFICATIONS` is granted with
`adb shell pm grant`.

:::note[Permission changes relaunch the app (v0.12.1+)]
`simctl privacy grant/revoke` silently **terminates the target app** — an Apple behavior, not a
probe one. As of v0.12.1, all four permission verbs automatically relaunch the app and reconnect
the session afterward, so a permission step costs a few seconds of restart instead of hanging the
next step. Follow a permission step with `wait until "<your screen>" appears` before asserting,
exactly as you would after `restart the app`.
:::

:::tip[Notification permissions]
`xcrun simctl privacy` cannot grant notifications, but probe can answer the system alert itself (see above), so
you no longer have to remove the request from your app. If you prefer to skip it under test anyway, guard the
request with `bool.fromEnvironment('PROBE_AGENT')`.
:::

## App language

```
set language "de"        # relaunches the simulator app with -AppleLanguages (de) -AppleLocale de
```

The language is passed as launch arguments on every later launch of the app (including `restart the app` and `clear app data`), which
Flutter's locale follows. System dialogs (permissions, share sheet) keep the simulator's language: use the role-based
`tap "Don't Allow" in system dialog` for those. Not supported on physical devices (change the language in Settings > Apps).
`probe test --locale de` and `--locales de,ja,ar` apply it per run.

## Device Media

```
add media "fixtures/photo.jpg"     # xcrun simctl addmedia
```

The file lands in the simulator's Photos library and is immediately visible to image pickers.
Simulators only — there is no `devicectl` equivalent for physical devices; the step skips with a
warning there.

## Deep Links

```
open link "myapp://profile/42" in the app
```

Dispatches `xcrun simctl openurl`, so a custom scheme or Universal Link registered by your app is
delivered to the app itself. Plain `open link "https://..."` still opens Safari via
`url_launcher`.

:::caution[No cold launch on iOS Simulator]
`simctl openurl` cannot launch a fully-terminated app — iOS shows an "Open in App?" confirmation
dialog that `simctl` has no way to dismiss. Deep links work reliably when the app is already
running (foreground or backgrounded); put a `restart the app` or equivalent warm-up before the
deep-link step if the app might not be running. Android has no such caveat.
:::

## Native UI Automation

The `tap native` / `see native` / `type native` verbs are **Android-only today** — iOS has no
`uiautomator` equivalent, and `simctl` cannot inspect or drive arbitrary UI. On iOS,
`tap native`/`type native` fail with a clear error rather than silently no-op'ing, and
`see native` reports "not found". iOS support via WebDriverAgent is a written, scoped proposal
(`docs/proposals/n2-ios-native-ui-bridging.md` in the repo), not yet implemented.

## Biometric Authentication (Face ID / Touch ID)

`enroll biometric`, `biometric match`, and `biometric no match` drive Face ID and Touch ID flows on iOS Simulator via `xcrun simctl spawn booted notifyutil`.

```
before all tests
  enroll biometric

test "Face ID unlocks the app"
  open the app
  tap "Sign in with Face ID"
  wait until "Sign in with Face ID" appears
  tap "Sign in with Face ID"
  biometric match
  wait until "Dashboard" appears
  see "Dashboard"

test "failed Face ID shows error"
  open the app
  tap "Sign in with Face ID"
  wait until "Sign in with Face ID" appears
  tap "Sign in with Face ID"
  biometric no match
  wait until "Authentication failed" appears
  see "Authentication failed"
```

:::caution[iOS 26+ simulator — use `awaitBiometricResult()` in your app]
On iOS 26 / Xcode 26.5, the `faceCapture.no-match` notifyutil notification **no longer resolves `LAContext.evaluatePolicy`**. If your app calls `local_auth.authenticate()` directly, no-match tests will hang indefinitely.

**Fix:** In PROBE_AGENT builds, use `awaitBiometricResult()` from `flutter_probe_agent` instead. The CLI delivers the result via `probe.biometric_signal` after every `biometric match` / `biometric no match` step:

```dart
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

final ok = const bool.fromEnvironment('PROBE_AGENT')
    ? await awaitBiometricResult()
    : await LocalAuthentication().authenticate(localizedReason: 'Sign in');
```

This pattern works on all iOS versions and requires no changes to `.probe` test files.
:::

**Multiple simulators:** If more than one simulator is booted, the biometric notifyutil fires on `booted` (which selects an arbitrary device). Always boot only the target simulator, or pass `--device <UDID>` so the CLI targets the right one.

## Video Recording

iOS simulator video is recorded via `xcrun simctl io <UDID> recordVideo`. Videos use the `h264` codec (not HEVC) for browser compatibility in HTML reports.

```bash
probe test tests/ --video
```

## Configuration

```yaml
# probe.yaml
device:
  simulator_boot_timeout: 60s
  boot_poll_interval: 2s
  token_file_retries: 5
  restart_delay: 500ms
  ios_device_id: A1B2C3D4-E5F6-7890-ABCD-EF1234567890  # pin to a simulator UDID (skips auto-detection)

agent:
  port: 48686
  dial_timeout: 30s
  token_read_timeout: 30s
```

Set `ios_device_id` to avoid passing `--device <UDID>` on every run. The `--device` flag takes precedence when provided.

## Troubleshooting

### Token not found

If the CLI times out waiting for the auth token:
- Verify the app is running: `xcrun simctl list devices booted`
- Check the token file: `cat ~/Library/Developer/CoreSimulator/Devices/<UDID>/data/tmp/probe/token`
- Check logs manually: `xcrun simctl spawn <UDID> log stream --predicate 'eventMessage CONTAINS "PROBE_TOKEN="'`

### Build fails with SDK mismatch

When your Xcode SDK version differs from the target simulator iOS version, use `xcodebuild` with explicit `-destination` instead of `flutter build`.

### Notification permission dialog blocks tests

If a native notification permission dialog appears and blocks the Flutter UI, your app is requesting notification permissions without checking for `PROBE_AGENT`. See the [Permissions section](#permissions) above for the fix.

### Data clearing on iOS

`clear app data` deletes the container subdirectories (Documents, Library, tmp) rather than uninstalling the app. Path validation prevents accidental deletion of non-container paths.
