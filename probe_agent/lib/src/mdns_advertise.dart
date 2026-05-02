import 'dart:async';

import 'package:bonsoir/bonsoir.dart';

/// mDNS service type used by all FlutterProbe agents. Studio (and any other
/// discovery client) browses this name to find agents on the LAN. The token
/// is intentionally not advertised in TXT records — anyone on the same
/// network would be able to read it.
const String mdnsServiceType = '_flutterprobe._tcp';

/// ProbeMDNS publishes the agent over Bonjour/NSD when the agent is running
/// in WiFi mode (i.e. listening on 0.0.0.0). On localhost-only deployments
/// the agent never calls into this class, so apps that only use simulators
/// pay zero overhead.
class ProbeMDNS {
  BonsoirBroadcast? _broadcast;

  /// Starts advertising the agent on the local network. Errors are logged
  /// but never thrown — mDNS failure must not prevent the agent from
  /// accepting direct connections.
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

  /// Stops advertising. Safe to call when the broadcast was never started or
  /// has already been stopped.
  Future<void> stop() async {
    try {
      await _broadcast?.stop();
    } catch (_) {
      // best-effort cleanup; failure here just leaks one record until TTL
    }
    _broadcast = null;
  }
}
