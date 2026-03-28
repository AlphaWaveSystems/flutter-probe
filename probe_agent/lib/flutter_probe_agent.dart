/// FlutterProbe on-device test agent.
///
/// Add as a dev dependency and initialize in your app's main entrypoint:
///
/// ```dart
/// import 'package:flutter_probe_agent/flutter_probe_agent.dart';
///
/// Future<void> main() async {
///   WidgetsFlutterBinding.ensureInitialized();
///
///   const probeEnabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
///   if (probeEnabled) {
///     await ProbeAgent.start();
///   }
///
///   runApp(const MyApp());
/// }
/// ```
///
/// Build with `--dart-define=PROBE_AGENT=true` to enable.
/// The agent is completely stripped from release builds.
///
/// Published by [Alpha Wave Systems](https://alphawavesystems.com).
/// Documentation at [flutterprobe.dev](https://flutterprobe.dev).
library flutter_probe_agent;

export 'src/agent.dart' show ProbeAgent;
export 'src/relay_client.dart' show ProbeRelayClient;
export 'src/server.dart' show ProbeServer;
export 'src/finder.dart' show ProbeFinder;
export 'src/executor.dart' show ProbeExecutor;
export 'src/sync.dart' show ProbeSync;
export 'src/recorder.dart' show ProbeRecorder;
export 'src/protocol.dart' show ProbeRequest, ProbeResponse, ProbeError, ProbeMethods;
