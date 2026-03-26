import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart' show ElevatedButton, TextButton, OutlinedButton, TextField;
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import 'finder.dart';
import 'protocol.dart';
import 'recorder.dart';
import 'sync.dart';

typedef SendFn = void Function(String message);

/// ProbeExecutor handles all JSON-RPC method calls from the CLI.
class ProbeExecutor {
  final SendFn _send;
  final ProbeFinder _finder = ProbeFinder.instance;
  final ProbeSync _sync = ProbeSync.instance;
  final ProbeRecorder _recorder = ProbeRecorder();

  // Mock registry: method+path -> {status, body}
  final Map<String, Map<String, dynamic>> _mocks = {};

  // Tracks external URL launches (populated by url_launcher interceptor)
  final List<String> _externalUrlLaunches = [];

  ProbeExecutor(this._send) {
    _interceptUrlLauncher();
  }

  /// Intercepts url_launcher platform channel to track external browser launches.
  void _interceptUrlLauncher() {
    const channel = MethodChannel('plugins.flutter.io/url_launcher');
    channel.setMethodCallHandler((MethodCall call) async {
      if (call.method == 'launch' || call.method == 'launchUrl') {
        final url = call.arguments is String
            ? call.arguments as String
            : (call.arguments is Map
                ? (call.arguments as Map)['url']?.toString() ?? ''
                : '');
        if (url.isNotEmpty) {
          _externalUrlLaunches.add(url);
        }
      }
      return true;
    });
  }

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
      case ProbeMethods.ping:
        return {'ok': true};

      case ProbeMethods.settled:
        final timeout = (req.params['timeout'] as num?)?.toDouble() ?? 10.0;
        await _sync.waitForSettled(
          timeout: Duration(milliseconds: (timeout * 1000).toInt()),
        );
        return {'ok': true};

      // ---- Navigation ----
      case ProbeMethods.open:
        final screen = req.params['screen'] as String? ?? '';
        if (screen.isEmpty) {
          // Restart the app
          await _restartApp();
        }
        await _sync.waitForSettled();
        return {'ok': true};

      // ---- Touch actions ----
      case ProbeMethods.tap:
        await _tap(req.params['selector'] as Map<String, dynamic>);
        await _sync.waitForSettled();
        return {'ok': true};

      case ProbeMethods.doubleTap:
        await _doubleTap(req.params['selector'] as Map<String, dynamic>);
        await _sync.waitForSettled();
        return {'ok': true};

      case ProbeMethods.longPress:
        await _longPress(req.params['selector'] as Map<String, dynamic>);
        await _sync.waitForSettled();
        return {'ok': true};

      // ---- Text input ----
      case ProbeMethods.type_:
        final sel = req.params['selector'] as Map<String, dynamic>;
        final text = req.params['text'] as String;
        await _typeText(sel, text);
        await _sync.waitForSettled();
        return {'ok': true};

      case ProbeMethods.clear:
        final sel = req.params['selector'] as Map<String, dynamic>;
        await _clearText(sel);
        await _sync.waitForSettled();
        return {'ok': true};

      // ---- Assertions ----
      case ProbeMethods.see:
        await _see(req.params);
        return {'ok': true};

      // ---- Wait ----
      case ProbeMethods.wait:
        await _wait(req.params);
        return {'ok': true};

      // ---- Gestures ----
      case ProbeMethods.swipe:
        final dir = req.params['direction'] as String;
        final sel = req.params['selector'] as Map<String, dynamic>?;
        await _swipe(dir, sel);
        await _sync.waitForSettled();
        return {'ok': true};

      case ProbeMethods.scroll:
        final dir = req.params['direction'] as String;
        final sel = req.params['selector'] as Map<String, dynamic>?;
        await _scroll(dir, sel);
        await _sync.waitForSettled();
        return {'ok': true};

      case ProbeMethods.drag:
        await _drag(
          req.params['from'] as Map<String, dynamic>,
          req.params['to'] as Map<String, dynamic>,
        );
        await _sync.waitForSettled();
        return {'ok': true};

