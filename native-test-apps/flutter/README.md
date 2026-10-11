# probe_flutter_fixture

Flutter twin of the native fixture apps in `../android` and `../ios` — see the parent
`native-test-apps/README.md` for the shared element table and cross-framework story.

## On a physical iPhone, over WiFi

The app passes a `bonsoir`-based `ProbeAdvertiser` (`lib/probe_advertiser.dart`) to the agent when it is built with
`--dart-define=PROBE_WIFI=true`, and `ios/Runner/Info.plist` has the `NSLocalNetworkUsageDescription` and `NSBonjourServices`
(`_flutterprobe._tcp`) entries the advertisement needs on iOS 14+. Build in profile mode, never release.

```bash
scripts/physical-ios-fixture.sh --ip <phone-ip>    # prints the exact steps for your phone; --check tests reachability
```

Signing the app is the owner's step (an Apple developer team in Xcode). Full conditions:
https://flutterprobe.dev/platform/ios/ ("Physical iPhone over WiFi").
