import 'dart:async';
import 'dart:io';

import 'executor.dart';
import 'protocol.dart';

/// ProbeRelayClient connects the agent OUT to a ProbeRelay server instead of
/// listening for incoming connections. Used for cloud device farms where
/// inbound ports are not reachable.
///
/// The relay is a "dumb pipe" — it forwards WebSocket frames verbatim between
/// agent and CLI. From the executor's perspective, nothing changes: it receives
/// [ProbeRequest]s and sends responses via [SendFn].
class ProbeRelayClient {
  final String relayUrl;
  final String agentToken;
  final Duration reconnectDelay;

  WebSocket? _ws;
  ProbeExecutor? _executor;
  bool _stopped = false;

  ProbeRelayClient({
    required this.relayUrl,
    required this.agentToken,
    this.reconnectDelay = const Duration(seconds: 2),
  });

  /// The current executor, if connected.
  ProbeExecutor? get executor => _executor;

  /// Whether the relay client is actively connected.
  bool get isConnected => _ws != null;

  /// Connects to the relay server and starts dispatching incoming requests.
  /// Automatically reconnects on disconnect unless [stop] is called.
  Future<void> connect() async {
    if (_stopped) return;

    try {
      final uri = Uri.parse(relayUrl);
      final params = Map<String, String>.from(uri.queryParameters);
      params['role'] = 'agent';
      params['token'] = agentToken;
      final connectUri = uri.replace(queryParameters: params);

      // ignore: avoid_print
      print('ProbeAgent: connecting to relay ${uri.host}:${uri.port}...');
      _ws = await WebSocket.connect(connectUri.toString());
      // ignore: avoid_print
      print('ProbeAgent: relay connected');

      final executor = ProbeExecutor((msg) => _ws?.add(msg));
      _executor = executor;

      _ws!.listen(
        (data) async {
          if (data is! String) return;
          final req = ProbeRequest.tryParse(data);
          if (req == null) return;
          await executor.dispatch(req);
        },
        onDone: () {
          // ignore: avoid_print
          print('ProbeAgent: relay disconnected');
          _executor = null;
          _ws = null;
          _reconnect();
        },
        onError: (e) {
          // ignore: avoid_print
          print('ProbeAgent: relay error: $e');
        },
        cancelOnError: false,
      );
    } catch (e) {
      // ignore: avoid_print
      print('ProbeAgent: relay connect failed: $e');
      _ws = null;
      _executor = null;
      _reconnect();
    }
  }

  /// Stops the relay client and prevents reconnection.
  Future<void> stop() async {
    _stopped = true;
    _executor = null;
    await _ws?.close();
    _ws = null;
  }

  void _reconnect() {
    if (_stopped) return;
    Future.delayed(reconnectDelay, connect);
  }
}
