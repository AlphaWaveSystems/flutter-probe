import 'dart:convert';
import 'dart:io';

import 'package:flutter_probe_agent/src/textfold.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('foldText agrees with the Go implementation on the shared cases', () {
    final raw = File('../internal/textfold/testdata/cases.json').readAsStringSync();
    final cases = (jsonDecode(raw) as List).cast<Map<String, dynamic>>();
    expect(cases, isNotEmpty);
    for (final c in cases) {
      expect(foldText(c['in'] as String), c['out'], reason: 'input ${jsonEncode(c['in'])}');
    }
  });

  test('foldedContains / foldedEquals', () {
    expect(foldedEquals('Don’t Allow', "don't allow"), isTrue);
    expect(foldedContains('Müller Straße 5', 'strasse'), isTrue);
    expect(foldedEquals('a', 'b'), isFalse);
  });
}
