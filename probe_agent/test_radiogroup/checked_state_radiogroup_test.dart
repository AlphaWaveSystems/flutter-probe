// Needs Flutter 3.35 or newer, where `RadioGroup` is a framework widget. It is not under test/ because `flutter test` there also runs on older Flutter
// (the CI pin), where this file would not compile. Run it by hand: `cd probe_agent && flutter test test_radiogroup`.
// The same behaviour with a stand-in group is in test/checked_state_radio_test.dart, which runs everywhere.

import 'dart:convert';

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_test/flutter_test.dart';

Future<Map<String, dynamic>> _see(WidgetTester tester, String selector, {bool negated = false}) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final isId = selector.startsWith('#');
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
        'selector': {'kind': isId ? 'id' : 'text', 'text': selector},
        'check': 'checked',
        'negated': negated,
      }))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

Future<bool> _checked(WidgetTester tester, String selector) async => (await _see(tester, selector))['error'] == null;

void main() {
  testWidgets('under the real RadioGroup the selected RadioListTile is checked and the others are not', (tester) async {
    var size = 'medium';
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: StatefulBuilder(
          builder: (context, setState) => RadioGroup<String>(
            groupValue: size,
            onChanged: (v) => setState(() => size = v ?? size),
            child: const Column(children: [
              RadioListTile<String>(key: ValueKey('small'), title: Text('Small'), value: 'small'),
              RadioListTile<String>(key: ValueKey('medium'), title: Text('Medium'), value: 'medium'),
              RadioListTile<String>(key: ValueKey('large'), title: Text('Large'), value: 'large'),
            ]),
          ),
        ),
      ),
    ));
    expect(await _checked(tester, 'Medium'), isTrue);
    expect(await _checked(tester, 'Small'), isFalse);
    expect(await _checked(tester, '#large'), isFalse);

    // a tap moves the selection, and the check follows it
    await tester.tap(find.text('Large'));
    await tester.pump();
    expect(await _checked(tester, 'Large'), isTrue);
    expect(await _checked(tester, 'Medium'), isFalse);
  });

  testWidgets('under the real RadioGroup a bare Radio and a CupertinoRadio are read from the group too', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          RadioGroup<int>(
            groupValue: 2,
            onChanged: (_) {},
            child: const Column(children: [
              Radio<int>(key: ValueKey('one'), value: 1),
              Radio<int>(key: ValueKey('two'), value: 2),
            ]),
          ),
          // a group holds one selection, so the Cupertino radios have a group of their own
          RadioGroup<int>(
            groupValue: 2,
            onChanged: (_) {},
            child: const Column(children: [
              CupertinoRadio<int>(key: ValueKey('c_two'), value: 2),
              CupertinoRadio<int>(key: ValueKey('c_three'), value: 3),
            ]),
          ),
        ]),
      ),
    ));
    expect(await _checked(tester, '#two'), isTrue);
    expect(await _checked(tester, '#one'), isFalse);
    expect(await _checked(tester, '#c_two'), isTrue);
    expect(await _checked(tester, '#c_three'), isFalse);
  });

  testWidgets('a RadioGroup with no selection checks none of its radios', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: RadioGroup<String>(
          groupValue: null,
          onChanged: (_) {},
          child: const Column(children: [
            RadioListTile<String>(key: ValueKey('a'), title: Text('Alpha'), value: 'a'),
            RadioListTile<String>(key: ValueKey('b'), title: Text('Beta'), value: 'b'),
          ]),
        ),
      ),
    ));
    expect(await _checked(tester, 'Alpha'), isFalse);
    expect(await _checked(tester, 'Beta'), isFalse);
  });
}
