import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

// Synthetic values only. The secret stands in for a password typed into an
// obscured field; the email stands in for a plain field, whose text may show.
const _secret = 'zq-obscured-7731';
const _email = 'dev@example.invalid';

void main() {
  group('an obscured (password) field never reaches see or wait output', () {
    late String raw;
    late ProbeExecutor executor;

    setUp(() {
      raw = '';
      executor = ProbeExecutor((msg) => raw = msg);
    });

    /// Runs one request to completion and returns the decoded response. The
    /// clock is pumped while it runs, and real time passes between pumps,
    /// because `wait` polls on the real clock (DateTime.now is not faked).
    Future<Map<String, dynamic>> run(
      WidgetTester tester,
      String method,
      Map<String, dynamic> params,
    ) async {
      var done = false;
      final call = executor
          .dispatch(ProbeRequest(
              jsonrpc: '2.0', id: 1, method: method, params: params))
          .whenComplete(() => done = true);
      for (var i = 0; i < 200 && !done; i++) {
        await tester.runAsync(
            () => Future<void>.delayed(const Duration(milliseconds: 20)));
        await tester.pump(const Duration(milliseconds: 20));
      }
      await call;
      return jsonDecode(raw) as Map<String, dynamic>;
    }

    testWidgets(
        'see contains: a failing check says the field is obscured and omits its value',
        (tester) async {
      final password = TextEditingController(text: _secret);
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

      final res = await run(tester, ProbeMethods.see, {
        'selector': {'kind': 'id', 'text': '#password'},
        'check': 'contains',
        'check_val': 'not-the-value',
      });
      expect(res['error'], isNotNull);
      final message = res['error']['message'] as String;
      expect(message.contains('obscured field'), isTrue,
          reason: 'the message should say why the check could not run');
      expect(raw.contains(_secret), isFalse,
          reason: 'the response must not carry the obscured value');
    });

    testWidgets(
        'see matching: a failing pattern on an obscured field omits its value',
        (tester) async {
      final password = TextEditingController(text: _secret);
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

      final res = await run(tester, ProbeMethods.see, {
        'selector': {'kind': 'id', 'text': '#password'},
        'pattern': r'^not-the-value$',
      });
      expect(res['error'], isNotNull);
      final message = res['error']['message'] as String;
      expect(message.contains('obscured field'), isTrue,
          reason: 'the message should say why the pattern could not match');
      expect(raw.contains(_secret), isFalse,
          reason: 'the response must not carry the obscured value');
    });

    testWidgets(
        "see with don't: a contains check on an obscured field fails, not passes",
        (tester) async {
      final password = TextEditingController(text: _secret);
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

      final res = await run(tester, ProbeMethods.see, {
        'selector': {'kind': 'id', 'text': '#password'},
        'negated': true,
        'check': 'contains',
        'check_val': 'not-the-value',
      });
      expect(res['error'], isNotNull,
          reason: 'hidden text cannot be shown to lack the value, so the '
              'negation must not pass silently');
      expect(raw.contains(_secret), isFalse,
          reason: 'the response must not carry the obscured value');
    });

    testWidgets(
        'wait until appears matching: an obscured field cannot satisfy the pattern, and its value stays out of the timeout',
        (tester) async {
      final password = TextEditingController(text: _secret);
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

      final res = await run(tester, ProbeMethods.wait, {
        'kind': 'appears',
        'target': '#password',
        'pattern': r'^not-the-value$',
        'timeout': 0.4,
      });
      expect(res['error'], isNotNull,
          reason:
              'the pattern cannot match hidden text, so the wait times out');
      expect(raw.contains(_secret), isFalse,
          reason: 'the timeout message must not carry the obscured value');
    });

    testWidgets(
        'a plain field still reports its text in a failing contains check',
        (tester) async {
      final email = TextEditingController(text: _email);
      addTearDown(email.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(
            key: const ValueKey('email'),
            controller: email,
          ),
        ),
      ));

      final res = await run(tester, ProbeMethods.see, {
        'selector': {'kind': 'id', 'text': '#email'},
        'check': 'contains',
        'check_val': 'not-the-value',
      });
      expect(res['error'], isNotNull);
      final message = res['error']['message'] as String;
      expect(message.contains(_email), isTrue,
          reason: 'a plain field must keep reporting its text');
    });

    testWidgets(
        'a bare EditableText that is obscured gives no text to a failing contains check',
        (tester) async {
      final password = TextEditingController(text: _secret);
      final focusNode = FocusNode();
      addTearDown(password.dispose);
      addTearDown(focusNode.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: EditableText(
            key: const ValueKey('bare_password'),
            controller: password,
            focusNode: focusNode,
            style: const TextStyle(),
            cursorColor: Colors.blue,
            backgroundCursorColor: Colors.grey,
            obscureText: true,
          ),
        ),
      ));

      final res = await run(tester, ProbeMethods.see, {
        'selector': {'kind': 'id', 'text': '#bare_password'},
        'check': 'contains',
        'check_val': 'not-the-value',
      });
      expect(res['error'], isNotNull);
      expect(raw.contains(_secret), isFalse,
          reason: 'the bare field branch must not hand its value to a message');
    });

    testWidgets(
        'a password field revealed after an earlier check still gives no text',
        (tester) async {
      final password = TextEditingController(text: _secret);
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
      // The first check sees the field obscured, which keeps it secret for the rest of the run.
      await run(tester, ProbeMethods.see, {
        'selector': {'kind': 'id', 'text': '#password'},
        'check': 'contains',
        'check_val': 'not-the-value',
      });

      // The show-password toggle: the same field, no longer obscured.
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(
            key: const ValueKey('password'),
            controller: password,
            obscureText: false,
          ),
        ),
      ));
      await tester.pump();

      final res = await run(tester, ProbeMethods.see, {
        'selector': {'kind': 'id', 'text': '#password'},
        'check': 'contains',
        'check_val': 'not-the-value',
      });
      expect(res['error'], isNotNull);
      expect(raw.contains(_secret), isFalse,
          reason: 'a revealed password field must stay secret');
    });

    testWidgets(
        'a keyed obscured field swapped for a plain field with the same key: see, wait and visible_summary never show its value',
        (tester) async {
      final first = TextEditingController(text: _secret);
      final plain = TextEditingController(text: _secret);
      addTearDown(first.dispose);
      addTearDown(plain.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: TextField(
            key: const ValueKey('swap_pw'),
            controller: first,
            obscureText: true,
          ),
        ),
      ));
      // A check that reads the field while it is obscured keeps its key secret.
      await run(tester, ProbeMethods.see, {
        'selector': {'kind': 'id', 'text': '#swap_pw'},
        'check': 'contains',
        'check_val': 'not-the-value',
      });

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
      await tester.pump();

      final see = await run(tester, ProbeMethods.see, {
        'selector': {'kind': 'id', 'text': '#swap_pw'},
        'check': 'contains',
        'check_val': 'not-the-value',
      });
      expect(see['error'], isNotNull);
      expect(jsonEncode(see), isNot(contains(_secret)),
          reason: 'see must not carry the value of the swapped field');

      final wait = await run(tester, ProbeMethods.wait, {
        'kind': 'appears',
        'target': '#swap_pw',
        'pattern': r'^not-the-value$',
        'timeout': 0.4,
      });
      expect(wait['error'], isNotNull);
      expect(jsonEncode(wait), isNot(contains(_secret)),
          reason: 'the wait timeout must not carry the value of the swapped field');

      final summary = await run(tester, ProbeMethods.visibleSummary, const {});
      expect(summary['error'], isNull);
      expect(jsonEncode(summary), isNot(contains(_secret)),
          reason: 'visible_summary must not carry the value of the swapped field');
    });

    testWidgets('a failing tap hint shows the whole visible text, not a cut of it',
        (tester) async {
      const long = 'This visible sentence is well over forty characters long';
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(body: Text(long)),
      ));

      final res = await run(tester, ProbeMethods.tap, {
        'selector': {'kind': 'id', 'text': '#missing_button'},
      });
      expect(res['error'], isNotNull);
      final message = res['error']['message'] as String;
      expect(message.contains(long), isTrue,
          reason: 'the hint must carry the whole text, not its first forty characters');
    });
  });
}
