import 'package:flutter_probe_agent/src/advertiser.dart';
import 'package:flutter_probe_agent/src/server.dart';
import 'package:flutter_test/flutter_test.dart';

class _FakeAdvertiser extends ProbeAdvertiser {
  _FakeAdvertiser();
  final calls = <String>[];
  int? port;
  @override
  Future<void> start({required String name, required int port, required String agentVersion}) async {
    calls.add('start');
    this.port = port;
  }

  @override
  Future<void> stop() async => calls.add('stop');
}

void main() {
  // FP-15: advertising is an optional hook — the core agent has no mDNS (and no
  // native plugin) of its own.
  test('a supplied advertiser is started in WiFi mode with the real bound port and stopped on stop()', () async {
    final adv = _FakeAdvertiser();
    final server = ProbeServer(port: 49100, portRange: 5, allowRemoteConnections: true, advertiser: adv);
    await server.start();
    expect(adv.calls, ['start']);
    expect(adv.port, server.actualPort);
    await server.stop();
    expect(adv.calls, ['start', 'stop']);
  });

  test('localhost-only agents never advertise', () async {
    final adv = _FakeAdvertiser();
    final server = ProbeServer(port: 49110, portRange: 5, advertiser: adv);
    await server.start();
    expect(adv.calls, isEmpty);
    await server.stop();
  });

  test('WiFi mode without an advertiser still starts (no mDNS, no crash)', () async {
    final server = ProbeServer(port: 49120, portRange: 5, allowRemoteConnections: true);
    await server.start();
    expect(server.actualPort, greaterThanOrEqualTo(49120));
    await server.stop();
  });
}
