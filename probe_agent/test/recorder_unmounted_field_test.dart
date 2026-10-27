import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_probe_agent/src/recorder.dart';

// Synthetic value only. It stands in for text typed into a field that later leaves the tree or is rebuilt.
const _typed = 'unmounted-field-typed';

/// The params of every recorded `type` event in [sent], in the order sent.
List<Map<String, dynamic>> _typeEvents(List<String> sent) {
  final events = <Map<String, dynamic>>[];
  for (final raw in sent) {
    final msg = jsonDecode(raw) as Map<String, dynamic>;
    if (msg['method'] != ProbeMethods.notifyRecordedEvent) continue;
    final params = msg['params'] as Map<String, dynamic>;
    if (params['action'] == 'type') events.add(params);
  }
  return events;
}

/// An obscured field and a plain field under the same key. The scan reads the obscured field first, so the key is
/// remembered before the plain field is read, and the plain field's controller is recorded as secret.
Widget _keyedPair(TextEditingController password, TextEditingController plain, {required bool plainFieldIn}) {
  return MaterialApp(
    home: Scaffold(
      body: Column(children: [
        TextField(key: const ValueKey('gone_pw'), controller: password, obscureText: true),
        if (plainFieldIn) Container(child: TextField(key: const ValueKey('gone_pw'), controller: plain)),
      ]),
    ),
  );
}

/// The same pair with the plain field's key removed, so the field is rebuilt as a new element.
Widget _unkeyedPlain(TextEditingController password, TextEditingController plain) {
  return MaterialApp(
    home: Scaffold(
      body: Column(children: [
        TextField(key: const ValueKey('gone_pw'), controller: password, obscureText: true),
        Container(child: TextField(controller: plain)),
      ]),
    ),
  );
}

/// One plain field, present or removed.
Widget _plainField(TextEditingController controller, {required bool present}) {
  return MaterialApp(
    home: Scaffold(
      body: Column(children: [
        if (present) TextField(controller: controller),
      ]),
    ),
  );
}

/// One plain field under a key that changes with [generation]: each generation is a new element for the controller.
Widget _generationField(TextEditingController controller, int generation) {
  return MaterialApp(
    home: Scaffold(body: TextField(key: ValueKey('generation-$generation'), controller: controller)),
  );
}

