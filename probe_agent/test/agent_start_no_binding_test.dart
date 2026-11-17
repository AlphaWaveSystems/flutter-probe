import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';
import 'package:flutter_probe_agent/src/view_ready.dart';

// An app that calls ProbeAgent.start() before its binding exists must still launch, and must still be able to install its OWN binding
// afterwards: start() never makes the binding. The agent only starts with --dart-define=PROBE_AGENT=true, so run this file that way:
//   flutter test --dart-define=PROBE_AGENT=true test/agent_start_no_binding_test.dart
// Without the define start() returns at once and the test is skipped.
class AppBinding extends WidgetsFlutterBinding {}

void main() {
  const enabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  test('ProbeAgent.start() before any binding does not throw', () async {
    await expectLater(ProbeAgent.start(port: 48799), completes);
    expect(ViewReady.instance.attached, isFalse,
        reason:
            'start() makes no binding, so there is nothing to attach to yet');
    expect(() => AppBinding(), returnsNormally,
        reason: 'the app installs its own binding after start()');
    ViewReady.instance.attachToAppBinding();
    expect(ViewReady.instance.attached, isTrue);
  }, skip: enabled ? false : 'needs --dart-define=PROBE_AGENT=true');
}
