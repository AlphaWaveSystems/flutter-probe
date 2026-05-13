import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  tests: [
    ProbeTest('mocks api and calls webhook', steps: [
      Mock(method: HttpMethod.get, path: '/api/products', status: 200,
          body: '[{"id":1,"name":"Widget"}]'),
      Mock(method: HttpMethod.post, path: '/api/orders', status: 201),
      CallHttp(method: HttpMethod.post, url: 'https://example.com/hook',
          body: '{"event":"order_created"}'),
      CallHttp(method: HttpMethod.get, url: 'https://example.com/health'),
      Open(),
      See('Welcome'),
    ]),
  ],
)
class MockAndCallScreen {}
