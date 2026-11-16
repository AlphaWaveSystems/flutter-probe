import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

/// Sends one request to a fresh executor and returns the response it wrote.
Future<Map<String, dynamic>> _dispatch(
    WidgetTester tester, String method, Map<String, dynamic> params) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(
          ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

/// A 300 by 400 target whose pointer-down positions are recorded in [downs].
Widget _target(List<Offset> downs) {
  return MaterialApp(
    home: Scaffold(
      body: Center(
        child: Listener(
          onPointerDown: (e) => downs.add(e.position),
          child: Container(
              key: const ValueKey('drawer'),
              width: 300,
              height: 400,
              color: Colors.blue),
        ),
      ),
    ),
  );
}

void main() {
  testWidgets(
      'a swipe from 10 percent starts 10 percent in from the edge it starts at',
      (tester) async {
    final downs = <Offset>[];
    await tester.pumpWidget(_target(downs));
    final box = tester.getRect(find.byKey(const ValueKey('drawer')));
    final res = await _dispatch(tester, ProbeMethods.swipe, {
      'direction': 'right',
      'selector': {'kind': 'id', 'text': '#drawer'},
      'start_percent': 10,
    });
    expect(res['error'], isNull);
    expect(downs, hasLength(1));
    expect(downs.single, swipeStart('right', box.topLeft, box.size, 10));
  });

  testWidgets(
      'a swipe without a start begins at the centre of the target, as before',
      (tester) async {
    final downs = <Offset>[];
    await tester.pumpWidget(_target(downs));
    final box = tester.getRect(find.byKey(const ValueKey('drawer')));
    final res = await _dispatch(tester, ProbeMethods.swipe, {
      'direction': 'right',
      'selector': {'kind': 'id', 'text': '#drawer'},
    });
    expect(res['error'], isNull);
    expect(downs, hasLength(1));
    expect(downs.single, box.center);
  });

  testWidgets(
      'ping reports the swipe_from capability, so a CLI can send a swipe with a start to this agent',
      (tester) async {
    final res = await _dispatch(tester, ProbeMethods.ping, {});
    expect(res['result']['capabilities'], contains('swipe_from'));
  });
}
