import 'dart:async';
import 'dart:ffi';
import 'dart:isolate';

import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

import 'probe_advertiser.dart';

/// Flutter twin of the native fixture apps (native-test-apps/android and
/// native-test-apps/ios): the identical 8-element surface with the identical
/// identifiers and visible strings, so the same test scenarios run in three
/// frameworks — Flutter via probe's normal verbs (both OSes), Kotlin/Android
/// via the `native` verbs, SwiftUI/iOS via the future N-2 WDA bridging.
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  const probeEnabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  // PROBE_WIFI=true makes the agent listen on the network (a physical phone); the advertiser then lets
  // Studio find it. Without it the agent listens on localhost only and nothing is advertised.
  const wifi = bool.fromEnvironment('PROBE_WIFI', defaultValue: false);
  if (probeEnabled) {
    await ProbeAgent.start(advertiser: wifi ? BonsoirAdvertiser() : null);
  }
  runApp(const FixtureApp());
}

class FixtureApp extends StatelessWidget {
  const FixtureApp({super.key});

  @override
  Widget build(BuildContext context) {
    // PROBE_FIXTURE_LIST=true builds the list screen instead of the default surface (used by probe-tests/list/). The default
    // surface stays the same 8-element surface as the Android and iOS fixtures.
    const listScreen = bool.fromEnvironment('PROBE_FIXTURE_LIST', defaultValue: false);
    // PROBE_FIXTURE_CRASH=true builds the crash screen (used by probe-tests/crash/): one button that aborts the process.
    const crashScreen = bool.fromEnvironment('PROBE_FIXTURE_CRASH', defaultValue: false);
    // PROBE_FIXTURE_LOAD=true builds the load screen (used by probe-tests/load/): sustained background work, the kind a real
    // app's SDKs do (PT-25). See FixtureLoadScreen.
    const loadScreen = bool.fromEnvironment('PROBE_FIXTURE_LOAD', defaultValue: false);
    return MaterialApp(
      title: 'Probe Fixture (Flutter)',
      home: listScreen
          ? const FixtureListScreen()
          : crashScreen
              ? const FixtureCrashScreen()
              : loadScreen
                  ? const FixtureLoadScreen()
                  : const FixtureScreen(),
    );
  }
}

/// Burns CPU for about [budget] and churns the allocator, so the garbage collector runs too. Returns a value so the work is not
/// optimised away.
int _burnSlice(Duration budget) {
  final sw = Stopwatch()..start();
  var x = 1;
  while (sw.elapsed < budget) {
    x = (x * 1103515245 + 12345) & 0x7fffffff;
    if (x % 64 == 0) {
      final junk = List<int>.filled(4096, x);
      x ^= junk.length;
    }
  }
  return x;
}

/// The entry point of the background isolate in PROBE_FIXTURE_LOAD_MODE=isolate.
void _burnForever(Object? _) {
  while (true) {
    _burnSlice(const Duration(milliseconds: 40));
  }
}

/// The load screen for the sustained-load reproduction (probe-tests/load/): one text, "Load running", and background work that
/// never stops while the screen is up. PROBE_FIXTURE_LOAD_MODE=ui (the default) burns about 40 ms of every 50 ms on the UI
/// isolate, which is also where the agent's socket is served; PROBE_FIXTURE_LOAD_MODE=isolate burns it on a second isolate and
/// leaves the UI isolate free. Key: load_ticks (the tick count, which moves while the load runs).
class FixtureLoadScreen extends StatefulWidget {
  const FixtureLoadScreen({super.key});

  @override
  State<FixtureLoadScreen> createState() => _FixtureLoadScreenState();
}

class _FixtureLoadScreenState extends State<FixtureLoadScreen> {
  static const _mode = String.fromEnvironment('PROBE_FIXTURE_LOAD_MODE', defaultValue: 'ui');
  Timer? _timer;
  Isolate? _worker;
  int _ticks = 0;

  @override
  void initState() {
    super.initState();
    if (_mode == 'isolate') {
      Isolate.spawn(_burnForever, null).then((isolate) => _worker = isolate);
    } else {
      _timer = Timer.periodic(const Duration(milliseconds: 50), (_) {
        _burnSlice(const Duration(milliseconds: 40));
        setState(() => _ticks++);
      });
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    _worker?.kill(priority: Isolate.immediate);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Load running')),
      body: Center(child: Text('Ticks: $_ticks', key: const ValueKey('load_ticks'))),
    );
  }
}

/// The crash screen for the crash checks: one button, fixture_crash, that aborts the process from native code, the way a crash
/// in a plugin ends the app. The step after the tap loses its connection to the agent, so a run with --crash-evidence keeps why.
class FixtureCrashScreen extends StatelessWidget {
  const FixtureCrashScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Crash fixture')),
      body: Center(
        child: Semantics(
          identifier: 'fixture_crash',
          child: ElevatedButton(
            onPressed: _abortProcess,
            child: const Text('Crash the app'),
          ),
        ),
      ),
    );
  }
}

// abort is the C library's abort(), looked up in the process itself (no plugin): it ends the process with SIGABRT.
final _abort = DynamicLibrary.process().lookupFunction<Void Function(), void Function()>('abort');

void _abortProcess() => _abort();

/// The list screen for the scroll checks: a full-height ListView with no other scrollable around it, and its offset in the app
/// bar. Keys: fixture_list (the list) and fixture_row_N (its rows).
class FixtureListScreen extends StatefulWidget {
  const FixtureListScreen({super.key});

  @override
  State<FixtureListScreen> createState() => _FixtureListScreenState();
}

class _FixtureListScreenState extends State<FixtureListScreen> {
  final _controller = ScrollController();
  double _offset = 0;

  @override
  void initState() {
    super.initState();
    _controller.addListener(() => setState(() => _offset = _controller.offset));
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text('Offset ${_offset.toStringAsFixed(1)}')),
      body: ListView.builder(
        key: const ValueKey('fixture_list'),
        controller: _controller,
        itemCount: 200,
        itemBuilder: (context, i) => SizedBox(
          height: 56,
          child: ListTile(key: ValueKey('fixture_row_$i'), title: Text('Row $i')),
        ),
      ),
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
