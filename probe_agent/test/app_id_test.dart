import 'dart:convert';
import 'dart:io';

import 'package:flutter_probe_agent/src/app_id.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_probe_agent/src/server.dart';
import 'package:flutter_test/flutter_test.dart';

List<int> _cmdline(String s) => [...s.codeUnits, 0];

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('plainAppId', () {
    test('accepts what an Android package name or an iOS bundle id looks like',
        () {
      for (final ok in [
        'com.example.app',
        'a.b',
        'com.example.app_2',
        'com.my-co.app',
        'dev.probe.portspikea',
        'com.alphawavesystems.probe_fixture_b',
        'Com.Example.App',
        'com.example.2fa',
      ]) {
        expect(plainAppId(ok), ok, reason: ok);
      }
    });

    test('leaves out everything else, and repairs nothing', () {
      for (final bad in [
        '',
        'single',
        '.a.b',
        'a..b',
        'a.b.',
        'com.example.app:remote', // a process name with a suffix
        'co:m.example.app', // a colon in the first segment
        'com.ex:ample.app', // a colon in a middle segment
        'com.example.a pp',
        'c\$m.example.app',
        'com.example.app\n',
        'com.example. app',
        'com.example.app;rm',
        'com.exämple.app',
        '${'a' * 126}.${'b' * 130}', // 257 characters
        '-a.b',
        'a.-b',
      ]) {
        expect(plainAppId(bad), isNull, reason: bad);
      }
      expect(plainAppId(null), isNull);
    });

    test('accepts the longest id, 255 characters', () {
      final id = '${'a' * 126}.${'b' * 128}';
      expect(id.length, 255);
      expect(plainAppId(id), id);
    });
  });

  group('processNameOf', () {
    test('is the first NUL-terminated field of the command line', () {
      expect(
          processNameOf(
              [...'com.example.app'.codeUnits, 0, ...'--flag'.codeUnits, 0]),
          'com.example.app');
      expect(processNameOf('com.example.app'.codeUnits),
          'com.example.app'); // no NUL at all
    });

    test('is null when there is none', () {
      expect(processNameOf(null), isNull);
      expect(processNameOf(const []), isNull);
      expect(processNameOf(const [0, 65]),
          isNull); // starts with the end of the field
    });
  });

  group('probeAppIdFrom', () {
    test('the process name wins over the build define', () {
      expect(
          probeAppIdFrom(
              define: 'com.defined.app', cmdline: _cmdline('com.process.app')),
          'com.process.app');
    });

    test(
        'falls back to the define when the process name is not an application id',
        () {
      expect(
          probeAppIdFrom(
              define: 'com.defined.app',
              cmdline: _cmdline('com.example.app:remote')),
          'com.defined.app');
      expect(probeAppIdFrom(define: 'com.defined.app'), 'com.defined.app');
      expect(
          probeAppIdFrom(
              define: 'com.defined.app',
              cmdline: _cmdline('/system/bin/app_process')),
          'com.defined.app');
    });

    test('knows nothing when neither is an application id', () {
      expect(probeAppIdFrom(), isNull);
      expect(probeAppIdFrom(define: 'not an id', cmdline: _cmdline('zygote')),
          isNull);
      expect(probeAppIdFrom(define: '', cmdline: const []), isNull);
    });
  });

  group('probe.ping and GET /probe/status report it', () {
    final saved = probeAppIdReader;
    tearDown(() => probeAppIdReader = saved);

    Future<Map<String, dynamic>> ping() async {
      String? sent;
      final executor = ProbeExecutor((msg) => sent = msg);
      await executor.dispatch(ProbeRequest(
          jsonrpc: '2.0', id: 1, method: ProbeMethods.ping, params: const {}));
      return (jsonDecode(sent!) as Map<String, dynamic>)['result']
          as Map<String, dynamic>;
    }

    test('the ping has app_id when the id is known', () async {
      probeAppIdReader = () => 'com.example.app';
      final r = await ping();
      expect(r['app_id'], 'com.example.app');
      expect(r['ok'], isTrue);
      expect(r['agent_version'], probeAgentVersion); // what was there stays
    });

    test('the ping has no app_id key when it is not known', () async {
      probeAppIdReader = () => null;
      expect((await ping()).containsKey('app_id'), isFalse);
    });

    // A raw socket: TestWidgetsFlutterBinding answers every HttpClient request of a test with 400, which is not the agent.
    Future<Map<String, dynamic>> status(int port) async {
      final socket = await Socket.connect('127.0.0.1', port);
      try {
        socket.write('GET /probe/status HTTP/1.0\r\nHost: 127.0.0.1\r\n\r\n');
        await socket.flush();
        final raw = await utf8.decoder.bind(socket).join();
        expect(raw, matches(RegExp(r'^HTTP/1\.[01] 200 ')));
        return jsonDecode(raw.substring(raw.indexOf('\r\n\r\n') + 4))
            as Map<String, dynamic>;
      } finally {
        socket.destroy();
      }
    }

    test(
        'the status endpoint has app_id next to agent and version, and needs no token',
        () async {
      probeAppIdReader = () => 'com.example.app';
      final server = ProbeServer(port: 49130, portRange: 5);
      await server.start();
      try {
        final s = await status(server.actualPort);
        expect(s, {
          'agent': 'flutter_probe',
          'version': probeAgentVersion,
          'app_id': 'com.example.app'
        });
      } finally {
        await server.stop();
      }
    });

    test('the status endpoint is unchanged when the id is not known', () async {
      probeAppIdReader = () => null;
      final server = ProbeServer(port: 49140, portRange: 5);
      await server.start();
      try {
        expect(await status(server.actualPort),
            {'agent': 'flutter_probe', 'version': probeAgentVersion});
      } finally {
        await server.stop();
      }
    });
  });
}
