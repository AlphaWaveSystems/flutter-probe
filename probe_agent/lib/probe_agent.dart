/// ProbeAgent — FlutterProbe on-device test agent.
///
/// Usage (in your app's main_debug.dart or via conditional import):
///
/// ```dart
/// import 'package:probe_agent/probe_agent.dart';
///
/// void main() async {
///   WidgetsFlutterBinding.ensureInitialized();
///   await ProbeAgent.start();   // starts WS server on localhost:8686
///   runApp(const MyApp());
/// }
/// ```
library probe_agent;

export 'src/agent.dart' show ProbeAgent;
export 'src/server.dart' show ProbeServer;
export 'src/finder.dart' show ProbeFinder;
export 'src/executor.dart' show ProbeExecutor;
export 'src/sync.dart' show ProbeSync;
export 'src/protocol.dart' show ProbeRequest, ProbeResponse, ProbeError;