      // ---- Device actions ----
      case ProbeMethods.deviceAction:
        await _deviceAction(
          req.params['action'] as String,
          req.params['value'] as String? ?? '',
        );
        await _sync.waitForSettled();
        return {'ok': true};

      case ProbeMethods.close:
        await SystemNavigator.pop();
        return {'ok': true};

      // ---- Diagnostics ----
      case ProbeMethods.screenshot:
        final name = req.params['name'] as String? ?? 'screenshot';
        final path = await _screenshot(name);
        // Include base64-encoded PNG data so CLI can save locally (essential for cloud mode
        // where the file is on a remote device and can't be pulled via ADB).
        final fileBytes = await File(path).readAsBytes();
        final b64 = base64Encode(fileBytes);
        return {'path': path, 'data': b64};

      case ProbeMethods.dumpTree:
        final tree = _dumpWidgetTree();
        return {'tree': tree};

      case ProbeMethods.saveLogs:
        return {'ok': true}; // device logs collected by CLI via adb logcat

      // ---- Dart execution ----
      case ProbeMethods.runDart:
        // Dart eval is handled by the host app via a registered callback.
        // The agent sends a notification for the app to handle.
        final code = req.params['code'] as String;
        _send(ProbeNotification(ProbeMethods.notifyExecDart, {'code': code}).encode());
        return {'ok': true, 'note': 'dart execution delegated to app'};

      // ---- Recording ----
      case ProbeMethods.startRecording:
        _recorder.start(_send);
        return {'ok': true};

      case ProbeMethods.stopRecording:
        _recorder.stop();
        return {'ok': true};

      // ---- HTTP mocking ----
      case ProbeMethods.mock:
        _registerMock(req.params);
        return {'ok': true};

      // ---- Clipboard ----
      case ProbeMethods.copyClipboard:
        final text = req.params['text'] as String? ?? '';
        await Clipboard.setData(ClipboardData(text: text));
        await _sync.waitForSettled();
        return {'ok': true};

      case ProbeMethods.pasteClipboard:
        final data = await Clipboard.getData(Clipboard.kTextPlain);
        return {'text': data?.text ?? ''};

