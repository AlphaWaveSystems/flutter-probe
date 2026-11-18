import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:probe_flutter_fixture/widget_families.dart';

// What the agent does with the children of an IndexedStack that are not the shown one, and with the items of a closed DropdownButton (which keeps its items in
// an IndexedStack): it does not see them, with Flutter 3.44.8 and with Flutter 3.50.0. Before the fix of issue #900 it saw them with Flutter 3.50.0: the agent
// ignores a widget behind an `Offstage` or a `Visibility(visible: false)`, 3.44.8 wraps the children that are not shown in a `Visibility(visible: false)`, and
// 3.50.0 does not (only `RenderIndexedStack` hides them there). The agent's own test of the rule is probe_agent/test/indexed_stack_hidden_test.dart (added with
// the fix, #901); this one runs the same check on the fixture's pickers screen and on a plain IndexedStack, through the same `see` the device suite uses.
//
// Run it BY HAND, once with each Flutter, with `flutter test test/pickers_indexed_stack_regression_test.dart` in this folder; it needs no device, and CI does not run it (CI
// runs the tests of probe_agent only).

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
    'the agent does not see the children of an IndexedStack that are not shown',
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
      expect(
        await _passes(tester, 'Hidden page', negated: false),
        isFalse,
        reason: 'see "Hidden page"',
      );
      expect(
        await _passes(tester, 'Hidden page', negated: true),
        isTrue,
        reason: 'don\'t see "Hidden page"',
      );
    },
  );

  testWidgets(
    'the agent does not see the items of a closed DropdownButton on the pickers screen',
    (tester) async {
      await tester.pumpWidget(const MaterialApp(home: PickersFamilyScreen()));
      await tester.pumpAndSettle();
      expect(await _passes(tester, 'Pick a color', negated: false), isTrue);
      expect(
        await _passes(tester, 'Blue', negated: false),
        isFalse,
        reason: 'see "Blue", an item of the closed dropdown',
      );
      expect(
        await _passes(tester, 'Blue', negated: true),
        isTrue,
        reason: 'don\'t see "Blue"',
      );
      // The items of a PopupMenuButton exist only while its menu is open.
      expect(await _passes(tester, 'Cut', negated: false), isFalse);
    },
  );
}
