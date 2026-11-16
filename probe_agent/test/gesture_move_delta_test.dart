import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

/// Sends one request to a fresh executor and returns the response it wrote.
Future<Map<String, dynamic>> _dispatch(WidgetTester tester, String method, Map<String, dynamic> params) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  // A swipe used to send each move with its position and no movement. Flutter's drag recognizers read the movement, so a drag
  // was recognized and moved nothing. These tests fail on that code and pass once each move carries its movement.

  testWidgets('a swipe up over a ListView scrolls it by half its height, the default distance', (tester) async {
    final controller = ScrollController();
    addTearDown(controller.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: SizedBox(
          height: 300,
          child: ListView.builder(
            key: const ValueKey('list'),
            controller: controller,
            itemCount: 100,
            itemBuilder: (c, i) => SizedBox(height: 50, child: Text('row $i')),
          ),
        ),
      ),
    ));
    final res = await _dispatch(tester, ProbeMethods.swipe, {
      'direction': 'up',
      'selector': {'kind': 'id', 'text': '#list'},
    });
    expect(res['error'], isNull);
    // The default swipe travels 50 percent of the 300-high list: the offset moves by 150.
    expect(controller.offset, closeTo(150, 1));
  });

  testWidgets('each move of a swipe carries its own movement, and the moves add up to the whole travel', (tester) async {
    final moves = <Offset>[];
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Center(
          child: Listener(
            onPointerMove: (e) => moves.add(e.delta),
            child: Container(key: const ValueKey('pad'), width: 300, height: 400, color: Colors.blue),
          ),
        ),
      ),
    ));
    final res = await _dispatch(tester, ProbeMethods.swipe, {
      'direction': 'up',
      'selector': {'kind': 'id', 'text': '#pad'},
    });
    expect(res['error'], isNull);
    expect(moves.length, greaterThan(1));
    expect(moves.every((d) => d.dy < 0), isTrue, reason: 'every move of an upward swipe goes up');
    final total = moves.fold(Offset.zero, (sum, d) => sum + d);
    // 50 percent of the 400-high element.
    expect(total.dy, closeTo(-200, 0.5));
    expect(total.dx, closeTo(0, 0.001));
  });

  testWidgets('a horizontal swipe over a PageView turns the page', (tester) async {
    final controller = PageController();
    addTearDown(controller.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: PageView(
          key: const ValueKey('pager'),
          controller: controller,
          children: [Container(color: Colors.red), Container(color: Colors.green)],
        ),
      ),
    ));
    final res = await _dispatch(tester, ProbeMethods.swipe, {
      'direction': 'left',
      'selector': {'kind': 'id', 'text': '#pager'},
    });
    expect(res['error'], isNull);
    await tester.pumpAndSettle();
    expect(controller.page, closeTo(1, 0.01));
  });

  testWidgets('a horizontal swipe over a Dismissible row dismisses it', (tester) async {
    var dismissed = false;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Center(
          child: SizedBox(
            width: 200,
            height: 100,
            child: Dismissible(
              key: const ValueKey('row'),
              onDismissed: (_) => dismissed = true,
              child: Container(color: Colors.orange),
            ),
          ),
        ),
      ),
    ));
    final res = await _dispatch(tester, ProbeMethods.swipe, {
      'direction': 'left',
      'selector': {'kind': 'id', 'text': '#row'},
    });
    expect(res['error'], isNull);
    await tester.pumpAndSettle();
    expect(dismissed, isTrue);
  });

  testWidgets('a swipe over something that does not scroll still completes, and ends where it always did', (tester) async {
    Offset? up;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Center(
          child: Listener(
            onPointerUp: (e) => up = e.position,
            child: Container(key: const ValueKey('plain'), width: 300, height: 400, color: Colors.grey),
          ),
        ),
      ),
    ));
    final start = tester.getCenter(find.byKey(const ValueKey('plain')));
    final res = await _dispatch(tester, ProbeMethods.swipe, {
      'direction': 'up',
      'selector': {'kind': 'id', 'text': '#plain'},
    });
    expect(res['error'], isNull);
    expect(up, isNotNull);
    // The release is where the swipe's travel ends: 50 percent of the element's height above the centre.
    expect(up!.dy, closeTo(start.dy - 200, 0.5));
    expect(up!.dx, closeTo(start.dx, 0.001));
  });
}
