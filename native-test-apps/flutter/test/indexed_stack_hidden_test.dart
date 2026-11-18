import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:probe_flutter_fixture/widget_families.dart';

// What the agent does with the children of an IndexedStack that are not the shown one, and with the items of a closed DropdownButton (which keeps its items in
// an IndexedStack). Measured: with Flutter 3.44.8 the agent does not see them (`see` fails, `don't see` passes); with Flutter 3.50.0 it does (`see` passes, `don't
// see` fails). The agent ignores a widget with an `Offstage(offstage: true)` or `Visibility(visible: false)` ancestor; Flutter 3.44.8 wraps the children that are
// not shown in a `Visibility(visible: false)`, and Flutter 3.50.0 does not (it keeps `_VisibilityScope` and `_RawIndexedStack` only).
//
// This test PINS that difference: it expects "not seen" on the Dart of Flutter 3.44.8 (3.12) and "seen" from the Dart of Flutter 3.50.0 (3.14), so it passes on both
// today. When the agent is fixed, the expectation for the newer Flutter fails: change it then, and the suite's matrix section with it. Run it BY HAND with
// `flutter test test/indexed_stack_hidden_test.dart` in this folder; it needs no device, and CI does not run it (CI runs the tests of probe_agent only).

/// Whether this Flutter is the one that shows the behaviour of 3.50: the Dart that ships with it is 3.14 or newer.
bool get _isFlutter350OrNewer {
  final m = RegExp(r'^(\d+)\.(\d+)').firstMatch(Platform.version)!;
  return int.parse(m.group(1)!) > 3 ||
      (int.parse(m.group(1)!) == 3 && int.parse(m.group(2)!) >= 14);
}

Future<bool> _passes(
  WidgetTester tester,
  String text, {
  required bool negated,
}) async {
  String? raw;
  final executor = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = executor
      .dispatch(
        ProbeRequest(
          jsonrpc: '2.0',
          id: 1,
          method: ProbeMethods.see,
          params: {
            'selector': {'kind': 'text', 'text': text},
            'negated': negated,
          },
        ),
      )
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return !(jsonDecode(raw!) as Map<String, dynamic>).containsKey('error');
}

void main() {
  testWidgets(
    'the agent and the children of an IndexedStack that are not shown',
    (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(
            body: IndexedStack(
              index: 0,
              children: [Text('Shown page'), Text('Hidden page')],
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(await _passes(tester, 'Shown page', negated: false), isTrue);
      final seen = _isFlutter350OrNewer;
      expect(
        await _passes(tester, 'Hidden page', negated: false),
        seen,
        reason:
            'see "Hidden page": passes only where the agent cannot tell the child is not shown',
      );
      expect(
        await _passes(tester, 'Hidden page', negated: true),
        !seen,
        reason: 'don\'t see "Hidden page": the opposite',
      );
    },
  );

  testWidgets(
    'the agent and the items of a closed DropdownButton on the pickers screen',
    (tester) async {
      await tester.pumpWidget(const MaterialApp(home: PickersFamilyScreen()));
      await tester.pumpAndSettle();
      final seen = _isFlutter350OrNewer;
      expect(await _passes(tester, 'Pick a color', negated: false), isTrue);
      expect(
        await _passes(tester, 'Blue', negated: false),
        seen,
        reason: 'see "Blue", an item of the closed dropdown',
      );
      expect(
        await _passes(tester, 'Blue', negated: true),
        !seen,
        reason: 'don\'t see "Blue"',
      );
      // The items of a PopupMenuButton exist only while its menu is open: not seen on either version.
      expect(await _passes(tester, 'Cut', negated: false), isFalse);
    },
  );
}
