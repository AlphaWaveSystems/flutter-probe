import 'dart:convert';

import 'package:flutter/gestures.dart';
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

// The follow-ups to the swipe delta fix: a drag that changes what the widget under it does, a slider that follows the pointer,
// a swipe that stops where its moves put it, and one pointer-up per gesture.

/// Counts the pointer-up events the binding sees, from the start of the test. The returned function gives the count.
int Function() _countPointerUps() {
  var ups = 0;
  void route(PointerEvent event) {
    if (event is PointerUpEvent) {
      ups++;
    }
  }

  GestureBinding.instance.pointerRouter.addGlobalRoute(route);
  addTearDown(() => GestureBinding.instance.pointerRouter.removeGlobalRoute(route));
  return () => ups;
}

void main() {
  testWidgets('a drag step from a Draggable to a DragTarget is accepted by the target (it was not before the delta fix)',
      (tester) async {
    var accepted = false;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          Draggable<String>(
            data: 'item',
            feedback: Container(width: 50, height: 50, color: Colors.red),
            child: Container(key: const ValueKey('source'), width: 80, height: 80, color: Colors.blue),
          ),
          const SizedBox(height: 200),
          DragTarget<String>(
            key: const ValueKey('target'),
            onAcceptWithDetails: (_) => accepted = true,
            builder: (context, candidate, rejected) => Container(width: 120, height: 120, color: Colors.green),
          ),
        ]),
      ),
    ));
    final res = await _dispatch(tester, ProbeMethods.drag, {
      'from': {'kind': 'id', 'text': '#source'},
      'to': {'kind': 'id', 'text': '#target'},
    });
    expect(res['error'], isNull);
    await tester.pumpAndSettle();
    expect(accepted, isTrue);
  });

  testWidgets('a swipe over a Slider moves its value, the way a drag over it does', (tester) async {
    var value = 0.0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Center(
          child: StatefulBuilder(
            builder: (context, setState) => SizedBox(
              width: 300,
              child: Slider(
                key: const ValueKey('slider'),
                value: value,
                onChanged: (v) => setState(() => value = v),
              ),
            ),
          ),
        ),
      ),
    ));
    final res = await _dispatch(tester, ProbeMethods.swipe, {
      'direction': 'right',
      'selector': {'kind': 'id', 'text': '#slider'},
    });
    expect(res['error'], isNull);
    await tester.pumpAndSettle();
    // A tap at the centre sets the value to about 0.5; the movement to the right end takes it past 0.8.
    expect(value, greaterThan(0.8), reason: 'the slider followed the pointer to the right; it stops at about 0.5 without the movement');
  });

  testWidgets('a swipe leaves the list where its moves put it: the offset after the step is the offset after settling',
      (tester) async {
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
    final afterStep = controller.offset;
    await tester.pumpAndSettle();
    // No fling: the release carries no velocity, so the list does not go on moving after the step returns.
    expect(controller.offset, closeTo(afterStep, 0.01));
    expect(afterStep, closeTo(150, 1));
  });

  testWidgets('a swipe sends exactly one pointer-up', (tester) async {
    // Counted on the binding's global route, which sees every event: a second up after the first is not routed to a widget.
    final ups = _countPointerUps();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Center(
          child: Container(key: const ValueKey('pad'), width: 300, height: 400, color: Colors.blue),
        ),
      ),
    ));
    final res = await _dispatch(tester, ProbeMethods.swipe, {
      'direction': 'up',
      'selector': {'kind': 'id', 'text': '#pad'},
    });
    expect(res['error'], isNull);
    expect(ups(), 1);
  });

  testWidgets('a drag step sends exactly one pointer-up', (tester) async {
    final ups = _countPointerUps();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          Container(key: const ValueKey('from'), width: 80, height: 80, color: Colors.blue),
          const SizedBox(height: 200),
          Container(key: const ValueKey('to'), width: 80, height: 80, color: Colors.green),
        ]),
      ),
    ));
    final res = await _dispatch(tester, ProbeMethods.drag, {
      'from': {'kind': 'id', 'text': '#from'},
      'to': {'kind': 'id', 'text': '#to'},
    });
    expect(res['error'], isNull);
    expect(ups(), 1);
  });
}
