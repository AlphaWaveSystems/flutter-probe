import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

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

void main() {
  testWidgets('is checked reads the value of Switch, SwitchListTile, Checkbox and Radio', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          Switch(key: const ValueKey('sw_off'), value: false, onChanged: (_) {}),
          Switch(key: const ValueKey('sw_on'), value: true, onChanged: (_) {}),
          SwitchListTile(key: const ValueKey('tile_off'), value: false, onChanged: (_) {}, title: const Text('Analytics')),
          SwitchListTile(key: const ValueKey('tile_on'), value: true, onChanged: (_) {}, title: const Text('Reminders')),
          Checkbox(key: const ValueKey('cb_on'), value: true, onChanged: (_) {}),
          const Text('Plain label'),
        ]),
      ),
    ));

    // positive: checked controls pass, unchecked fail
    expect((await _see(tester, '#sw_on'))['error'], isNull);
    expect((await _see(tester, '#sw_off'))['error'], isNotNull, reason: 'an OFF switch is not checked');
    expect((await _see(tester, '#tile_on'))['error'], isNull);
    expect((await _see(tester, '#tile_off'))['error'], isNotNull);
    expect((await _see(tester, '#cb_on'))['error'], isNull);

    // through the label text of a tile
    expect((await _see(tester, 'Reminders'))['error'], isNull);
    expect((await _see(tester, 'Analytics'))['error'], isNotNull);

    // negated: don't see ... is checked
    expect((await _see(tester, '#sw_off', negated: true))['error'], isNull);
    expect((await _see(tester, '#sw_on', negated: true))['error'], isNotNull);
    expect((await _see(tester, '#tile_off', negated: true))['error'], isNull);

    // a widget that cannot be checked says so instead of passing
    final plain = await _see(tester, 'Plain label');
    expect(jsonEncode(plain['error']), contains('not a checkable control'));
  });
}
