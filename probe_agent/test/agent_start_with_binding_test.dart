import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';
import 'package:flutter_probe_agent/src/view_ready.dart';

// An app that makes its binding first and then starts the agent: the agent attaches at once. Run with the define:
//   flutter test --dart-define=PROBE_AGENT=true test/agent_start_with_binding_test.dart
void main() {
  const enabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  test('ProbeAgent.start() after the app made its binding attaches at once',
      () async {
    WidgetsFlutterBinding.ensureInitialized();
    await ProbeAgent.start(port: 48798);
    expect(ViewReady.instance.attached, isTrue);
  }, skip: enabled ? false : 'needs --dart-define=PROBE_AGENT=true');
}
