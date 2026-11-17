import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_probe_agent/src/agent_version.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('probe.ping handshake', () {
    late ProbeExecutor executor;
    late String? lastSent;

    setUp(() {
      lastSent = null;
      executor = ProbeExecutor((msg) => lastSent = msg);
    });

    Map<String, dynamic> decodeResult() {
      final decoded = jsonDecode(lastSent!) as Map<String, dynamic>;
      return decoded['result'] as Map<String, dynamic>;
    }

    test('returns ok and the agent version', () async {
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 1,
        method: ProbeMethods.ping,
        params: const {},
      ));

      final result = decodeResult();
      expect(result['ok'], isTrue);
      expect(result['agent_version'], equals(probeAgentVersion));
    });

    test('accepts an unknown client_version field without error', () async {
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 2,
        method: ProbeMethods.ping,
        params: const {'client_version': '0.9.3'},
      ));

      final result = decodeResult();
      expect(result['ok'], isTrue);
      expect(result['agent_version'], equals(probeAgentVersion));
    });

    test('does not error when params omit client_version entirely '
        '(older CLI compatibility)', () async {
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 3,
        method: ProbeMethods.ping,
        params: const {},
      ));

      final decoded = jsonDecode(lastSent!) as Map<String, dynamic>;
      expect(decoded.containsKey('error'), isFalse);
    });

    test('carries the build info fields, plain text, next to the existing ones', () async {
      await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0',
        id: 4,
        method: ProbeMethods.ping,
        params: const {},
      ));

      final result = decodeResult();
      // the shape older CLIs read is unchanged
      expect(result['ok'], isTrue);
      expect(result['agent_version'], equals(probeAgentVersion));
      expect(result['capabilities'], equals(probeAgentCapabilities));
      // the new fields are optional: when present they are plain version text, and nothing else is added
      // view_ready_ms: how long the request waited for the view's real size (issue #793), an int, 0 when it did not wait
      const known = {'ok', 'agent_version', 'capabilities', 'flutter_version', 'flutter_channel', 'framework_revision', 'dart_version', 'view_ready_ms'};
      expect(known.containsAll(result.keys), isTrue, reason: '${result.keys}');
      for (final k in ['flutter_version', 'flutter_channel', 'framework_revision', 'dart_version']) {
        if (result.containsKey(k)) {
          expect(RegExp(r'^[A-Za-z0-9.+-]{1,64}$').hasMatch(result[k] as String), isTrue, reason: '$k=${result[k]}');
        }
      }
      expect(result['dart_version'], isNotNull);
    });
  });
}
