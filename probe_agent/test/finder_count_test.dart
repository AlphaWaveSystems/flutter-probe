import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/finder.dart';

// Issue #800: one visible text is one match. A Text builds a RichText inside it, and both used to match, so `see exactly 1` found 2.
Future<List<Element>> _find(
  WidgetTester tester,
  Widget body,
  String text,
) async {
  await tester.pumpWidget(MaterialApp(home: Scaffold(body: body)));
  return ProbeFinder.instance.findElements({'kind': 'text', 'text': text});
}

void main() {
  testWidgets('a plain Text is one match', (tester) async {
    expect(
      await _find(tester, const Text('Order total'), 'Order total'),
      hasLength(1),
    );
  });

  testWidgets('a part of a plain Text is one match', (tester) async {
    expect(
      await _find(tester, const Text('Order total'), 'total'),
      hasLength(1),
    );
  });

  testWidgets('a Text.rich is one match', (tester) async {
    final body = Text.rich(
      const TextSpan(
        text: 'Hello ',
        children: [TextSpan(text: 'world')],
      ),
    );
    expect(await _find(tester, body, 'Hello world'), hasLength(1));
  });

  testWidgets('a RichText is one match', (tester) async {
    final body = RichText(
      text: const TextSpan(
        text: 'Painted ',
        style: TextStyle(color: Colors.black),
        children: [TextSpan(text: 'rich text')],
      ),
    );
    expect(await _find(tester, body, 'Painted rich text'), hasLength(1));
  });

  testWidgets('the label of a button is one match', (tester) async {
    final body = ElevatedButton(
      onPressed: () {},
      child: const Text('Press me'),
    );
    expect(await _find(tester, body, 'Press me'), hasLength(1));
  });

  testWidgets('two Texts with the same text are two matches', (tester) async {
    const body = Column(children: [Text('Row'), Text('Row')]);
    expect(await _find(tester, body, 'Row'), hasLength(2));
  });

  testWidgets(
    'the 2nd of two equal Texts is the second Text, not the first one\'s inner RichText',
    (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(
            body: Column(
              children: [
                Text('Row', key: ValueKey('a')),
                Text('Row', key: ValueKey('b')),
              ],
            ),
          ),
        ),
      );
      final second = ProbeFinder.instance.findElements({
        'kind': 'ordinal',
        'text': 'Row',
        'ordinal': 2,
      });
      expect(second, hasLength(1));
      expect(second.single.widget.key, const ValueKey('b'));
    },
  );

  testWidgets(
      'inside a container, the Text and its inner RichText are one match',
      (tester) async {
    await tester.pumpWidget(
        const MaterialApp(home: Scaffold(body: Text('Hello world'))));
    final found = ProbeFinder.instance.findElements(
        {'kind': 'positional', 'text': 'Hello', 'container': 'Hello world'});
    expect(found, hasLength(1));
  });

  testWidgets(
      'a plain Text and a Text.rich with the same words are two matches',
      (tester) async {
    final body = Column(children: [
      const Text('Hello world'),
      Text.rich(
          const TextSpan(text: 'Hello ', children: [TextSpan(text: 'world')]))
    ]);
    expect(await _find(tester, body, 'Hello world'), hasLength(2));
  });
}
