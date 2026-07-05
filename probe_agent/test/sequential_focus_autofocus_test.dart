import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

class _AutoFocusFirstField extends StatefulWidget {
  const _AutoFocusFirstField();

  @override
  State<_AutoFocusFirstField> createState() => _AutoFocusFirstFieldState();
}

class _AutoFocusFirstFieldState extends State<_AutoFocusFirstField> {
  final controllerA = TextEditingController();
  final controllerB = TextEditingController();
  final focusA = FocusNode();
  final focusB = FocusNode();

  @override
  void initState() {
    super.initState();
    // Mirrors the reported repro: one field requests focus during
    // initState, before the user (or probe) ever taps anything.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      focusA.requestFocus();
    });
  }

  @override
  void dispose() {
    controllerA.dispose();
    controllerB.dispose();
    focusA.dispose();
    focusB.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      home: Scaffold(
        body: Column(
          children: [
            TextField(
              key: const ValueKey('field_a'),
              controller: controllerA,
              focusNode: focusA,
            ),
            TextField(
              key: const ValueKey('field_b'),
              controller: controllerB,
              focusNode: focusB,
            ),
          ],
        ),
      ),
    );
  }
}

void main() {
  testWidgets(
      'PT-21 reopened: tapping field B when field A auto-focused on load',
      (tester) async {
    final widget = const _AutoFocusFirstField();
    await tester.pumpWidget(widget);
    // Let the postFrameCallback's requestFocus() actually land.
    await tester.pump();
    await tester.pump();

    final state =
        tester.state<_AutoFocusFirstFieldState>(find.byWidget(widget));

    expect(state.focusA.hasFocus, isTrue,
        reason: 'sanity check: field A should have auto-focused on load');

    final executor = ProbeExecutor((_) {});

    await executor.dispatch(ProbeRequest(
      jsonrpc: '2.0',
      id: 1,
      method: ProbeMethods.tap,
      params: {
        'selector': {'kind': 'id', 'text': '#field_b'},
      },
    ));
    await executor.dispatch(ProbeRequest(
      jsonrpc: '2.0',
      id: 2,
      method: ProbeMethods.type_,
      params: {
        'selector': {'kind': 'id', 'text': '#field_b'},
        'text': 'typed_into_b',
      },
    ));

    print('focusA.hasFocus = ${state.focusA.hasFocus}');
    print('focusB.hasFocus = ${state.focusB.hasFocus}');
    print('controllerA.text = "${state.controllerA.text}"');
    print('controllerB.text = "${state.controllerB.text}"');

    expect(state.focusB.hasFocus, isTrue,
        reason: 'tapping field B should shift focus to B, even though A '
            'auto-focused on load');
    expect(state.controllerB.text, 'typed_into_b',
        reason: 'typed text should land in B, not the auto-focused A');
    expect(state.controllerA.text, isEmpty,
        reason: 'A should be untouched by typing meant for B');
  });
}
