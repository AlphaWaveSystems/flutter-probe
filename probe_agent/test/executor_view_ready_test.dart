import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_probe_agent/src/view_ready.dart';

// Issue #793: the handshake (probe.ping) of a relaunched app answers only once the view has its real size, and says how long it waited.
void main() {
  tearDown(() => ViewReady.instance.reset());

  testWidgets(
      'a ping during a 0×0 view is answered once the real size is laid out, with view_ready_ms',
      (tester) async {
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(const MaterialApp(home: Text('Home')));
    String? sent;
    final executor = ProbeExecutor((msg) => sent = msg);
    executor.dispatch(ProbeRequest(
        jsonrpc: '2.0', id: 1, method: ProbeMethods.ping, params: const {}));
    await tester.pump(const Duration(milliseconds: 200));
    expect(sent, isNull, reason: 'no answer while the view is 0×0');
    tester.view.physicalSize = const Size(1080, 2400);
    await tester.pump(const Duration(milliseconds: 20));
    await tester.pump(const Duration(milliseconds: 20));
    expect(sent, isNotNull);
    final result = (jsonDecode(sent!) as Map<String, dynamic>)['result']
        as Map<String, dynamic>;
    expect(result['ok'], isTrue);
    expect(result['view_ready_ms'], isA<int>());
    expect(result['capabilities'], contains('waits_for_view_size'));
  });

  testWidgets('a ping with a real size answers at once, with view_ready_ms 0',
      (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Text('Home')));
    String? sent;
    final executor = ProbeExecutor((msg) => sent = msg);
    await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0', id: 2, method: ProbeMethods.ping, params: const {}));
    final result = (jsonDecode(sent!) as Map<String, dynamic>)['result']
        as Map<String, dynamic>;
    expect(result['view_ready_ms'], 0);
  });

  testWidgets(
      'a ping in the gap (real size reported, no frame laid out at it yet) waits for that frame',
      (tester) async {
    ViewReady.instance.attach();
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(const MaterialApp(home: Text('Home')));
    tester.view.physicalSize = const Size(1080, 2400);
    String? sent;
    final executor = ProbeExecutor((msg) => sent = msg);
    executor.dispatch(ProbeRequest(
        jsonrpc: '2.0', id: 3, method: ProbeMethods.ping, params: const {}));
    await tester.binding
        .delayed(const Duration(milliseconds: 100)); // time passes, no frame
    expect(sent, isNull,
        reason: 'no answer before a frame is laid out at the real size');
    await tester.pump();
    await tester.pump();
    expect(sent, isNotNull);
  });
}
