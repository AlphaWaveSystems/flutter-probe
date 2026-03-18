import 'dart:io';

import 'relay_client.dart';
import 'server.dart';

/// ProbeAgent is the top-level entry point for embedding in a Flutter app.
///
/// Add to your debug/profile main entrypoint:
///
/// ```dart
/// void main() async {
///   WidgetsFlutterBinding.ensureInitialized();
///
///   // Only run agent in debug mode or when explicitly enabled
///   const bool probeEnabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
///   if (probeEnabled) {
///     await ProbeAgent.start();
///   }
///
///   runApp(const MyApp());
/// }
/// ```
///
/// ## Relay mode
///
/// When built with `PROBE_RELAY_URL` and `PROBE_RELAY_TOKEN` dart-defines,
/// the agent connects OUT to a ProbeRelay server instead of listening locally.
/// This enables cloud device farm testing where inbound ports are unreachable.
class ProbeAgent {
  ProbeAgent._();

  static ProbeServer? _server;
  static ProbeRelayClient? _relayClient;

  /// Starts the ProbeAgent in the appropriate mode:
  /// - **Relay mode** if `PROBE_RELAY_URL` and `PROBE_RELAY_TOKEN` are set
  ///   via `--dart-define` (connects out to a relay server).
  /// - **Local mode** otherwise (listens on localhost for CLI connections).
  ///
  /// No-op on non-debug builds (checked via assert).
  static Future<void> start({int port = 48686}) async {
    // Safety guard — only meaningful in debug / profile builds.
    // In release mode this is a no-op because asserts are disabled.
    assert(() {
      _startInternal(port);
      return true;
    }());
  }

  static Future<void> _startInternal(int port) async {
    if (_server != null || _relayClient != null) return; // already running

    const relayUrl = String.fromEnvironment('PROBE_RELAY_URL', defaultValue: '');
    const relayToken = String.fromEnvironment('PROBE_RELAY_TOKEN', defaultValue: '');

    if (relayUrl.isNotEmpty && relayToken.isNotEmpty) {
      // Relay mode: connect OUT to the relay server
      _relayClient = ProbeRelayClient(
        relayUrl: relayUrl,
        agentToken: relayToken,
      );
      await _relayClient!.connect();
    } else {
      // Local mode: listen on port (existing behavior)
      _server = ProbeServer(port: port);
      await _server!.start();
    }
  }

  /// Stops the ProbeAgent server or relay client.
  static Future<void> stop() async {
    await _server?.stop();
    _server = null;
    await _relayClient?.stop();
    _relayClient = null;
  }

  /// Whether the agent is currently running.
  static bool get isRunning => _server != null || _relayClient != null;

  /// The port the agent is listening on (0 in relay mode).
  static int get port => _server?.port ?? 0;

  /// Whether the agent is running in relay mode.
  static bool get isRelayMode => _relayClient != null;
}

/// Utility: true when running in a probe-enabled build.
bool get isProbeEnabled {
  const value = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  return value || Platform.environment.containsKey('PROBE_AGENT');
}
