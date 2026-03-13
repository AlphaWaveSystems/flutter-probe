import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/gestures.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart'
    show find, TestGesture, WidgetController;

import 'finder.dart';
import 'protocol.dart';
import 'sync.dart';

typedef SendFn = void Function(String message);

/// ProbeExecutor handles all JSON-RPC method calls from the CLI.
class ProbeExecutor {
  final SendFn _send;
  final ProbeFinder _finder = ProbeFinder.instance;
  final ProbeSync _sync = ProbeSync.instance;

  // Mock registry: method+path -> {status, body}
  final Map<String, Map<String, dynamic>> _mocks = {};

  ProbeExecutor(this._send);

  /// Dispatch a JSON-RPC request and respond via [_send].
  Future<void> dispatch(ProbeRequest req) async {
    try {
      final result = await _handle(req);
      _send(ProbeResponse.ok(req.id, result).encode());
    } on ProbeError catch (e) {
      _send(ProbeResponse.err(req.id, e).encode());
    } catch (e, st) {
      _send(ProbeResponse.err(
        req.id,
        ProbeError(ProbeError.internalError, '$e\n$st'),
      ).encode());
    }
  }

  Future<dynamic> _handle(ProbeRequest req) async {
    switch (req.method) {
      // ---- Lifecycle ----
      case 'probe.ping':
        return {'ok': true};

      case 'probe.settled':
        final timeout = (req.params['timeout'] as num?)?.toDouble() ?? 10.0;
        await _sync.waitForSettled(
          timeout: Duration(milliseconds: (timeout * 1000).toInt()),
        );
        return {'ok': true};

      // ---- Navigation ----
      case 'probe.open':
        final screen = req.params['screen'] as String? ?? '';
        if (screen.isEmpty) {
          // Restart the app
          await _restartApp();
        }
        await _sync.waitForSettled();
        return {'ok': true};

      // ---- Touch actions ----
      case 'probe.tap':
        await _tap(req.params['selector'] as Map<String, dynamic>);
        await _sync.waitForSettled();
        return {'ok': true};

      case 'probe.double_tap':
        await _doubleTap(req.params['selector'] as Map<String, dynamic>);
        await _sync.waitForSettled();
        return {'ok': true};

      case 'probe.long_press':
        await _longPress(req.params['selector'] as Map<String, dynamic>);
        await _sync.waitForSettled();
        return {'ok': true};

      // ---- Text input ----
      case 'probe.type':
        final sel = req.params['selector'] as Map<String, dynamic>;
        final text = req.params['text'] as String;
        await _typeText(sel, text);
        await _sync.waitForSettled();
        return {'ok': true};

      case 'probe.clear':
        final sel = req.params['selector'] as Map<String, dynamic>;
        await _clearText(sel);
        await _sync.waitForSettled();
        return {'ok': true};

      // ---- Assertions ----
      case 'probe.see':
        await _see(req.params);
        return {'ok': true};

      // ---- Wait ----
      case 'probe.wait':
        await _wait(req.params);
        return {'ok': true};

      // ---- Gestures ----
      case 'probe.swipe':
        final dir = req.params['direction'] as String;
        final sel = req.params['selector'] as Map<String, dynamic>?;
        await _swipe(dir, sel);
        await _sync.waitForSettled();
        return {'ok': true};

      case 'probe.scroll':
        final dir = req.params['direction'] as String;
        final sel = req.params['selector'] as Map<String, dynamic>?;
        await _scroll(dir, sel);
        await _sync.waitForSettled();
        return {'ok': true};

      case 'probe.drag':
        await _drag(
          req.params['from'] as Map<String, dynamic>,
          req.params['to'] as Map<String, dynamic>,
        );
        await _sync.waitForSettled();
        return {'ok': true};

      // ---- Device actions ----
      case 'probe.device_action':
        await _deviceAction(
          req.params['action'] as String,
          req.params['value'] as String? ?? '',
        );
        await _sync.waitForSettled();
        return {'ok': true};

      case 'probe.close':
        await SystemNavigator.pop();
        return {'ok': true};

      // ---- Diagnostics ----
      case 'probe.screenshot':
        final name = req.params['name'] as String? ?? 'screenshot';
        final path = await _screenshot(name);
        return {'path': path};

      case 'probe.dump_tree':
        final tree = _dumpWidgetTree();
        return {'tree': tree};

      case 'probe.save_logs':
        return {'ok': true}; // device logs collected by CLI via adb logcat

      // ---- Dart execution ----
      case 'probe.run_dart':
        // Dart eval is handled by the host app via a registered callback.
        // The agent sends a notification for the app to handle.
        final code = req.params['code'] as String;
        _send(ProbeNotification('probe.exec_dart', {'code': code}).encode());
        return {'ok': true, 'note': 'dart execution delegated to app'};

      // ---- HTTP mocking ----
      case 'probe.mock':
        _registerMock(req.params);
        return {'ok': true};

      default:
        throw ProbeError(ProbeError.methodNotFound, 'Unknown method: ${req.method}');
    }
  }

