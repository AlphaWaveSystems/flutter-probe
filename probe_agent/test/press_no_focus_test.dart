import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

/// Sends one press_key request to a fresh executor and returns the response it wrote.
Future<Map<String, dynamic>> _press(WidgetTester tester, String key) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(
          jsonrpc: '2.0', id: 1, method: ProbeMethods.deviceAction, params: {'action': 'press_key', 'value': key}))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

// "Nothing focused" means no widget holds the focus. The route's own focus scope is still the primary focus then, so these
// tests check the outcome of each key, not whether primaryFocus is null. The behaviour below was measured on these screens
// and is what the dictionary and the CHANGELOG say.

Widget _textOnly() => const MaterialApp(home: Scaffold(body: Text('Home')));

void main() {
  testWidgets('press tab with nothing focused moves the focus to the first focusable widget, and does not warn',
      (tester) async {
    final first = FocusNode();
    final second = FocusNode();
    addTearDown(first.dispose);
    addTearDown(second.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [TextField(focusNode: first), TextField(focusNode: second)]),
      ),
    ));
    final res = await _press(tester, 'tab');
    expect(res['error'], isNull);
    expect(res['result']['warning'], isNull);
    expect(first.hasFocus, isTrue);
    expect(second.hasFocus, isFalse);
  });

  testWidgets('press tab with nothing focused and nothing focusable does nothing, and does not warn', (tester) async {
    await tester.pumpWidget(_textOnly());
    final res = await _press(tester, 'tab');
    expect(res['error'], isNull);
    expect(res['result']['warning'], isNull);
  });

  testWidgets('press escape with nothing focused does nothing, and does not warn', (tester) async {
    await tester.pumpWidget(_textOnly());
    final res = await _press(tester, 'escape');
    expect(res['error'], isNull);
    expect(res['result']['warning'], isNull);
  });

  testWidgets('press escape with a field focused leaves it, and does not warn', (tester) async {
    final field = FocusNode();
    addTearDown(field.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: Column(children: [TextField(focusNode: field)])),
    ));
    field.requestFocus();
    await tester.pump();
    final res = await _press(tester, 'escape');
    expect(res['error'], isNull);
    expect(res['result']['warning'], isNull);
    expect(field.hasFocus, isFalse);
  });

  for (final key in ['up', 'down', 'left', 'right']) {
    testWidgets('press $key with nothing focused and nothing focusable does nothing, and does not warn', (tester) async {
      await tester.pumpWidget(_textOnly());
      final res = await _press(tester, key);
      expect(res['error'], isNull);
      expect(res['result']['warning'], isNull);
    });

    testWidgets('press $key with nothing focused moves the focus into a field, and does not warn', (tester) async {
      final a = FocusNode();
      final b = FocusNode();
      addTearDown(a.dispose);
      addTearDown(b.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(body: Column(children: [TextField(focusNode: a), TextField(focusNode: b)])),
      ));
      final res = await _press(tester, key);
      expect(res['error'], isNull);
      expect(res['result']['warning'], isNull);
      expect(a.hasFocus || b.hasFocus, isTrue);
    });

    testWidgets('press $key with a field focused and nothing that way warns that there is nothing to move to',
        (tester) async {
      final a = FocusNode();
      addTearDown(a.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(body: Column(children: [TextField(focusNode: a)])),
      ));
      a.requestFocus();
      await tester.pump();
      final res = await _press(tester, key);
      expect(res['error'], isNull);
      expect(res['result']['warning'], contains('nothing to move the focus to in that direction'));
    });
  }

  testWidgets('press backspace with nothing focused warns that no text field has focus, and deletes nothing',
      (tester) async {
    await tester.pumpWidget(_textOnly());
    final res = await _press(tester, 'backspace');
    expect(res['error'], isNull);
    expect(res['result']['warning'], contains('no text field has focus'));
  });

  testWidgets('press space with nothing focused warns that no text field has focus, and types nothing', (tester) async {
    await tester.pumpWidget(_textOnly());
    final res = await _press(tester, 'space');
    expect(res['error'], isNull);
    expect(res['result']['warning'], contains('no text field has focus'));
  });
}
