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
  });
}
