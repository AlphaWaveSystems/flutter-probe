import 'package:bonsoir/bonsoir.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

/// Advertises the FlutterProbe agent over Bonjour/NSD so Studio can discover
/// physical devices on the LAN without typing an IP.
///
/// Only has an effect in WiFi mode (`--dart-define=PROBE_WIFI=true`, agent bound
/// to 0.0.0.0). Add this package only to the build you test on devices — the
/// native Bonjour plugin it brings is linked into every build of an app that
/// depends on it, which is exactly why it is not part of the core agent.
///
/// ```dart
/// await ProbeAgent.start(advertiser: const ProbeMdns());
/// ```
class ProbeMdns extends ProbeAdvertiser {
  const ProbeMdns();

  // The broadcast is per running agent; keep it static so the const instance
  // stays const.
  static BonsoirBroadcast? _broadcast;

  /// Starts advertising. Errors are logged but never thrown — mDNS failure
  /// must not prevent the agent from accepting direct connections.
  @override
  Future<void> start({
    required String name,
    required int port,
    required String agentVersion,
  }) async {
    try {
      final service = BonsoirService(
        name: name,
        type: mdnsServiceType,
        port: port,
        // The token is intentionally NOT advertised: anyone on the same
        // network could read it.
        attributes: {
          'version': agentVersion,
          'port': '$port',
        },
      );
      final broadcast = BonsoirBroadcast(service: service);
      await broadcast.ready;
      await broadcast.start();
      _broadcast = broadcast;
      // ignore: avoid_print
      print('PROBE_MDNS=advertising as "$name" on $mdnsServiceType:$port');
    } catch (e) {
      // ignore: avoid_print
      print('ProbeAgent: mDNS advertise failed: $e');
    }
  }

  /// Stops advertising. Safe to call when never started or already stopped.
  @override
  Future<void> stop() async {
    try {
      await _broadcast?.stop();
    } catch (_) {
      // best-effort cleanup; failure here just leaks one record until TTL
    }
    _broadcast = null;
  }
}
