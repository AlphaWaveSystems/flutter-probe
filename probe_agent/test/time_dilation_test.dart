import 'dart:convert';

import 'package:flutter/scheduler.dart' show timeDilation;
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

void main() {
  testWidgets('set_time_dilation never sets an invalid (zero) dilation', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('x'))));
    String? raw;
    final ex = ProbeExecutor((s) => raw = s);
    var done = false;
    final call = ex
        .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.setTimeDilation, params: {'factor': 0}))
        .whenComplete(() => done = true);
    for (var i = 0; i < 20 && !done; i++) {
      await tester.pump(const Duration(milliseconds: 50));
    }
    await call;
    expect(jsonDecode(raw!)['error'], isNull);
    final applied = timeDilation;
    timeDilation = 1.0; // the test framework requires it to be reset before the test ends
    expect(applied, greaterThan(0), reason: 'Flutter requires timeDilation > 0');
  });
}
