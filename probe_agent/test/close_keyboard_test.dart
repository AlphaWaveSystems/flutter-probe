import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

void main() {
  group("close keyboard actually dismisses focus (PT-12)", () {
    // `close keyboard` (parser.VerbClose with Name: 'keyboard') dispatches
    // to probe.device_action with action='close', value='keyboard'.
    // _deviceAction's switch had no 'close' case at all, so this — and
    // `close the app` — were both silent no-ops. Verifies the fix unfocuses
    // the current field directly, without depending on any OS-level
    // gesture (immune to the iOS Back-swipe collision the original symptom
    // described).
    testWidgets('unfocuses a focused text field', (tester) async {
      final focusNode = FocusNode();
      addTearDown(focusNode.dispose);
      final controller = TextEditingController();
      addTearDown(controller.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(key: const ValueKey('field'), focusNode: focusNode, controller: controller),
        ),
      ));

      focusNode.requestFocus();
      await tester.pump();
      expect(focusNode.hasFocus, isTrue);

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.deviceAction,
        params: {'action': 'close', 'value': 'keyboard'},
      ));
      await tester.pump();

      expect(focusNode.hasFocus, isFalse);
    });
  });
}
