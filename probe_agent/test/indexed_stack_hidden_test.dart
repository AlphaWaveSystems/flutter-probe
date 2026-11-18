import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/device_view.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/finder.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

// An IndexedStack shows one child. The others stay mounted, laid out at their size and attached, and only some Flutter versions
// (3.44) also wrap them in a Visibility(visible: false); in others (3.50) nothing but the render object hides them (issue #900).
// The agent must treat a widget under a child that is not shown as not visible, in every version, and a DropdownButton keeps its
// items in an IndexedStack.

Future<Map<String, dynamic>> _rpc(
    WidgetTester tester, String method, Map<String, dynamic> params) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(
          ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

Map<String, dynamic> _sel(String text, {bool negated = false}) => {
      'selector': {'kind': 'text', 'text': text},
      'negated': negated,
    };

/// True when `see "text"` passes.
Future<bool> _sees(WidgetTester tester, String text) async =>
    (await _rpc(tester, ProbeMethods.see, _sel(text)))['error'] == null;

/// True when `don't see "text"` passes.
Future<bool> _doesNotSee(WidgetTester tester, String text) async =>
    (await _rpc(
        tester, ProbeMethods.see, _sel(text, negated: true)))['error'] ==
    null;

Widget _app(Widget body) => MaterialApp(home: Scaffold(body: body));

class _Counter extends StatefulWidget {
  const _Counter();
  @override
  State<_Counter> createState() => _CounterState();
}

class _CounterState extends State<_Counter> {
  int n = 0;
  @override
  Widget build(BuildContext context) =>
      TextButton(onPressed: () => setState(() => n++), child: Text('Count $n'));
}

void main() {
  testWidgets(
      'a child of an IndexedStack that is not shown is not visible; the shown one is',
      (tester) async {
    await tester.pumpWidget(_app(const SizedBox(
      height: 60,
      child: IndexedStack(
          index: 0, children: [Text('Shown page'), Text('Hidden page')]),
    )));
    await tester.pumpAndSettle();

    expect(await _sees(tester, 'Shown page'), isTrue);
    expect(await _doesNotSee(tester, 'Shown page'), isFalse);
    expect(await _sees(tester, 'Hidden page'), isFalse);
    expect(await _doesNotSee(tester, 'Hidden page'), isTrue);
    // The hidden child keeps its size and stays attached, so only the stack's index can tell: it is what this test pins.
    final hidden =
        tester.element(find.text('Hidden page', skipOffstage: false));
    expect(hidden.renderObject!.attached, isTrue);
  });

  testWidgets('tap on a child that is not shown finds nothing', (tester) async {
    var taps = 0;
    await tester.pumpWidget(_app(SizedBox(
      height: 60,
      child: IndexedStack(index: 0, children: [
        const Text('Shown page'),
        TextButton(onPressed: () => taps++, child: const Text('Hidden button')),
      ]),
    )));
    await tester.pumpAndSettle();

    final r = await _rpc(tester, ProbeMethods.tap, _sel('Hidden button'));
    expect(r['error'], isNotNull);
    expect(taps, 0);
  });

  testWidgets(
      'an index change shows the other child, and a child shown again is found again with its state',
      (tester) async {
    final index = ValueNotifier<int>(0);
    addTearDown(index.dispose);
    await tester.pumpWidget(_app(ValueListenableBuilder<int>(
      valueListenable: index,
      builder: (_, i, __) => IndexedStack(
          index: i, children: const [Text('First page'), _Counter()]),
    )));
    await tester.pumpAndSettle();
    expect(await _sees(tester, 'First page'), isTrue);
    expect(await _sees(tester, 'Count 0'), isFalse);

    index.value = 1;
    await tester.pumpAndSettle();
    expect(await _sees(tester, 'Count 0'), isTrue);
    expect(await _sees(tester, 'First page'), isFalse);
    expect(await _doesNotSee(tester, 'First page'), isTrue);

    // A tap through the agent counts on the page now shown, and the count survives the page being hidden and shown again.
    expect((await _rpc(tester, ProbeMethods.tap, _sel('Count 0')))['error'],
        isNull);
    await tester.pumpAndSettle();
    expect(await _sees(tester, 'Count 1'), isTrue);
    index.value = 0;
    await tester.pumpAndSettle();
    expect(await _sees(tester, 'Count 1'), isFalse);
    expect(await _sees(tester, 'First page'), isTrue);
    index.value = 1;
    await tester.pumpAndSettle();
    expect(await _sees(tester, 'Count 1'), isTrue,
        reason: 'the page kept its state while hidden and is found again');
  });

  testWidgets('an IndexedStack with an index of null shows nothing',
      (tester) async {
    await tester.pumpWidget(_app(const SizedBox(
      height: 60,
      child: IndexedStack(
          index: null, children: [Text('Page one'), Text('Page two')]),
    )));
    await tester.pumpAndSettle();
    expect(await _sees(tester, 'Page one'), isFalse);
    expect(await _doesNotSee(tester, 'Page two'), isTrue);
  });

  testWidgets(
      'nested IndexedStacks: only the text under the shown child of each stack is visible',
      (tester) async {
    await tester.pumpWidget(_app(const SizedBox(
      height: 100,
      child: IndexedStack(index: 0, children: [
        IndexedStack(index: 1, children: [Text('Inner A'), Text('Inner B')]),
        Text('Outer second'),
      ]),
    )));
    await tester.pumpAndSettle();
    expect(await _sees(tester, 'Inner B'), isTrue);
    expect(await _sees(tester, 'Inner A'), isFalse,
        reason: 'hidden by the inner stack');
    expect(await _sees(tester, 'Outer second'), isFalse,
        reason: 'hidden by the outer stack');
  });

  testWidgets(
      'a nested stack under a child that is not shown stays hidden, whatever its own index',
      (tester) async {
    await tester.pumpWidget(_app(const SizedBox(
      height: 100,
      child: IndexedStack(index: 1, children: [
        IndexedStack(index: 0, children: [Text('Inner shown')]),
        Text('Outer shown'),
      ]),
    )));
    await tester.pumpAndSettle();
    expect(await _sees(tester, 'Outer shown'), isTrue);
    expect(await _sees(tester, 'Inner shown'), isFalse,
        reason:
            'the inner stack shows its child, but is itself under a hidden one');
  });

  testWidgets('an ordinary Stack shows all its children and is unchanged',
      (tester) async {
    await tester.pumpWidget(_app(const SizedBox(
      height: 60,
      child: Stack(children: [
        Text('Back layer'),
        Positioned(top: 20, child: Text('Front layer'))
      ]),
    )));
    await tester.pumpAndSettle();
    expect(await _sees(tester, 'Back layer'), isTrue);
    expect(await _sees(tester, 'Front layer'), isTrue);
  });

  testWidgets(
      'a closed DropdownButton shows its selection or hint, not its other items; an open one shows its menu',
      (tester) async {
    await tester.pumpWidget(_app(Column(children: [
      DropdownButton<String>(
        key: const ValueKey('colors'),
        hint: const Text('Pick a color'),
        value: null,
        items: const [
          DropdownMenuItem(value: 'Red', child: Text('Red')),
          DropdownMenuItem(value: 'Blue', child: Text('Blue')),
        ],
        onChanged: (_) {},
      ),
      DropdownButton<String>(
        value: 'Green',
        items: const [
          DropdownMenuItem(value: 'Green', child: Text('Green')),
          DropdownMenuItem(value: 'Yellow', child: Text('Yellow')),
        ],
        onChanged: (_) {},
      ),
    ])));
    await tester.pumpAndSettle();

    expect(await _sees(tester, 'Pick a color'), isTrue);
    expect(await _sees(tester, 'Blue'), isFalse);
    expect(await _doesNotSee(tester, 'Blue'), isTrue);
    expect(await _doesNotSee(tester, 'Red'), isTrue);
    expect(await _sees(tester, 'Green'), isTrue,
        reason: 'the selected item is shown');
    expect(await _doesNotSee(tester, 'Yellow'), isTrue);

    await tester.tap(find.byKey(const ValueKey('colors')));
    await tester.pumpAndSettle();
    expect(await _sees(tester, 'Blue'), isTrue,
        reason: 'the open menu lists its items');
    expect(await _sees(tester, 'Red'), isTrue);
  });

  testWidgets(
      'the visible summary and the device view list only what the stack shows',
      (tester) async {
    await tester.pumpWidget(_app(const SizedBox(
      height: 60,
      child: IndexedStack(
          index: 1, children: [Text('Page zero'), Text('Page one')]),
    )));
    await tester.pumpAndSettle();

    final summary = await _rpc(tester, ProbeMethods.visibleSummary, {});
    final texts = ((summary['result'] as Map)['texts'] as List).cast<String>();
    expect(texts, contains('Page one'));
    expect(texts, isNot(contains('Page zero')));

    final listed = [for (final n in collectProbeNodes()) n.text];
    expect(listed, contains('Page one'));
    expect(listed, isNot(contains('Page zero')));
  });

  testWidgets(
      'a password field under a child that is not shown is not a field on screen',
      (tester) async {
    final index = ValueNotifier<int>(0);
    addTearDown(index.dispose);
    await tester.pumpWidget(_app(ValueListenableBuilder<int>(
      valueListenable: index,
      builder: (_, i, __) => IndexedStack(index: i, children: const [
        Text('Sign in'),
        TextField(obscureText: true),
      ]),
    )));
    await tester.pumpAndSettle();
    expect(ProbeFinder.instance.hasSecretField(), isFalse,
        reason: 'the field is mounted but not painted');

    index.value = 1;
    await tester.pumpAndSettle();
    expect(ProbeFinder.instance.hasSecretField(), isTrue);
  });
}
