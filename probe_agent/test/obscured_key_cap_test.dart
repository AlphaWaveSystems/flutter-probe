import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/finder.dart';

// The cap on remembered keys. Each test file runs in its own isolate, so the finder's memory here is this file's alone.
void main() {
  testWidgets('a keyed field becomes secret once the cap of remembered keys is reached', (tester) async {
    final plain = TextEditingController(text: 'cap-plain-text');
    final unkeyed = TextEditingController(text: 'cap-unkeyed-text');
    final controllers = <TextEditingController>[plain, unkeyed];
    addTearDown(() {
      for (final c in controllers) {
        c.dispose();
      }
    });

    // Before any key is remembered, a plain keyed field is not secret.
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: TextField(key: const ValueKey('cap-plain'), controller: plain)),
    ));
    final plainText = find.descendant(of: find.byKey(const ValueKey('cap-plain')), matching: find.byType(EditableText));
    expect(ProbeFinder.instance.isSecretEditable(tester.element(plainText)), isFalse);

    // 256 fields, each with its own key, are read as obscured, so 256 keys are remembered. The plain keyed field is
    // read first, while the memory is still empty.
    final fields = <Widget>[TextField(key: const ValueKey('cap-plain'), controller: plain)];
    for (var i = 0; i < 256; i++) {
      final c = TextEditingController();
      controllers.add(c);
      fields.add(TextField(key: ValueKey('cap-$i'), controller: c, obscureText: true));
    }
    fields.add(TextField(controller: unkeyed));
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: SingleChildScrollView(child: Column(children: fields))),
    ));
    for (final element in tester.elementList(find.byType(EditableText))) {
      ProbeFinder.instance.isSecretEditable(element);
    }
    expect(ProbeFinder.instance.obscuredKeyCount, 256, reason: 'exactly the cap of keys is remembered');

    // The cap is reached, so the keyed plain field is now secret. The unkeyed field is still tracked per controller only.
    expect(ProbeFinder.instance.isSecretEditable(tester.element(plainText)), isTrue,
        reason: 'once the memory is full, a keyed field counts as secret, so it cannot grow without bound');
    final unkeyedField = find.byWidgetPredicate((w) => w is TextField && w.controller == unkeyed);
    final unkeyedText = find.descendant(of: unkeyedField, matching: find.byType(EditableText));
    expect(ProbeFinder.instance.isSecretEditable(tester.element(unkeyedText)), isFalse,
        reason: 'an unkeyed field is tracked per controller, so the cap does not change it');

    // Keys past the cap are not remembered, so the count stays at the cap however many more obscured keyed fields are read.
    final more = <Widget>[];
    for (var i = 256; i < 300; i++) {
      final c = TextEditingController();
      controllers.add(c);
      more.add(TextField(key: ValueKey('cap-$i'), controller: c, obscureText: true));
    }
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: SingleChildScrollView(child: Column(children: more))),
    ));
    for (final element in tester.elementList(find.byType(EditableText))) {
      ProbeFinder.instance.isSecretEditable(element);
    }
    expect(ProbeFinder.instance.obscuredKeyCount, 256,
        reason: 'a key past the cap is not remembered, so the memory stays bounded');
  });
}
