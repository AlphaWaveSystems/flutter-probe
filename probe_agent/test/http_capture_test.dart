import 'dart:convert';
import 'dart:io';

import 'package:flutter_probe_agent/src/http_capture.dart';
import 'package:flutter_test/flutter_test.dart';

Future<(int, String)> _get(HttpClient c, Uri uri, {String method = 'get', String? body}) async {
  final req = await c.openUrl(method, uri);
  if (body != null) req.write(body);
  final resp = await req.close();
  final text = await resp.transform(utf8.decoder).join();
  return (resp.statusCode, text);
}

void main() {
  late HttpServer server;
  late Uri base;

  setUpAll(() async {
    ProbeHttpOverrides.install();
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    base = Uri.parse('http://127.0.0.1:${server.port}');
    server.listen((req) async {
      req.response.headers.set('x-from', 'real');
      if (req.uri.path == '/api/me') {
        req.response.write('{"data":{"plan":"pro"}}');
      } else {
        req.response.statusCode = 404;
        req.response.write('nope');
      }
      await req.response.close();
    });
  });

  tearDownAll(() => server.close(force: true));
  setUp(() => ProbeHttpLog.instance.clear(mocks: true));

  test('url patterns match whole paths, globs and full urls', () {
    final u = Uri.parse('https://api.example.com/api/orders/42?x=1');
    expect(probeUrlMatches('/api/orders/*', u), isTrue);
    expect(probeUrlMatches('/api/orders', u), isFalse);
    expect(probeUrlMatches('/api/orders/42', u), isTrue);
    expect(probeUrlMatches('*/orders/*', u), isTrue);
    expect(probeUrlMatches('https://api.example.com/api/orders/42', u), isTrue);
    expect(probeUrlMatches('/api/orders/42?x=1', u), isTrue);
    expect(probeUrlMatches('/api/orders/42?x=2', u), isFalse);
    expect(probeUrlMatches('', u), isTrue);
  });

  test('captures method, url, status, headers and bodies; redacts secrets', () async {
    final client = HttpClient();
    final req = await client.postUrl(base.replace(path: '/api/me'));
    req.headers.set('authorization', 'Bearer secret');
    req.write('{"q":1}');
    final resp = await req.close();
    await resp.drain<void>();

    final found = ProbeHttpLog.instance.query(method: 'POST', pattern: '/api/me');
    expect(found, hasLength(1));
    final j = found.single.toJson();
    expect(j['status'], 200);
    expect(j['mocked'], false);
    expect(j['requestBody'], '{"q":1}');
    expect(j['responseBody'], '{"data":{"plan":"pro"}}');
    expect((j['requestHeaders'] as Map)['authorization'], '<redacted>');
    expect((j['responseHeaders'] as Map)['x-from'], 'real');
    expect(ProbeHttpLog.instance.query(method: 'GET', pattern: '/api/me'), isEmpty);
  });

  test('a mock answers instead of the network: status, body, headers', () async {
    ProbeHttpLog.instance.addMock(ProbeMock(
      method: 'GET',
      pattern: '/api/me',
      status: 503,
      body: '{"error":"down"}',
      headers: {'x-mock': '1'},
    ));
    final client = HttpClient();
    final req = await client.getUrl(base.replace(path: '/api/me'));
    final resp = await req.close();
    final text = await resp.transform(utf8.decoder).join();
    expect(resp.statusCode, 503);
    expect(text, '{"error":"down"}');
    expect(resp.headers.value('x-mock'), '1');
    final e = ProbeHttpLog.instance.query(pattern: '/api/me').single;
    expect(e.mocked, isTrue);
    expect(e.url, base.replace(path: '/api/me').toString(), reason: 'the log keeps the URL the app asked for');
  });

  test('mock delay and failure', () async {
    ProbeHttpLog.instance.addMock(ProbeMock(method: 'GET', pattern: '/slow', status: 200, body: 'ok', delayMs: 300));
    final sw = Stopwatch()..start();
    final r = await _get(HttpClient(), base.replace(path: '/slow'));
    expect(r.$1, 200);
    expect(sw.elapsedMilliseconds, greaterThanOrEqualTo(280));

    ProbeHttpLog.instance.addMock(ProbeMock(method: 'GET', pattern: '/boom', fail: true));
    await expectLater(_get(HttpClient(), base.replace(path: '/boom')), throwsA(isA<Exception>()));
    final e = ProbeHttpLog.instance.query(pattern: '/boom').single;
    expect(e.error, isNotNull);
  });

  test('since cursor, last and clear', () async {
    final c = HttpClient();
    await _get(c, base.replace(path: '/api/me'));
    final first = ProbeHttpLog.instance.latest;
    await _get(c, base.replace(path: '/api/me'));
    expect(ProbeHttpLog.instance.query(pattern: '/api/me'), hasLength(2));
    expect(ProbeHttpLog.instance.query(pattern: '/api/me', since: first), hasLength(1));
    ProbeHttpLog.instance.clear();
    expect(ProbeHttpLog.instance.query(pattern: '/api/me'), isEmpty);
  });
}
