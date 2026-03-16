import 'dart:io';

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
class ProbeAgent {
  ProbeAgent._();

  static ProbeServer? _server;

  /// Starts the ProbeAgent WebSocket server.
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
    if (_server != null) return; // already running
    _server = ProbeServer(port: port);
    await _server!.start();
  }

  /// Stops the ProbeAgent server.
  static Future<void> stop() async {
    await _server?.stop();
    _server = null;
  }

  /// Whether the agent is currently running.
  static bool get isRunning => _server != null;

  /// The port the agent is listening on.
  static int get port => _server?.port ?? 48686;
}

/// Utility: true when running in a probe-enabled build.
bool get isProbeEnabled {
  const value = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  return value || Platform.environment.containsKey('PROBE_AGENT');
}
