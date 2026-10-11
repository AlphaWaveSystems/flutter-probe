import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/src/perf.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('measuring window reports memory and frames, and stops cleanly', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Center(child: Text('x')))));
    final perf = ProbePerf.instance;
    perf.start();
    expect(perf.measuring, isTrue);
    await tester.pump(const Duration(milliseconds: 300));
    final snap = perf.snapshot();
    expect(snap['rssStartBytes'], isA<int>());
    expect((snap['rssPeakBytes'] as int) >= (snap['rssStartBytes'] as int), isTrue);
    expect(snap['measuring'], isTrue);
    perf.stopWindow();
    expect(perf.measuring, isFalse);
    expect(perf.snapshot()['measuring'], isFalse);
  });

  test('a second start resets the window', () {
    final perf = ProbePerf.instance;
    perf.start();
    perf.start();
    expect(perf.snapshot()['frames'], 0);
    perf.stopWindow();
  });
}