      // ---- Browser verification ----
      case ProbeMethods.verifyBrowser:
        if (_externalUrlLaunches.isEmpty) {
          throw ProbeError(ProbeError.assertFailed, 'No external browser launch detected');
        }
        return {'ok': true, 'urls': _externalUrlLaunches};

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
    // Find the nearest EditableText in the widget tree near the selector
    final element = _requireElement(sel);
    final controller = _findTextController(element);
    if (controller != null) {
      controller.text = text;
      controller.selection = TextSelection.collapsed(offset: text.length);
    } else {
      // Fallback: tap to focus, then try to find any focused text field
      await _tap(sel);
      await Future.delayed(const Duration(milliseconds: 200));
      final focusedController = _findFocusedTextController();
      if (focusedController != null) {
        focusedController.text = text;
        focusedController.selection = TextSelection.collapsed(offset: text.length);
      } else {
        throw ProbeError(ProbeError.widgetNotFound, 'No text field found for: ${_selDesc(sel)}');
      }
    }
  }

  /// Walks up and down from an element to find a TextEditingController.
  TextEditingController? _findTextController(Element element) {
    // Search within the element's subtree for an EditableText
    TextEditingController? result;
    void visit(Element e) {
      if (result != null) return;
      if (e.widget is EditableText) {
        result = (e.widget as EditableText).controller;
        return;
      }
      if (e.widget is TextField) {
        result = (e.widget as TextField).controller;
        return;
      }
      e.visitChildren(visit);
    }

    // First search in parent chain up to find a TextField ancestor
    Element? current = element;
    for (int i = 0; i < 20 && current != null; i++) {
      if (current.widget is TextField) {
        result = (current.widget as TextField).controller;
        if (result != null) return result;
      }
      if (current.widget is EditableText) {
        return (current.widget as EditableText).controller;
      }
      // Try searching children of current
      visit(current);
      if (result != null) return result;
      // Go up
      Element? parent;
      current.visitAncestorElements((ancestor) {
        parent = ancestor;
        return false;
      });
      current = parent;
    }
    return result;
  }

  /// Finds any currently focused text controller.
  TextEditingController? _findFocusedTextController() {
    TextEditingController? result;
    _finder.walkTree((e) {
      if (result != null) return;
      if (e.widget is EditableText) {
        final editableText = e.widget as EditableText;
        if (editableText.focusNode.hasFocus) {
          result = editableText.controller;
        }
      }
    });
    return result;
  }

  Future<void> _clearText(Map<String, dynamic> sel) async {
    final element = _requireElement(sel);
    final controller = _findTextController(element);
    if (controller != null) {
      controller.clear();
    }
  }

  // ---- Assertions ----

  Future<void> _see(Map<String, dynamic> params) async {
    final sel = params['selector'] as Map<String, dynamic>;
    final negated = params['negated'] as bool? ?? false;
    final count = (params['count'] as num?)?.toInt() ?? 0;
    final check = params['check'] as String? ?? '';
    final checkVal = params['check_val'] as String? ?? '';
    final pattern = params['pattern'] as String? ?? '';

    final elements = _finder.findElements(sel);

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
      final found = _finder.findElements(sel).isNotEmpty;
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
      final view = WidgetsBinding.instance.platformDispatcher.implicitView;
      final viewSize = view != null
          ? Size(view.physicalSize.width / view.devicePixelRatio,
                 view.physicalSize.height / view.devicePixelRatio)
          : const Size(390, 844); // fallback
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
    // ignore: deprecated_member_use
    final renderView = WidgetsBinding.instance.renderView;
    final layer = renderView.layer;
    if (layer == null || layer is! OffsetLayer) {
      throw ProbeError(ProbeError.internalError, 'No renderable layer for screenshot');
    }
    final image = await layer.toImage(
      renderView.paintBounds,
      pixelRatio: 2.0,
    );
    final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
    final dir = '${Directory.systemTemp.path}/probe_screenshots';
    final path = '$dir/${name}_${DateTime.now().millisecondsSinceEpoch}.png';
    final file = File(path);
    await file.parent.create(recursive: true);
    await file.writeAsBytes(bytes!.buffer.asUint8List());
    return path;
  }

  // ---- Widget tree dump ----

  String _dumpWidgetTree() {
    final buffer = StringBuffer();
    WidgetsBinding.instance.rootElement?.visitChildren((e) {
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
    final elements = _finder.findElements(sel);
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
    WidgetsBinding.instance.rootElement?.visitChildren(visit);
    return nav;
  }

  int _nextPointer = 900; // Start high to avoid collisions with real touches

  Future<_ProbeGesture> _createGesture(Offset position) async {
    final binding = GestureBinding.instance;
    final pointerId = _nextPointer++;
    final pointer = PointerDownEvent(
      pointer: pointerId,
      position: position,
    );
    binding.handlePointerEvent(pointer);
    return _ProbeGesture(position, binding, pointerId);
  }

  Future<void> _restartApp() async {
    // Signal the app to restart
    _send(ProbeNotification(ProbeMethods.notifyRestartApp, {}).encode());
    await Future.delayed(const Duration(milliseconds: 500));
  }
}

// ---- Minimal gesture wrapper ----

class _ProbeGesture {
  Offset _position;
  final GestureBinding _binding;
  final int _pointer;

  _ProbeGesture(this._position, this._binding, this._pointer);

  Future<void> up() async {
    _binding.handlePointerEvent(PointerUpEvent(
      pointer: _pointer,
      position: _position,
    ));
  }

  Future<void> moveTo(Offset location, {Duration? timeStamp}) async {
    _binding.handlePointerEvent(PointerMoveEvent(
      pointer: _pointer,
      position: location,
    ));
    _position = location;
  }

  Future<void> moveBy(Offset delta, {Duration? timeStamp}) async {
    await moveTo(_position + delta, timeStamp: timeStamp);
  }
}
