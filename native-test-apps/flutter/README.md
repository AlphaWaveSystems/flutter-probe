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

## On a physical iPhone, over WiFi

The app passes a `bonsoir`-based `ProbeAdvertiser` (`lib/probe_advertiser.dart`) to the agent when it is built with
`--dart-define=PROBE_WIFI=true`, and `ios/Runner/Info.plist` has the `NSLocalNetworkUsageDescription` and `NSBonjourServices`
(`_flutterprobe._tcp`) entries the advertisement needs on iOS 14+. Build in profile mode, never release.

```bash
scripts/physical-ios-fixture.sh --ip <phone-ip>    # prints the exact steps for your phone; --check tests reachability
```

Signing the app is the owner's step (an Apple developer team in Xcode). Full conditions:
https://flutterprobe.dev/platform/ios/ ("Physical iPhone over WiFi").
