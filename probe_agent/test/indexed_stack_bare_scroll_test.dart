import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _rpc(WidgetTester tester, String method, Map<String, dynamic> params) async {
  String? raw;
  final executor = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = executor
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

Widget _list(ScrollController c, String prefix, int count) => ListView.builder(
      controller: c,
      itemCount: count,
      itemBuilder: (_, i) => SizedBox(height: 48, child: Text('$prefix $i')),
    );

void main() {
  // The bare `scroll down` picks the scrollable that can be scrolled and receives touches. When the page an IndexedStack shows has
  // a list too short to scroll, a longer list on a page that is not shown ranked higher on "can be scrolled" and was the one moved,
  // so nothing on screen changed (the case FP-16 did not cover: there both pages had a long list).
  testWidgets('a bare scroll does not move a list on a page the IndexedStack does not show', (tester) async {
    final hidden = ScrollController();
    final shown = ScrollController();
    addTearDown(hidden.dispose);
    addTearDown(shown.dispose);

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: IndexedStack(
          index: 1,
          children: [_list(hidden, 'hidden', 100), _list(shown, 'shown', 2)],
        ),
      ),
    ));

    final res = await _rpc(tester, ProbeMethods.scroll, {'direction': 'down'});
    await tester.pump();
    expect(res['error'], isNull);
    expect(hidden.offset, 0, reason: 'the list of a page that is not shown must be left alone');
  });

  testWidgets('the same with the page that is not shown after the shown one', (tester) async {
    final shown = ScrollController();
    final hidden = ScrollController();
    addTearDown(shown.dispose);
    addTearDown(hidden.dispose);

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: IndexedStack(
          index: 0,
          children: [_list(shown, 'shown', 2), _list(hidden, 'hidden', 100)],
        ),
      ),
    ));

    await _rpc(tester, ProbeMethods.scroll, {'direction': 'down'});
    await tester.pump();
    expect(hidden.offset, 0);
  });

  testWidgets('an IndexedStack that shows nothing (index null) gives a bare scroll no list to move', (tester) async {
    final hidden = ScrollController();
    addTearDown(hidden.dispose);

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: IndexedStack(index: null, children: [_list(hidden, 'hidden', 100)])),
    ));

    await _rpc(tester, ProbeMethods.scroll, {'direction': 'down'});
    await tester.pump();
    expect(hidden.offset, 0);
  });

  testWidgets('the list on the shown page of a stack is the one that scrolls', (tester) async {
    final outer = ScrollController();
    final inner = ScrollController();
    addTearDown(outer.dispose);
    addTearDown(inner.dispose);

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: IndexedStack(index: 0, children: [_list(inner, 'inner', 100), _list(outer, 'other', 100)]),
      ),
    ));

    await _rpc(tester, ProbeMethods.scroll, {'direction': 'down'});
    await tester.pump();
    expect(inner.offset, greaterThan(0));
    expect(outer.offset, 0);
  });
}
