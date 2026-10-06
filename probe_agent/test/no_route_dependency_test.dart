import 'dart:async' show unawaited;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/finder.dart';

/// FP-13, from Water Sip: since agent 0.10.0 the finder resolved the route with
/// `ModalRoute.of(element)`, which subscribes the element to the route's
/// inherited scope from outside a build. Water Sip's bisect (0.9.9 passes,
/// 0.10.0+ fails) showed a red screen (`InheritedElement.notifyClients`
/// assertion) after closing sheets/dialogs. The finder now reads the route
/// without subscribing (`probeRouteOf`).
///
/// NOTE: the framework assertion itself could not be reproduced in a widget
/// test here — these tests pin the behavior of the replacement (correct route
/// and isCurrent, no exceptions through open/query/close/reparent), not the
/// original crash. The crash fix is verified against the Water Sip app.
void main() {
  testWidgets('querying a dialog and then closing it raises no framework error',
      (tester) async {
    late BuildContext pageContext;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Builder(builder: (context) {
          pageContext = context;
          return const Text('Page');
        }),
      ),
    ));

    unawaited(showDialog<void>(
      context: pageContext,
      builder: (_) => const AlertDialog(content: Text('Hello dialog')),
    ));
    await tester.pumpAndSettle();

    // The agent's kind of query: from outside any build, on dialog content.
    final found = ProbeFinder.instance.findElements({'kind': 'text', 'text': 'Hello dialog'});
    expect(found, isNotEmpty);
    ProbeFinder.instance.visibleSummary();

    Navigator.of(pageContext).pop();
    await tester.pumpAndSettle();

    expect(tester.takeException(), isNull,
        reason: 'closing a route the agent queried must not trip the framework');
    expect(find.text('Hello dialog'), findsNothing);
  });

  // A suspected failing shape (not confirmed to reproduce): an element the agent
  // queried is later REPARENTED out of the route scope (GlobalKey move) before
  // that scope notifies its dependents (a dialog opening flips isCurrent).
  testWidgets('an element reparented after being queried does not trip the framework',
      (tester) async {
    final key = GlobalKey();
    final moved = ValueNotifier<bool>(false);
    addTearDown(moved.dispose);
    late BuildContext pageContext;

    await tester.pumpWidget(MaterialApp(
      builder: (context, child) => ValueListenableBuilder<bool>(
        valueListenable: moved,
        builder: (_, isMoved, __) => Column(
          textDirection: TextDirection.ltr,
          children: [
            if (isMoved) Text('Keyed', key: key, textDirection: TextDirection.ltr),
            Expanded(child: child!),
          ],
        ),
      ),
      home: Scaffold(
        body: ValueListenableBuilder<bool>(
          valueListenable: moved,
          builder: (context, isMoved, _) {
            pageContext = context;
            return Center(
              child: isMoved
                  ? const SizedBox()
                  : Text('Keyed', key: key, textDirection: TextDirection.ltr),
            );
          },
        ),
      ),
    ));

    // The agent queries the element while it is inside the page's route scope.
    expect(ProbeFinder.instance.findElements({'kind': 'text', 'text': 'Keyed'}), isNotEmpty);

    // It moves out of that scope (same frame, GlobalKey reparent).
    moved.value = true;
    await tester.pump();

    // The page route's scope now notifies its dependents.
    unawaited(showDialog<void>(
      context: pageContext,
      builder: (_) => const AlertDialog(content: Text('Hello dialog')),
    ));
    await tester.pumpAndSettle();

    expect(tester.takeException(), isNull,
        reason: 'the finder must not leave dependencies on route scopes');
  });

  testWidgets('probeRouteOf reads the route and its isCurrent flag', (tester) async {
    late BuildContext pageContext;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Builder(builder: (context) {
          pageContext = context;
          return const Text('Page');
        }),
      ),
    ));
    final pageRoute = probeRouteOf(tester.element(find.text('Page')));
    expect(pageRoute, isNotNull);
    expect(pageRoute!.isCurrent, isTrue);

    unawaited(showDialog<void>(
      context: pageContext,
      builder: (_) => const AlertDialog(content: Text('Hello dialog')),
    ));
    await tester.pumpAndSettle();

    expect(probeRouteOf(tester.element(find.text('Page')))!.isCurrent, isFalse,
        reason: 'the page is covered by the dialog');
    expect(probeRouteOf(tester.element(find.text('Hello dialog')))!.isCurrent, isTrue);
  });
}
