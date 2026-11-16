# probe_flutter_fixture

Flutter twin of the native fixture apps in `../android` and `../ios` — see the parent
`native-test-apps/README.md` for the shared element table and cross-framework story.

## The list screen (scroll checks)

The default app is the 8-element surface the native fixtures share, and it stays that way. The scroll checks in
`probe-tests/list/` run against a second screen, a full-height list (`#fixture_list`, rows `#fixture_row_N`) with no other
scrollable around it, whose offset is shown in the app bar. Build it with a flag and install it over the default build:

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_LIST=true
probe test probe-tests/list/list_scroll.probe --config probe.yaml --device <serial>
```

The list has to stand alone. The agent's `scroll ... on <target>` moves the nearest scrollable above the target: inside the
default screen's scroll view that is the outer page, so the list's own offset stayed at 0.0 when the list was placed there.

## The crash screen (forced crash)

`probe-tests/crash/forced_crash.probe` runs against a third screen: one button, `#fixture_crash` ("Crash the app"), that aborts the
process from native code through `dart:ffi` (the C library's `abort()`, found in the process itself, so no plugin). The default
build and the list build do not have it. Build it with a flag and install it over the default build:

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_CRASH=true
probe test probe-tests/crash/forced_crash.probe --config probe.yaml --device <serial> --crash-evidence
```

The test fails on purpose: the tap ends the app, so the step that loses its connection to the agent fails. With `--crash-evidence`
(Android only) the failed test keeps why: one line for the test (`the app died during this test: native_crash (pid …): Fatal signal 6
(SIGABRT)`), a `crash` object in its JSON report, and the exit reason and the crash text under `reports/crash/`. Without the flag the
run reports only the lost connection (`connection closed`). Not in any CI run: it is run by hand, like the other scenarios here.

On a shared emulator give the agent its own port: `--dart-define=PROBE_PORT=<n>` in the build, and `agent: port: <n>` in
`probe.yaml`, the same number in both, since probe forwards that port to the device.

## On a physical iPhone, over WiFi

The app passes a `bonsoir`-based `ProbeAdvertiser` (`lib/probe_advertiser.dart`) to the agent when it is built with
`--dart-define=PROBE_WIFI=true`, and `ios/Runner/Info.plist` has the `NSLocalNetworkUsageDescription` and `NSBonjourServices`
(`_flutterprobe._tcp`) entries the advertisement needs on iOS 14+. Build in profile mode, never release.

```bash
scripts/physical-ios-fixture.sh --ip <phone-ip>    # prints the exact steps for your phone; --check tests reachability
```

Signing the app is the owner's step (an Apple developer team in Xcode). Full conditions:
https://flutterprobe.dev/platform/ios/ ("Physical iPhone over WiFi").
