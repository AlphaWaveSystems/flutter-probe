import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/finder.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

// The visible_summary RPC answer's additive keys (docs/proposals/studio-replace-line-step.md, the agent change): editable_texts recorded
// independently of the de-duplication of texts, the texts of raw RichTexts, and max, counts and truncated.
Future<Map<String, dynamic>> _rpc() async {
  String? sent;
  final executor = ProbeExecutor((msg) => sent = msg);
  await executor.dispatch(ProbeRequest(
      jsonrpc: '2.0',
      id: 1,
      method: ProbeMethods.visibleSummary,
      params: const {}));
  return (jsonDecode(sent!) as Map<String, dynamic>)['result']
      as Map<String, dynamic>;
}

Future<void> _pump(WidgetTester tester, List<Widget> children) =>
    tester.pumpWidget(MaterialApp(
        home: Scaffold(
            body: SingleChildScrollView(child: Column(children: children)))));

void main() {
  testWidgets(
      'a field holding the same text as a Text is still listed in editable_texts',
      (tester) async {
    await _pump(tester, [
      const Text('Sign In'),
      SizedBox(
          width: 300,
          child: TextField(controller: TextEditingController(text: 'Sign In'))),
    ]);
    final r = await _rpc();
    expect(r['texts'], ['Sign In'],
        reason: 'texts stays de-duplicated, as before');
    expect(r['editable_texts'], ['Sign In'],
        reason: 'the field is recorded although its text is already in texts');
    expect(r['counts'],
        {'texts': 1, 'keys': 0, 'editable_texts': 1, 'rich_texts': 0});
    expect(r['truncated'], isFalse);
    expect(r['max'], 25);
  });

  testWidgets('an obscured field is in no list', (tester) async {
    await _pump(tester, [
      SizedBox(
          width: 300,
          child: TextField(
              obscureText: true,
              controller: TextEditingController(text: 'zq-secret-9'))),
      SizedBox(
          width: 300,
          child: TextField(
              controller: TextEditingController(text: 'visible value'))),
    ]);
    final r = await _rpc();
    expect(jsonEncode(r), isNot(contains('zq-secret-9')));
    expect(r['editable_texts'], ['visible value']);
    expect(r['counts'],
        {'texts': 1, 'keys': 0, 'editable_texts': 1, 'rich_texts': 0},
        reason:
            'an obscured field is not counted either: a count must not reveal that it holds a value');
  });

  testWidgets(
      'the text of a raw RichText is listed in rich_texts; a Text\'s own paragraph is not',
      (tester) async {
    await _pump(tester, [
      const Text('Plain'),
      Text.rich(
          const TextSpan(text: 'Rich ', children: [TextSpan(text: 'by Text')])),
      RichText(
        text: const TextSpan(
            text: 'Painted ',
            style: TextStyle(color: Colors.black),
            children: [TextSpan(text: 'raw')]),
      ),
    ]);
    final r = await _rpc();
    expect(r['rich_texts'], ['Painted raw']);
    expect(r['texts'], ['Plain', 'Rich by Text'],
        reason:
            'texts is unchanged: a raw RichText is reported in its own key');
  });

  for (final n in [24, 25, 26]) {
    testWidgets(
        '$n distinct texts: the list is cut at 25 and truncated says so',
        (tester) async {
      await _pump(tester, [
        for (var i = 0; i < n; i++) SizedBox(height: 10, child: Text('T$i'))
      ]);
      final r = await _rpc();
      expect((r['texts'] as List).length, n > 25 ? 25 : n);
      expect((r['counts'] as Map)['texts'], n,
          reason: 'counts are the distinct items seen, cut or not');
      expect(r['truncated'], n > 25);
    });
  }

  testWidgets(
      'a text over 40 characters is listed whole (the agent cuts nothing; a reader decides)',
      (tester) async {
    const long = 'Sign In with your email address to continue';
    expect(long.length, greaterThan(40));
    await _pump(tester, [const Text(long)]);
    final r = await _rpc();
    expect(r['texts'], [long]);
    expect(r['truncated'], isFalse);
  });

  testWidgets('too many fields set truncated even when the other lists fit',
      (tester) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await _pump(tester, [
      for (var i = 0; i < 26; i++)
        SizedBox(
            width: 300,
            height: 30,
            child: TextField(controller: TextEditingController(text: 'v$i'))),
    ]);
    final r = ProbeFinder.instance.visibleSummaryFull(max: 26);
    expect(r['truncated'], isFalse);
    final cut = ProbeFinder.instance.visibleSummaryFull(max: 25);
    expect((cut['editable_texts'] as List).length, 25);
    expect(cut['truncated'], isTrue);
  });

  testWidgets(
      'visibleSummary (the failure hints) keeps its two lists unchanged',
      (tester) async {
    await _pump(tester, [
      const Text('Sign In', key: ValueKey('title')),
      SizedBox(
          width: 300,
          child: TextField(controller: TextEditingController(text: 'typed'))),
      RichText(
          text: const TextSpan(
              text: 'raw', style: TextStyle(color: Colors.black))),
    ]);
    final s = ProbeFinder.instance.visibleSummary();
    expect(s.keys.toSet(), {'texts', 'keys'});
    expect(s['texts'], ['Sign In', 'typed']);
    expect(s['keys'], contains('title'));
  });

  testWidgets(
      'a field and a RichText that are not visible (hidden, or laid out off the screen) are not listed',
      (tester) async {
    await _pump(tester, [
      Offstage(
          child: SizedBox(
              width: 300,
              child: TextField(
                  controller: TextEditingController(text: 'offstage value')))),
      Offstage(
          child: RichText(
              text: const TextSpan(
                  text: 'offstage rich',
                  style: TextStyle(color: Colors.black)))),
      Visibility(
          visible: false,
          maintainState: true,
          child: RichText(
              text: const TextSpan(
                  text: 'hidden rich', style: TextStyle(color: Colors.black)))),
      // Past the walker (which skips Offstage and Visibility): laid out off the screen, and zero-size. Only _isVisible leaves these out.
      Transform.translate(
          offset: const Offset(-5000, 0),
          child: SizedBox(
              width: 300,
              child: TextField(
                  controller:
                      TextEditingController(text: 'off-screen value')))),
      Transform.translate(
          offset: const Offset(-5000, 0),
          child: RichText(
              text: const TextSpan(
                  text: 'off-screen rich',
                  style: TextStyle(color: Colors.black)))),
      SizedBox(
          width: 300,
          child: TextField(
              controller: TextEditingController(text: 'shown value'))),
    ]);
    final r = await _rpc();
    expect(r['editable_texts'], ['shown value']);
    expect(r['rich_texts'], isEmpty);
    expect(r['counts'],
        {'texts': 1, 'keys': 0, 'editable_texts': 1, 'rich_texts': 0});
  });

  testWidgets(
      'an Icon\'s glyph is not a rich text: 26 icons do not truncate the summary',
      (tester) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await _pump(tester, [for (var i = 0; i < 26; i++) const Icon(Icons.star)]);
    final r = await _rpc();
    expect(r['rich_texts'], isEmpty);
    expect(r['truncated'], isFalse);
  });
}