  // ---- Touch helpers ----

  Future<void> _tap(Map<String, dynamic> sel) async {
    final element = _requireElement(sel);
    final box = element.renderObject as RenderBox;
    final center = box.localToGlobal(box.size.center(Offset.zero));
    final gesture = await _createGesture(center);
    await gesture.up();
  }

  Future<void> _doubleTap(Map<String, dynamic> sel) async {
    final element = _requireElement(sel);
    final box = element.renderObject as RenderBox;
    final center = box.localToGlobal(box.size.center(Offset.zero));
    final g1 = await _createGesture(center);
    await g1.up();
    await Future.delayed(const Duration(milliseconds: 50));
    final g2 = await _createGesture(center);
    await g2.up();
  }

  Future<void> _longPress(Map<String, dynamic> sel) async {
    final element = _requireElement(sel);
    final box = element.renderObject as RenderBox;
    final center = box.localToGlobal(box.size.center(Offset.zero));
    final gesture = await _createGesture(center);
    await Future.delayed(const Duration(milliseconds: 500));
    await gesture.up();
  }

  // ---- Text input helpers ----

  Future<void> _typeText(Map<String, dynamic> sel, String text) async {
    // Tap the field first to focus it
    await _tap(sel);
    await Future.delayed(const Duration(milliseconds: 50));
    // Inject text via platform channel
    await _injectText(text);
  }

  Future<void> _injectText(String text) async {
    final encoded = jsonEncode({'text': text});
    await ServicesBinding.instance.defaultBinaryMessenger.handlePlatformMessage(
      SystemChannels.textInput.name,
      SystemChannels.textInput.codec.encodeMethodCall(
        MethodCall('TextInputClient.updateEditingState', <dynamic>[
          -1,
          TextEditingValue(text: text, selection: TextSelection.collapsed(offset: text.length))
              .toJSON(),
        ]),
      ),
      (_) {},
    );
    _ = encoded; // suppress unused warning
  }

  Future<void> _clearText(Map<String, dynamic> sel) async {
    await _tap(sel);
    await Future.delayed(const Duration(milliseconds: 50));
    // Select all + delete
    await _injectText('');
  }

  // ---- Assertions ----

  Future<void> _see(Map<String, dynamic> params) async {
    final sel = params['selector'] as Map<String, dynamic>;
    final negated = params['negated'] as bool? ?? false;
    final count = (params['count'] as num?)?.toInt() ?? 0;
    final check = params['check'] as String? ?? '';
    final checkVal = params['check_val'] as String? ?? '';
    final pattern = params['pattern'] as String? ?? '';

    final finder = _finder.forSelector(sel);
    final elements = finder.evaluate().toList();

    if (negated) {
      if (elements.isNotEmpty) {
        throw ProbeError(
          ProbeError.assertFailed,
          'Expected NOT to see "${_selDesc(sel)}" but found ${elements.length} element(s)',
        );
      }
      return;
    }

    if (count > 0) {
      if (elements.length != count) {
        throw ProbeError(
          ProbeError.assertFailed,
          'Expected exactly $count "${_selDesc(sel)}" but found ${elements.length}',
        );
      }
      return;
    }

    if (elements.isEmpty) {
      if (pattern.isNotEmpty) {
        throw ProbeError(
          ProbeError.assertFailed,
          'No element matching pattern "$pattern"',
        );
      }
      throw ProbeError(
        ProbeError.assertFailed,
        'Expected to see "${_selDesc(sel)}" but it was not found',
      );
    }

    // State checks
    if (check.isNotEmpty && elements.isNotEmpty) {
      final element = elements.first;
      switch (check) {
        case 'enabled':
          if (_isDisabled(element)) {
            throw ProbeError(ProbeError.assertFailed, '"${_selDesc(sel)}" is disabled');
          }
        case 'disabled':
          if (!_isDisabled(element)) {
            throw ProbeError(ProbeError.assertFailed, '"${_selDesc(sel)}" is enabled');
          }
        case 'contains':
          final text = _textOf(element);
          if (!text.contains(checkVal)) {
            throw ProbeError(
              ProbeError.assertFailed,
              '"${_selDesc(sel)}" contains "$text", not "$checkVal"',
            );
          }
      }
    }
  }

