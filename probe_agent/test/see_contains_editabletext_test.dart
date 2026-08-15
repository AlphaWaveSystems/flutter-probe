import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

void main() {
  group('see #id contains "..." reads TextField/TextFormField content', () {
    // _textOf only checked Text/RichText, so `see #field contains "..."`
    // silently returned '' for any TextField/TextFormField selector — the
    // check always failed no matter what the field actually contained.
    late String? lastSent;
    late ProbeExecutor executor;

    setUp(() {
      lastSent = null;
      executor = ProbeExecutor((msg) => lastSent = msg);
    });

    Future<bool> isError() async {
      final decoded = jsonDecode(lastSent!) as Map<String, dynamic>;
      return decoded.containsKey('error');
    }

    Future<void> seeContains(String id, String value) => executor.dispatch(ProbeRequest(
          jsonrpc: '2.0',
          id: 1,
          method: ProbeMethods.see,
          params: {
            'selector': {'kind': 'id', 'text': id},
            'check': 'contains',
            'check_val': value,
          },
        ));

    testWidgets('passes when a TextField pre-filled via a controller contains the checked value',
        (tester) async {
      final controller = TextEditingController(text: 'user@example.com');
      addTearDown(controller.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(key: const ValueKey('email_field'), controller: controller),
        ),
      ));

      await seeContains('#email_field', 'user@example.com');
      expect(await isError(), isFalse, reason: 'expected no error, got: $lastSent');
    });

    testWidgets('passes when a TextFormField pre-filled via a controller contains the checked value',
        (tester) async {
      final controller = TextEditingController(text: 'Jane Doe');
      addTearDown(controller.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Form(
            child: TextFormField(key: const ValueKey('name_field'), controller: controller),
          ),
        ),
      ));

      await seeContains('#name_field', 'Jane');
      expect(await isError(), isFalse, reason: 'expected no error, got: $lastSent');
    });

    testWidgets('fails with the actual field content in the message, not a blank string',
        (tester) async {
      final controller = TextEditingController(text: 'wrong value');
      addTearDown(controller.dispose);

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(key: const ValueKey('field_a'), controller: controller),
        ),
      ));

      await seeContains('#field_a', 'expected value');
      expect(await isError(), isTrue);
      final decoded = jsonDecode(lastSent!) as Map<String, dynamic>;
      final message = decoded['error']['message'] as String;
      expect(message, contains('wrong value'),
          reason: 'error should report the real field content, not an empty string: $message');
    });
  });
}
