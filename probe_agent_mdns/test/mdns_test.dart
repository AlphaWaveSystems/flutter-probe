import 'package:flutter_probe_agent/flutter_probe_agent.dart';
import 'package:flutter_probe_agent_mdns/flutter_probe_agent_mdns.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('ProbeMdns is a const ProbeAdvertiser using the shared service type', () {
    const ProbeAdvertiser advertiser = ProbeMdns();
    expect(advertiser, isA<ProbeMdns>());
    expect(mdnsServiceType, '_flutterprobe._tcp');
  });

  test('stop() before start() is safe', () async {
    await const ProbeMdns().stop();
  });
}
