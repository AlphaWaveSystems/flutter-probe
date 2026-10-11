import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _see(WidgetTester tester, String text, String check) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
        'selector': {'kind': 'text', 'text': text},
        'check': check,
      }))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  testWidgets('is disabled / is enabled read the control that labels the text', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          const SizedBox(child: ElevatedButton(onPressed: null, child: Text('Submit'))),
          ElevatedButton(onPressed: () {}, child: const Text('Save')),
          const TextField(enabled: false, decoration: InputDecoration(hintText: 'Locked')),
          Switch(value: false, onChanged: null),
          IconButton(tooltip: 'Edit', onPressed: null, icon: const Icon(Icons.edit)),
        ]),
      ),
    ));
    expect((await _see(tester, 'Submit', 'disabled'))['error'], isNull);
    expect((await _see(tester, 'Submit', 'enabled'))['error'], isNotNull);
    expect((await _see(tester, 'Save', 'enabled'))['error'], isNull);
    expect((await _see(tester, 'Save', 'disabled'))['error'], isNotNull);
    expect((await _see(tester, 'Edit', 'disabled'))['error'], isNull);
  });
}
