import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

/// Runs one RPC while driving frames: dispatch waits for the app to settle, which
/// only happens if the (fake-async) test pumps.
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

Widget _list(ScrollController c, String prefix) => ListView.builder(
      controller: c,
      itemCount: 100,
      itemBuilder: (_, i) => SizedBox(height: 48, child: Text('$prefix $i')),
    );

void main() {
  // FP-16 (reported from a real app): a bare `scroll down` picked the first of two
  // equally sized scrollables — the hidden tab of an IndexedStack — so the visible
  // list never moved.
  testWidgets('a bare scroll moves the visible tab, not a hidden IndexedStack tab',
      (tester) async {
    final hidden = ScrollController();
    final visible = ScrollController();
    addTearDown(hidden.dispose);
    addTearDown(visible.dispose);

    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: IndexedStack(
          index: 1,
          children: [_list(hidden, 'hidden'), _list(visible, 'visible')],
        ),
      ),
    ));

    final res = await _rpc(tester, ProbeMethods.scroll, {'direction': 'down'});
    expect(res['error'], isNull);
    await tester.pump();

    expect(visible.offset, greaterThan(0), reason: 'the list the user sees must scroll');
    expect(hidden.offset, 0, reason: 'the hidden tab must be left alone');
  });

  testWidgets('with a single scrollable nothing changes', (tester) async {
    final c = ScrollController();
    addTearDown(c.dispose);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: _list(c, 'only'))));
    await _rpc(tester, ProbeMethods.scroll, {'direction': 'down'});
    await tester.pump();
    expect(c.offset, greaterThan(0));
  });

  testWidgets('id selectors are shown once, as written, in errors', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('x'))));
    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'id', 'text': '#settings_screen'},
    });
    final message = res['error']['message'] as String;
    expect(message, contains('#settings_screen'));
    expect(message, isNot(contains('id("#')));
  });
}
