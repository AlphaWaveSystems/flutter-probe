import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/view_ready.dart';

// An app may start the agent before its binding exists (before runApp). attach must never throw then: the agent keeps the earlier rule.
// This file creates no binding (plain `test`, no `testWidgets`, no ensureInitialized).
void main() {
  test(
      'attach without a binding does not throw, and leaves the agent unattached',
      () {
    expect(() => ViewReady.instance.attach(), returnsNormally);
    expect(ViewReady.instance.attached, isFalse);
  });
}
