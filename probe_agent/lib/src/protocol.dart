import 'dart:convert';

/// JSON-RPC 2.0 request received from the probe CLI.
class ProbeRequest {
  final String jsonrpc;
  final int id;
  final String method;
  final Map<String, dynamic> params;

  const ProbeRequest({
    required this.jsonrpc,
    required this.id,
    required this.method,
    required this.params,
  });

  factory ProbeRequest.fromJson(Map<String, dynamic> json) {
    return ProbeRequest(
      jsonrpc: json['jsonrpc'] as String? ?? '2.0',
      id: (json['id'] as num).toInt(),
      method: json['method'] as String,
      params: (json['params'] as Map<String, dynamic>?) ?? {},
    );
  }

  static ProbeRequest? tryParse(String raw) {
    try {
      final map = jsonDecode(raw) as Map<String, dynamic>;
      if (!map.containsKey('id')) return null; // notification
      return ProbeRequest.fromJson(map);
    } catch (_) {
      return null;
    }
  }
}

/// JSON-RPC 2.0 response sent back to the probe CLI.
class ProbeResponse {
  final int id;
  final dynamic result;
  final ProbeError? error;

  const ProbeResponse.ok(this.id, this.result) : error = null;
  const ProbeResponse.err(this.id, this.error) : result = null;

  Map<String, dynamic> toJson() {
    if (error != null) {
      return {'jsonrpc': '2.0', 'id': id, 'error': error!.toJson()};
    }
    return {'jsonrpc': '2.0', 'id': id, 'result': result};
  }

  String encode() => jsonEncode(toJson());
}

/// JSON-RPC 2.0 error object.
class ProbeError {
  final int code;
  final String message;

  const ProbeError(this.code, this.message);

  // Standard codes
  static const int parseError     = -32700;
  static const int methodNotFound = -32601;
  static const int invalidParams  = -32602;
  static const int internalError  = -32603;
  static const int timeout        = -32000;
  static const int widgetNotFound = -32001;
  static const int assertFailed   = -32002;

  Map<String, dynamic> toJson() => {'code': code, 'message': message};
}

/// JSON-RPC 2.0 notification (no id) sent from agent to CLI.
class ProbeNotification {
  final String method;
  final dynamic params;

  const ProbeNotification(this.method, this.params);

  String encode() => jsonEncode({
        'jsonrpc': '2.0',
        'method': method,
        'params': params,
      });
}

/// JSON-RPC method name constants — mirrors Go probelink/protocol.go.
class ProbeMethods {
  ProbeMethods._();

  // Request methods (CLI → agent)
  static const ping = 'probe.ping';
  static const settled = 'probe.settled';
  static const open = 'probe.open';
  static const tap = 'probe.tap';
  static const doubleTap = 'probe.double_tap';
  static const longPress = 'probe.long_press';
  static const type_ = 'probe.type';
  static const clear = 'probe.clear';
  static const see = 'probe.see';
  static const wait = 'probe.wait';
  static const swipe = 'probe.swipe';
  static const scroll = 'probe.scroll';
  static const drag = 'probe.drag';
  static const deviceAction = 'probe.device_action';
  static const close = 'probe.close';
  static const screenshot = 'probe.screenshot';
  static const dumpTree = 'probe.dump_tree';
  static const saveLogs = 'probe.save_logs';
  static const runDart = 'probe.run_dart';
  static const mock = 'probe.mock';
  static const startRecording = 'probe.start_recording';
  static const stopRecording = 'probe.stop_recording';

  // Notification methods (agent → CLI)
  static const notifyRecordedEvent = 'probe.recorded_event';
  static const notifyExecDart = 'probe.exec_dart';
  static const notifyRestartApp = 'probe.restart_app';
}
