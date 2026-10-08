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
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  testWidgets('tap scrolls a built but off-screen chip into view; see does not find it', (tester) async {
    String? tapped;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: SizedBox(
          height: 80,
          child: ListView(
            scrollDirection: Axis.horizontal,
            cacheExtent: 4000, // build chips far beyond the viewport
            children: [
              for (final name in ['All', 'General', 'For Sale', 'For Rent', 'Land', 'Service', 'Event', 'Alert'])
                Padding(
                  padding: const EdgeInsets.all(8),
                  child: SizedBox(width: 220, child: ElevatedButton(onPressed: () => tapped = name, child: Text(name))),
                ),
            ],
          ),
        ),
      ),
    ));

    final seen = await _rpc(tester, ProbeMethods.see, {
      'selector': {'kind': 'text', 'text': 'Alert'},
    });
    expect(seen['error'], isNotNull, reason: 'a chip beyond the viewport is not visible to see');

    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'text', 'text': 'Alert'},
    });
    expect(res['error'], isNull, reason: 'tap should scroll it into view first');
    expect(tapped, 'Alert');
  });
}
