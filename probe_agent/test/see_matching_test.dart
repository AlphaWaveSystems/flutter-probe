import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _see(WidgetTester tester, String text, {String pattern = ''}) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
        'selector': {'kind': 'text', 'text': text},
        if (pattern.isNotEmpty) 'pattern': pattern,
      }))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  testWidgets('text selectors match substrings; an anchored matching pattern asserts the exact text', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('250 ml'))));

    // substring: "0 ml" is found inside "250 ml"
    expect((await _see(tester, '0 ml'))['error'], isNull);

    // exact form: the same selector with an anchored pattern must fail...
    final bad = await _see(tester, '0 ml', pattern: '^0 ml\$');
    expect(bad['error'], isNotNull, reason: 'the pattern must be applied');
    expect(jsonEncode(bad['error']), contains('matches'));

    // ...and pass for the real text
    expect((await _see(tester, '250 ml', pattern: '^250 ml\$'))['error'], isNull);
  });

  testWidgets('an invalid pattern is reported, not ignored', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('abc'))));
    final res = await _see(tester, 'abc', pattern: '(');
    expect(res['error'], isNotNull);
  });
}
