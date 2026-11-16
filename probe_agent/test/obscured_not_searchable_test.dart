import 'dart:convert';

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_test/flutter_test.dart';

// The value of an obscured field (a password) is never a match target of any selector. A selector that could find a field by the text it holds would let a test
// learn the value by asking about it, one prefix at a time. Synthetic values only.
const _secret = 'zq-obscured-7731';
const _sentence = "an obscured field's value is never searched";

Future<Map<String, dynamic>> _call(WidgetTester tester, ProbeExecutor ex, String method, Map<String, dynamic> params) async {
  String? raw;
  final e = ProbeExecutor((s) => raw = s);
  var done = false;
  final c = e.dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params)).whenComplete(() => done = true);
  for (var i = 0; i < 120 && !done; i++) {
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 10)));
    await tester.pump(const Duration(milliseconds: 20));
  }
  await c;
  final json = jsonDecode(raw!) as Map<String, dynamic>;
  return {'raw': raw!, 'error': json['error']};
}

Map<String, dynamic> _text(String t) => {'kind': 'text', 'text': t};
Map<String, dynamic> _id(String t) => {'kind': 'id', 'text': t};

void main() {
  late ProbeExecutor ex;
  setUp(() => ex = ProbeExecutor((_) {}));

  Future<void> expectNoSelectorFinds(WidgetTester tester, String text) async {
    final see = await _call(tester, ex, ProbeMethods.see, {'selector': _text(text)});
    expect(see['error'], isNotNull, reason: 'see "$text" must not find an obscured field by its value');
    final bounds = await _call(tester, ex, ProbeMethods.selectorBounds, {'selector': _text(text)});
    expect(bounds['error'], isNotNull, reason: 'selector_bounds "$text"');
    final wait = await _call(tester, ex, ProbeMethods.wait, {'kind': 'appears', 'target': text, 'timeout': 0.2});
    expect(wait['error'], isNotNull, reason: 'wait until "$text" appears');
    final tap = await _call(tester, ex, ProbeMethods.tap, {'selector': _text(text)});
    expect(tap['error'], isNotNull, reason: 'tap "$text"');
    for (final r in [see, bounds, wait, tap]) {
      // an error repeats what the caller asked for; take that out, and nothing of the value is left
      expect((r['raw'] as String).replaceAll(text, '<asked>'), isNot(contains(_secret)), reason: 'a response must not carry the value');
    }
  }

  testWidgets('a Material obscured field is not found by its value, whole or by a prefix', (tester) async {
    final pw = TextEditingController(text: _secret);
    addTearDown(pw.dispose);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: TextField(key: const ValueKey('pw'), controller: pw, obscureText: true))));
    await expectNoSelectorFinds(tester, _secret);
    await expectNoSelectorFinds(tester, 'zq-obscured');
    await expectNoSelectorFinds(tester, 'z');
  });

  testWidgets('a Cupertino obscured field is not found by its value either', (tester) async {
    final pw = TextEditingController(text: _secret);
    addTearDown(pw.dispose);
    await tester.pumpWidget(CupertinoApp(home: CupertinoPageScaffold(child: CupertinoTextField(key: const ValueKey('cpw'), controller: pw, obscureText: true))));
    await expectNoSelectorFinds(tester, _secret);
    await expectNoSelectorFinds(tester, 'zq-obscured');
  });

  testWidgets('what is not the value still finds the field and stays matchable: its id, its hint, a plain field, a Text that mirrors', (tester) async {
    final pw = TextEditingController(text: _secret);
    final plain = TextEditingController(text: 'plain-visible-text');
    addTearDown(pw.dispose);
    addTearDown(plain.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          TextField(key: const ValueKey('pw'), controller: pw, obscureText: true, decoration: const InputDecoration(labelText: 'Password label')),
          TextField(key: const ValueKey('plain'), controller: plain),
          Text('Password length: ${pw.text.length}'),
        ]),
      ),
    ));
    expect((await _call(tester, ex, ProbeMethods.see, {'selector': _id('#pw')}))['error'], isNull, reason: 'the field is found by its id');
    expect((await _call(tester, ex, ProbeMethods.see, {'selector': _text('Password label')}))['error'], isNull, reason: 'by its label');
    expect((await _call(tester, ex, ProbeMethods.see, {'selector': _text('plain-visible-text')}))['error'], isNull, reason: 'a plain field is matched by its text');
    expect((await _call(tester, ex, ProbeMethods.see, {'selector': _text('Password length: 16')}))['error'], isNull, reason: 'a Text is just text');
  });

  testWidgets('the failure message says it once, the same whether or not the asked text would have matched', (tester) async {
    final pw = TextEditingController(text: _secret);
    addTearDown(pw.dispose);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: Column(children: [TextField(key: const ValueKey('pw'), controller: pw, obscureText: true), const Text('Other')]))));
    final hit = jsonEncode((await _call(tester, ex, ProbeMethods.see, {'selector': _text(_secret)}))['error']);
    final miss = jsonEncode((await _call(tester, ex, ProbeMethods.see, {'selector': _text('not-the-value-9')}))['error']);
    expect(hit, contains(_sentence));
    expect(miss, contains(_sentence));
    // nothing tells the two apart but the text that was asked
    expect(hit.replaceAll(_secret, 'X'), equals(miss.replaceAll('not-the-value-9', 'X')));
  });

  testWidgets('a screen with no obscured field does not carry the sentence', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('Hello'))));
    final r = jsonEncode((await _call(tester, ex, ProbeMethods.see, {'selector': _text('nothing-here')}))['error']);
    expect(r, isNot(contains(_sentence)));
  });

  testWidgets('a keyed field that was seen obscured stays secret when it is rebuilt with a new controller that is not obscured', (tester) async {
    final first = TextEditingController(text: 'x');
    final second = TextEditingController(text: _secret);
    addTearDown(first.dispose);
    addTearDown(second.dispose);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: TextField(key: const ValueKey('pw'), controller: first, obscureText: true))));
    await _call(tester, ex, ProbeMethods.see, {'selector': _id('#pw')}); // seen obscured
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: TextField(key: const ValueKey('pw'), controller: second))));
    await expectNoSelectorFinds(tester, _secret);
  });

  testWidgets('the stated limit: a field that was never seen obscured is ordinary text, so its value matches', (tester) async {
    final shown = TextEditingController(text: 'visible-from-the-start');
    addTearDown(shown.dispose);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: TextField(key: const ValueKey('show'), controller: shown))));
    expect((await _call(tester, ex, ProbeMethods.see, {'selector': _text('visible-from-the-start')}))['error'], isNull);
  });
}
