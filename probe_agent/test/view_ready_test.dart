import 'dart:ui' show FlutterView;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/view_ready.dart';

// Issue #793: after a relaunch an app can draw frames with a 0×0 view for about a second. ViewReady makes the agent wait for the real
// size, without ever slowing an app that has nothing to wait for.
Widget _list() => MaterialApp(
      home: Scaffold(
        body: ListView(children: [
          for (var i = 0; i < 30; i++)
            SizedBox(height: 60, child: Text('Row $i'))
        ]),
      ),
    );

void main() {
  final views = ViewReady.views;
  tearDown(() {
    ViewReady.instance.reset();
    ViewReady.views = views;
  });

  testWidgets(
      'a 0×0 view builds part of the screen; wait returns once the real size is laid out, and the whole screen is there',
      (tester) async {
    ViewReady.instance.attach();
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_list());
    expect(find.text('Row 5'), findsNothing,
        reason: 'at 0×0 the list builds only what fits its cache extent');
    expect(ViewReady.instance.isReady, isFalse);

    var waited = -1;
    ViewReady.instance.wait().then((ms) => waited = ms);
    await tester.pump(const Duration(milliseconds: 300));
    expect(waited, -1, reason: 'still waiting while the view is 0×0');

    tester.view.physicalSize = const Size(1080, 2400);
    tester.view.devicePixelRatio = 3;
    await tester.pump(const Duration(milliseconds: 20));
    await tester.pump(const Duration(milliseconds: 20));
    expect(waited, isNot(-1),
        reason: 'the wait ended once the real size was laid out');
    expect(find.text('Row 5'), findsOneWidget);
    expect(ViewReady.instance.isReady, isTrue);
  });

  testWidgets('a real size: nothing to wait for, answered synchronously',
      (tester) async {
    ViewReady.instance.attach();
    await tester.pumpWidget(_list());
    expect(ViewReady.instance.isReady, isTrue);
    expect(await ViewReady.instance.wait(), 0);
  });

  testWidgets(
      'the cap: a size that never comes stops the wait at ViewReady.cap, and only once per episode',
      (tester) async {
    ViewReady.instance.attach();
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_list());
    var waited = -1;
    ViewReady.instance.wait().then((ms) => waited = ms);
    await tester.pump(ViewReady.cap - const Duration(milliseconds: 100));
    expect(waited, -1);
    await tester.pump(const Duration(milliseconds: 200));
    expect(waited, isNot(-1), reason: 'the cap ended the wait');
    expect(ViewReady.instance.gaveUp, isTrue);
    // The same episode: no second capped wait.
    expect(ViewReady.instance.isReady, isTrue);
    expect(await ViewReady.instance.wait(), 0);
  });

  testWidgets(
      'a new size starts a new episode; other metrics (insets, pixel ratio) do not',
      (tester) async {
    ViewReady.instance.attach();
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_list());
    ViewReady.instance.wait();
    await tester.pump(ViewReady.cap + const Duration(milliseconds: 50));
    expect(ViewReady.instance.gaveUp, isTrue);
    // A keyboard inset or a pixel ratio change, the size unchanged: still the same episode.
    tester.view.viewInsets = const FakeViewPadding(bottom: 300);
    tester.view.devicePixelRatio = 2;
    await tester.pump();
    expect(ViewReady.instance.gaveUp, isTrue);
    expect(ViewReady.instance.isReady, isTrue);
    // A new size (still empty): a new episode, the agent waits again.
    tester.view.physicalSize = const Size(0, 10);
    await tester.pump();
    expect(ViewReady.instance.gaveUp, isFalse);
    expect(ViewReady.instance.isReady, isFalse);
  });

  testWidgets(
      'a real size with no frame laid out at it within the cap: the same episode is not waited for again',
      (tester) async {
    ViewReady.instance.attach();
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_list());
    tester.view.physicalSize = const Size(1080, 2400);
    var waited = -1;
    ViewReady.instance.wait().then((ms) => waited = ms);
    // time passes, no frame is drawn: the cap ends the wait
    await tester.binding
        .delayed(ViewReady.cap + const Duration(milliseconds: 100));
    expect(waited, isNot(-1));
    expect(ViewReady.instance.gaveUp, isTrue);
    expect(ViewReady.instance.isReady, isTrue,
        reason: 'one capped wait per episode');
  });

  testWidgets('an app in the background is never waited for', (tester) async {
    ViewReady.instance.attach();
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_list());
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    addTearDown(() => tester.binding
        .handleAppLifecycleStateChanged(AppLifecycleState.resumed));
    expect(ViewReady.instance.isReady, isTrue);
    expect(await ViewReady.instance.wait(), 0);
  });

  testWidgets(
      'an app that goes to the background during the wait stops waiting',
      (tester) async {
    ViewReady.instance.attach();
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_list());
    var waited = -1;
    ViewReady.instance.wait().then((ms) => waited = ms);
    await tester.pump(const Duration(milliseconds: 100));
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
    addTearDown(() => tester.binding
        .handleAppLifecycleStateChanged(AppLifecycleState.resumed));
    await tester.pump(const Duration(milliseconds: 50));
    expect(waited, isNot(-1),
        reason: 'going to the background ended the wait before the cap');
    expect(ViewReady.instance.gaveUp, isFalse);
  });

  testWidgets('no view at all: no wait and no error', (tester) async {
    ViewReady.instance.attach();
    ViewReady.views = () => const <FlutterView>[];
    expect(ViewReady.instance.isReady, isTrue);
    expect(await ViewReady.instance.wait(), 0);
  });

  testWidgets(
      'a resize from one real size to another (a rotation) waits for one frame laid out at the new size, no more',
      (tester) async {
    ViewReady.instance.attach();
    tester.view.physicalSize = const Size(1080, 2400);
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_list());
    expect(ViewReady.instance.isReady, isTrue);
    tester.view.physicalSize = const Size(2400, 1080);
    expect(ViewReady.instance.isReady, isFalse,
        reason: 'the new size is reported but nothing is laid out at it yet');
    var waited = -1;
    ViewReady.instance.wait().then((ms) => waited = ms);
    await tester.pump(const Duration(milliseconds: 20));
    await tester.pump();
    expect(waited, isNot(-1),
        reason: 'one frame at the new size ends the wait');
    expect(ViewReady.instance.isReady, isTrue);
  });

  testWidgets(
      'once the size is real, the wait still lasts until a frame is laid out at it',
      (tester) async {
    ViewReady.instance.attach();
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_list());
    var waited = -1;
    ViewReady.instance.wait().then((ms) => waited = ms);
    await tester.pump(const Duration(milliseconds: 50));
    tester.view.physicalSize = const Size(1080, 2400);
    // time passes, but no frame is drawn: the wait has seen the size and is waiting for the frame
    await tester.binding.delayed(const Duration(milliseconds: 100));
    expect(waited, -1, reason: 'no frame was laid out at the real size yet');
    await tester.pump();
    await tester.pump();
    expect(waited, isNot(-1));
  });

  testWidgets(
      'the gap: the real size is reported but no frame is laid out at it yet, so the view is not ready (issue #793)',
      (tester) async {
    ViewReady.instance.attach();
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_list());
    tester.view.physicalSize = const Size(1080, 2400);
    // The size changed, but the tree is still the 0×0 layout: a request now would see half the screen.
    expect(find.text('Row 5'), findsNothing);
    expect(ViewReady.instance.isReady, isFalse);
    await tester.pump();
    expect(ViewReady.instance.isReady, isTrue);
    expect(find.text('Row 5'), findsOneWidget);
  });

  testWidgets('without attach, the earlier rule applies: a real size is ready',
      (tester) async {
    tester.view.physicalSize = Size.zero;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(_list());
    tester.view.physicalSize = const Size(1080, 2400);
    expect(ViewReady.instance.isReady, isTrue);
  });
}
