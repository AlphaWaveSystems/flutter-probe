import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/finder.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

// Synthetic values only. The email is a plain field's value that must stay
// visible; the secret stands in for a password typed into an obscured field.
const _email = 'dev@example.invalid';
const _secret = 'quorvex-secret-123';

/// Dispatches one request and returns the decoded JSON-RPC response. Pumps the
/// test clock while waiting, because some actions (the covered-tap check)
/// settle over several frames before they answer.
Future<Map<String, dynamic>> _dispatch(
  WidgetTester tester,
  String method,
  Map<String, dynamic> params,
) async {
  String? raw;
  final executor = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = executor
      .dispatch(
          ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

void main() {
  group('visible summary never shows an obscured (password) field', () {
    testWidgets('a plain field is listed, the obscured field is not',
        (tester) async {
      final email = TextEditingController(text: _email);
      final password = TextEditingController(text: _secret);
      addTearDown(email.dispose);
      addTearDown(password.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(children: [
            TextField(controller: email),
            TextField(controller: password, obscureText: true),
          ]),
        ),
      ));

      final s = ProbeFinder.instance.visibleSummary();
      expect(s['texts'], contains(_email));
      expect(s['texts'], everyElement(isNot(contains(_secret))));
      expect(s['keys'], everyElement(isNot(contains(_secret))));
    });

    testWidgets(
        'with only the obscured field on screen, no text mentions the secret',
        (tester) async {
      final password = TextEditingController(text: _secret);
      addTearDown(password.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(children: [
            TextField(controller: password, obscureText: true),
          ]),
        ),
      ));

      final s = ProbeFinder.instance.visibleSummary();
      expect(s['texts'], everyElement(isNot(contains(_secret))));
      expect(s['keys'], everyElement(isNot(contains(_secret))));
    });

    testWidgets('the visible_summary RPC does not return the obscured value',
        (tester) async {
      final email = TextEditingController(text: _email);
      final password = TextEditingController(text: _secret);
      addTearDown(email.dispose);
      addTearDown(password.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(children: [
            TextField(controller: email),
            TextField(controller: password, obscureText: true),
          ]),
        ),
      ));

      final res =
          await _dispatch(tester, ProbeMethods.visibleSummary, const {});
      expect(res['error'], isNull);
      expect(res['result']['texts'], contains(_email));
      expect(jsonEncode(res), isNot(contains(_secret)));
    });

    testWidgets('a covered-tap warning does not print the obscured value',
        (tester) async {
      final password = TextEditingController(text: _secret);
      addTearDown(password.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Stack(children: [
            Column(children: [
              const Text('Sign in'),
              TextField(controller: password, obscureText: true),
              Center(
                child: ElevatedButton(
                  key: const ValueKey('share_button'),
                  onPressed: () {},
                  child: const Text('Share'),
                ),
              ),
            ]),
            // A full-screen overlay that takes the tap, as a loading veil would.
            Positioned.fill(
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: () {},
                child: const SizedBox.expand(),
              ),
            ),
          ]),
        ),
      ));

      final res = await _dispatch(tester, ProbeMethods.tap, {
        'selector': {'kind': 'id', 'text': '#share_button'},
      });
      expect(res['error'], isNull);
      final warning = res['result']['warning'] as String?;
      expect(warning, isNotNull,
          reason: 'the button is covered, so the tap must warn');
      expect(warning, contains('covered'));
      expect(warning, contains('Sign in'),
          reason: 'the hint still lists what is visible');
      expect(warning, isNot(contains(_secret)));
    });

    testWidgets(
        'a password field revealed after a summary stays hidden in later summaries',
        (tester) async {
      final password = TextEditingController(text: _secret);
      addTearDown(password.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(children: [
            TextField(controller: password, obscureText: true),
          ]),
        ),
      ));
      // This summary sees the field obscured, which keeps it secret from then on.
      expect(ProbeFinder.instance.visibleSummary()['texts'],
          everyElement(isNot(contains(_secret))));

      // The show-password toggle: the same field, no longer obscured.
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(children: [
            TextField(controller: password, obscureText: false),
          ]),
        ),
      ));
      await tester.pump();

      final s = ProbeFinder.instance.visibleSummary();
      expect(s['texts'], everyElement(isNot(contains(_secret))));
      expect(s['keys'], everyElement(isNot(contains(_secret))));
    });

    testWidgets(
        'a keyed obscured field swapped for a plain field with the same key is not listed',
        (tester) async {
      final first = TextEditingController(text: _secret);
      final plain = TextEditingController(text: _secret);
      addTearDown(first.dispose);
      addTearDown(plain.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(children: [
            TextField(
              key: const ValueKey('swap_pw'),
              controller: first,
              obscureText: true,
            ),
          ]),
        ),
      ));
      // This summary sees the field obscured, which keeps its key secret.
      expect(ProbeFinder.instance.visibleSummary()['texts'],
          everyElement(isNot(contains(_secret))));

      // The app swaps in a plain field under the same key, with its own controller.
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(children: [
            Padding(
              padding: EdgeInsets.zero,
              child: TextField(
                key: const ValueKey('swap_pw'),
                controller: plain,
              ),
            ),
          ]),
        ),
      ));
      await tester.pump();

      final s = ProbeFinder.instance.visibleSummary();
      expect(s['texts'], everyElement(isNot(contains(_secret))));
      expect(s['keys'], everyElement(isNot(contains(_secret))));
    });

    testWidgets(
        'a plain unkeyed field in a keyed container beside an obscured field is still listed',
        (tester) async {
      final email = TextEditingController(text: _email);
      final password = TextEditingController(text: _secret);
      addTearDown(email.dispose);
      addTearDown(password.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(
            key: const ValueKey('login_form'),
            children: [
              TextField(controller: email),
              TextField(controller: password, obscureText: true),
            ],
          ),
        ),
      ));

      // The first summary sees the password field obscured; a later summary must
      // still list the plain field beside it.
      ProbeFinder.instance.visibleSummary();
      final s = ProbeFinder.instance.visibleSummary();
      expect(s['texts'], contains(_email),
          reason: 'a keyed container is not the field, so its key must not hide the plain field');
      expect(s['texts'], everyElement(isNot(contains(_secret))));
    });

    testWidgets(
        'documented limit: an unkeyed field rebuilt with a new controller is not covered',
        (tester) async {
      final first = TextEditingController(text: _secret);
      final rebuilt = TextEditingController(text: _secret);
      addTearDown(first.dispose);
      addTearDown(rebuilt.dispose);
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(children: [
            TextField(controller: first, obscureText: true),
          ]),
        ),
      ));
      expect(ProbeFinder.instance.visibleSummary()['texts'],
          everyElement(isNot(contains(_secret))));

      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(children: [
            Padding(
              padding: EdgeInsets.zero,
              child: TextField(controller: rebuilt),
            ),
          ]),
        ),
      ));
      await tester.pump();

      // Pins the limit documented on ProbeFinder.isSecretEditable. Closing the
      // limit makes this fail, and then the comment and this test change together.
      expect(ProbeFinder.instance.visibleSummary()['texts'], contains(_secret),
          reason: 'known limit: an unkeyed field has no key, so a new controller is not known to be secret');
    });
  });
}
