import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

void main() {
  group('scroll drives ScrollPosition directly, not a gesture (PT-15)', () {
    // scroll used to be a thin delegate to swipe's pointer-gesture
    // simulation. A Dismissible-wrapped row installs its own
    // HorizontalDragGestureRecognizer alongside the enclosing ListView's
    // VerticalDragGestureRecognizer — reproduced against a real iOS
    // simulator, a 50-item Dismissible list never scrolled past the first
    // screen this way, while the identical verb worked fine on a plain
    // list. scroll's job is "reveal more content," unlike swipe (which
    // tests a real gesture interaction) — it doesn't need to enter the
    // gesture arena at all, so it now drives the Scrollable's own
    // ScrollPosition directly via jumpTo, which takes effect synchronously
    // (no need to await the dispatch call's own frame-settling tail here).
    testWidgets('scrolls a list of Dismissible rows via #id selector',
        (tester) async {
      final controller = ScrollController();
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: ListView.builder(
            controller: controller,
            key: const ValueKey('list'),
            itemCount: 30,
            itemBuilder: (context, i) => Dismissible(
              key: ValueKey('row_$i'),
              onDismissed: (_) {},
              child: ListTile(title: Text('Item $i')),
            ),
          ),
        ),
      ));

      expect(controller.offset, 0);

      final executor = ProbeExecutor((_) {});
      executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.scroll,
        params: {
          'direction': 'down',
          'selector': {'kind': 'id', 'text': '#list'},
        },
      ));
      await tester.pump();

      expect(controller.offset, greaterThan(0),
          reason: 'scroll down should move the list via ScrollPosition, '
              'not get intercepted by a Dismissible row\'s drag recognizer');
    });

    testWidgets('picks the largest Scrollable when no selector is given',
        (tester) async {
      // A TextField's own internal cursor-scrolling Scrollable sits
      // alongside the actual content list on many real screens (e.g. a
      // search field above a list) — a bare `scroll down` (no selector)
      // must not pick that one just because it's found first in tree
      // order.
      final listController = ScrollController();
      await tester.pumpWidget(MaterialApp(
        home: Scaffold(
          body: Column(
            children: [
              const TextField(key: ValueKey('search_field')),
              Expanded(
                child: ListView.builder(
                  controller: listController,
                  itemCount: 30,
                  itemBuilder: (context, i) => ListTile(title: Text('Item $i')),
                ),
              ),
            ],
          ),
        ),
      ));

      expect(listController.offset, 0);

      final executor = ProbeExecutor((_) {});
      executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.scroll,
        params: {'direction': 'down'},
      ));
      await tester.pump();

      expect(listController.offset, greaterThan(0),
          reason: 'a bare scroll down should move the main content list, '
              'not a TextField\'s own internal Scrollable');
    });
  });
}