void main() {
  testWidgets(
      'text typed into a keyed field just before it leaves the tree is sent as obscured, never as text',
      (tester) async {
    final password = TextEditingController();
    final plain = TextEditingController();
    addTearDown(password.dispose);
    addTearDown(plain.dispose);
    await tester.pumpWidget(_keyedPair(password, plain, plainFieldIn: true));
    await tester.pumpAndSettle();

    final sent = <String>[];
    final recorder = ProbeRecorder()..start(sent.add);

    // Typed while the field is in the tree, so the debounced text is still pending when the field leaves.
    plain.text = _typed;
    await tester.pumpWidget(_keyedPair(password, plain, plainFieldIn: false));
    await tester.pump(const Duration(milliseconds: 310));
    recorder.stop();

    expect(sent.any((m) => m.contains(_typed)), isFalse,
        reason: 'the pending text of a field that left the tree must not be recorded as text');
    final events = _typeEvents(sent);
    expect(events, hasLength(1));
    expect(events.single['text'], '');
    expect(events.single['obscured'], isTrue);
  });

  testWidgets('text changed after a keyed field has left the tree is not read, and nothing is sent',
      (tester) async {
    final password = TextEditingController();
    final plain = TextEditingController();
    addTearDown(password.dispose);
    addTearDown(plain.dispose);
    await tester.pumpWidget(_keyedPair(password, plain, plainFieldIn: true));
    await tester.pumpAndSettle();

    final sent = <String>[];
    final recorder = ProbeRecorder()..start(sent.add);

    // The field leaves the tree first, then its controller changes before the next field scan. The change is not read
    // from the element that has left, and no exception is raised.
    await tester.pumpWidget(_keyedPair(password, plain, plainFieldIn: false));
    plain.text = _typed;
    await tester.pump(const Duration(milliseconds: 400));
    recorder.stop();

    expect(sent.any((m) => m.contains(_typed)), isFalse,
        reason: 'the value of a field that left the tree must not be sent');
    expect(_typeEvents(sent), isEmpty);
  });

  testWidgets('a field rebuilt under the same controller is recorded through its new element', (tester) async {
    final controller = TextEditingController();
    addTearDown(controller.dispose);
    await tester.pumpWidget(_generationField(controller, 0));
    await tester.pumpAndSettle();

    final sent = <String>[];
    final recorder = ProbeRecorder()..start(sent.add);

    // The rebuild unmounts the first element. The scan, every 500 ms, hands the tracker the new one.
    await tester.pumpWidget(_generationField(controller, 1));
    await tester.pump(const Duration(milliseconds: 600));
    controller.text = 'after-rebuild';
    await tester.pump(const Duration(milliseconds: 310));
    recorder.stop();

    final events = _typeEvents(sent);
    expect(events, hasLength(1), reason: 'a rebuilt field must keep being recorded');
    expect(events.single['text'], 'after-rebuild');
    expect(events.single['obscured'], isNull);
  });

  testWidgets('a field secret earlier in this recording stays obscured after it loses its key', (tester) async {
    final password = TextEditingController();
    final plain = TextEditingController();
    addTearDown(password.dispose);
    addTearDown(plain.dispose);
    await tester.pumpWidget(_keyedPair(password, plain, plainFieldIn: true));
    await tester.pumpAndSettle();

    final sent = <String>[];
    final recorder = ProbeRecorder()..start(sent.add);

    // Without its key the plain field is not secret on its own. Its controller stays obscured for this recording.
    await tester.pumpWidget(_unkeyedPlain(password, plain));
    await tester.pump(const Duration(milliseconds: 600));
    plain.text = _typed;
    await tester.pump(const Duration(milliseconds: 310));
    recorder.stop();

    expect(sent.any((m) => m.contains(_typed)), isFalse,
        reason: 'a field that was secret earlier in the recording must not be sent as text');
    final events = _typeEvents(sent);
    expect(events, hasLength(1));
    expect(events.single['text'], '');
    expect(events.single['obscured'], isTrue);
  });

  testWidgets('text pending in a plain field that leaves the tree is withheld, not sent as text', (tester) async {
    final plain = TextEditingController();
    addTearDown(plain.dispose);
    await tester.pumpWidget(_plainField(plain, present: true));
    await tester.pumpAndSettle();

    final sent = <String>[];
    final recorder = ProbeRecorder()..start(sent.add);

    // The field is removed before its debounced emit. Nothing about it can be read now, so its text is withheld.
    plain.text = 'plain-pending';
    await tester.pumpWidget(_plainField(plain, present: false));
    await tester.pump(const Duration(milliseconds: 310));
    recorder.stop();

    expect(sent.any((m) => m.contains('plain-pending')), isFalse,
        reason: 'a field that has left the tree cannot be read, so its text is withheld');
    final events = _typeEvents(sent);
    expect(events, hasLength(1));
    expect(events.single['text'], '');
    expect(events.single['obscured'], isTrue);
  });

  testWidgets('text pending when a recording stops is not sent by the next recording', (tester) async {
    final plain = TextEditingController();
    addTearDown(plain.dispose);
    await tester.pumpWidget(_plainField(plain, present: true));
    await tester.pumpAndSettle();

    final sent = <String>[];
    final recorder = ProbeRecorder()..start((_) {});
    plain.text = 'pending-text';
    recorder.stop(); // before the 300 ms debounce fires

    recorder.start(sent.add);
    await tester.pump(const Duration(milliseconds: 310));
    recorder.stop();

    expect(sent.any((m) => m.contains('pending-text')), isFalse,
        reason: 'a timer from an earlier recording must not emit into a later one');
    expect(_typeEvents(sent), isEmpty);
  });
}
