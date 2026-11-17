import 'dart:convert';

import 'package:flutter/material.dart' as m;
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

// Issue #852: a control of another library with the framework's name (package:material_ui's ElevatedButton, cupertino_ui's
// CupertinoSwitch) is not the class the agent reads, and `is enabled` passed on a disabled one. The agent now refuses the state check
// rather than guess. These stand-ins are such classes: the framework's names, not its types.
class ElevatedButton extends StatelessWidget {
  const ElevatedButton(
      {super.key, required this.onPressed, required this.child});
  final VoidCallback? onPressed;
  final Widget child;
  @override
  Widget build(BuildContext context) =>
      GestureDetector(onTap: onPressed, child: child);
}

class Checkbox extends StatelessWidget {
  const Checkbox({super.key, required this.value, required this.onChanged});
  final bool value;
  final ValueChanged<bool?>? onChanged;
  @override
  Widget build(BuildContext context) => GestureDetector(
      onTap: () => onChanged?.call(!value), child: Text(value ? '[x]' : '[ ]'));
}

class CupertinoSwitch extends StatelessWidget {
  const CupertinoSwitch(
      {super.key, required this.value, required this.onChanged});
  final bool value;
  final ValueChanged<bool>? onChanged;
  @override
  Widget build(BuildContext context) => GestureDetector(
      onTap: () => onChanged?.call(!value), child: Text(value ? 'on' : 'off'));
}

// A generic one: its runtime type is `Radio<int>`, still the name of a control the agent reads.
class Radio<T> extends StatelessWidget {
  const Radio({super.key, required this.value});
  final T value;
  @override
  Widget build(BuildContext context) => Text('$value');
}

// An app's own control with its own name: not the framework's name, so the agent's old rule applies.
class LoginButton extends StatelessWidget {
  const LoginButton({super.key});
  @override
  Widget build(BuildContext context) =>
      GestureDetector(onTap: () {}, child: const Text('Log in'));
}

Future<String?> _see(String id, String check, {bool negated = false}) async {
  String? sent;
  final executor = ProbeExecutor((msg) => sent = msg);
  await executor.dispatch(
      ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
    'selector': {'kind': 'id', 'text': '#$id'},
    'check': check,
    if (negated) 'negated': true,
  }));
  final error = (jsonDecode(sent!) as Map<String, dynamic>)['error'];
  return error == null ? null : '$error';
}

Future<void> _pump(WidgetTester tester, List<Widget> children) =>
    tester.pumpWidget(
        m.MaterialApp(home: m.Scaffold(body: Column(children: children))));

void main() {
  testWidgets(
      'a disabled ElevatedButton of another library: is enabled and is disabled refuse instead of guessing',
      (tester) async {
    await _pump(tester, [
      const ElevatedButton(
          key: ValueKey('off'), onPressed: null, child: Text('Off'))
    ]);
    for (final check in ['enabled', 'disabled']) {
      expect(await _see('off', check), contains('cannot be evaluated'),
          reason: 'is $check');
      expect(await _see('off', check), contains('issue #852'));
      expect(await _see('off', check, negated: true),
          contains('cannot be evaluated'),
          reason: "don't see ... is $check must not pass");
    }
  });

  testWidgets(
      'a text inside such a button refuses too (the nearest control decides)',
      (tester) async {
    await _pump(tester, [
      const ElevatedButton(
          onPressed: null, child: Text('Submit', key: ValueKey('label')))
    ]);
    expect(await _see('label', 'enabled'),
        contains('the target is a ElevatedButton'));
  });

  testWidgets(
      'a Checkbox and a CupertinoSwitch of another library: is checked refuses, positive and negated',
      (tester) async {
    await _pump(tester, [
      Checkbox(key: const ValueKey('cb'), value: true, onChanged: (_) {}),
      CupertinoSwitch(
          key: const ValueKey('cs'), value: false, onChanged: (_) {}),
      const Radio<int>(key: ValueKey('rd'), value: 1),
    ]);
    for (final id in ['cb', 'cs', 'rd']) {
      expect(await _see(id, 'checked'), contains('cannot be evaluated'),
          reason: id);
      expect(await _see(id, 'checked', negated: true),
          contains('cannot be evaluated'),
          reason: "$id: don't see ... is checked must not pass");
    }
  });

  testWidgets('the framework controls are read as before', (tester) async {
    await _pump(tester, [
      const m.ElevatedButton(
          key: ValueKey('fm_off'), onPressed: null, child: Text('Off')),
      m.ElevatedButton(
          key: const ValueKey('fm_on'),
          onPressed: () {},
          child: const Text('On')),
      m.Checkbox(key: const ValueKey('fm_cb'), value: true, onChanged: (_) {}),
      SizedBox(
          width: 300, child: m.TextFormField(key: const ValueKey('fm_tff'))),
    ]);
    expect(await _see('fm_off', 'disabled'), isNull);
    expect(await _see('fm_off', 'enabled'), contains('is disabled'));
    expect(await _see('fm_on', 'enabled'), isNull);
    expect(await _see('fm_cb', 'checked'), isNull);
    expect(await _see('fm_tff', 'enabled'), isNull,
        reason:
            'a framework control the agent does not read keeps the old rule');
  });

  testWidgets("an app's own control with its own name keeps the old rule",
      (tester) async {
    await _pump(tester, [const LoginButton(key: ValueKey('login'))]);
    expect(await _see('login', 'enabled'), isNull);
  });
}
