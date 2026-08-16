import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

/// Flutter twin of the native fixture apps (native-test-apps/android and
/// native-test-apps/ios): the identical 8-element surface with the identical
/// identifiers and visible strings, so the same test scenarios run in three
/// frameworks — Flutter via probe's normal verbs (both OSes), Kotlin/Android
/// via the `native` verbs, SwiftUI/iOS via the future N-2 WDA bridging.
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  const probeEnabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  if (probeEnabled) {
    await ProbeAgent.start();
  }
  runApp(const FixtureApp());
}

class FixtureApp extends StatelessWidget {
  const FixtureApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Probe Fixture (Flutter)',
      home: const FixtureScreen(),
    );
  }
}

class FixtureScreen extends StatefulWidget {
  const FixtureScreen({super.key});

  @override
  State<FixtureScreen> createState() => _FixtureScreenState();
}

class _FixtureScreenState extends State<FixtureScreen> {
  int _taps = 0;
  String _echo = '';
  bool _switchOn = false;
  bool _messageVisible = false;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Probe Fixture (Flutter)')),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Semantics(
              identifier: 'native_title',
              child: const Text('Native Test App',
                  style: TextStyle(fontSize: 28, fontWeight: FontWeight.bold)),
            ),
            const SizedBox(height: 16),
            Semantics(
              identifier: 'native_button',
              child: ElevatedButton(
                onPressed: () => setState(() => _taps++),
                child: const Text('Tap Me'),
              ),
            ),
            const SizedBox(height: 8),
            Semantics(
              identifier: 'native_counter',
              child: Text('Taps: $_taps', style: const TextStyle(fontSize: 18)),
            ),
            const SizedBox(height: 16),
            Semantics(
              identifier: 'native_input',
              child: TextField(
                // ValueKey directly on the TextField, not just the outer
                // Semantics wrapper — the Dart agent's synthetic
                // focus/tap routing doesn't reliably reach an interactive
                // widget through a Semantics render layer above it (see
                // feedback_semantics_gesture_issue memory).
                key: const ValueKey('native_input'),
                decoration: const InputDecoration(hintText: 'Enter text here'),
                onChanged: (v) => setState(() => _echo = v),
              ),
            ),
            const SizedBox(height: 8),
            Semantics(
              identifier: 'native_echo',
              child: Text(
                _echo.isEmpty ? 'Echo: (empty)' : 'Echo: $_echo',
                style: const TextStyle(fontSize: 18),
              ),
            ),
            const SizedBox(height: 16),
            Row(
              children: [
                const Text('Native Switch', style: TextStyle(fontSize: 18)),
                const SizedBox(width: 8),
                Semantics(
                  identifier: 'native_switch',
                  child: Switch(
                    value: _switchOn,
                    onChanged: (v) => setState(() => _switchOn = v),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 16),
            Semantics(
              identifier: 'native_message_button',
              child: ElevatedButton(
                onPressed: () => setState(() => _messageVisible = true),
                child: const Text('Show Message'),
              ),
            ),
            const SizedBox(height: 8),
            // Conditionally RENDERED, not just hidden — genuinely absent from
            // the widget tree until revealed, mirroring the Kotlin app's
            // visibility=gone and the SwiftUI app's `if messageVisible`.
            if (_messageVisible)
              Semantics(
                identifier: 'native_message',
                child: const Text('Message Revealed',
                    style: TextStyle(fontSize: 18)),
              ),
          ],
        ),
      ),
    );
  }
}
