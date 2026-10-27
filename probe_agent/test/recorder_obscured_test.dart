import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_probe_agent/src/recorder.dart';

// Synthetic values only. The secret stands in for a password typed into an
// obscured field; the email stands in for a plain field, which is still sent.
const _secret = 'zq-obscured-7731';
const _email = 'dev@example.invalid';

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

/// A bare [EditableText] with [key] on it. The recorder's label search checks
/// the EditableText's own key first, so the event carries the lookup. A
/// Material TextField's key sits 15 ancestors above its EditableText, one past
/// that search's limit, so a TextField event has no selector today.
Widget _editable(
  TextEditingController controller,
  FocusNode focusNode, {
  required String key,
  required bool obscured,
}) {
  return MaterialApp(
    home: Scaffold(
      body: Center(
        child: EditableText(
          key: ValueKey(key),
          controller: controller,
          focusNode: focusNode,
          style: const TextStyle(),
          cursorColor: Colors.blue,
          backgroundCursorColor: Colors.grey,
          obscureText: obscured,
        ),
      ),
    ),
  );
}

void main() {
  group('ProbeRecorder text events for obscured and plain fields', () {
    testWidgets(
        'an obscured TextField emits a type event with empty text and obscured true, never the value',
        (tester) async {
      final password = TextEditingController();
      addTearDown(password.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(
            key: const ValueKey('password'),
            controller: password,
            obscureText: true,
          ),
        ),
      ));
      await tester.pumpAndSettle();

      final sent = <String>[];
      final recorder = ProbeRecorder()..start(sent.add);
      // The field scan runs every 500 ms, so let it pick the field up.
      await tester.pump(const Duration(milliseconds: 600));

      password.text = _secret;
      // Past the 300 ms text debounce.
      await tester.pump(const Duration(milliseconds: 500));
      recorder.stop();

      expect(sent.any((m) => m.contains(_secret)), isFalse,
          reason: 'no recorded message may carry the obscured value');
      final events = _typeEvents(sent);
      expect(events, hasLength(1));
      expect(events.single['text'], '');
      expect(events.single['obscured'], isTrue);
      // No selector is asserted here: see _editable for why a TextField's key
      // is not reached by the lookup.
    });

    testWidgets(
        'an obscured field that has a lookup carries it: selector, empty text, obscured true',
        (tester) async {
      final password = TextEditingController();
      final focusNode = FocusNode();
      addTearDown(password.dispose);
      addTearDown(focusNode.dispose);
      await tester.pumpWidget(_editable(
        password,
        focusNode,
        key: 'password',
        obscured: true,
      ));
      await tester.pumpAndSettle();

      final sent = <String>[];
      final recorder = ProbeRecorder()..start(sent.add);
      await tester.pump(const Duration(milliseconds: 600));

      password.text = _secret;
      await tester.pump(const Duration(milliseconds: 500));
      recorder.stop();

      expect(sent.any((m) => m.contains(_secret)), isFalse,
          reason: 'no recorded message may carry the obscured value');
      final events = _typeEvents(sent);
      expect(events, hasLength(1));
      expect(events.single, {
        'action': 'type',
        'text': '',
        'obscured': true,
        'selector': {'kind': 'id', 'text': '#password'},
        'timestamp': events.single['timestamp'],
      });
    });

    testWidgets(
        'a plain field still emits its text with its lookup, and no obscured flag',
        (tester) async {
      final email = TextEditingController();
      final focusNode = FocusNode();
      addTearDown(email.dispose);
      addTearDown(focusNode.dispose);
      await tester.pumpWidget(_editable(
        email,
        focusNode,
        key: 'email',
        obscured: false,
      ));
      await tester.pumpAndSettle();

      final sent = <String>[];
      final recorder = ProbeRecorder()..start(sent.add);
      await tester.pump(const Duration(milliseconds: 600));

      email.text = _email;
      await tester.pump(const Duration(milliseconds: 500));
      recorder.stop();

      final events = _typeEvents(sent);
      expect(events, hasLength(1));
      expect(events.single, {
        'action': 'type',
        'text': _email,
        'selector': {'kind': 'id', 'text': '#email'},
        'timestamp': events.single['timestamp'],
      });
    });

    testWidgets(
        'a password field revealed after typing keeps its value out of later events',
        (tester) async {
      final password = TextEditingController();
      final focusNode = FocusNode();
      addTearDown(password.dispose);
      addTearDown(focusNode.dispose);
      await tester.pumpWidget(_editable(
        password,
        focusNode,
        key: 'password',
        obscured: true,
      ));
      await tester.pumpAndSettle();

      final sent = <String>[];
      final recorder = ProbeRecorder()..start(sent.add);
      await tester.pump(const Duration(milliseconds: 600));

      password.text = _secret;
      await tester.pump(const Duration(milliseconds: 500));

      // The show-password toggle: the same field, no longer obscured.
      await tester.pumpWidget(_editable(
        password,
        focusNode,
        key: 'password',
        obscured: false,
      ));
      await tester.pump();

      password.text = '$_secret!';
      await tester.pump(const Duration(milliseconds: 500));
      recorder.stop();

      expect(sent.any((m) => m.contains(_secret)), isFalse,
          reason: 'no recorded message may carry the revealed value');
      final events = _typeEvents(sent);
      expect(events, hasLength(2));
      for (final event in events) {
        expect(event['text'], '');
        expect(event['obscured'], isTrue);
      }
    });

    testWidgets(
        'a password field revealed before any typing is still treated as one',
        (tester) async {
      final password = TextEditingController();
      final focusNode = FocusNode();
      addTearDown(password.dispose);
      addTearDown(focusNode.dispose);
      await tester.pumpWidget(_editable(
        password,
        focusNode,
        key: 'password',
        obscured: true,
      ));
      await tester.pumpAndSettle();

      final sent = <String>[];
      final recorder = ProbeRecorder()..start(sent.add);
      // The field scan records the field as obscured before the reveal.
      await tester.pump(const Duration(milliseconds: 600));

      await tester.pumpWidget(_editable(
        password,
        focusNode,
        key: 'password',
        obscured: false,
      ));
      await tester.pump();

      password.text = _secret;
      await tester.pump(const Duration(milliseconds: 500));
      recorder.stop();

      expect(sent.any((m) => m.contains(_secret)), isFalse,
          reason: 'no recorded message may carry the revealed value');
      final events = _typeEvents(sent);
      expect(events, hasLength(1));
      expect(events.single['text'], '');
      expect(events.single['obscured'], isTrue);
    });

    testWidgets(
        'a field obscured only at a text change, then revealed, emits no text when the listener fires again',
        (tester) async {
      final password = TextEditingController();
      final focusNode = FocusNode();
      addTearDown(password.dispose);
      addTearDown(focusNode.dispose);
      await tester.pumpWidget(_editable(
        password,
        focusNode,
        key: 'listener_pw',
        obscured: false,
      ));
      await tester.pumpAndSettle();

      final sent = <String>[];
      final recorder = ProbeRecorder()..start(sent.add);
      // The field scan sees the field plain, so only the listener can record it.
      await tester.pump(const Duration(milliseconds: 100));

      await tester.pumpWidget(_editable(
        password,
        focusNode,
        key: 'listener_pw',
        obscured: true,
      ));
      await tester.pump();
      password.text = _secret;
      // Revealed again before the 300 ms debounce fires.
      await tester.pumpWidget(_editable(
        password,
        focusNode,
        key: 'listener_pw',
        obscured: false,
      ));
      await tester.pump(const Duration(milliseconds: 400));

      password.text = '$_secret!';
      await tester.pump(const Duration(milliseconds: 400));
      recorder.stop();

      expect(sent.any((m) => m.contains(_secret)), isFalse,
          reason: 'no recorded message may carry the value');
      final events = _typeEvents(sent);
      expect(events, hasLength(2));
      for (final event in events) {
        expect(event['text'], '');
        expect(event['obscured'], isTrue);
      }
    });

    testWidgets(
        'a field that becomes obscured before its debounced text is sent emits no text',
        (tester) async {
      final password = TextEditingController();
      final focusNode = FocusNode();
      addTearDown(password.dispose);
      addTearDown(focusNode.dispose);
      await tester.pumpWidget(_editable(
        password,
        focusNode,
        key: 'pending_pw',
        obscured: false,
      ));
      await tester.pumpAndSettle();

      final sent = <String>[];
      final recorder = ProbeRecorder()..start(sent.add);
      await tester.pump(const Duration(milliseconds: 100));

      // Typed while plain, so the listener records nothing secret yet.
      password.text = _secret;
      await tester.pump(const Duration(milliseconds: 50));
      // The app switches to password mode before the 300 ms debounce sends the text.
      await tester.pumpWidget(_editable(
        password,
        focusNode,
        key: 'pending_pw',
        obscured: true,
      ));
      await tester.pump(const Duration(milliseconds: 300));
      recorder.stop();

      expect(sent.any((m) => m.contains(_secret)), isFalse,
          reason: 'no recorded message may carry the value');
      final events = _typeEvents(sent);
      expect(events, hasLength(1));
      expect(events.single['text'], '');
      expect(events.single['obscured'], isTrue);
    });

    testWidgets(
        'a keyed password field swapped for a plain field with the same key emits no text',
        (tester) async {
      final password = TextEditingController();
      final plain = TextEditingController();
      addTearDown(password.dispose);
      addTearDown(plain.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(
            key: const ValueKey('swap_pw'),
            controller: password,
            obscureText: true,
          ),
        ),
      ));
      await tester.pumpAndSettle();

      final sent = <String>[];
      final recorder = ProbeRecorder()..start(sent.add);
      // The field scan records the password field as obscured.
      await tester.pump(const Duration(milliseconds: 600));

      // The app swaps in a plain field under the same key, with a new controller.
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Padding(
            padding: EdgeInsets.zero,
            child: TextField(
              key: const ValueKey('swap_pw'),
              controller: plain,
            ),
          ),
        ),
      ));
      // The next field scan picks up the new controller.
      await tester.pump(const Duration(milliseconds: 500));

      plain.text = _secret;
      await tester.pump(const Duration(milliseconds: 500));
      recorder.stop();

      expect(sent.any((m) => m.contains(_secret)), isFalse,
          reason: 'no recorded message may carry the value of the swapped field');
      final events = _typeEvents(sent);
      expect(events, hasLength(1));
      expect(events.single['text'], '');
      expect(events.single['obscured'], isTrue);
    });
  });
}
