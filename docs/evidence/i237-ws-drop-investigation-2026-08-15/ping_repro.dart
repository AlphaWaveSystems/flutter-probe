// Minimal, isolated repro for issue #237's leading hypothesis:
// does a `ws.pingInterval`-configured dart:io WebSocket server close the
// connection when the isolate's event loop is starved for longer than the
// ping interval — even though the client and the underlying TCP socket are
// both perfectly healthy? This mirrors probe_agent/lib/src/server.dart's
// exact setup (ws.pingInterval = Duration(seconds: N)) at a shortened
// interval so the test runs in seconds, not minutes.
//
// Two scenarios:
//   A) baseline: server idle, no isolate starvation -> connection should stay up.
//   B) server isolate busy (CPU-bound work occupying the event loop) for
//      longer than pingInterval -> does the connection get force-closed?

import 'dart:async';
import 'dart:io';

const pingInterval = Duration(seconds: 2);

Future<void> main() async {
  await scenario('A) baseline (idle server, no starvation)', starve: false);
  await scenario('B) server isolate busy > pingInterval', starve: true);
}

Future<void> scenario(String label, {required bool starve}) async {
  print('\n=== $label ===');
  final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
  final port = server.port;

  WebSocket? serverSocket;
  final serverClosedCompleter = Completer<void>();
  var serverGotDone = false;

  server.listen((req) async {
    if (!WebSocketTransformer.isUpgradeRequest(req)) return;
    final ws = await WebSocketTransformer.upgrade(req);
    serverSocket = ws;
    ws.pingInterval = pingInterval; // exact same line as ProbeServer._handleConnection
    ws.listen(
      (data) {},
      onDone: () {
        serverGotDone = true;
        print('  [server] onDone fired (connection closed) at ${DateTime.now()}');
        if (!serverClosedCompleter.isCompleted) serverClosedCompleter.complete();
      },
      onError: (e) => print('  [server] onError: $e'),
      cancelOnError: false,
    );
  });

  final client = await WebSocket.connect('ws://127.0.0.1:$port/probe');
  var clientGotDone = false;
  client.listen(
    (data) {},
    onDone: () {
      clientGotDone = true;
      print('  [client] onDone fired (connection closed), closeCode=${client.closeCode}, closeReason=${client.closeReason}');
    },
    onError: (e) => print('  [client] onError: $e'),
  );

  await Future<void>.delayed(const Duration(milliseconds: 300)); // let the upgrade settle
  print('  connection established, pingInterval=$pingInterval');

  if (starve) {
    final blockFor = pingInterval * 4; // comfortably longer than one interval
    print('  blocking the SERVER isolate synchronously for ${blockFor.inSeconds}s (simulating heavy sync/microtask work)...');
    final stopwatch = Stopwatch()..start();
    // Tight CPU-bound loop -- occupies the isolate, event loop cannot run
    // timers or process I/O callbacks (including the WS ping timer/pong
    // handling) until this returns.
    var x = 0;
    while (stopwatch.elapsed < blockFor) {
      x = (x + 1) % 1000000007;
    }
    print('  ...unblocked after ${stopwatch.elapsed} (junk=$x)');
  } else {
    await Future<void>.delayed(pingInterval * 4);
  }

  // Give the event loop a moment to process anything queued during/after
  // the block (e.g. a deferred close).
  await Future.any([
    serverClosedCompleter.future,
    Future<void>.delayed(const Duration(seconds: 3)),
  ]);

  print('  RESULT: serverGotDone=$serverGotDone clientGotDone=$clientGotDone');
  if (starve && serverGotDone) {
    print('  ==> CONFIRMED: isolate starvation past pingInterval closes an otherwise-healthy connection.');
  } else if (starve && !serverGotDone) {
    print('  ==> NOT confirmed this way: connection survived the starvation window.');
  }

  await client.close();
  if (serverSocket != null && !serverGotDone) {
    await serverSocket!.close();
  }
  await server.close(force: true);
}
