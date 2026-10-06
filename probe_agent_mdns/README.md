# flutter_probe_agent_mdns

Optional mDNS/Bonjour advertising for [`flutter_probe_agent`](https://pub.dev/packages/flutter_probe_agent), so
[FlutterProbe Studio](https://flutterprobe.dev/tools/studio/) can find physical devices on your network without
typing an IP.

## Why a separate package

A native plugin listed in an app's dependencies is linked into **every** build of that app, release builds
included, whatever `--dart-define` flags are used. The core agent therefore has no native dependency. mDNS
advertising needs Bonjour/NSD, so it lives here. Add this package only to the build you test on devices.

## Use

```yaml
dependencies:
  flutter_probe_agent: ^0.15.0
  flutter_probe_agent_mdns: ^0.15.0
```

```dart
import 'package:flutter_probe_agent/flutter_probe_agent.dart';
import 'package:flutter_probe_agent_mdns/flutter_probe_agent_mdns.dart';

Future<void> main() async {
  await ProbeAgent.start(advertiser: const ProbeMdns());
  runApp(const MyApp());
}
```

Build with `--dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true`. It only advertises in WiFi mode (agent
bound to `0.0.0.0`).

iOS: add `_flutterprobe._tcp` to `NSBonjourServices` and a `NSLocalNetworkUsageDescription` to the `Info.plist`
of that build. The auth token is never advertised.

Without this package WiFi testing still works: `probe test --host <device-ip> --token <token>`.
