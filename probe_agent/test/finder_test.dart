import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/finder.dart';

void main() {
  group('ProbeFinder route-awareness (PT-03)', () {
    // Flutter's Navigator keeps previous routes mounted underneath the
    // current one by default — this reproduces that shape (two live routes,
    // each with its own distinguishing content) and confirms the finder
    // resolves only to the current (topmost) route's content, not the one
    // still mounted underneath it.
    testWidgets('excludes text belonging to a route mounted underneath the current one',
        (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Builder(
          builder: (context) => Scaffold(
            body: Column(
              children: [
                const Text('Only On Page One'),
                ElevatedButton(
                  onPressed: () => Navigator.of(context).push(
                    MaterialPageRoute(
                      builder: (_) => const Scaffold(
                        body: Text('Only On Page Two'),
                      ),
                    ),
                  ),
                  child: const Text('Go to page two'),
                ),
              ],
            ),
          ),
        ),
      ));

      final finder = ProbeFinder.instance;

      // Sanity check: page one's text is visible before navigating.
      expect(
        finder.findElements({'kind': 'text', 'text': 'Only On Page One'}),
        isNotEmpty,
      );

      await tester.tap(find.text('Go to page two'));
      await tester.pumpAndSettle();

      // Page one is still mounted underneath (Navigator default behavior),
      // but must no longer resolve as visible.
      expect(
        finder.findElements({'kind': 'text', 'text': 'Only On Page One'}),
        isEmpty,
        reason: 'text belonging to the route mounted underneath the current '
            'one must not resolve as visible',
      );

      // The current route's own content must still resolve normally.
      expect(
        finder.findElements({'kind': 'text', 'text': 'Only On Page Two'}),
        isNotEmpty,
      );
    });

    testWidgets('content with no Navigator ancestor is still treated as visible',
        (tester) async {
      await tester.pumpWidget(
        const Directionality(
          textDirection: TextDirection.ltr,
          child: Text('No navigator here'),
        ),
      );

      expect(
        ProbeFinder.instance
            .findElements({'kind': 'text', 'text': 'No navigator here'}),
        isNotEmpty,
      );
    });
  });

  group('text selectors on ListTile title/subtitle (PT-11)', () {
    // The reported symptom (a title text selector failing on iOS 26.3+
    // because "the OS accessibility layer merges the title and subtitle
    // into one combined node") describes real XCUITest/VoiceOver behavior,
    // but doesn't reproduce here: _findByText walks the live Flutter
    // element tree directly and never touches the platform accessibility
    // tree at all. ListTile builds title/subtitle as fully independent Text
    // widgets (no MergeSemantics, no string concatenation anywhere in the
    // widget tree), so each resolves on its own regardless of platform.
    testWidgets('title and subtitle resolve independently, not merged',
        (tester) async {
      await tester.pumpWidget(const MaterialApp(
        home: Scaffold(
          body: ListTile(
            title: Text('Home'),
            subtitle: Text('Recently viewed'),
          ),
        ),
      ));

      final finder = ProbeFinder.instance;
      expect(finder.findElements({'kind': 'text', 'text': 'Home'}), isNotEmpty,
          reason: 'title text must resolve on its own');
      expect(
        finder.findElements({'kind': 'text', 'text': 'Recently viewed'}),
        isNotEmpty,
        reason: 'subtitle text must resolve on its own',
      );
      // Neither should spuriously match the other's exact text.
      expect(
        finder
            .findElements({'kind': 'text', 'text': 'Home, Recently viewed'}),
        isEmpty,
        reason: 'title and subtitle must not appear merged into one string '
            'anywhere in the element tree',
      );
    });
  });
}
