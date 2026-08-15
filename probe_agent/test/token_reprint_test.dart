import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/server.dart';

void main() {
  group('periodic token re-print also re-attempts the token file write (PT-27)', () {
    // The app-cache-dir copy of the token file is only written once, at
    // startup, by _writeTokenFile() — but Android can clear an app's cache
    // directory at any time (confirmed via a real device: the file
    // disappeared permanently, reproducibly, after visiting a screen that
    // touches ImagePicker). The fix makes the existing every-3-second
    // re-print also re-attempt the write, so a cleared cache dir gets a
    // fresh copy back within seconds instead of staying gone for the rest
    // of the session. _writeTokenFile() itself no-ops safely on the host
    // test platform (neither Platform.isIOS nor Platform.isAndroid), so
    // this asserts on the tokenFileWriteAttempts counter rather than real
    // filesystem state — that counter is the only thing that actually
    // distinguishes pre-fix (incremented once, at start()) from post-fix
    // (incremented again on every periodic tick). The real Android
    // cache-eviction filesystem behavior is verified on a real device
    // instead (see docs/evidence/).
    late ProbeServer server;

    tearDown(() async {
      await server.stop();
    });

    test('re-attempts the token file write on every periodic tick, not just at startup',
        () async {
      server = ProbeServer(port: 0, portRange: 1);
      await server.start();

      // start() itself calls _writeTokenFile() once, synchronously.
      expect(server.tokenFileWriteAttempts, 1,
          reason: 'expected exactly one write attempt from start() before any tick');

      // Wait past two 3-second periodic ticks.
      await Future.delayed(const Duration(seconds: 7));

      expect(server.tokenFileWriteAttempts, greaterThanOrEqualTo(3),
          reason: 'expected start()\'s write plus at least 2 periodic re-attempts '
              'within 7s, got ${server.tokenFileWriteAttempts}');
    });
  });
}
