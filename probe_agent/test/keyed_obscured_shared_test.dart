import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/finder.dart';

// Synthetic values only. The shared key names one field in the app, and two fields carry it in this test.
const _secret = 'shared-key-secret';
const _plain = 'shared-key-plain';

void main() {
  testWidgets('two fields that share a key share the obscured state', (tester) async {
    final secret = TextEditingController(text: _secret);
    final plain = TextEditingController(text: _plain);
    addTearDown(secret.dispose);
    addTearDown(plain.dispose);
    // The fields sit under different parents: keys only have to be unique among siblings.
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          Container(child: TextField(key: const ValueKey('shared'), controller: secret, obscureText: true)),
          Container(child: TextField(key: const ValueKey('shared'), controller: plain)),
        ]),
      ),
    ));

    // The obscured field comes first in the tree, so its key is remembered before the plain field is read.
    final summary = ProbeFinder.instance.visibleSummary();
    expect(summary['texts'], isNot(contains(_secret)));
    expect(summary['texts'], isNot(contains(_plain)));

    final plainField = find.byWidgetPredicate((w) => w is TextField && w.controller == plain);
    final plainText = find.descendant(of: plainField, matching: find.byType(EditableText));
    expect(ProbeFinder.instance.isSecretEditable(tester.element(plainText)), isTrue,
        reason: 'a plain field that shares a key with an obscured field shares its obscured state');
  });
}
