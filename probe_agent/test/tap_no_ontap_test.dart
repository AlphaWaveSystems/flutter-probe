import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

void main() {
  group('tap #id reaches buttons with no onTap SemanticsAction (PT-05)', () {
    testWidgets(
        'InkResponse-based buttons (e.g. IconButton) are invoked directly, '
        'not just via a hit-tested fallback', (tester) async {
      var tapped = false;
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Semantics(
            identifier: 'custom_button',
            button: true,
            child: InkResponse(
              onTap: () => tapped = true,
              child: const SizedBox(width: 48, height: 48, child: Icon(Icons.add)),
            ),
          ),
        ),
      ));

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.tap,
        params: {
          'selector': {'kind': 'id', 'text': '#custom_button'},
        },
      ));
      await tester.pump();

      expect(tapped, isTrue,
          reason: '_tryDirectTap now recognizes InkResponse (IconButton and '
              'friends build one directly, not always an InkWell), so this '
              'should be invoked on the fast direct-tap path');
    });

    testWidgets(
        'a button with neither GestureDetector/InkResponse discoverable '
        'still gets tapped via the real hit-tested pointer fallback',
        (tester) async {
      var tapCount = 0;
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Semantics(
            identifier: 'raw_listener_button',
            button: true,
            child: Listener(
              onPointerDown: (_) => tapCount++,
              child: const SizedBox(width: 48, height: 48, child: Icon(Icons.add)),
            ),
          ),
        ),
      ));

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.tap,
        params: {
          'selector': {'kind': 'id', 'text': '#raw_listener_button'},
        },
      ));
      await tester.pump();

      // Semantics doesn't participate in hit-testing at all, so a real
      // synthetic pointer tap at the Semantics node's geometric center
      // reaches whatever is actually rendered there regardless of Semantics
      // wrapping/shadowing — this is what makes PT-05's literal scenario
      // (no onTap SemanticsAction, or shadowed by an overlapping Semantics
      // node) already work without any code change.
      expect(tapCount, equals(1),
          reason: '_tryDirectTap finds nothing here (no GestureDetector/'
              'InkResponse in the subtree), so this must be reached via the '
              'real hit-tested pointer tap fallback in _tap');
    });
  });
}