  // ---- Wait helpers ----

  Future<void> _wait(Map<String, dynamic> params) async {
    final kind = params['kind'] as String? ?? 'settled';
    final target = params['target'] as String? ?? '';
    final duration = (params['duration'] as num?)?.toDouble() ?? 0;
    final timeout = (params['timeout'] as num?)?.toDouble() ?? 30.0;
    final timeoutDur = Duration(milliseconds: (timeout * 1000).toInt());

    switch (kind) {
      case 'duration':
        await Future.delayed(Duration(milliseconds: (duration * 1000).toInt()));

      case 'appears':
        await _waitUntilVisible(target, timeoutDur, expect: true);

      case 'disappears':
        await _waitUntilVisible(target, timeoutDur, expect: false);

      case 'page_load':
      case 'network_idle':
      case 'settled':
        await _sync.waitForSettled(timeout: timeoutDur);
    }
  }

  Future<void> _waitUntilVisible(String text, Duration timeout, {required bool expect}) async {
    final deadline = DateTime.now().add(timeout);
    while (DateTime.now().isBefore(deadline)) {
      final sel = {'kind': 'text', 'text': text};
      final found = _finder.forSelector(sel).evaluate().isNotEmpty;
      if (found == expect) return;
      await Future.delayed(const Duration(milliseconds: 100));
      await _sync.waitForSettled(timeout: const Duration(seconds: 1));
    }
    final desc = expect ? 'appear' : 'disappear';
    throw ProbeError(
      ProbeError.timeout,
      'Timed out waiting for "$text" to $desc',
    );
  }

  // ---- Gesture helpers ----

  Future<void> _swipe(String direction, Map<String, dynamic>? sel) async {
    Offset center;
    Size size;

    if (sel != null) {
      final element = _requireElement(sel);
      final box = element.renderObject as RenderBox;
      center = box.localToGlobal(box.size.center(Offset.zero));
      size = box.size;
    } else {
      final binding = WidgetsBinding.instance;
      final viewSize = binding.renderView.size;
      center = Offset(viewSize.width / 2, viewSize.height / 2);
      size = viewSize;
    }

    Offset delta;
    switch (direction) {
      case 'up':
        delta = Offset(0, -size.height * 0.5);
      case 'down':
        delta = Offset(0, size.height * 0.5);
      case 'left':
        delta = Offset(-size.width * 0.5, 0);
      case 'right':
      default:
        delta = Offset(size.width * 0.5, 0);
    }

    final gesture = await _createGesture(center);
    await gesture.moveBy(delta, timeStamp: const Duration(milliseconds: 300));
    await gesture.up();
  }

  Future<void> _scroll(String direction, Map<String, dynamic>? sel) async {
    await _swipe(direction, sel);
  }

  Future<void> _drag(
    Map<String, dynamic> fromSel,
    Map<String, dynamic> toSel,
  ) async {
    final fromEl = _requireElement(fromSel);
    final toEl = _requireElement(toSel);
    final fromBox = fromEl.renderObject as RenderBox;
    final toBox = toEl.renderObject as RenderBox;
    final from = fromBox.localToGlobal(fromBox.size.center(Offset.zero));
    final to = toBox.localToGlobal(toBox.size.center(Offset.zero));

    final gesture = await _createGesture(from);
    await gesture.moveTo(to, timeStamp: const Duration(milliseconds: 500));
    await gesture.up();
  }

  // ---- Device actions ----

  Future<void> _deviceAction(String action, String value) async {
    switch (action) {
      case 'go_back':
        final nav = _navigator;
        if (nav != null && nav.canPop()) {
          nav.pop();
        } else {
          await SystemNavigator.pop();
        }
      case 'rotate':
        // Rotation handled at system level; notify app
        break;
      case 'toggle':
        // Dark mode toggle etc.
        break;
      case 'shake':
        break;
    }
  }

  // ---- Screenshot ----

