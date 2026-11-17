import 'dart:io' show Platform;

import 'package:flutter/material.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';
import 'package:webview_flutter/webview_flutter.dart';
import 'package:webview_flutter_android/webview_flutter_android.dart';

/// A Flutter app with a WebView, to run probe's WebView steps (`see`, `tap`, `type`, `wait until ... appears`, `store text of ...` with `in webview`) through
/// `probe test`. Unlike native-test-apps/android-webview, this app is a ProbeAgent host, so a ProbeScript can mix the Dart-agent verbs (on the Flutter widgets
/// around the page) with the WebView steps (on the page).
///
/// The page is assets/page.html, a copy of native-test-apps/android-webview/assets/page.html (a test keeps the two equal), loaded from the app's own assets: no
/// network. The WebView steps reach the page through its DevTools socket, which exists only while WebView debugging is on, so it is turned on here.
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  const probeEnabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
  if (probeEnabled) {
    await ProbeAgent.start();
  }
  if (Platform.isAndroid) {
    await AndroidWebViewController.enableDebugging(true);
  }
  runApp(const WebViewFixtureApp());
}

class WebViewFixtureApp extends StatelessWidget {
  const WebViewFixtureApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Probe WebView Fixture',
      theme: ThemeData(useMaterial3: true, colorSchemeSeed: Colors.indigo),
      home: const WebViewFixtureScreen(),
    );
  }
}

class WebViewFixtureScreen extends StatefulWidget {
  const WebViewFixtureScreen({super.key});

  @override
  State<WebViewFixtureScreen> createState() => _WebViewFixtureScreenState();
}

class _WebViewFixtureScreenState extends State<WebViewFixtureScreen> {
  late final WebViewController _controller;
  int _loads = 0;
  bool _loading = true;
  String _echo = '';

  @override
  void initState() {
    super.initState();
    _controller = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setNavigationDelegate(
        NavigationDelegate(
          onPageStarted: (_) {
            if (mounted) setState(() => _loading = true);
          },
          onPageFinished: (_) {
            if (mounted) {
              setState(() {
                _loading = false;
                _loads++;
              });
            }
          },
        ),
      );
    _load();
  }

  void _load() => _controller.loadFlutterAsset('assets/page.html');

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('WebView fixture')),
      body: Column(
        children: [
          // Flutter widgets next to the page: Dart-agent steps work on these while the WebView steps work on the page.
          Padding(
            padding: const EdgeInsets.all(8),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    _loading ? 'Page loading' : 'Page loaded ($_loads)',
                    key: const ValueKey('page_status'),
                  ),
                ),
                ElevatedButton(
                  key: const ValueKey('reload_page'),
                  onPressed: _load,
                  child: const Text('Reload page'),
                ),
              ],
            ),
          ),
          // A field and its echo, the same pair as the native fixtures have: a test types what it stored from the page into the field and reads the echo,
          // which shows exactly what was stored.
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: TextField(
              key: const ValueKey('echo_input'),
              decoration: const InputDecoration(hintText: 'Type here'),
              onChanged: (v) => setState(() => _echo = v),
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(8),
            child: Align(
              alignment: Alignment.centerLeft,
              child: Text(
                _echo.isEmpty ? 'Echo: (empty)' : 'Echo: $_echo',
                key: const ValueKey('echo_text'),
              ),
            ),
          ),
          Expanded(child: WebViewWidget(controller: _controller)),
        ],
      ),
    );
  }
}
