import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _see(WidgetTester tester, String text, {bool loose = false}) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
        'selector': {'kind': 'text', 'text': text, if (loose) 'loose': true},
      }))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  testWidgets('loose text matching folds case, accents and apostrophes; exact stays exact', (tester) async {
    await tester.pumpWidget(const MaterialApp(
      home: Scaffold(
        body: Column(children: [
          Text('Änderungen speichern'),
          Text('Don’t Allow'),
          Text('Café Crème'),
        ]),
      ),
    ));

    // exact (default): case and accents matter
    expect((await _see(tester, 'anderungen SPEICHERN'))['error'], isNotNull);
    expect((await _see(tester, "Don't Allow"))['error'], isNotNull);
    expect((await _see(tester, 'Änderungen'))['error'], isNull);

    // loose
    expect((await _see(tester, 'anderungen SPEICHERN', loose: true))['error'], isNull);
    expect((await _see(tester, "don't allow", loose: true))['error'], isNull);
    expect((await _see(tester, 'cafe creme', loose: true))['error'], isNull);
    expect((await _see(tester, 'nothing like it', loose: true))['error'], isNotNull);
  });
}
