import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

Future<Map<String, dynamic>> _rpc(WidgetTester tester, String method, Map<String, dynamic> params,
    {ProbeExecutor? executor, void Function(int pumps)? onPump}) async {
  String? raw;
  final ex = executor ?? ProbeExecutor((s) => raw = s);
  if (executor != null) ex.sendFn = (s) => raw = s;
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    onPump?.call(i);
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  snackbarCase();
  // FP-19 (reported from an Android gate): a tap "succeeded" and nothing
  // happened. When something else is on top at the tap point, say so.
  testWidgets('a tap on a covered target returns a warning instead of failing silently',
      (tester) async {
    var covered = 0, under = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Stack(children: [
          Center(
            child: ElevatedButton(
              key: const ValueKey('share_button'),
              onPressed: () => under++,
              child: const Text('Share'),
            ),
          ),
          // A full-screen overlay (loading veil, sticky bar, ...) that eats the tap.
          Positioned.fill(
            child: GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTap: () => covered++,
              child: const SizedBox.expand(),
            ),
          ),
        ]),
      ),
    ));

    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'id', 'text': '#share_button'},
    });

    expect(res['error'], isNull, reason: 'the tap itself still happens; it is a warning, not a failure');
    final warning = res['result']['warning'] as String?;
    expect(warning, isNotNull);
    expect(warning, contains('#share_button'));
    expect(warning, contains('covered'));
    expect(covered, 1, reason: 'the overlay received the tap, as a real user\'s would');
    expect(under, 0);
  });

  testWidgets('a normal tap carries no warning', (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Center(
          child: ElevatedButton(
            key: const ValueKey('ok_button'),
            onPressed: () => taps++,
            child: const Text('OK'),
          ),
        ),
      ),
    ));
    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'id', 'text': '#ok_button'},
    });
    expect(res['result'].containsKey('warning'), isFalse);
    expect(taps, 1);
  });

  testWidgets('a tap right after a scroll waits for the scroll to finish', (tester) async {
    final c = ScrollController();
    addTearDown(c.dispose);
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView.builder(
          controller: c,
          itemCount: 60,
          itemBuilder: (_, i) => i == 3
              ? ElevatedButton(key: const ValueKey('row3'), onPressed: () => taps++, child: const Text('Row 3'))
              : SizedBox(height: 48, child: Text('Row $i')),
        ),
      ),
    ));
    // A scroll is still in flight when the tap request arrives.
    final pos = c.position;
    unawaited(pos.animateTo(60, duration: const Duration(milliseconds: 600), curve: Curves.linear));
    await tester.pump(const Duration(milliseconds: 50));
    expect(pos.isScrollingNotifier.value, isTrue);

    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'id', 'text': '#row3'},
    });
    expect(res['error'], isNull);
    expect(pos.isScrollingNotifier.value, isFalse, reason: 'the tap must not fire mid-scroll');
    expect(taps, 1, reason: 'after the scroll settled the tap lands on the button');
  });

  // FP-19: after a scroll the target can be transiently un-hittable (an
  // IgnorePointer that lifts a moment later, a settling overlay, ...). The tap
  // used to fire into the void; it now waits (bounded) until the target is reachable.
  testWidgets('a tap right after a scroll waits until the target is hit-testable', (tester) async {
    final blocked = ValueNotifier<bool>(true);
    addTearDown(blocked.dispose);
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ValueListenableBuilder<bool>(
          valueListenable: blocked,
          builder: (_, isBlocked, __) => ListView(children: [
            for (var i = 0; i < 3; i++) SizedBox(height: 48, child: Text('Row $i')),
            IgnorePointer(
              ignoring: isBlocked,
              child: ElevatedButton(key: const ValueKey('late_button'), onPressed: () => taps++, child: const Text('Share')),
            ),
            for (var i = 3; i < 40; i++) SizedBox(height: 48, child: Text('Row $i')),
          ]),
        ),
      ),
    ));
    final ex = ProbeExecutor((_) {});
    await _rpc(tester, ProbeMethods.scroll, {'direction': 'up'}, executor: ex); // at the top: stamps the scroll time, moves nothing

    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'id', 'text': '#late_button'},
    }, executor: ex, onPump: (i) {
      // The block lifts a few frames after the tap request arrived.
      if (i == 6) blocked.value = false;
    });
    expect(res['error'], isNull);
    expect(taps, 1, reason: 'the tap must wait for the target to become reachable instead of being lost');
  });

  testWidgets('a hit that stops at a parent handler is normal and does not warn', (tester) async {
    var taps = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Center(
          child: GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: () => taps++,
            // The label itself ignores pointers; the parent handles the tap
            // (a common button/snackbar-action structure).
            child: const Padding(
              padding: EdgeInsets.all(24),
              child: IgnorePointer(child: Text('Undo')),
            ),
          ),
        ),
      ),
    ));
    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'text', 'text': 'Undo'},
    });
    expect(taps, 1, reason: 'the parent handler receives the tap');
    expect(res['result'].containsKey('warning'), isFalse, reason: 'no unrelated widget covers the target');
  });
}

