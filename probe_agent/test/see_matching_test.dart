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
  // The wait loop uses the real clock (DateTime.now is not faked), so let real
  // time pass between pumps.
  for (var i = 0; i < 200 && !done; i++) {
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 20)));
    await tester.pump(const Duration(milliseconds: 20));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  waitMatchingTests();
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

Future<Map<String, dynamic>> _wait(WidgetTester tester, String target, {String pattern = ''}) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.wait, params: {
        'kind': 'appears',
        'target': target,
        'timeout': 0.4,
        if (pattern.isNotEmpty) 'pattern': pattern,
      }))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void waitMatchingTests() {
  testWidgets('wait until appears matching succeeds when the text matches the pattern', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('250 ml'))));
    expect((await _wait(tester, '0 ml'))['error'], isNull, reason: 'substring match');
    expect((await _wait(tester, '250 ml', pattern: '^250 ml\$'))['error'], isNull);
  });

  testWidgets('a tooltip or Semantics label is found when no text shows it', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          IconButton(tooltip: 'Show password', onPressed: () {}, icon: const Icon(Icons.visibility)),
          Semantics(label: 'Profile photo', child: const SizedBox(width: 20, height: 20)),
          const Text('Visible text'),
        ]),
      ),
    ));
    expect((await _see(tester, 'Show password'))['error'], isNull);
    expect((await _see(tester, 'Profile photo'))['error'], isNull);
    expect((await _see(tester, 'Nothing like this'))['error'], isNotNull);
  });
}
