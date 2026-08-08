import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

void main() {
  group('tap/type focus a text field like a real tap (PT-04)', () {
    testWidgets('tap #id requests focus on the underlying EditableText',
        (tester) async {
      final focusNode = FocusNode();
      addTearDown(focusNode.dispose);
      final controller = TextEditingController();
      addTearDown(controller.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(
            key: const ValueKey('email_field'),
            controller: controller,
            focusNode: focusNode,
          ),
        ),
      ));

      expect(focusNode.hasFocus, isFalse);

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.tap,
        params: {
          'selector': {'kind': 'id', 'text': '#email_field'},
        },
      ));

      expect(focusNode.hasFocus, isTrue);
    });

    testWidgets('type without a preceding tap also requests focus',
        (tester) async {
      final focusNode = FocusNode();
      addTearDown(focusNode.dispose);
      final controller = TextEditingController();
      addTearDown(controller.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(
            key: const ValueKey('search_field'),
            controller: controller,
            focusNode: focusNode,
          ),
        ),
      ));

      expect(focusNode.hasFocus, isFalse);

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.type_,
        params: {
          'selector': {'kind': 'id', 'text': '#search_field'},
          'text': 'hello',
        },
      ));

      expect(focusNode.hasFocus, isTrue);
      expect(controller.text, equals('hello'));
    });

    testWidgets(
        'tap on a Semantics-wrapped field still requests real focus, '
        'not just a throwaway one', (tester) async {
      final focusNode = FocusNode();
      addTearDown(focusNode.dispose);
      final controller = TextEditingController();
      addTearDown(controller.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Semantics(
            identifier: 'wrapped_field',
            child: TextField(controller: controller, focusNode: focusNode),
          ),
        ),
      ));

      expect(focusNode.hasFocus, isFalse);

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.tap,
        params: {
          'selector': {'kind': 'id', 'text': '#wrapped_field'},
        },
      ));

      expect(focusNode.hasFocus, isTrue);
    });
  });
}
