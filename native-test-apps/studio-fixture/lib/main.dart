// Studio fixture app — a small, deterministic Flutter app that gives
// FlutterProbe Studio real content to open, run, record and inspect.
//
// Screens: Counter (tap), Form (type + dialog), List (scroll + perf).
// Every interactive widget carries a ValueKey so recorded flows are stable.

import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

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
      title: 'Studio Fixture',
      theme: ThemeData(colorSchemeSeed: Colors.teal, useMaterial3: true),
      home: const HomeShell(),
    );
  }
}

class HomeShell extends StatefulWidget {
  const HomeShell({super.key});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  int _tab = 0;

  static const _pages = [CounterPage(), FormPage(), ListPage()];

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Studio Fixture')),
      body: _pages[_tab],
      bottomNavigationBar: NavigationBar(
        selectedIndex: _tab,
        onDestinationSelected: (i) => setState(() => _tab = i),
        destinations: const [
          NavigationDestination(
            key: ValueKey('nav_counter'),
            icon: Icon(Icons.exposure_plus_1),
            label: 'Counter',
          ),
          NavigationDestination(
            key: ValueKey('nav_form'),
            icon: Icon(Icons.edit),
            label: 'Form',
          ),
          NavigationDestination(
            key: ValueKey('nav_list'),
            icon: Icon(Icons.list),
            label: 'List',
          ),
        ],
      ),
    );
  }
}

class CounterPage extends StatefulWidget {
  const CounterPage({super.key});

  @override
  State<CounterPage> createState() => _CounterPageState();
}

class _CounterPageState extends State<CounterPage> {
  int _count = 0;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text('Taps: $_count',
              key: const ValueKey('counter_value'),
              style: Theme.of(context).textTheme.headlineMedium),
          const SizedBox(height: 16),
          FilledButton(
            key: const ValueKey('counter_button'),
            onPressed: () => setState(() => _count++),
            child: const Text('Tap Me'),
          ),
          const SizedBox(height: 8),
          TextButton(
            key: const ValueKey('counter_reset'),
            onPressed: () => setState(() => _count = 0),
            child: const Text('Reset'),
          ),
        ],
      ),
    );
  }
}

class FormPage extends StatefulWidget {
  const FormPage({super.key});

  @override
  State<FormPage> createState() => _FormPageState();
}

class _FormPageState extends State<FormPage> {
  final _name = TextEditingController();
  String _greeting = '';

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Confirm'),
        content: Text('Greet ${_name.text}?'),
        actions: [
          TextButton(
            key: const ValueKey('dialog_cancel'),
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            key: const ValueKey('dialog_ok'),
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('OK'),
          ),
        ],
      ),
    );
    if (ok == true) {
      setState(() => _greeting = 'Hello, ${_name.text}!');
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        children: [
          TextField(
            key: const ValueKey('name_field'),
            controller: _name,
            decoration: const InputDecoration(labelText: 'Name'),
          ),
          const SizedBox(height: 16),
          FilledButton(
            key: const ValueKey('submit_button'),
            onPressed: _submit,
            child: const Text('Submit'),
          ),
          const SizedBox(height: 24),
          Text(_greeting, key: const ValueKey('greeting')),
        ],
      ),
    );
  }
}

class ListPage extends StatelessWidget {
  const ListPage({super.key});

  @override
  Widget build(BuildContext context) {
    return ListView.builder(
      key: const ValueKey('item_list'),
      itemCount: 200,
      itemBuilder: (context, i) => ListTile(
        key: ValueKey('item_$i'),
        leading: CircleAvatar(child: Text('${i % 10}')),
        title: Text('Item $i'),
        subtitle: Text('Row number ${i + 1}'),
      ),
    );
  }
}
