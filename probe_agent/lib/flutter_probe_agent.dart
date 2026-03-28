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

// Public API — only what app developers need
export 'src/agent.dart' show ProbeAgent, isProbeEnabled;
