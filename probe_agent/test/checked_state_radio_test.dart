import 'dart:convert';

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_test/flutter_test.dart';

// `see ... is checked` on a radio, for the two places a radio's group value can live: on the radio itself (the form before Flutter's RadioGroup
// API) and on the nearest RadioGroup above it (the form since). RadioGroup does not exist before Flutter 3.32 and this file has to compile on
// every Flutter the package supports, so the group is a stand-in here with the shape the agent looks for (a class named RadioGroup with a
// groupValue); the real class is tested in test_radiogroup/ on a Flutter that has it.

class RadioGroup<T> extends StatelessWidget {
  const RadioGroup({super.key, required T? groupValue, required this.child, this.broken = false}) : _groupValue = groupValue;
  final T? _groupValue;
  final Widget child;

  /// A group whose value cannot be read, as a member that a later Flutter removed would be.
  final bool broken;

  T? get groupValue {
    if (broken) throw StateError('gone in a later Flutter');
    return _groupValue;
  }

  @override
  Widget build(BuildContext context) => child;
}

Future<Map<String, dynamic>> _see(WidgetTester tester, String selector, {bool negated = false}) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final isId = selector.startsWith('#');
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: ProbeMethods.see, params: {
        'selector': {'kind': isId ? 'id' : 'text', 'text': selector},
        'check': 'checked',
        'negated': negated,
      }))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

Future<bool> _checked(WidgetTester tester, String selector) async => (await _see(tester, selector))['error'] == null;

void main() {
  testWidgets('the radio keeps its group value itself: the selected one is checked, the others are not', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          // ignore: deprecated_member_use
          Radio<String>(key: const ValueKey('r_a'), value: 'a', groupValue: 'b', onChanged: (_) {}),
          // ignore: deprecated_member_use
          Radio<String>(key: const ValueKey('r_b'), value: 'b', groupValue: 'b', onChanged: (_) {}),
          // ignore: deprecated_member_use
          RadioListTile<String>(key: const ValueKey('t_small'), title: const Text('Small'), value: 'small', groupValue: 'large', onChanged: (_) {}),
          // ignore: deprecated_member_use
          RadioListTile<String>(key: const ValueKey('t_large'), title: const Text('Large'), value: 'large', groupValue: 'large', onChanged: (_) {}),
        ]),
      ),
    ));
    expect(await _checked(tester, '#r_b'), isTrue);
    expect(await _checked(tester, '#r_a'), isFalse);
    expect(await _checked(tester, 'Large'), isTrue);
    expect(await _checked(tester, 'Small'), isFalse);
    expect((await _see(tester, '#r_a', negated: true))['error'], isNull);
    expect((await _see(tester, '#r_b', negated: true))['error'], isNotNull);
  });

  testWidgets('a Cupertino radio that keeps its group value itself is read the same way', (tester) async {
    await tester.pumpWidget(CupertinoApp(
      home: CupertinoPageScaffold(
        child: Column(children: [
          // ignore: deprecated_member_use
          CupertinoRadio<String>(key: const ValueKey('c_a'), value: 'a', groupValue: 'a', onChanged: (_) {}),
          // ignore: deprecated_member_use
          CupertinoRadio<String>(key: const ValueKey('c_b'), value: 'b', groupValue: 'a', onChanged: (_) {}),
        ]),
      ),
    ));
    expect(await _checked(tester, '#c_a'), isTrue);
    expect(await _checked(tester, '#c_b'), isFalse);
  });

  testWidgets('a radio whose group value lives on the RadioGroup above it is read from the group', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: RadioGroup<String>(
          groupValue: 'medium',
          child: Column(children: [
            // the radios carry no group value of their own, as under the RadioGroup API
            Radio<String>(key: const ValueKey('g_small'), value: 'small', groupValue: null, onChanged: (_) {}),
            Radio<String>(key: const ValueKey('g_medium'), value: 'medium', groupValue: null, onChanged: (_) {}),
            RadioListTile<String>(key: const ValueKey('g_tile_medium'), title: const Text('Medium'), value: 'medium', groupValue: null, onChanged: (_) {}),
            RadioListTile<String>(key: const ValueKey('g_tile_large'), title: const Text('Large'), value: 'large', groupValue: null, onChanged: (_) {}),
          ]),
        ),
      ),
    ));
    expect(await _checked(tester, '#g_medium'), isTrue);
    expect(await _checked(tester, '#g_small'), isFalse);
    expect(await _checked(tester, 'Medium'), isTrue, reason: 'the label of a tile reads the tile');
    expect(await _checked(tester, 'Large'), isFalse);
    expect((await _see(tester, 'Large', negated: true))['error'], isNull);
  });

  testWidgets('a RadioGroup with nothing selected checks none of its radios, and two groups do not mix', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          RadioGroup<String>(
            groupValue: null,
            child: Radio<String>(key: const ValueKey('none_a'), value: 'a', groupValue: null, onChanged: (_) {}),
          ),
          RadioGroup<String>(
            groupValue: 'a',
            child: Radio<String>(key: const ValueKey('other_a'), value: 'a', groupValue: null, onChanged: (_) {}),
          ),
        ]),
      ),
    ));
    expect(await _checked(tester, '#none_a'), isFalse);
    expect(await _checked(tester, '#other_a'), isTrue);
  });

  testWidgets('a radio with neither a group value nor a RadioGroup is not checked, and is still a checkable control', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: Radio<String>(key: const ValueKey('lone'), value: 'a', groupValue: null, onChanged: (_) {})),
    ));
    final res = await _see(tester, '#lone');
    expect(res['error'], isNotNull);
    expect(jsonEncode(res['error']), isNot(contains('not a checkable control')), reason: 'it is checkable, just not on');
  });

  testWidgets('the nearest RadioGroup decides, even when it has nothing selected and an outer one has', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: RadioGroup<String>(
          groupValue: 'a',
          child: RadioGroup<String>(
            groupValue: null,
            child: Radio<String>(key: const ValueKey('inner_a'), value: 'a', groupValue: null, onChanged: (_) {}),
          ),
        ),
      ),
    ));
    expect(await _checked(tester, '#inner_a'), isFalse, reason: 'the inner group has no selection; the outer one does not count');
  });

  testWidgets('a group whose value cannot be read makes the radio "not a checkable control", never a wrong yes', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: RadioGroup<String>(
          groupValue: 'a',
          broken: true,
          child: Radio<String>(key: const ValueKey('broken_a'), value: 'a', groupValue: null, onChanged: (_) {}),
        ),
      ),
    ));
    final res = await _see(tester, '#broken_a');
    expect(res['error'], isNotNull);
    expect(jsonEncode(res['error']), contains('not a checkable control'));
    // (the negated form, `don't see ... is checked`, passes on any control the agent cannot read: that is how the check already behaves, not something this test pins)
  });
}