void unawaited(Future<void> f) {}

void snackbarCase() {
  testWidgets('a "covered" tap that visibly changes the screen is not reported', (tester) async {
    var label = 'Before';
    late StateSetter set;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: StatefulBuilder(builder: (c, setState) {
          set = setState;
          return Stack(children: [
            Center(child: ElevatedButton(key: const ValueKey('b'), onPressed: () {}, child: const Text('Go'))),
            Positioned.fill(
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: () => set(() => label = 'After'),
                child: Center(child: Text(label)),
              ),
            ),
          ]);
        }),
      ),
    ));
    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'id', 'text': '#b'},
    });
    expect(label, 'After');
    expect(res['result']['warning'], isNull, reason: '${res['result']['warning']}');
  });

  testWidgets('with two wide labels, the copy whose button is hit wins, no warning', (tester) async {
    var hidden = 0, shown = 0;
    Widget wide(VoidCallback f) => TextButton(
          onPressed: f,
          child: const SizedBox(width: 200, child: Text('Undo')), // glyphs only at the left edge
        );
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Stack(children: [
          IgnorePointer(child: wide(() => hidden++)),
          Align(alignment: Alignment.bottomCenter, child: wide(() => shown++)),
        ]),
      ),
    ));
    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'text', 'text': 'Undo'},
    });
    expect(res['result']['warning'], isNull, reason: '${res['result']['warning']}');
    expect(shown, 1);
    expect(hidden, 0);
  });

  testWidgets('with two matches, the tap goes to the one that is reachable', (tester) async {
    var hidden = 0, shown = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Stack(children: [
          // First in tree order, but under an IgnorePointer (like a leaving overlay).
          IgnorePointer(child: TextButton(onPressed: () => hidden++, child: const Text('Undo'))),
          Align(
            alignment: Alignment.bottomCenter,
            child: TextButton(onPressed: () => shown++, child: const Text('Undo')),
          ),
        ]),
      ),
    ));
    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'text', 'text': 'Undo'},
    });
    expect(res['error'], isNull);
    expect(res['result']['warning'], isNull, reason: '${res['result']['warning']}');
    expect(shown, 1);
    expect(hidden, 0);
  });

  testWidgets('tapping a SnackBar action gives no covered warning', (tester) async {
    var undone = 0;
    late BuildContext ctx;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Builder(builder: (c) {
          ctx = c;
          return const Center(child: Text('Body'));
        }),
        floatingActionButton: FloatingActionButton(onPressed: () {}),
      ),
    ));
    ScaffoldMessenger.of(ctx).showSnackBar(SnackBar(
      content: const Text('Added'),
      action: SnackBarAction(label: 'Undo', onPressed: () => undone++),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100)); // mid slide-in

    final res = await _rpc(tester, ProbeMethods.tap, {
      'selector': {'kind': 'text', 'text': 'Undo'},
    });
    expect(res['error'], isNull);
    expect(res['result']['warning'], isNull, reason: '${res['result']['warning']}');
    expect(undone, 1);
  });
}
