import 'dart:io';

import 'package:flutter_probe_agent/src/agent_version.dart';
import 'package:flutter_test/flutter_test.dart';

// The agent reports probeAgentVersion at handshake, and the CLI compares it with its own version. A release that bumps
// pubspec.yaml but not this constant reports the old version, so a matching CLI warns about a mismatch.
void main() {
  test('probeAgentVersion matches the version in pubspec.yaml', () {
    final pubspec = File('pubspec.yaml').readAsStringSync();
    final match = RegExp(r'^version:\s*(\S+)\s*$', multiLine: true).firstMatch(pubspec);
    expect(match, isNotNull, reason: 'pubspec.yaml has no version: line');
    expect(probeAgentVersion, match!.group(1));
  });
}
