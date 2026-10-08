import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _see(WidgetTester tester, String text) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
        'selector': {'kind': 'text', 'text': text},
      }))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  testWidgets('a SnackBar under an open dialog is found; a page under an opaque page is not', (tester) async {
    late BuildContext ctx;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: Builder(builder: (c) {
        ctx = c;
        return const Text('Settings page');
      })),
    ));

    showDialog<void>(context: ctx, barrierDismissible: false, builder: (_) => const AlertDialog(title: Text('Export Data')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    ScaffoldMessenger.of(ctx).showSnackBar(const SnackBar(content: Text('No hydration data available')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));

    expect((await _see(tester, 'Export Data'))['error'], isNull);
    expect((await _see(tester, 'No hydration data available'))['error'], isNull,
        reason: 'the snackbar is painted under the see-through dialog barrier');
    expect((await _see(tester, 'Settings page'))['error'], isNull, reason: 'the page under a dialog is still on screen');

    // Push an opaque page over everything: what is under it is no longer on screen.
    Navigator.of(ctx, rootNavigator: true).push(MaterialPageRoute<void>(builder: (_) => const Scaffold(body: Text('Detail page'))));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));
    expect((await _see(tester, 'Detail page'))['error'], isNull);
    expect((await _see(tester, 'Settings page'))['error'], isNotNull, reason: 'hidden under an opaque page');
  });
}
