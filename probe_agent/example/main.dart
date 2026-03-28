import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Only start the agent when built with --dart-define=PROBE_AGENT=true
  const probeEnabled =
      bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  if (probeEnabled) {
    await ProbeAgent.start();
  }

  runApp(const MyApp());
}

class MyApp extends StatelessWidget {
  const MyApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      home: Scaffold(
        appBar: AppBar(title: const Text('FlutterProbe Example')),
        body: const Center(child: Text('Hello, FlutterProbe!')),
      ),
    );
  }
}
