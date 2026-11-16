import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_test/flutter_test.dart';

// An app's own widget that happens to be called RadioGroup and has no `groupValue`: the agent finds it by its class name, fails to read a group value from it,
// and says "not a checkable control". It never turns that failure into a yes or a no. (One that does have a `groupValue` is read as a group, which
// test/checked_state_radio_test.dart shows with its stand-in; that is a stated limit.) This file has its own class because two classes of one name cannot share a file.
class RadioGroup<T> extends StatelessWidget {
  const RadioGroup({super.key, required this.child});
  final Widget child;
  @override
  Widget build(BuildContext context) => child;
}

void main() {
  testWidgets('a user widget called RadioGroup with no groupValue makes the radio unreadable, not "not checked"', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: RadioGroup<String>(
          child: Radio<String>(key: const ValueKey('u'), value: 'a', groupValue: null, onChanged: (_) {}),
        ),
      ),
    ));
    String? raw;
    final ex = ProbeExecutor((s) => raw = s);
    var done = false;
    final call = ex
        .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
          'selector': {'kind': 'id', 'text': '#u'},
          'check': 'checked',
        }))
        .whenComplete(() => done = true);
    for (var i = 0; i < 100 && !done; i++) {
      await tester.pump(const Duration(milliseconds: 50));
    }
    await call;
    final error = (jsonDecode(raw!) as Map<String, dynamic>)['error'];
    expect(jsonEncode(error), contains('not a checkable control'));
  });
}
