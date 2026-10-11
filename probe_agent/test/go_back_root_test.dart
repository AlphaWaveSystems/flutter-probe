import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _goBack(WidgetTester tester) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(
          jsonrpc: '2.0', id: 1, method: ProbeMethods.deviceAction, params: {'action': 'go_back', 'value': ''}))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  testWidgets('go back at the root route does not leave the app and says so', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('Home'))));
    final res = await _goBack(tester);
    expect(res['error'], isNull);
    expect(res['result']['warning'], contains('root route'));
  });

  testWidgets('go back pops a pushed route without a warning', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Builder(
        builder: (c) => Scaffold(
          body: TextButton(
            onPressed: () => Navigator.of(c).push(MaterialPageRoute(builder: (_) => const Scaffold(body: Text('Detail')))),
            child: const Text('Open'),
          ),
        ),
      ),
    ));
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
    expect(find.text('Detail'), findsOneWidget);
    final res = await _goBack(tester);
    await tester.pumpAndSettle();
    expect(res['result']['warning'], isNull);
    expect(find.text('Detail'), findsNothing);
  });
}
