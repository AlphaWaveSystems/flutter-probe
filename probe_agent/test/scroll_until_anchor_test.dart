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
  for (var i = 0; i < 400 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  testWidgets('scroll down in "<anchor>" until X appears keeps going once the anchor has scrolled away', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView(
          cacheExtent: 0,
          children: [
            const Padding(padding: EdgeInsets.all(16), child: Text('Last 30 Days')),
            for (var i = 0; i < 60; i++) SizedBox(height: 80, child: Text('row $i')),
            const Text('the very end'),
          ],
        ),
      ),
    ));

    final res = await _rpc(tester, ProbeMethods.scroll, {
      'direction': 'down',
      'selector': {'kind': 'text', 'text': 'Last 30 Days'},
      'until': {'kind': 'text', 'text': 'the very end'},
    });
    expect(res['error'], isNull, reason: 'the anchor leaves the screen after the first scroll; the scrollable must be resolved once');
  });
}
