import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _see(WidgetTester tester, String text, {bool negated = false}) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
        'selector': {'kind': 'text', 'text': text},
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
  testWidgets('content of a neighbouring PageView page is not on screen', (tester) async {
    final controller = PageController(keepPage: true);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: PageView(
          controller: controller,
          allowImplicitScrolling: true, // builds the neighbours too
          children: const [
            Center(child: Text('Home page')),
            Center(child: Text('Second page')),
          ],
        ),
      ),
    ));
    await tester.pump();

    expect((await _see(tester, 'Home page'))['error'], isNull);
    expect((await _see(tester, 'Second page', negated: true))['error'], isNull,
        reason: 'the page beside the visible one is laid out off screen');
    expect((await _see(tester, 'Second page'))['error'], isNotNull);

    controller.jumpToPage(1);
    await tester.pumpAndSettle();
    expect((await _see(tester, 'Second page'))['error'], isNull);
  });
}
