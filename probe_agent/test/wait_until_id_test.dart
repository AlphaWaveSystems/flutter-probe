import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/finder.dart';

void main() {
  group('id selectors resolve through Material/Tooltip nesting (PT-06)', () {
    // wait until #id appears previously always built a *text* selector
    // regardless of the '#' prefix, so it searched for a widget whose
    // visible text literally read "#my_button" — which never matches a
    // non-text widget like an icon button, timing out even though the
    // target is indisputably mounted and visible. The fix detects the '#'
    // prefix and dispatches an id selector instead. This test isolates the
    // part of that fix that actually matters here — id-selector resolution
    // through Material/Tooltip wrapping (exactly the shape the doc
    // theorized was broken) — via ProbeFinder directly, rather than the
    // full wait-polling loop (which involves real Duration-based timers
    // this test doesn't need to exercise).
    testWidgets(
        'finds an IconButton wrapped in Material/Tooltip by its key, as real '
        'icon buttons commonly are', (tester) async {
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Material(
            child: Tooltip(
              message: 'Refresh',
              child: IconButton(
                key: const ValueKey('refresh_button'),
                icon: const Icon(Icons.refresh),
                onPressed: () {},
              ),
            ),
          ),
        ),
      ));

      final matches = ProbeFinder.instance
          .findElements({'kind': 'id', 'text': '#refresh_button'});
      expect(matches, isNotEmpty,
          reason: 'id selector should find the IconButton through its '
              'Material/Tooltip ancestors');
    });

    testWidgets('a genuinely absent id finds nothing', (tester) async {
      await tester.pumpWidget(const MaterialApp(
        home: Scaffold(body: Text('nothing relevant here')),
      ));

      final matches = ProbeFinder.instance
          .findElements({'kind': 'id', 'text': '#does_not_exist'});
      expect(matches, isEmpty);
    });
  });
}
