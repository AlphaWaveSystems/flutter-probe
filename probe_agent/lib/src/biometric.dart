import 'dart:async';

// Single pending completer — only one biometric prompt can be active at a time.
Completer<bool>? _pending;

/// Returns a [Future] that resolves to `true` (match) or `false` (no-match)
/// when the FlutterProbe CLI delivers a [probe.biometric_signal] command.
///
/// Call this in PROBE_AGENT builds instead of `local_auth.authenticate()`:
///
/// ```dart
/// import 'package:flutter_probe_agent/flutter_probe_agent.dart';
///
/// final ok = const bool.fromEnvironment('PROBE_AGENT')
///     ? await awaitBiometricResult()
///     : await localAuth.authenticate(...);
/// ```
///
/// The CLI resolves this automatically after the `biometric match` or
/// `biometric no match` ProbeScript step fires — no app-side changes to
/// the test script are required.
Future<bool> awaitBiometricResult() {
  _pending = Completer<bool>();
  return _pending!.future;
}

/// Called by [ProbeExecutor] when [probe.biometric_signal] arrives.
void completeBiometricResult(bool result) {
  _pending?.complete(result);
  _pending = null;
}
