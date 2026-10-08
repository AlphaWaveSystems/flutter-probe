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
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  testWidgets('a bare type goes to the focused field, not the first one', (tester) async {
    final email = TextEditingController(), password = TextEditingController();
    final passwordFocus = FocusNode();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          TextField(key: const ValueKey('email'), controller: email),
          TextField(key: const ValueKey('password'), controller: password, focusNode: passwordFocus),
        ]),
      ),
    ));
    passwordFocus.requestFocus();
    await tester.pump();

    final res = await _rpc(tester, ProbeMethods.type_, {
      'selector': {'kind': 'text', 'text': ''},
      'text': 'secret',
    });
    expect(res['error'], isNull);
    expect(password.text, 'secret');
    expect(email.text, isEmpty, reason: 'the first field must not receive the text');

    final cleared = await _rpc(tester, ProbeMethods.clear, {
      'selector': {'kind': 'text', 'text': ''},
    });
    expect(cleared['error'], isNull);
    expect(password.text, isEmpty);
  });

  testWidgets('a bare type with nothing focused fails with a clear message', (tester) async {
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: TextField(controller: TextEditingController()))));
    final res = await _rpc(tester, ProbeMethods.type_, {
      'selector': {'kind': 'text', 'text': ''},
      'text': 'x',
    });
    expect(jsonEncode(res['error']), contains('no text field has focus'));
  });
}
