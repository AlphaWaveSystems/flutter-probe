import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

// Issue #842: a tap on a widget inside an IgnorePointer used to pass with no warning while the tap did nothing; even --fail-on-warning
// passed. The hit test skips the widget and stops at the list's own pointer listener, an ancestor that handles pointers, which the agent
// took for a parent GestureDetector around the target.
Future<Map<String, dynamic>> _tap(WidgetTester tester, String text) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(
          jsonrpc: '2.0',
          id: 1,
          method: ProbeMethods.tap,
          params: {
            'selector': {'kind': 'text', 'text': text},
          }))
      .whenComplete(() => done = true);
  for (var i = 0; i < 200 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return (jsonDecode(raw!) as Map<String, dynamic>)['result']
      as Map<String, dynamic>;
}

void main() {
  testWidgets(
      'a tap on a button inside an IgnorePointer in a list warns, and the button gets no tap',
      (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView(children: [
          IgnorePointer(
              child: ElevatedButton(
                  onPressed: () => taps++,
                  child: const Text('Ignored button'))),
        ]),
      ),
    ));
    final result = await _tap(tester, 'Ignored button');
    expect(taps, 0);
    expect(result['warning'],
        contains('is inside an IgnorePointer, so it receives no taps'));
  });

  testWidgets(
      'an IgnorePointer that does not ignore: the tap arrives, no warning',
      (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView(children: [
          IgnorePointer(
              ignoring: false,
              child: ElevatedButton(
                  onPressed: () => taps++, child: const Text('Live button'))),
        ]),
      ),
    ));
    final result = await _tap(tester, 'Live button');
    expect(taps, 1);
    expect(result['warning'], isNull);
  });

  testWidgets(
      'a text inside a parent GestureDetector in a list: the parent gets the tap, no warning',
      (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView(children: [
          GestureDetector(
              onTap: () => taps++,
              child: const Padding(
                  padding: EdgeInsets.all(16), child: Text('Row title'))),
        ]),
      ),
    ));
    final result = await _tap(tester, 'Row title');
    expect(taps, 1);
    expect(result['warning'], isNull);
  });

  testWidgets(
      'an IgnorePointer around the parent GestureDetector: the tap does nothing and warns',
      (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView(children: [
          IgnorePointer(
            child: GestureDetector(
                onTap: () => taps++,
                child: const Padding(
                    padding: EdgeInsets.all(16), child: Text('Frozen row'))),
          ),
        ]),
      ),
    ));
    final result = await _tap(tester, 'Frozen row');
    expect(taps, 0);
    expect(result['warning'], contains('is inside an IgnorePointer'));
  });

  testWidgets('an AbsorbPointer button keeps the covered warning',
      (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView(children: [
          AbsorbPointer(
              child: ElevatedButton(
                  onPressed: () => taps++,
                  child: const Text('Absorbed button'))),
        ]),
      ),
    ));
    final result = await _tap(tester, 'Absorbed button');
    expect(taps, 0);
    expect(result['warning'], contains('is covered by another widget'));
  });

  testWidgets('a tap by id on a button inside an IgnorePointer warns too',
      (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView(children: [
          IgnorePointer(
              child: ElevatedButton(
                  key: const ValueKey('ignored_id'),
                  onPressed: () => taps++,
                  child: const Text('Ignored by id'))),
        ]),
      ),
    ));
    String? raw;
    final ex = ProbeExecutor((s) => raw = s);
    var done = false;
    final call = ex
        .dispatch(ProbeRequest(
            jsonrpc: '2.0',
            id: 1,
            method: ProbeMethods.tap,
            params: {
              'selector': {'kind': 'id', 'text': '#ignored_id'},
            }))
        .whenComplete(() => done = true);
    for (var i = 0; i < 200 && !done; i++) {
      await tester.pump(const Duration(milliseconds: 50));
    }
    await call;
    final result = (jsonDecode(raw!) as Map<String, dynamic>)['result']
        as Map<String, dynamic>;
    expect(taps, 0);
    expect(result['warning'], contains('is inside an IgnorePointer'));
  });

  // The review's case: a tile that takes the tap, whose title is inside an IgnorePointer. The tile does get the tap, so the message must
  // not say the target receives none.
  testWidgets(
      'a ListTile whose title is inside an IgnorePointer gets the tap, and no IgnorePointer message',
      (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView(children: [
          ListTile(
              onTap: () => taps++,
              title: const IgnorePointer(child: Text('Row title'))),
        ]),
      ),
    ));
    final result = await _tap(tester, 'Row title');
    expect(taps, 1);
    expect('${result['warning']}', isNot(contains('inside an IgnorePointer')));
  });
}
