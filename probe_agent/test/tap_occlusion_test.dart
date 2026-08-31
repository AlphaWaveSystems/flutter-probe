import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

void main() {
  group('tap #id respects occlusion (FP-10)', () {
    testWidgets(
        'a button covered by an opaque overlay is not tapped via the direct-invoke fast path',
        (tester) async {
      var tapped = false;
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Stack(
            children: [
              Semantics(
                identifier: 'covered_button',
                button: true,
                child: InkResponse(
                  onTap: () => tapped = true,
                  child: const SizedBox(width: 48, height: 48, child: Icon(Icons.add)),
                ),
              ),
              // Painted after the button, so it's on top in the same Stack —
              // a real user's finger would hit this, not the button beneath it.
              Positioned.fill(
                child: ModalBarrier(color: Colors.black.withValues(alpha: 0.5)),
              ),
            ],
          ),
        ),
      ));

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.tap,
        params: {
          'selector': {'kind': 'id', 'text': '#covered_button'},
        },
      ));
      await tester.pump();

      expect(tapped, isFalse,
          reason: '_tryDirectTap invokes onTap by walking the Element tree '
              'structurally, with no relationship to paint order — without '
              'an occlusion guard it would fire even though the ModalBarrier '
              'covers the button on screen. A real tap should land on the '
              'barrier instead, exactly like a real user\'s finger would.');
    });

    testWidgets('an unobstructed button in the same shape is still tapped directly',
        (tester) async {
      var tapped = false;
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Stack(
            children: [
              Semantics(
                identifier: 'uncovered_button',
                button: true,
                child: InkResponse(
                  onTap: () => tapped = true,
                  child: const SizedBox(width: 48, height: 48, child: Icon(Icons.add)),
                ),
              ),
            ],
          ),
        ),
      ));

      final executor = ProbeExecutor((_) {});
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.tap,
        params: {
          'selector': {'kind': 'id', 'text': '#uncovered_button'},
        },
      ));
      await tester.pump();

      expect(tapped, isTrue,
          reason: 'PT-05 regression check: the occlusion guard must not '
              'block a tap on a button that genuinely is the topmost thing '
              'at its own screen position.');
    });
  });
}
