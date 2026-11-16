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

/// A 300 by 400 target, keyed #drawer, whose pointer-down positions are recorded in [downs]. A pointer-down is recorded only
/// when it hits the target, so [downs] is the proof that a start lands on the target.
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
      'a swipe right from 10 percent starts 30 points in from the left edge, on the centre line',
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
    expect(downs.single.dx, closeTo(box.left + 30, 0.01)); // 10 percent of the 300-wide target
    expect(downs.single.dy, closeTo(box.center.dy, 0.01));
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
      'a start at 1, 50 and 100 percent hits the target, for each of the four directions',
      (tester) async {
    for (final direction in ['up', 'down', 'left', 'right']) {
      for (final percent in [1, 50, 100]) {
        final downs = <Offset>[];
        await tester.pumpWidget(_target(downs));
        final box = tester.getRect(find.byKey(const ValueKey('drawer')));
        final res = await _dispatch(tester, ProbeMethods.swipe, {
          'direction': direction,
          'selector': {'kind': 'id', 'text': '#drawer'},
          'start_percent': percent,
        });
        expect(res['error'], isNull, reason: '$direction from $percent percent');
        expect(downs, hasLength(1),
            reason: 'a pointer-down that hits nothing: $direction from $percent percent');
        expect(box.contains(downs.single), isTrue,
            reason: '$direction from $percent percent started at ${downs.single}, outside $box');
      }
    }
  });

  testWidgets(
      'ping reports the swipe_from capability, so a CLI can send a swipe with a start to this agent',
      (tester) async {
    final res = await _dispatch(tester, ProbeMethods.ping, {});
    expect(res['result']['capabilities'], contains('swipe_from'));
  });
}
