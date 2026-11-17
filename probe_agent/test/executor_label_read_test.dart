import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

// Issue #818: a widget with no text is found by its semantic label or tooltip, and reads as that label, so `see "X" contains "..."` and
// `see "X" matching "..."` check it instead of reading "".
Future<String?> _seeError(String text, Map<String, String> check) async {
  String? sent;
  final executor = ProbeExecutor((msg) => sent = msg);
  await executor.dispatch(
      ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
    'selector': {'kind': 'text', 'text': text},
    ...check,
  }));
  final error = (jsonDecode(sent!) as Map<String, dynamic>)['error'];
  return error == null ? null : '$error';
}

Future<String?> _seeErrorSel(
    Map<String, dynamic> sel, Map<String, String> check) async {
  String? sent;
  final executor = ProbeExecutor((msg) => sent = msg);
  await executor.dispatch(ProbeRequest(
      jsonrpc: '2.0',
      id: 1,
      method: ProbeMethods.see,
      params: {'selector': sel, ...check}));
  final error = (jsonDecode(sent!) as Map<String, dynamic>)['error'];
  return error == null ? null : '$error';
}

Future<String?> _contains(String text, String value) =>
    _seeError(text, {'check': 'contains', 'check_val': value});

// A 1×1 PNG.
final _pixel = Uint8List.fromList(base64Decode(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=='));

Future<void> _pump(WidgetTester tester, Widget body) =>
    tester.pumpWidget(MaterialApp(home: Scaffold(body: Center(child: body))));

void main() {
  testWidgets('an Icon with a semanticLabel reads as its label',
      (tester) async {
    await _pump(tester, const Icon(Icons.favorite, semanticLabel: 'Favorite'));
    expect(await _contains('Favorite', 'Favorite'), isNull);
    expect(await _contains('Favorite', 'Fav'), isNull);
    expect(await _contains('Favorite', 'Heart'),
        contains('contains "Favorite", not "Heart"'));
  });

  testWidgets('an Image with a semanticLabel reads as its label',
      (tester) async {
    await _pump(tester,
        Image.memory(_pixel, width: 48, height: 48, semanticLabel: 'Logo'));
    expect(await _contains('Logo', 'Logo'), isNull);
    expect(await _contains('Logo', 'Banner'),
        contains('contains "Logo", not "Banner"'));
  });

  testWidgets('an IconButton found by its tooltip reads as the tooltip',
      (tester) async {
    await _pump(
        tester,
        IconButton(
            onPressed: () {},
            tooltip: 'Add item',
            icon: const Icon(Icons.add)));
    expect(await _contains('Add item', 'Add'), isNull);
    expect(await _contains('Add item', 'Remove'),
        contains('contains "Add item", not "Remove"'));
  });

  testWidgets('a widget wrapped in a Tooltip reads as the message',
      (tester) async {
    await _pump(
        tester,
        const Tooltip(
            message: 'Sync status', child: SizedBox(width: 40, height: 40)));
    expect(await _contains('Sync status', 'Sync'), isNull);
    expect(await _contains('Sync status', 'Upload'),
        contains('contains "Sync status", not "Upload"'));
  });

  testWidgets('a pattern is matched against the label', (tester) async {
    await _pump(
        tester, const Icon(Icons.favorite, semanticLabel: 'Favorite 3'));
    expect(await _seeError('Favorite', {'pattern': r'^Favorite \d$'}), isNull);
    expect(await _seeError('Favorite', {'pattern': r'^\d$'}), isNotNull);
  });

  testWidgets('a Text is read as before, not as a Semantics label around it',
      (tester) async {
    await _pump(
        tester,
        Semantics(
            label: 'Total label', child: const Text('Order total: 42.00')));
    expect(await _contains('Order total', '42.00'), isNull);
    expect(await _contains('Order total', 'Total label'),
        contains('contains "Order total: 42.00"'));
  });

  // A labelled wrapper around a text field still reads as the field's text: the label is read only for a widget with no text of its own.
  Future<String?> byId(String id, String value) => _seeErrorSel(
      {'kind': 'id', 'text': '#$id'},
      {'check': 'contains', 'check_val': value});

  testWidgets(
      'a Semantics with a label around a TextField, found by id, reads as the field',
      (tester) async {
    await _pump(
        tester,
        Semantics(
            identifier: 'email_id',
            label: 'Email',
            child: TextField(
                controller: TextEditingController(text: 'typed value'))));
    expect(await byId('email_id', 'typed value'), isNull);
    expect(await byId('email_id', 'Email'),
        contains('contains "typed value", not "Email"'));
  });

  testWidgets(
      'a Semantics with a label around a TextField, found by the label, reads as the field',
      (tester) async {
    await _pump(
        tester,
        Semantics(
            label: 'Email field',
            child: TextField(
                controller: TextEditingController(text: 'typed value'))));
    expect(await _contains('Email field', 'typed value'), isNull);
  });

  testWidgets(
      'a Tooltip around a TextField, found by the message, reads as the field',
      (tester) async {
    await _pump(
        tester,
        Tooltip(
            message: 'Name',
            child: SizedBox(
                width: 200,
                child: TextField(
                    controller: TextEditingController(text: 'Ada')))));
    expect(await _contains('Name', 'Ada'), isNull);
  });

  testWidgets(
      'a labelled Semantics around an obscured field still reads as nothing',
      (tester) async {
    await _pump(
        tester,
        Semantics(
            identifier: 'pw_id',
            label: 'Password',
            child: TextField(
                obscureText: true,
                controller: TextEditingController(text: 'zq-secret-1'))));
    final error = await byId('pw_id', 'Password');
    expect(error, contains('obscured field'),
        reason: 'the label must not stand in for an obscured field');
    expect(error, isNot(contains('zq-secret-1')));
  });

  // Out of scope (documented): found by id, an icon button is the IconButton, not its Tooltip, so it reads as "".
  testWidgets('an IconButton found by id reads as empty, not as its tooltip',
      (tester) async {
    await _pump(
        tester,
        IconButton(
            key: const ValueKey('add_id'),
            onPressed: () {},
            tooltip: 'Add item',
            icon: const Icon(Icons.add)));
    expect(await byId('add_id', 'Add'), contains('contains "", not "Add"'));
  });
}
