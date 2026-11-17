import 'dart:convert';

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/src/agent_version.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_test/flutter_test.dart';

// `see #pw is filled` / `is empty`: whether a text field holds at least one character. The check never reads the text out: nothing the agent sends
// (an answer, an error, a hint) may hold what a field holds, and a password field is answered like any other.
const _secret = 'hunter2-SECRET-value';

void main() {
  late List<String> sent;
  late ProbeExecutor executor;

  setUp(() {
    sent = [];
    executor = ProbeExecutor((msg) => sent.add(msg));
  });

  var nextId = 1;

  /// Sends a `see` and returns the decoded answer (it has an `error` key when the step failed).
  Future<Map<String, dynamic>> see(String id, String check,
      {bool negated = false, String pattern = ''}) async {
    await executor.dispatch(ProbeRequest(
      jsonrpc: '2.0',
      id: nextId++,
      method: ProbeMethods.see,
      params: {
        'selector': {'kind': 'id', 'text': id},
        'negated': negated,
        'check': check,
        if (pattern.isNotEmpty) 'pattern': pattern,
      },
    ));
    return jsonDecode(sent.last) as Map<String, dynamic>;
  }

  String errorOf(Map<String, dynamic> answer) =>
      (answer['error'] as Map<String, dynamic>)['message'] as String;

  void expectNoSecretAnywhere() {
    for (final m in sent) {
      expect(m.contains(_secret), isFalse,
          reason: 'the value of a field reached a message: $m');
      expect(m.contains('hunter2'), isFalse,
          reason: 'part of the value of a field reached a message: $m');
    }
  }

  Widget app(List<Widget> children) =>
      MaterialApp(home: Scaffold(body: Column(children: children)));

  testWidgets('an empty field is empty, not filled', (tester) async {
    await tester.pumpWidget(app([const TextField(key: ValueKey('f'))]));
    expect((await see('#f', 'empty')).containsKey('error'), isFalse);
    final answer = await see('#f', 'filled');
    expect(errorOf(answer), '"#f" is empty');
    expect((await see('#f', 'filled', negated: true)).containsKey('error'),
        isFalse);
    expect(errorOf(await see('#f', 'empty', negated: true)),
        startsWith('Expected NOT to see'));
  });

  testWidgets('a field with text is filled, not empty', (tester) async {
    final c = TextEditingController(text: 'hello');
    await tester
        .pumpWidget(app([TextField(key: const ValueKey('f'), controller: c)]));
    expect((await see('#f', 'filled')).containsKey('error'), isFalse);
    expect(errorOf(await see('#f', 'empty')), '"#f" is filled');
    expect((await see('#f', 'empty', negated: true)).containsKey('error'),
        isFalse);
    expect(errorOf(await see('#f', 'filled', negated: true)),
        startsWith('Expected NOT to see'));
    // Neither the passing nor the failing answers say what the field holds.
    for (final m in sent) {
      expect(m.contains('hello'), isFalse,
          reason: 'the text of a field reached a message: $m');
    }
  });

  testWidgets('spaces are characters: a field of spaces is filled',
      (tester) async {
    final c = TextEditingController(text: '   ');
    await tester
        .pumpWidget(app([TextField(key: const ValueKey('f'), controller: c)]));
    expect((await see('#f', 'filled')).containsKey('error'), isFalse);
    expect(errorOf(await see('#f', 'empty')), '"#f" is filled');
  });

  testWidgets('the state follows the field when it changes', (tester) async {
    final c = TextEditingController();
    await tester
        .pumpWidget(app([TextField(key: const ValueKey('f'), controller: c)]));
    expect((await see('#f', 'empty')).containsKey('error'), isFalse);
    c.text = 'x';
    await tester.pump();
    expect((await see('#f', 'filled')).containsKey('error'), isFalse);
    c.clear();
    await tester.pump();
    expect((await see('#f', 'empty')).containsKey('error'), isFalse);
  });

  testWidgets(
      'an obscured field is answered like any other, and its value reaches no message',
      (tester) async {
    final c = TextEditingController(text: _secret);
    await tester.pumpWidget(app([
      TextField(key: const ValueKey('pw'), obscureText: true, controller: c)
    ]));
    expect((await see('#pw', 'filled')).containsKey('error'), isFalse);
    expect(errorOf(await see('#pw', 'empty')), '"#pw" is filled');
    expect((await see('#pw', 'empty', negated: true)).containsKey('error'),
        isFalse);
    expect(errorOf(await see('#pw', 'filled', negated: true)),
        startsWith('Expected NOT to see'));
    expectNoSecretAnywhere();

    c.clear();
    await tester.pump();
    expect((await see('#pw', 'empty')).containsKey('error'), isFalse);
    expect(errorOf(await see('#pw', 'filled')), '"#pw" is empty');
    expectNoSecretAnywhere();
  });

  testWidgets(
      'a label next to a field is not the field: neither filled nor empty, and a negated check refuses too',
      (tester) async {
    final c = TextEditingController(text: _secret);
    await tester.pumpWidget(app([
      const Text('Password', key: ValueKey('label')),
      TextField(key: const ValueKey('pw'), obscureText: true, controller: c),
    ]));
    for (final check in ['filled', 'empty']) {
      expect(
          errorOf(await see('#label', check)), contains('is not a text field'),
          reason: 'is $check on a label');
      final negated = await see('#label', check, negated: true);
      expect(negated.containsKey('error'), isTrue,
          reason: "don't see a label is $check must not pass silently");
      expect(errorOf(negated), contains('cannot be evaluated'));
    }
    expectNoSecretAnywhere();
  });

  testWidgets('a container with two fields says it cannot tell which one',
      (tester) async {
    await tester.pumpWidget(app([
      Column(
          key: const ValueKey('form'),
          children: const [TextField(), TextField()]),
    ]));
    expect(
        errorOf(await see('#form', 'filled')), contains('holds 2 text fields'));
    expect(errorOf(await see('#form', 'empty', negated: true)),
        contains('holds 2 text fields'));
  });

  testWidgets(
      'TextFormField, CupertinoTextField and a bare EditableText are text fields too',
      (tester) async {
    final fc = TextEditingController(text: 'a');
    final cc = TextEditingController();
    final ec = TextEditingController(text: 'b');
    await tester.pumpWidget(app([
      TextFormField(key: const ValueKey('form_field'), controller: fc),
      CupertinoTextField(key: const ValueKey('cupertino'), controller: cc),
      EditableText(
        key: const ValueKey('bare'),
        controller: ec,
        focusNode: FocusNode(),
        style: const TextStyle(),
        cursorColor: Colors.black,
        backgroundCursorColor: Colors.grey,
      ),
    ]));
    expect((await see('#form_field', 'filled')).containsKey('error'), isFalse);
    expect((await see('#cupertino', 'empty')).containsKey('error'), isFalse);
    expect((await see('#bare', 'filled')).containsKey('error'), isFalse);
  });

  testWidgets('a field that is not on screen is not found', (tester) async {
    await tester.pumpWidget(app([const TextField(key: ValueKey('f'))]));
    expect(errorOf(await see('#nope', 'filled')), contains('was not found'));
  });

  testWidgets(
      'a pattern cannot be combined with filled or empty: there is no text to match',
      (tester) async {
    final c = TextEditingController(text: _secret);
    await tester.pumpWidget(app([
      TextField(key: const ValueKey('pw'), obscureText: true, controller: c)
    ]));
    for (final check in ['filled', 'empty']) {
      final answer = await see('#pw', check, pattern: '^hunter');
      expect((answer['error'] as Map<String, dynamic>)['code'],
          ProbeError.invalidParams);
      expect(errorOf(answer), contains('never reads the text'));
    }
    expectNoSecretAnywhere();
  });

  test(
      'the agent reports the see_filled capability, so the CLI sends the check to it',
      () {
    expect(probeAgentCapabilities, contains('see_filled'));
  });
}
