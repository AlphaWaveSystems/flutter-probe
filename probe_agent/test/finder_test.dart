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
}
