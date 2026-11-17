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

// A lookalike that builds its child directly, so the number of elements between it and the target is exact (the 40-ancestor bound).
class FilledButton extends StatelessWidget {
  const FilledButton({super.key, required this.onPressed, required this.child});
  final VoidCallback? onPressed;
  final Widget child;
  @override
  Widget build(BuildContext context) => child;
}

// A pass-through lookalike for the checked bound.
class CheckboxListTile extends StatelessWidget {
  const CheckboxListTile({super.key, required this.child});
  final Widget child;
  @override
  Widget build(BuildContext context) => child;
}

Widget _nest(int n, Widget child) => n == 0
    ? child
    : Padding(padding: EdgeInsets.zero, child: _nest(n - 1, child));

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
        contains('the target is inside an ElevatedButton'));
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

  testWidgets(
      'a keyed wrapper around a lookalike refuses too, positive and negated (the readers look below the target)',
      (tester) async {
    await _pump(tester, [
      SizedBox(
          key: const ValueKey('w_cb'),
          child: Checkbox(value: true, onChanged: (_) {})),
      const SizedBox(
          key: ValueKey('w_btn'),
          child: ElevatedButton(onPressed: null, child: Text('Off'))),
    ]);
    expect(
        await _see('w_cb', 'checked'), contains('the target holds a Checkbox'));
    expect(await _see('w_cb', 'checked', negated: true),
        contains('cannot be evaluated'),
        reason: "don't see #w is checked must not pass");
    for (final check in ['enabled', 'disabled']) {
      expect(await _see('w_btn', check),
          contains('the target holds an ElevatedButton'),
          reason: check);
      expect(await _see('w_btn', check, negated: true),
          contains('cannot be evaluated'),
          reason: 'negated $check');
    }
  });

  testWidgets('the message says what to do instead', (tester) async {
    await _pump(tester, [
      const ElevatedButton(
          key: ValueKey('off'), onPressed: null, child: Text('Off'))
    ]);
    final message = await _see('off', 'enabled');
    expect(message, contains('the target is an ElevatedButton'));
    expect(message, contains('Assert on its text or content instead'));
    expect(message, contains('https://flutterprobe.dev/tools/agent/'));
  });

  testWidgets(
      'the ancestor bound: a lookalike 40 ancestors up refuses, 41 up does not (the old rule)',
      (tester) async {
    // The target Text is the element; its ancestors are the Paddings, then the lookalike: 39 Paddings put it at ancestor 40.
    await _pump(tester, [
      FilledButton(
          onPressed: null,
          child: _nest(39, const Text('Near', key: ValueKey('near'))))
    ]);
    expect(await _see('near', 'enabled'),
        contains('the target is inside a FilledButton'));
    await _pump(tester, [
      FilledButton(
          onPressed: null,
          child: _nest(40, const Text('Far', key: ValueKey('far'))))
    ]);
    expect(await _see('far', 'enabled'), isNull,
        reason:
            'past the bound nothing is found and the old rule (enabled) applies');
  });

  testWidgets(
      'the checked bound: a lookalike 40 ancestors up refuses, 41 up gives the old answer',
      (tester) async {
    await _pump(tester, [
      CheckboxListTile(
          child: _nest(39, const Text('Near', key: ValueKey('cnear'))))
    ]);
    expect(await _see('cnear', 'checked'),
        contains('the target is inside a CheckboxListTile'));
    await _pump(tester, [
      CheckboxListTile(
          child: _nest(40, const Text('Far', key: ValueKey('cfar'))))
    ]);
    expect(
        await _see('cfar', 'checked'), contains('is not a checkable control'));
  });

  testWidgets(
      'a wrapper around a framework button keeps the old rule, whatever the button holds',
      (tester) async {
    await _pump(tester, [
      SizedBox(
        key: const ValueKey('w_fm'),
        child: m.ElevatedButton(
            onPressed: () {}, child: Checkbox(value: false, onChanged: (_) {})),
      ),
    ]);
    expect(await _see('w_fm', 'enabled'), isNull,
        reason:
            'the framework button is the control below; a lookalike inside it is not');
  });
}
