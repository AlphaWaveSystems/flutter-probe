import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/agent_version.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

/// Sends one request to a fresh executor and returns the response it wrote.
Future<Map<String, dynamic>> _dispatch(WidgetTester tester, String method, Map<String, dynamic> params) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

Future<Map<String, dynamic>> _press(WidgetTester tester, String key) =>
    _dispatch(tester, ProbeMethods.deviceAction, {'action': 'press_key', 'value': key});

void main() {
  testWidgets('press backspace deletes the character before the caret', (tester) async {
    final controller = TextEditingController(text: 'ab');
    final focus = FocusNode();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: TextField(controller: controller, focusNode: focus)),
    ));
    focus.requestFocus();
    controller.selection = const TextSelection.collapsed(offset: 2);
    await tester.pump();
    final res = await _press(tester, 'backspace');
    expect(res['error'], isNull);
    expect(res['result']['warning'], isNull);
    expect(controller.text, 'a');
    expect(controller.selection, const TextSelection.collapsed(offset: 1));
  });

  testWidgets('press backspace at the start of the field changes nothing', (tester) async {
    final controller = TextEditingController(text: 'ab');
    final focus = FocusNode();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: TextField(controller: controller, focusNode: focus)),
    ));
    focus.requestFocus();
    controller.selection = const TextSelection.collapsed(offset: 0);
    await tester.pump();
    final res = await _press(tester, 'backspace');
    expect(res['error'], isNull);
    expect(controller.text, 'ab');
  });

  testWidgets('press space inserts a space at the caret', (tester) async {
    final controller = TextEditingController(text: 'ab');
    final focus = FocusNode();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: TextField(controller: controller, focusNode: focus)),
    ));
    focus.requestFocus();
    controller.selection = const TextSelection.collapsed(offset: 2);
    await tester.pump();
    final res = await _press(tester, 'space');
    expect(res['error'], isNull);
    expect(controller.text, 'ab ');
  });

  testWidgets('press space with no focused field warns instead of failing', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('Home'))));
    final res = await _press(tester, 'space');
    expect(res['error'], isNull);
    expect(res['result']['warning'], contains('no text field has focus'));
  });

  testWidgets('press escape leaves the focused field', (tester) async {
    final focus = FocusNode();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: TextField(focusNode: focus)),
    ));
    focus.requestFocus();
    await tester.pump();
    expect(focus.hasFocus, isTrue);
    final res = await _press(tester, 'escape');
    expect(res['error'], isNull);
    expect(focus.hasFocus, isFalse);
  });

  testWidgets('press tab moves the focus to the next field', (tester) async {
    final first = FocusNode();
    final second = FocusNode();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          TextField(focusNode: first),
          TextField(focusNode: second),
        ]),
      ),
    ));
    first.requestFocus();
    await tester.pump();
    final res = await _press(tester, 'tab');
    expect(res['error'], isNull);
    expect(second.hasFocus, isTrue);
  });

  testWidgets('an unknown key is an error that names the keys', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('Home'))));
    final res = await _press(tester, 'home');
    expect(res['error'], isNotNull);
    expect(res['error'].toString(), contains('tab, escape, backspace, space'));
  });

  testWidgets('a device action this agent does not know is an error, not an ok', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: Text('Home'))));
    final res = await _dispatch(tester, ProbeMethods.deviceAction, {'action': 'bogus', 'value': ''});
    expect(res['error'], isNotNull);
    expect(res['error'].toString(), contains('unknown device action "bogus"'));
    expect(res['result'], isNull);
  });

  testWidgets('ping reports the agent version and the press_key capability', (tester) async {
    final res = await _dispatch(tester, ProbeMethods.ping, {});
    expect(res['result']['agent_version'], probeAgentVersion);
    expect(res['result']['capabilities'], contains('press_key'));
  });
}
