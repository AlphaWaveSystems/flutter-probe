import 'dart:async';
import 'dart:io';
import 'dart:math';

import 'executor.dart';
import 'protocol.dart';

/// ProbeServer is a WebSocket server that listens on localhost:48686.
///
/// The probe CLI connects to this server after the app starts.
/// Authentication is token-based: the server emits a one-time token
/// to stdout/logcat which the CLI reads before connecting.
class ProbeServer {
  final int port;
  HttpServer? _server;
  String? _token;
  ProbeExecutor? _executor;

  ProbeServer({this.port = 48686});

  Timer? _tokenTimer;

  /// Starts the WebSocket server and prints the session token.
  Future<void> start() async {
    _token = _generateToken();
    _server = await HttpServer.bind(InternetAddress.loopbackIPv4, port);

    // Emit token so the CLI (via adb logcat / simctl log) can read it
    // ignore: avoid_print
    print('PROBE_TOKEN=$_token');

    // Write token to a file so the CLI can read it directly
    await _writeTokenFile();

    // Re-print token periodically so late-connecting CLI can pick it up
    _tokenTimer = Timer.periodic(const Duration(seconds: 3), (_) {
      // ignore: avoid_print
      print('PROBE_TOKEN=$_token');
    });

    _serve();
  }

  Future<void> _serve() async {
    await for (final req in _server!) {
      if (!WebSocketTransformer.isUpgradeRequest(req)) {
        req.response
          ..statusCode = HttpStatus.badRequest
          ..close();
        continue;
      }

      // Validate token
      final queryToken = req.uri.queryParameters['token'];
      if (queryToken != _token) {
        req.response
          ..statusCode = HttpStatus.unauthorized
          ..close();
        continue;
      }

      final ws = await WebSocketTransformer.upgrade(req);
      _handleConnection(ws);
    }
  }

  void _handleConnection(WebSocket ws) {
    // ignore: avoid_print
    print('ProbeAgent: CLI connected');
    final executor = ProbeExecutor((msg) => ws.add(msg));
    _executor = executor;

    ws.listen(
      (data) async {
        if (data is! String) return;
        final req = ProbeRequest.tryParse(data);
        if (req == null) return;
        await executor.dispatch(req);
      },
      onDone: () {
        // ignore: avoid_print
        print('ProbeAgent: CLI disconnected');
        _executor = null;
      },
      onError: (e) {
        // ignore: avoid_print
        print('ProbeAgent: WebSocket error: $e');
      },
      cancelOnError: false,
    );
  }

  /// Stops the server.
  Future<void> stop() async {
    _tokenTimer?.cancel();
    _tokenTimer = null;
    await _server?.close(force: true);
    _server = null;
  }

  Future<void> _writeTokenFile() async {
    try {
      if (Platform.isIOS) {
        final dir = '${Directory.systemTemp.path}/probe';
        await Directory(dir).create(recursive: true);
        await File('$dir/token').writeAsString(_token!);
      } else if (Platform.isAndroid) {
        // Write to app's cache directory (readable via adb shell run-as)
        final cacheDir = Directory('/data/data/${_resolvePackageName()}/cache/probe');
        await cacheDir.create(recursive: true);
        await File('${cacheDir.path}/token').writeAsString(_token!);
        // Also try the world-readable tmp (may work on some devices/emulators)
        try {
          final tmpDir = Directory('/data/local/tmp/probe');
          await tmpDir.create(recursive: true);
          await File('${tmpDir.path}/token').writeAsString(_token!);
        } catch (_) {}
      }
    } catch (_) {
      // Non-fatal: CLI can still read from log stream
    }
  }

  static String _resolvePackageName() {
    // Extract package name from the app's data directory path
    try {
      final dataDir = Directory.current.path;
      // On Android, cwd or isolate info may contain the package name
      // Fallback: read from /proc/self/cmdline
      final cmdline = File('/proc/self/cmdline').readAsStringSync();
      final pkg = cmdline.split('\x00').first;
      if (pkg.contains('.')) return pkg;
    } catch (_) {}
    return 'com.unknown.app';
  }

  ProbeExecutor? get executor => _executor;

  static String _generateToken() {
    const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
    final rng = Random.secure();
    return List.generate(32, (_) => chars[rng.nextInt(chars.length)]).join();
  }
}
