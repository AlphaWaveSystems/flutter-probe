import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

// Issue #860: an id on a wrapper around a disabled control (a keyed SizedBox, a KeyedSubtree, a Semantics identifier) read as enabled, so
// `is enabled` and `don't see ... is disabled` passed. The enabled reader now looks below the target, as the checked reader does.
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

void main() {
  testWidgets(
      'a wrapper around a disabled control reads as disabled, positive and negated',
      (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          const SizedBox(
              key: ValueKey('wrap'),
              child: ElevatedButton(onPressed: null, child: Text('Off'))),
          Semantics(
              identifier: 'sem',
              child: const IconButton(onPressed: null, icon: Icon(Icons.add))),
          const KeyedSubtree(
              key: ValueKey('ks'),
              child: SizedBox(width: 200, child: TextField(enabled: false))),
        ]),
      ),
    ));
    for (final id in ['wrap', 'sem', 'ks']) {
      expect(await _see(id, 'disabled'), isNull, reason: '#$id is disabled');
      expect(await _see(id, 'enabled'), contains('is disabled'),
          reason: '#$id is enabled must fail');
      expect(await _see(id, 'disabled', negated: true), isNotNull,
          reason: "don't see #$id is disabled must fail");
    }
  });

  testWidgets('a wrapper around an enabled control reads as enabled',
      (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: SizedBox(
            key: const ValueKey('wrap_on'),
            child: ElevatedButton(onPressed: () {}, child: const Text('On'))),
      ),
    ));
    expect(await _see('wrap_on', 'enabled'), isNull);
    expect(await _see('wrap_on', 'disabled'), contains('is enabled'));
  });

  testWidgets('a text inside a disabled button still reads the button above it',
      (tester) async {
    await tester.pumpWidget(const MaterialApp(
      home: Scaffold(
          body: ElevatedButton(
              onPressed: null, child: Text('Submit', key: ValueKey('label')))),
    ));
    expect(await _see('label', 'disabled'), isNull);
  });

  testWidgets(
      'a wrapper over a long list with no control finishes quickly and keeps the old rule',
      (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: SizedBox(
          key: const ValueKey('list'),
          child: ListView(
              children: [for (var i = 0; i < 1000; i++) Text('Row $i')]),
        ),
      ),
    ));
    final watch = Stopwatch()..start();
    expect(await _see('list', 'enabled'), isNull);
    expect(watch.elapsedMilliseconds, lessThan(2000));
  });

  testWidgets(
      'a wrapper holding controls that disagree refuses; controls that agree give their answer',
      (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          Row(key: const ValueKey('mixed'), children: [
            ElevatedButton(onPressed: () {}, child: const Text('On')),
            const ElevatedButton(onPressed: null, child: Text('Off')),
          ]),
          const Row(key: ValueKey('both_off'), children: [
            ElevatedButton(onPressed: null, child: Text('Off 1')),
            IconButton(onPressed: null, icon: Icon(Icons.add)),
          ]),
        ]),
      ),
    ));
    for (final check in ['enabled', 'disabled']) {
      expect(await _see('mixed', check),
          contains('holds 2 controls that disagree'),
          reason: check);
      expect(await _see('mixed', check, negated: true),
          contains('cannot be evaluated'),
          reason: 'negated $check');
    }
    expect(await _see('both_off', 'disabled'), isNull);
    expect(await _see('both_off', 'enabled'), contains('is disabled'));
  });
}
