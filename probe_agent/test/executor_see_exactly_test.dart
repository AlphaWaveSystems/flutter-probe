import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

// Issue #800: `see exactly N "<text>"` counts each visible text once.
Future<Map<String, dynamic>> _see(
    WidgetTester tester, String text, int count) async {
  String? sent;
  final executor = ProbeExecutor((msg) => sent = msg);
  await executor.dispatch(
      ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
    'selector': {'kind': 'text', 'text': text},
    'count': count,
  }));
  return jsonDecode(sent!) as Map<String, dynamic>;
}

void main() {
  testWidgets('see exactly 1 on one Text passes, and exactly 2 fails',
      (tester) async {
    await tester.pumpWidget(const MaterialApp(
        home: Scaffold(body: Column(children: [Text('Alice'), Text('Bob')]))));
    expect((await _see(tester, 'Alice', 1))['error'], isNull);
    final two = await _see(tester, 'Alice', 2);
    expect(two['error'], isNotNull);
    expect('${two['error']}', contains('found 1'));
  });

  testWidgets('see exactly 2 on two equal Texts passes', (tester) async {
    await tester.pumpWidget(const MaterialApp(
        home: Scaffold(body: Column(children: [Text('Row'), Text('Row')]))));
    expect((await _see(tester, 'Row', 2))['error'], isNull);
  });

  testWidgets('a button label counts once', (tester) async {
    await tester.pumpWidget(MaterialApp(
        home: Scaffold(
            body:
                ElevatedButton(onPressed: () {}, child: const Text('Send')))));
    expect((await _see(tester, 'Send', 1))['error'], isNull);
  });
}
