import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'executor.dart';
import 'protocol.dart';

/// ProbeServer is a WebSocket server that listens on localhost:48686.
///
/// The probe CLI connects to this server after the app starts.
/// Authentication is token-based: the server emits a one-time token
/// to stdout/logcat which the CLI reads before connecting.
///
/// Supports two transport modes:
/// - **WebSocket** (default): persistent connection at `ws://host:port/probe?token=<token>`
/// - **HTTP POST** (fallback for physical devices): stateless `POST /probe/rpc?token=<token>`
class ProbeServer {
  final int port;
  final bool allowRemoteConnections;
  HttpServer? _server;
  String? _token;
  ProbeExecutor? _executor;

  /// Shared executor for HTTP mode — persists state (mocks, URL tracking)
  /// across stateless HTTP requests within the same session.
  ProbeExecutor? _httpExecutor;

  /// Creates a ProbeServer.
  /// Set [allowRemoteConnections] to true for WiFi testing (binds to 0.0.0.0
  /// instead of localhost). Only use in debug/profile builds — never in release.
  ProbeServer({this.port = 48686, this.allowRemoteConnections = false});

  Timer? _tokenTimer;

  /// Starts the WebSocket server and prints the session token.
  /// If a pre-shared token was persisted (via `set_next_token`), uses that
  /// instead of generating a random one. This enables reconnection after
  /// `restart the app` in WiFi mode where the CLI can't read device logs.
  Future<void> start() async {
    _token = await _readPersistedToken() ?? _generateToken();
    final bindAddress = allowRemoteConnections
        ? InternetAddress.anyIPv4
        : InternetAddress.loopbackIPv4;
    _server = await HttpServer.bind(bindAddress, port);

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
      // Validate token (shared by both WebSocket and HTTP paths)
      final queryToken = req.uri.queryParameters['token'] ??
          req.headers.value('x-probe-token');
      if (queryToken != _token) {
        req.response
          ..statusCode = HttpStatus.unauthorized
          ..close();
        continue;
      }

      // HTTP POST fallback: stateless JSON-RPC over HTTP
      if (req.method == 'POST' && req.uri.path == '/probe/rpc') {
        await _handleHttpRpc(req);
        continue;
      }

      // WebSocket upgrade
      if (!WebSocketTransformer.isUpgradeRequest(req)) {
        req.response
          ..statusCode = HttpStatus.badRequest
          ..close();
        continue;
      }

      final ws = await WebSocketTransformer.upgrade(req);
      _handleConnection(ws);
    }
  }

  /// Handles a stateless HTTP POST JSON-RPC request.
  Future<void> _handleHttpRpc(HttpRequest req) async {
    try {
      final body = await utf8.decoder.bind(req).join();
      final probeReq = ProbeRequest.tryParse(body);
      if (probeReq == null) {
        req.response
          ..statusCode = HttpStatus.badRequest
          ..headers.contentType = ContentType.json
          ..write('{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"}}')
          ..close();
        return;
      }

      // Use a shared executor so mocks and state persist across HTTP calls.
      // Swap the send function per request to route the response back.
      final completer = Completer<String>();
      _httpExecutor ??= ProbeExecutor((msg) {
        if (!completer.isCompleted) completer.complete(msg);
      });
      _httpExecutor!.sendFn = (msg) {
        if (!completer.isCompleted) completer.complete(msg);
      };

      await _httpExecutor!.dispatch(probeReq);

      final response = await completer.future.timeout(
        const Duration(seconds: 120),
        onTimeout: () => '{"jsonrpc":"2.0","id":${probeReq.id},"error":{"code":-32000,"message":"Timeout"}}',
      );

      req.response
        ..statusCode = HttpStatus.ok
        ..headers.contentType = ContentType.json
        ..write(response)
        ..close();
    } catch (e) {
      req.response
        ..statusCode = HttpStatus.internalServerError
        ..headers.contentType = ContentType.json
        ..write('{"jsonrpc":"2.0","error":{"code":-32603,"message":"${e.toString().replaceAll('"', '\\"')}"}}')
        ..close();
    }
  }

  void _handleConnection(WebSocket ws) {
    // ignore: avoid_print
    print('ProbeAgent: CLI connected');

    // Enable WebSocket-level ping/pong keepalive to prevent idle connections
    // from being dropped by iproxy or iOS network stack on physical devices.
    ws.pingInterval = const Duration(seconds: 5);

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

  /// Persists a token to disk so the agent uses it after restart.
  /// Called by the CLI via `probe.set_next_token` before `restart the app`.
  Future<void> setNextToken(String token) async {
    try {
      final file = File(_nextTokenPath());
      await file.parent.create(recursive: true);
      await file.writeAsString(token);
      // ignore: avoid_print
      print('ProbeAgent: next token persisted for restart');
    } catch (e) {
      // ignore: avoid_print
      print('ProbeAgent: failed to persist next token: $e');
    }
  }

  /// Reads a persisted next-token from disk (set before restart).
  /// Deletes the file after reading so it's only used once.
  Future<String?> _readPersistedToken() async {
    try {
      final file = File(_nextTokenPath());
      if (await file.exists()) {
        final token = (await file.readAsString()).trim();
        await file.delete();
        if (token.length >= 16) {
          // ignore: avoid_print
          print('ProbeAgent: using pre-shared token from restart');
          return token;
        }
      }
    } catch (_) {}
    return null;
  }

  String _nextTokenPath() {
    if (Platform.isIOS) {
      return '${Directory.systemTemp.path}/probe/next_token';
    } else if (Platform.isAndroid) {
      return '/data/data/${_resolvePackageName()}/cache/probe/next_token';
    }
    return '${Directory.systemTemp.path}/probe/next_token';
  }

  static String _generateToken() {
    const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
    final rng = Random.secure();
    return List.generate(32, (_) => chars[rng.nextInt(chars.length)]).join();
  }
}
