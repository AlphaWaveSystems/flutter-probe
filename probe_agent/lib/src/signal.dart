import 'dart:async';

// name → pending Completer
final _pending = <String, Completer<String>>{};

/// Waits for the CLI to deliver a named signal via `deliver signal "name"`.
///
/// Returns the value sent with the step (defaults to `"true"` when no value
/// is specified in the ProbeScript). Use this to block on any OS-level
/// interaction the probe can't tap directly — system permission dialogs,
/// payment sheets, push notification prompts, etc.:
///
/// ```dart
/// import 'package:flutter_probe_agent/flutter_probe_agent.dart';
///
/// // In your widget:
/// if (const bool.fromEnvironment('PROBE_AGENT')) {
///   await awaitSignal('push_permission_granted');
/// } else {
///   await requestNotificationPermission();
/// }
/// ```
///
/// ProbeScript side:
/// ```
/// allow permission "notifications"
/// deliver signal "push_permission_granted"
/// ```
Future<String> awaitSignal(String name) {
  final completer = Completer<String>();
  _pending[name] = completer;
  return completer.future;
}

/// Called by [ProbeExecutor] when [probe.signal] arrives from the CLI.
void deliverSignal(String name, String value) {
  _pending.remove(name)?.complete(value);
}
