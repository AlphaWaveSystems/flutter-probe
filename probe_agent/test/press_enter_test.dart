import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _pressEnter(WidgetTester tester) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(
          jsonrpc: '2.0', id: 1, method: ProbeMethods.deviceAction, params: {'action': 'press_enter', 'value': ''}))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  testWidgets('press enter submits the focused text field', (tester) async {
    String? submitted;
    final focus = FocusNode();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: TextField(focusNode: focus, onSubmitted: (v) => submitted = v, controller: TextEditingController(text: 'a@b.c')),
      ),
    ));
    focus.requestFocus();
    await tester.pump();
    final res = await _pressEnter(tester);
    expect(res['error'], isNull);
    expect(res['result']['warning'], isNull);
    expect(submitted, 'a@b.c');
  });

  testWidgets('press enter with no focused field warns instead of failing', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('Home'))));
    final res = await _pressEnter(tester);
    expect(res['error'], isNull);
    expect(res['result']['warning'], contains('no text field has focus'));
  });
}
