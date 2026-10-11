import 'package:bonsoir/bonsoir.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

/// Publishes the agent as `_flutterprobe._tcp` on the local network, so Studio's WiFi overlay can list this
/// phone without typing its address. The auth token is deliberately NOT advertised.
///
/// Needs, on iOS 14+, `NSLocalNetworkUsageDescription` and `_flutterprobe._tcp` in `NSBonjourServices`
/// (both are in ios/Runner/Info.plist). Advertising only happens when the agent listens on the network
/// (`--dart-define=PROBE_WIFI=true`).
class BonsoirAdvertiser extends ProbeAdvertiser {
  BonsoirBroadcast? _broadcast;

  @override
  Future<void> start({required String name, required int port, required String agentVersion}) async {
    try {
      final broadcast = BonsoirBroadcast(
        service: BonsoirService(
          name: name,
          type: mdnsServiceType, // _flutterprobe._tcp
          port: port,
          attributes: {'version': agentVersion, 'port': '$port'},
        ),
      );
      await broadcast.initialize();
      await broadcast.start();
      _broadcast = broadcast;
    } catch (e) {
      // ignore: avoid_print
      print('ProbeAgent: mDNS advertise failed: $e'); // never block the agent
    }
  }

  @override
  Future<void> stop() async {
    try {
      await _broadcast?.stop();
    } catch (_) {}
    _broadcast = null;
  }
}
