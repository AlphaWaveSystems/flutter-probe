import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';
import 'package:flutter_probe_agent/src/view_ready.dart';

// An app that calls ProbeAgent.start() before WidgetsFlutterBinding.ensureInitialized() (or runApp) must still launch: start() makes the
// binding itself and never throws over it. The agent only starts with --dart-define=PROBE_AGENT=true, so run this file that way:
//   flutter test --dart-define=PROBE_AGENT=true test/agent_start_no_binding_test.dart
// Without the define start() returns at once and the test is skipped.
void main() {
  const enabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  test('ProbeAgent.start() before any binding does not throw', () async {
    await expectLater(ProbeAgent.start(port: 48799), completes);
    expect(ViewReady.instance.attached, isTrue, reason: 'start() made the binding and attached');
  }, skip: enabled ? false : 'needs --dart-define=PROBE_AGENT=true');
}
