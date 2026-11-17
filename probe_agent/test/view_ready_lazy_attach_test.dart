import 'dart:convert';

import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';
import 'package:flutter_probe_agent/src/view_ready.dart';

// The agent never makes the binding. An app may start the agent first and install its OWN binding afterwards (its own subclass, an
// error-reporting package's): that must not fail, and the agent attaches on its first request, once. This file starts with no binding
// (plain `test`, no testWidgets).
class AppBinding extends WidgetsFlutterBinding {}

void main() {
  test(
      'no binding: attach does nothing; the app installs its own binding; the first request attaches, once',
      () async {
    ViewReady.instance.attach();
    ViewReady.instance.attachToAppBinding();
    expect(ViewReady.instance.attached, isFalse);

    expect(() => AppBinding(), returnsNormally,
        reason: "the agent must not have made a binding of its own");
    expect(WidgetsBinding.instance, isA<AppBinding>());

    String? sent;
    final executor = ProbeExecutor((msg) => sent = msg);
    await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0', id: 1, method: ProbeMethods.ping, params: const {}));
    expect((jsonDecode(sent!) as Map<String, dynamic>)['result']['ok'], isTrue);
    expect(
        (jsonDecode(sent!) as Map<String, dynamic>)['result']['view_ready_ms'],
        0,
        reason:
            'attached late with no frame pending: the current size is laid out, nothing to wait for');
    expect(ViewReady.instance.attached, isTrue,
        reason: 'the first request with the app binding in place attaches');
    final attempts = ViewReady.instance.attachAttempts;
    await executor.dispatch(ProbeRequest(
        jsonrpc: '2.0', id: 2, method: ProbeMethods.ping, params: const {}));
    expect(ViewReady.instance.attachAttempts, attempts,
        reason: 'once attached, no request tries again');
  });
}
