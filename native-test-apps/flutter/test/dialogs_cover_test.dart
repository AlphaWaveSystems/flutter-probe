import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:probe_flutter_fixture/widget_families.dart';

// The Cover page of the dialogs screen (PROBE_FIXTURE_WIDGETS=dialogs) is built so that a tap on its "Page button" lands on the dialog's body and a tap on its "Top button" lands
// on the barrier, on any screen: the button is at the centre of the page, the dialog is centred and its non-interactive body spans the centre, and the top button is above the
// dialog. The device suite (probe-tests/widgets/dialogs.probe) depends on it, so this runs the agent's own `tap` on four screen geometries (the surfaces a reviewer found different
// in an earlier layout) and checks where each tap lands. Run it BY HAND with `flutter test` in this folder; it needs no device, and CI does not run it (CI runs the tests of
// probe_agent only). The page is built for a portrait screen of at least about 400x800 dp (the four surfaces here): on a 360x640 dp screen the "Tall dialog" does not open the
// way the suite expects, and in landscape the dialog covers the Top button.

Future<Map<String, dynamic>> _agent(
  WidgetTester tester,
  String method,
  Map<String, dynamic> params,
) async {
  String? raw;
  final executor = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = executor
      .dispatch(
        ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params),
      )
      .whenComplete(() => done = true);
  for (var i = 0; i < 200 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

Map<String, dynamic> _text(String t) => {
  'selector': {'kind': 'text', 'text': t},
};

String _data(WidgetTester tester, String prefix) =>
    tester.widget<Text>(find.textContaining(prefix)).data!;

void main() {
  // physical size and device pixel ratio of the surfaces: a Pixel 6 profile, two taller ones and a short one.
  const surfaces = <(String, Size, double)>[
    ('1080x2400 @2.625', Size(1080, 2400), 2.625),
    ('1080x2340 @2.75', Size(1080, 2340), 2.75),
    ('1080x2400 @2.8', Size(1080, 2400), 2.8),
    ('1080x1920 @2.625', Size(1080, 1920), 2.625),
  ];
  for (final (name, size, ratio) in surfaces) {
    group('on $name', () {
      Future<void> openTallDialog(WidgetTester tester) async {
        tester.view.devicePixelRatio = ratio;
        tester.view.physicalSize = size;
        addTearDown(tester.view.reset);
        await tester.pumpWidget(const MaterialApp(home: DialogsFamilyScreen()));
        await tester.pumpAndSettle();
        await _agent(tester, ProbeMethods.tap, _text('Open cover page'));
        await tester.pumpAndSettle();
        await _agent(tester, ProbeMethods.tap, _text('Open tall dialog'));
        await tester.pumpAndSettle();
        expect(find.text('Tall dialog'), findsOneWidget);
      }

      testWidgets(
        'the Page button is behind the body of the dialog, not behind its action',
        (tester) async {
          await openTallDialog(tester);
          // The AlertDialog widget fills the screen (it holds the centring); the dialog itself is the Material it builds.
          final dialog = tester.getRect(
            find
                .descendant(
                  of: find.byType(AlertDialog),
                  matching: find.byType(Material),
                )
                .first,
          );
          final page = tester.getCenter(find.text('Page button'));
          final close = tester.getRect(find.text('Close'));
          expect(
            dialog.contains(page),
            isTrue,
            reason:
                'the centre of the Page button $page is outside the dialog $dialog',
          );
          expect(
            close.inflate(8).contains(page),
            isFalse,
            reason: 'the Page button is behind the Close button $close',
          );
          final top = tester.getCenter(find.text('Top button'));
          expect(
            dialog.contains(top),
            isFalse,
            reason: 'the Top button $top is behind the dialog $dialog',
          );
        },
      );

      testWidgets(
        'a tap on the Page button lands on the dialog: the counter stays 0 and the dialog stays open',
        (tester) async {
          await openTallDialog(tester);
          final answer = await _agent(
            tester,
            ProbeMethods.tap,
            _text('Page button'),
          );
          await tester.pumpAndSettle();
          expect(answer.containsKey('error'), isFalse, reason: '$answer');
          expect(
            answer.toString(),
            contains('covered by another widget'),
            reason: 'the agent should warn that the target is covered: $answer',
          );
          expect(_data(tester, 'Page taps:'), 'Page taps: 0');
          expect(
            find.text('Tall dialog'),
            findsOneWidget,
            reason: 'the dialog was dismissed',
          );
          final close = await _agent(tester, ProbeMethods.tap, _text('Close'));
          await tester.pumpAndSettle();
          expect(close.containsKey('error'), isFalse, reason: '$close');
          expect(_data(tester, 'Cover dialog:'), 'Cover dialog: closed');
          await _agent(tester, ProbeMethods.tap, _text('Page button'));
          await tester.pumpAndSettle();
          expect(_data(tester, 'Page taps:'), 'Page taps: 1');
        },
      );

      testWidgets(
        'a tap on the Top button lands on the barrier: the dialog is dismissed and the button does not count',
        (tester) async {
          await openTallDialog(tester);
          final answer = await _agent(
            tester,
            ProbeMethods.tap,
            _text('Top button'),
          );
          await tester.pumpAndSettle();
          expect(answer.containsKey('error'), isFalse, reason: '$answer');
          expect(
            find.text('Tall dialog'),
            findsNothing,
            reason: 'the barrier did not dismiss the dialog',
          );
          expect(_data(tester, 'Cover dialog:'), 'Cover dialog: none');
          expect(_data(tester, 'Top taps:'), 'Top taps: 0');
          final close = await _agent(tester, ProbeMethods.tap, _text('Close'));
          expect(
            close.containsKey('error'),
            isTrue,
            reason:
                'a tap on Close after the dialog was dismissed should find nothing: $close',
          );
          await _agent(tester, ProbeMethods.tap, _text('Top button'));
          await tester.pumpAndSettle();
          expect(_data(tester, 'Top taps:'), 'Top taps: 1');
        },
      );
    });
  }
}
