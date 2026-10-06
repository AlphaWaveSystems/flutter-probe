/// mDNS service type used by all FlutterProbe agents. Studio (and any other
/// discovery client) browses this name to find agents on the LAN. The token
/// is intentionally not advertised in TXT records — anyone on the same
/// network would be able to read it.
const String mdnsServiceType = '_flutterprobe._tcp';

/// Publishes the agent on the local network so Studio can find physical devices
/// without typing an IP.
///
/// The core agent deliberately has **no native dependency**: advertising needs a
/// platform plugin (Bonjour/NSD), and a plugin listed in an app's dependencies
/// is linked into every build of that app, release included. Implementations
/// live in the optional `flutter_probe_agent_mdns` package; pass one to
/// `ProbeAgent.start(advertiser: ...)` only in the build flavor that wants
/// auto-discovery. Without one, WiFi testing works exactly as before with
/// `--host <ip> --token <token>`.
abstract class ProbeAdvertiser {
  const ProbeAdvertiser();

  /// Starts advertising. Implementations must log and swallow errors: a failed
  /// advertisement must never stop the agent from accepting connections.
  Future<void> start({
    required String name,
    required int port,
    required String agentVersion,
  });

  /// Stops advertising. Must be safe to call when never started.
  Future<void> stop();
}
