import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/finder.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

/// Dispatches one request and returns the decoded JSON-RPC response.
Future<Map<String, dynamic>> _rpc(String method, Map<String, dynamic> params) async {
  String? raw;
  final executor = ProbeExecutor((s) => raw = s);
  await executor.dispatch(
    ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params),
  );
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  group('visible summary for failure diagnostics (FP-13)', () {
    testWidgets('lists visible texts and string keys, skips offstage content',
        (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(
            children: [
              const Text('Quick Add'),
              ElevatedButton(
                key: const ValueKey('nav_tab_history'),
                onPressed: () {},
                child: const Text('History'),
              ),
              const Offstage(offstage: true, child: Text('Hidden thing')),
            ],
          ),
        ),
      ));

      final s = ProbeFinder.instance.visibleSummary();
      expect(s['texts'], containsAll(['Quick Add', 'History']));
      expect(s['texts'], isNot(contains('Hidden thing')));
      expect(s['keys'], contains('nav_tab_history'));
    });

    testWidgets('is exposed over the visible_summary RPC', (tester) async {
      await tester.pumpWidget(const MaterialApp(
        home: Scaffold(body: Text('Hello')),
      ));
      final res = await _rpc(ProbeMethods.visibleSummary, const {});
      expect(res['result']['texts'], contains('Hello'));
    });

    testWidgets('widget-not-found errors carry what the screen did show',
        (tester) async {
      await tester.pumpWidget(const MaterialApp(
        home: Scaffold(body: Text('Settings')),
      ));
      final res = await _rpc(ProbeMethods.tap, {
        'selector': {'kind': 'text', 'text': 'Nope'},
      });
      final message = res['error']['message'] as String;
      expect(message, contains('Widget not found'));
      expect(message, contains('"Settings"'));
    });
  });
}
