import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _rpc(WidgetTester tester, String method, Map<String, dynamic> params) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params))
      .whenComplete(() => done = true);
  for (var i = 0; i < 200 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

Widget _covered({required VoidCallback onUnder, required VoidCallback onOver, bool cover = true}) {
  return MaterialApp(
    home: Scaffold(
      body: Stack(children: [
        Center(
          child: GestureDetector(
            key: const ValueKey('target'),
            behavior: HitTestBehavior.opaque,
            onDoubleTap: onUnder,
            onLongPress: onUnder,
            child: const SizedBox(width: 120, height: 60, child: Text('Target')),
          ),
        ),
        if (cover)
          Positioned.fill(
            child: GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTap: onOver,
              child: const SizedBox.expand(),
            ),
          ),
      ]),
    ),
  );
}

void main() {
  for (final verb in [(ProbeMethods.doubleTap, 'double tap'), (ProbeMethods.longPress, 'long press')]) {
    testWidgets('${verb.$2} on a covered target warns, like tap does', (tester) async {
      var under = 0, over = 0;
      await tester.pumpWidget(_covered(onUnder: () => under++, onOver: () => over++));
      final res = await _rpc(tester, verb.$1, {
        'selector': {'kind': 'id', 'text': '#target'},
      });
      expect(res['error'], isNull, reason: 'the action still happens; it is a warning');
      final warning = res['result']['warning'] as String?;
      expect(warning, isNotNull);
      expect(warning, contains(verb.$2));
      expect(warning, contains('covered'));
      expect(warning, contains('#target'));
      expect(under, 0, reason: 'the cover received the gesture, as a real finger would');
    });

    testWidgets('${verb.$2} on a free target carries no warning', (tester) async {
      var under = 0;
      await tester.pumpWidget(_covered(onUnder: () => under++, onOver: () {}, cover: false));
      final res = await _rpc(tester, verb.$1, {
        'selector': {'kind': 'id', 'text': '#target'},
      });
      expect(res['result'].containsKey('warning'), isFalse);
      await tester.pump(const Duration(seconds: 1)); // let the recognizer's timers finish
      expect(under, 1);
    });
  }
}
