import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

void main() {
  group("don't see honors state checks, not just existence (PT-04 follow-on)", () {
    // "don't see X is focused" used to silently degrade to a bare existence
    // check — it threw as soon as ANY matching element existed, regardless
    // of whether it actually satisfied the checked state. This meant
    // "don't see #field is focused" always failed whenever #field simply
    // existed, even when it plainly wasn't focused.
    late FocusNode focusNodeA;
    late FocusNode focusNodeB;
    late String? lastSent;
    late ProbeExecutor executor;

    setUp(() {
      focusNodeA = FocusNode();
      focusNodeB = FocusNode();
      lastSent = null;
      executor = ProbeExecutor((msg) => lastSent = msg);
    });

    tearDown(() {
      focusNodeA.dispose();
      focusNodeB.dispose();
    });

    Future<bool> isError() async {
      final decoded = jsonDecode(lastSent!) as Map<String, dynamic>;
      return decoded.containsKey('error');
    }

    Future<void> seeNegatedFocused(String id) => executor.dispatch(ProbeRequest(
          jsonrpc: '2.0',
          id: 1,
          method: ProbeMethods.see,
          params: {
            'selector': {'kind': 'id', 'text': id},
            'negated': true,
            'check': 'focused',
          },
        ));

    testWidgets('passes when the element exists but is not focused', (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(children: [
            TextField(key: const ValueKey('field_a'), focusNode: focusNodeA),
            TextField(key: const ValueKey('field_b'), focusNode: focusNodeB),
          ]),
        ),
      ));

      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 0,
        method: ProbeMethods.tap,
        params: {
          'selector': {'kind': 'id', 'text': '#field_a'},
        },
      ));
      expect(focusNodeA.hasFocus, isTrue);
      expect(focusNodeB.hasFocus, isFalse);

      // field_b exists but is not focused — the negated check must pass.
      await seeNegatedFocused('#field_b');
      expect(await isError(), isFalse,
          reason: 'expected no error, got: $lastSent');
    });

    testWidgets('still fails when the element genuinely is focused', (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(key: const ValueKey('field_a'), focusNode: focusNodeA),
        ),
      ));

      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 0,
        method: ProbeMethods.tap,
        params: {
          'selector': {'kind': 'id', 'text': '#field_a'},
        },
      ));
      expect(focusNodeA.hasFocus, isTrue);

      await seeNegatedFocused('#field_a');
      expect(await isError(), isTrue,
          reason: 'expected an error since #field_a genuinely is focused, got: $lastSent');
    });

    testWidgets(
        'passes after unfocus(), even though the enclosing ModalRoute scope '
        'becomes the fallback focus holder', (tester) async {
      // PT-12: the 'focused' check used to also walk *ancestors* looking
      // for a match. After FocusNode.unfocus(), Flutter falls back to the
      // enclosing ModalRoute's own FocusScopeNode holding primary focus —
      // and that scope is an ancestor of every widget on the current
      // screen, so the old ancestor walk reported every one of them as
      // "focused", including a field that plainly wasn't. This directly
      // reproduces that false positive.
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(key: const ValueKey('field_a'), focusNode: focusNodeA),
        ),
      ));

      focusNodeA.requestFocus();
      await tester.pump();
      expect(focusNodeA.hasFocus, isTrue);

      FocusManager.instance.primaryFocus?.unfocus();
      await tester.pump();
      expect(focusNodeA.hasFocus, isFalse);

      await seeNegatedFocused('#field_a');
      expect(await isError(), isFalse,
          reason: 'expected no error since #field_a was explicitly '
              'unfocused, got: $lastSent');
    });
  });
}