  Future<String> _screenshot(String name) async {
    final renderView = WidgetsBinding.instance.renderView;
    final image = await renderView.layer!.toImage(
      renderView.paintBounds,
      pixelRatio: 2.0,
    );
    final bytes = await image.toByteData(format: ImageByteFormat.png);
    final path = '/sdcard/probe_screenshots/${name}_${DateTime.now().millisecondsSinceEpoch}.png';
    final file = File(path);
    await file.parent.create(recursive: true);
    await file.writeAsBytes(bytes!.buffer.asUint8List());
    return path;
  }

  // ---- Widget tree dump ----

  String _dumpWidgetTree() {
    final buffer = StringBuffer();
    WidgetsBinding.instance.renderViewElement?.visitChildren((e) {
      _dumpElement(e, buffer, 0);
    });
    return buffer.toString();
  }

  void _dumpElement(Element e, StringBuffer buf, int depth) {
    buf.writeln('${'  ' * depth}${e.widget.runtimeType}(key=${e.widget.key})');
    e.visitChildren((child) => _dumpElement(child, buf, depth + 1));
  }

  // ---- Mock registration ----

  void _registerMock(Map<String, dynamic> params) {
    final method = (params['method'] as String).toUpperCase();
    final path = params['path'] as String;
    _mocks['$method:$path'] = {
      'status': (params['status'] as num?)?.toInt() ?? 200,
      'body': params['body'] as String? ?? '',
    };
  }

  /// Returns a mock response if one is registered, else null.
  Map<String, dynamic>? mockFor(String method, String path) =>
      _mocks['${method.toUpperCase()}:$path'];

  // ---- Internal helpers ----

  Element _requireElement(Map<String, dynamic> sel) {
    final finder = _finder.forSelector(sel);
    final elements = finder.evaluate().toList();
    if (elements.isEmpty) {
      throw ProbeError(
        ProbeError.widgetNotFound,
        'Widget not found: ${_selDesc(sel)}',
      );
    }
    return elements.first;
  }

  String _selDesc(Map<String, dynamic> sel) {
    final kind = sel['kind'] ?? 'text';
    final text = sel['text'] ?? '';
    return '$kind("$text")';
  }

  bool _isDisabled(Element e) {
    final widget = e.widget;
    if (widget is ElevatedButton) return widget.onPressed == null;
    if (widget is TextButton) return widget.onPressed == null;
    if (widget is OutlinedButton) return widget.onPressed == null;
    if (widget is GestureDetector) return widget.onTap == null;
    return false;
  }

  String _textOf(Element e) {
    final widget = e.widget;
    if (widget is Text) return widget.data ?? '';
    if (widget is RichText) return widget.text.toPlainText();
    return '';
  }

  NavigatorState? get _navigator {
    NavigatorState? nav;
    void visit(Element e) {
      if (e.widget is Navigator) {
        nav = (e as StatefulElement).state as NavigatorState;
        return;
      }
      e.visitChildren(visit);
    }
    WidgetsBinding.instance.renderViewElement?.visitChildren(visit);
    return nav;
  }

  Future<TestGesture> _createGesture(Offset position) async {
    // TestGesture requires a WidgetController which isn't available outside
    // of the test framework. We fall back to platform gesture simulation.
    final binding = GestureBinding.instance;
    final pointer = PointerDownEvent(position: position);
    binding.handlePointerEvent(pointer);
    // Return a thin wrapper
    return _FakeTestGesture(position, binding);
  }

  Future<void> _restartApp() async {
    // Signal the app to restart
    _send(ProbeNotification('probe.restart_app', {}).encode());
    await Future.delayed(const Duration(milliseconds: 500));
  }
}

// ---- Minimal TestGesture stand-in ----

class _FakeTestGesture implements TestGesture {
  Offset _position;
  final GestureBinding _binding;

  _FakeTestGesture(this._position, this._binding);

  @override
  Future<void> up({Duration? timeStamp}) async {
    _binding.handlePointerEvent(PointerUpEvent(position: _position));
  }

  @override
  Future<void> moveTo(Offset location, {Duration? timeStamp}) async {
    _binding.handlePointerEvent(PointerMoveEvent(position: location));
    _position = location;
  }

  @override
  Future<void> moveBy(Offset delta, {Duration? timeStamp}) async {
    await moveTo(_position + delta, timeStamp: timeStamp);
  }

  // ---- Unused stubs from TestGesture interface ----
  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}
