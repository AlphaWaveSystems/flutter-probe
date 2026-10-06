import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _rpc(WidgetTester tester, String method, Map<String, dynamic> params) async {
  String? raw;
  final executor = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = executor
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  // FP-19 (reported from an Android gate): a tap "succeeded" and nothing
  // happened. When something else is on top at the tap point, say so.
  testWidgets('a tap on a covered target returns a warning instead of failing silently',
      (tester) async {
    var covered = 0, under = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Stack(children: [
          Center(
            child: ElevatedButton(
              key: const ValueKey('share_button'),
              onPressed: () => under++,
              child: const Text('Share'),
            ),
          ),
          // A full-screen overlay (loading veil, sticky bar, ...) that eats the tap.
          Positioned.fill(
            child: GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTap: () => covered++,
              child: const SizedBox.expand(),
            ),
          ),
        ]),
      ),
    ));

    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'id', 'text': '#share_button'},
    });

    expect(res['error'], isNull, reason: 'the tap itself still happens; it is a warning, not a failure');
    final warning = res['result']['warning'] as String?;
    expect(warning, isNotNull);
    expect(warning, contains('#share_button'));
    expect(warning, contains('covered'));
    expect(covered, 1, reason: 'the overlay received the tap, as a real user\'s would');
    expect(under, 0);
  });

  testWidgets('a normal tap carries no warning', (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Center(
          child: ElevatedButton(
            key: const ValueKey('ok_button'),
            onPressed: () => taps++,
            child: const Text('OK'),
          ),
        ),
      ),
    ));
    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'id', 'text': '#ok_button'},
    });
    expect(res['result'].containsKey('warning'), isFalse);
    expect(taps, 1);
  });

  testWidgets('a tap right after a scroll waits for the scroll to finish', (tester) async {
    final c = ScrollController();
    addTearDown(c.dispose);
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView.builder(
          controller: c,
          itemCount: 60,
          itemBuilder: (_, i) => i == 3
              ? ElevatedButton(key: const ValueKey('row3'), onPressed: () => taps++, child: const Text('Row 3'))
              : SizedBox(height: 48, child: Text('Row $i')),
        ),
      ),
    ));
    // A scroll is still in flight when the tap request arrives.
    final pos = c.position;
    unawaited(pos.animateTo(60, duration: const Duration(milliseconds: 600), curve: Curves.linear));
    await tester.pump(const Duration(milliseconds: 50));
    expect(pos.isScrollingNotifier.value, isTrue);

    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'id', 'text': '#row3'},
    });
    expect(res['error'], isNull);
    expect(pos.isScrollingNotifier.value, isFalse, reason: 'the tap must not fire mid-scroll');
    expect(taps, 1, reason: 'after the scroll settled the tap lands on the button');
  });
}

void unawaited(Future<void> f) {}
