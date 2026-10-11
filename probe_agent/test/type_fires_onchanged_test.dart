import 'package:flutter/material.dart';
import 'package:flutter/services.dart' show FilteringTextInputFormatter;
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

void main() {
  group('type/clear go through the real text-input path (FP-13)', () {
    testWidgets('type fires onChanged so onChanged-driven UI rebuilds',
        (tester) async {
      final changes = <String>[];
      String notice = '';
      final controller = TextEditingController();
      addTearDown(controller.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: StatefulBuilder(
            builder: (context, setState) => Column(
              children: [
                TextField(
                  key: const ValueKey('goal_dialog_field'),
                  controller: controller,
                  onChanged: (v) {
                    changes.add(v);
                    setState(() => notice = int.parse(v) > 4000 ? 'above range' : '');
                  },
                ),
                Text(notice),
              ],
            ),
          ),
        ),
      ));

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.type_,
        params: {
          'selector': {'kind': 'id', 'text': '#goal_dialog_field'},
          'text': '4500',
        },
      ));
      await tester.pump();

      expect(controller.text, '4500');
      expect(changes, ['4500'], reason: 'onChanged must fire exactly once');
      expect(find.text('above range'), findsOneWidget,
          reason: 'the onChanged-driven notice must have rebuilt');
    });

    testWidgets('type respects inputFormatters like real keystrokes',
        (tester) async {
      final controller = TextEditingController();
      addTearDown(controller.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(
            key: const ValueKey('digits_only'),
            controller: controller,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
          ),
        ),
      ));

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.type_,
        params: {
          'selector': {'kind': 'id', 'text': '#digits_only'},
          'text': '12ab3',
        },
      ));

      expect(controller.text, '123');
    });

    testWidgets('clear fires onChanged with an empty value', (tester) async {
      final changes = <String>[];
      final controller = TextEditingController(text: 'abc');
      addTearDown(controller.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(
            key: const ValueKey('clearable'),
            controller: controller,
            onChanged: changes.add,
          ),
        ),
      ));

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.clear,
        params: {
          'selector': {'kind': 'id', 'text': '#clearable'},
        },
      ));

      expect(controller.text, '');
      expect(changes, ['']);
    });
  });
}
