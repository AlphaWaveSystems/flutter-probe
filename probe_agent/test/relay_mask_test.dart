import 'package:flutter_probe_agent/src/relay_client.dart';
import 'package:flutter_test/flutter_test.dart';

// Synthetic token only. It contains characters that are escaped in a query value, so both forms are covered.
const _token = 'quorvex-agent-token/+ value';

void main() {
  group('maskToken', () {
    test('a relay error that names the connect URI does not print the token',
        () {
      final encoded = Uri.encodeQueryComponent(_token);
      final error =
          'WebSocketException: wss://relay.example.invalid/connect?role=agent&token=$encoded';
      final masked = maskToken(error, _token);
      expect(masked, isNot(contains(encoded)));
      expect(masked, contains('token=***'));
    });

    test('a token written as it is is masked too', () {
      final masked = maskToken('rejected token $_token', _token);
      expect(masked, isNot(contains(_token)));
      expect(masked, 'rejected token ***');
    });

    test('an empty token leaves the text unchanged', () {
      expect(maskToken('relay connect failed', ''), 'relay connect failed');
    });
  });
}
