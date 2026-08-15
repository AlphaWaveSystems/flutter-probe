// Variant of ping_repro.dart: instead of one long synchronous CPU-bound
// block, flood the event loop with a dense chain of microtasks/short async
// hops (closer in shape to real Firestore-listener-churn async work) for
// longer than pingInterval, and separately try many repeated short blocks
// (bursty jank) rather than one continuous block.

import 'dart:async';
import 'dart:io';

const pingInterval = Duration(seconds: 2);

Future<void> main() async {
  await scenario('C) dense microtask flood > pingInterval', mode: Mode.microtaskFlood);
  await scenario('D) repeated short sync bursts (bursty jank)', mode: Mode.burstyJank);
}

enum Mode { microtaskFlood, burstyJank }

Future<void> scenario(String label, {required Mode mode}) async {
  print('\n=== $label ===');
  final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
  final port = server.port;

  var serverGotDone = false;
  final serverClosedCompleter = Completer<void>();

  server.listen((req) async {
    if (!WebSocketTransformer.isUpgradeRequest(req)) return;
    final ws = await WebSocketTransformer.upgrade(req);
    ws.pingInterval = pingInterval;
    ws.listen(
      (data) {},
      onDone: () {
        serverGotDone = true;
        print('  [server] onDone fired at ${DateTime.now()}');
        if (!serverClosedCompleter.isCompleted) serverClosedCompleter.complete();
      },
      onError: (e) => print('  [server] onError: $e'),
      cancelOnError: false,
    );
  });

  final client = await WebSocket.connect('ws://127.0.0.1:$port/probe');
  var clientGotDone = false;
  client.listen((data) {}, onDone: () {
    clientGotDone = true;
    print('  [client] onDone fired, closeCode=${client.closeCode}');
  });

  await Future<void>.delayed(const Duration(milliseconds: 300));
  print('  connection established, pingInterval=$pingInterval');

  final totalDuration = pingInterval * 4;
  final stopwatch = Stopwatch()..start();

  if (mode == Mode.microtaskFlood) {
    print('  flooding microtask queue for ${totalDuration.inSeconds}s...');
    // Chain millions of scheduleMicrotask calls -- keeps the event loop
    // "running" but never lets it drain to service timers/IO callbacks
    // (like the WS ping timer) until the chain finally empties.
    var count = 0;
    void pump() {
      count++;
      if (stopwatch.elapsed < totalDuration) {
        scheduleMicrotask(pump);
      }
    }
    final done = Completer<void>();
    void pumpWithCompletion() {
      count++;
      if (stopwatch.elapsed < totalDuration) {
        scheduleMicrotask(pumpWithCompletion);
      } else {
        done.complete();
      }
    }
    scheduleMicrotask(pumpWithCompletion);
    await done.future;
    print('  ...done, scheduled $count microtasks');
  } else {
    print('  bursty jank: 40x (200ms sync block + 50ms yield) for ${totalDuration.inSeconds}s...');
    while (stopwatch.elapsed < totalDuration) {
      final burstStart = Stopwatch()..start();
      var x = 0;
      while (burstStart.elapsed < const Duration(milliseconds: 200)) {
        x = (x + 1) % 1000000007;
      }
      await Future<void>.delayed(const Duration(milliseconds: 50));
    }
    print('  ...done bursting');
  }

  await Future.any([
    serverClosedCompleter.future,
    Future<void>.delayed(const Duration(seconds: 3)),
  ]);

  print('  RESULT: serverGotDone=$serverGotDone clientGotDone=$clientGotDone');
  if (serverGotDone) {
    print('  ==> CONFIRMED: this load shape closes an otherwise-healthy connection.');
  } else {
    print('  ==> NOT confirmed this way.');
  }

  await client.close();
  await server.close(force: true);
}
