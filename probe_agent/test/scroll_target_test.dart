import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

void main() {
  testWidgets('a bare scroll moves the page inside a non-scrollable PageView, not the PageView', (tester) async {
    final inner = ScrollController();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: PageView(
          physics: const NeverScrollableScrollPhysics(),
          children: [
            SingleChildScrollView(
              controller: inner,
              child: Column(children: const [SizedBox(height: 2400, child: Text('Top')), Text('Bottom')]),
            ),
          ],
        ),
      ),
    ));

    String? raw;
    final ex = ProbeExecutor((s) => raw = s);
    var done = false;
    final call = ex
        .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.scroll, params: {'direction': 'down'}))
        .whenComplete(() => done = true);
    for (var i = 0; i < 100 && !done; i++) {
      await tester.pump(const Duration(milliseconds: 50));
    }
    await call;
    expect(jsonDecode(raw!)['error'], isNull);
    expect(inner.offset, greaterThan(0), reason: 'the scrollable page must have moved');
  });
}
