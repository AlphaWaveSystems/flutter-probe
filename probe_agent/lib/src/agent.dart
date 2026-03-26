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
  /// Build modes:
  /// - **Debug**: works out of the box with `--dart-define=PROBE_AGENT=true`
  /// - **Profile**: works out of the box (needed for physical iOS devices)
  /// - **Release**: blocked by default. Pass `allowReleaseBuild: true` to
  ///   override, and `--dart-define=PROBE_AGENT_FORCE=true` to skip the
  ///   console warning.
  ///
  /// Requires `--dart-define=PROBE_AGENT=true` at build time. Without it,
  /// this is always a no-op.
  static Future<void> start({
    int port = 48686,
    bool allowReleaseBuild = false,
  }) async {
    const enabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
    if (!enabled) return;

    // Detect build mode
    const isRelease = bool.fromEnvironment('dart.vm.product', defaultValue: false);
    const isProfile = bool.fromEnvironment('dart.vm.profile', defaultValue: false);

    if (isRelease && !allowReleaseBuild) {
      // ignore: avoid_print
      print('⚠️  ProbeAgent: BLOCKED — running in release mode.');
      // ignore: avoid_print
      print('    The ProbeAgent opens a WebSocket server on the device.');
      // ignore: avoid_print
      print('    Do NOT ship this to production users.');
      // ignore: avoid_print
      print('');
      // ignore: avoid_print
      print('    To allow: ProbeAgent.start(allowReleaseBuild: true)');
      // ignore: avoid_print
      print('    To silence: add --dart-define=PROBE_AGENT_FORCE=true');
      return;
    }

    if (isRelease && allowReleaseBuild) {
      const force = bool.fromEnvironment('PROBE_AGENT_FORCE', defaultValue: false);
      if (!force) {
        // ignore: avoid_print
        print('⚠️  ProbeAgent: WARNING — starting in RELEASE mode.');
        // ignore: avoid_print
        print('    This build has a WebSocket debug server enabled.');
        // ignore: avoid_print
        print('    Do NOT distribute to end users.');
        // ignore: avoid_print
        print('    Add --dart-define=PROBE_AGENT_FORCE=true to suppress this warning.');
      }
    }

    if (isProfile) {
      // ignore: avoid_print
      print('ProbeAgent: starting in profile mode (physical device testing)');
    }

    await _startInternal(port);
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
      // Local mode: listen on port
      // PROBE_WIFI=true enables binding to 0.0.0.0 for WiFi testing
      const allowWifi = bool.fromEnvironment('PROBE_WIFI', defaultValue: false);
      _server = ProbeServer(port: port, allowRemoteConnections: allowWifi);
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
