import 'dart:async' show unawaited;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

/// FP-13: a tap issued right after a dialog/sheet is popped waits for the
/// route's exit animation to finish before it fires. Tapping mid-transition
/// races the Navigator's pointer handling and the closing route's still-mounted
/// widgets; settling first makes `tap` right after a close deterministic.
void main() {
  testWidgets('tap waits for a closing dialog to be fully gone', (tester) async {
    var taps = 0;
    bool? dialogMountedWhenTapped;
    late BuildContext pageContext;

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Builder(builder: (context) {
          pageContext = context;
          return Center(
            child: ElevatedButton(
              key: const ValueKey('under_button'),
              onPressed: () {
                taps++;
                dialogMountedWhenTapped =
                    find.text('Hello dialog').evaluate().isNotEmpty;
              },
              child: const Text('Under'),
            ),
          );
        }),
      ),
    ));

    unawaited(showDialog<void>(
      context: pageContext,
      builder: (_) => const AlertDialog(content: Text('Hello dialog')),
    ));
    await tester.pumpAndSettle();
    expect(find.text('Hello dialog'), findsOneWidget);

    // Start the exit animation, but don't let it finish.
    Navigator.of(pageContext).pop();
    await tester.pump(const Duration(milliseconds: 20));
    expect(find.text('Hello dialog'), findsOneWidget,
        reason: 'dialog must still be mounted mid exit-animation');

    String? raw;
    final executor = ProbeExecutor((s) => raw = s);
    final call = executor.dispatch(ProbeRequest(
      jsonrpc: '2.0',
      id: 1,
      method: ProbeMethods.tap,
      params: {
        'selector': {'kind': 'id', 'text': '#under_button'},
      },
    ));

    // Drive frames (fake-async clock) until the tap completes.
    var done = false;
    unawaited(call.whenComplete(() => done = true));
    for (var i = 0; i < 100 && !done; i++) {
      await tester.pump(const Duration(milliseconds: 50));
    }
    expect(done, isTrue, reason: 'tap must complete once the dialog is gone');

    expect(raw, isNotNull);
    expect(taps, 1);
    expect(dialogMountedWhenTapped, isFalse,
        reason: 'tap fired while the dialog was still animating out');
  });

  testWidgets('tap with no route transition in flight is not delayed',
      (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ElevatedButton(
          key: const ValueKey('plain'),
          onPressed: () => taps++,
          child: const Text('Plain'),
        ),
      ),
    ));

    final executor = ProbeExecutor((_) {});
    await executor.dispatch(ProbeRequest(
      jsonrpc: '2.0',
      id: 1,
      method: ProbeMethods.tap,
      params: {
        'selector': {'kind': 'id', 'text': '#plain'},
      },
    ));
    expect(taps, 1);
  });
}
