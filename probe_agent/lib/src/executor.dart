import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart' show ElevatedButton, GestureDetector, InkResponse, TextButton, OutlinedButton;
import 'package:flutter/rendering.dart';
import 'package:flutter/scheduler.dart' show timeDilation;
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import 'agent_version.dart';
import 'biometric.dart' as biometric;
import 'finder.dart';
import 'protocol.dart';
import 'recorder.dart';
import 'signal.dart' as signal_lib;
import 'sync.dart';

typedef SendFn = void Function(String message);

/// ProbeExecutor handles all JSON-RPC method calls from the CLI.
class ProbeExecutor {
  SendFn _send;
  final ProbeFinder _finder = ProbeFinder.instance;
  final ProbeSync _sync = ProbeSync.instance;
  final ProbeRecorder _recorder = ProbeRecorder();

  // Mock registry: method+path -> {status, body}
  final Map<String, Map<String, dynamic>> _mocks = {};

  // Tracks external URL launches (populated by url_launcher interceptor)
  final List<String> _externalUrlLaunches = [];

  // Output variables set by probe.set_output and drained by probe.drain_output
  final Map<String, String> _output = {};

  ProbeExecutor(this._send) {
    _interceptUrlLauncher();
  }

  /// Updates the send function. Used by HTTP mode to route responses
  /// to the current HTTP request's completer.
  set sendFn(SendFn fn) => _send = fn;

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
        // Doubles as the connect-time version handshake: an older CLI that
        // doesn't send client_version, or an older agent build a CLI talks
        // to that doesn't recognize agent_version, both degrade gracefully
        // (missing/extra JSON fields are simply ignored on either side).
        final clientVersion = req.params['client_version'] as String?;
        if (clientVersion != null && clientVersion.isNotEmpty) {
          stdout.writeln('ProbeAgent: CLI version $clientVersion connected (agent $probeAgentVersion)');
        }
        return {'ok': true, 'agent_version': probeAgentVersion};

      case ProbeMethods.settled:
        final timeout = (req.params['timeout'] as num?)?.toDouble() ?? 10.0;
        await _sync.waitForSettled(
          timeout: Duration(milliseconds: (timeout * 1000).toInt()),
        );
        return {'ok': true};

      case ProbeMethods.setNextToken:
        final token = req.params['token'] as String? ?? '';
        if (token.length < 16) {
          throw ProbeError(ProbeError.invalidParams, 'Token must be at least 16 characters');
        }
        // Access server via global to persist the token
        await _persistNextToken(token);
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

      // ---- Open link ----
      case ProbeMethods.openLink:
        final url = req.params['url'] as String? ?? '';
        await _openLink(url);
        return {'ok': true};

      // ---- Animation control ----
      case ProbeMethods.setTimeDilation:
        final factor = (req.params['factor'] as num?)?.toDouble() ?? 1.0;
        timeDilation = factor;
        return {'ok': true};

      // ---- Output variables ----
      case ProbeMethods.setOutput:
        final key = req.params['key'] as String? ?? '';
        final value = req.params['value'] as String? ?? '';
        if (key.isNotEmpty) _output[key] = value;
        return {'ok': true};

      case ProbeMethods.drainOutput:
        final result = Map<String, String>.from(_output);
        _output.clear();
        return result;

      case ProbeMethods.biometricSignal:
        final result = (req.params['result'] as bool?) ?? false;
        biometric.completeBiometricResult(result);
        return {};

      case ProbeMethods.signal:
        final name = (req.params['name'] as String?) ?? '';
        final value = (req.params['value'] as String?) ?? 'true';
        signal_lib.deliverSignal(name, value);
        return {};

      default:
        throw ProbeError(ProbeError.methodNotFound, 'Unknown method: ${req.method}');
    }
  }

  // ---- Touch helpers ----

  Future<void> _tap(Map<String, dynamic> sel) async {
    final element = _requireElement(sel);
    final box = element.renderObject as RenderBox;
    final center = box.localToGlobal(box.size.center(Offset.zero));

    // PT-04: a real pointer tap on (or inside) a text field requests focus
    // as part of EditableText's own internal tap handling. Neither the
    // direct-tap fallback below nor a synthetic pointer tap reliably
    // reaches that internal recognizer — a Semantics wrapper or a
    // surrounding GestureDetector/InkWell (invoked directly by
    // _tryDirectTap, or hit first by the synthetic gesture) can intercept
    // the tap before it gets there — so `tap #id` on a text field could
    // report success while leaving the field genuinely unfocused. Request
    // focus on the field's real FocusNode explicitly, the way a real tap
    // would, regardless of which path below actually resolves the tap.
    final editable = _findEditableTarget(element);
    editable?.focusNode.requestFocus();

    // Check if the matched element is a Semantics wrapper — if so, the
    // synthetic gesture may not reach the GestureDetector child. In that
    // case, invoke onTap directly instead of using pointer events.
    if (element.widget is Semantics) {
      final tapped = _tryDirectTap(element);
      if (tapped) return;
    }

    final gesture = await _createGesture(center);
    await gesture.up();
  }

  /// Walks down from [element] to find a GestureDetector or InkResponse
  /// child and invokes its onTap directly. Only used when the matched
  /// element is a Semantics wrapper where synthetic pointer events are
  /// unreliable. Returns true if onTap was invoked.
  ///
  /// PT-05: checks `InkResponse` rather than only `InkWell` — `InkWell` is
  /// just a subclass of `InkResponse` with a fixed splash shape, and modern
  /// Material buttons (IconButton, ElevatedButton, etc.) commonly build an
  /// `InkResponse` directly rather than an `InkWell`, so the old `is InkWell`
  /// check missed them, always falling through to the slower synthetic-tap
  /// path below even though it also works). Buttons with neither widget
  /// findable in the subtree (or any other case this direct-tap heuristic
  /// misses) already fall through to a real hit-tested pointer tap via
  /// _createGesture, which is unaffected by Semantics-tree structure —
  /// verified this already correctly handles PT-05's literal scenario (a
  /// Semantics-wrapped button with no onTap SemanticsAction, or shadowed by
  /// an overlapping Semantics node) since Semantics doesn't participate in
  /// hit-testing at all.
  bool _tryDirectTap(Element element) {
    bool found = false;
    void visit(Element e) {
      if (found) return;
      try {
        final widget = e.widget;
        if (widget is GestureDetector && widget.onTap != null) {
          widget.onTap!();
          found = true;
          return;
        }
        if (widget is InkResponse && widget.onTap != null) {
          widget.onTap!();
          found = true;
          return;
        }
        e.visitChildren(visit);
      } catch (_) {
        // Element may be disposed during tree walk — skip safely
      }
    }
    visit(element);
    return found;
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
    final editable = _findEditableTarget(element);
    if (editable != null) {
      // PT-04: focus the field the way a real tap would before typing into
      // it — matters when `type` is used without a preceding `tap`, and
      // keeps behaviour consistent with the same fix in _tap.
      editable.focusNode.requestFocus();
      editable.controller.text = text;
      editable.controller.selection = TextSelection.collapsed(offset: text.length);
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
  TextEditingController? _findTextController(Element element) =>
      _findEditableTarget(element)?.controller;

  /// Walks up and down from an element to find the underlying EditableText's
  /// controller and FocusNode together. TextField/TextFormField don't expose
  /// their FocusNode directly (they create one internally if none is given),
  /// so unlike the old controller-only search, this always descends into the
  /// matched TextField/TextFormField to find its actual EditableText child —
  /// the one thing in the tree that genuinely owns the FocusNode a real tap
  /// would request focus on.
  _EditableTarget? _findEditableTarget(Element element) {
    _EditableTarget? result;
    void visit(Element e) {
      if (result != null) return;
      if (e.widget is EditableText) {
        final w = e.widget as EditableText;
        result = _EditableTarget(w.controller, w.focusNode);
        return;
      }
      e.visitChildren(visit);
    }

    Element? current = element;
    for (int i = 0; i < 20 && current != null; i++) {
      if (current.widget is EditableText) {
        final w = current.widget as EditableText;
        return _EditableTarget(w.controller, w.focusNode);
      }
      // TextField/TextFormField (and design-system wrappers around them)
      // always build an EditableText descendant — descend to find it.
      visit(current);
      if (result != null) return result;
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
      // PT-04 (related fix): a state check suffix (e.g. "don't see #id is
      // focused") used to be silently ignored here — this branch returned
      // as soon as *any* element existed, regardless of whether it actually
      // satisfied the checked state. "don't see X in state Y" must fail
      // only when X exists *and* is in state Y; if X exists but isn't in
      // that state, the negation is correctly satisfied.
      final matching = check.isEmpty
          ? elements
          : elements.where((e) => _stateCheckFailureReason(e, check, checkVal) == null).toList();
      if (matching.isNotEmpty) {
        final desc = check.isEmpty ? _selDesc(sel) : '${_selDesc(sel)} ($check)';
        throw ProbeError(
          ProbeError.assertFailed,
          'Expected NOT to see "$desc" but found ${matching.length} element(s)',
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
      final reason = _stateCheckFailureReason(elements.first, check, checkVal);
      if (reason != null) {
        throw ProbeError(ProbeError.assertFailed, '"${_selDesc(sel)}" $reason');
      }
    }
  }

  /// Evaluates a `see`/`don't see` state check ("enabled", "disabled",
  /// "contains", "focused") against [element]. Returns null if the state is
  /// satisfied, or a human-readable reason (for the error message, without
  /// the selector prefix) if it isn't. Shared by the positive and negated
  /// paths in [_see] so a negated state check ("don't see X is focused")
  /// evaluates the same state, instead of degrading to a bare existence
  /// check.
  String? _stateCheckFailureReason(Element element, String check, String checkVal) {
    switch (check) {
      case 'enabled':
        if (_isDisabled(element)) return 'is disabled';
        return null;
      case 'disabled':
        if (!_isDisabled(element)) return 'is enabled';
        return null;
      case 'contains':
        final text = _textOf(element);
        if (!text.contains(checkVal)) return 'contains "$text", not "$checkVal"';
        return null;
      case 'focused':
        final focused = WidgetsBinding.instance.focusManager.primaryFocus;
        if (focused == null || !element.renderObject!.attached) {
          return 'does not have focus';
        }
        final focusedRenderObject = focused.context?.findRenderObject();
        // A selector almost always matches a composite widget like
        // TextField/TextFormField, not the EditableText it builds
        // internally — the actually-focused widget is a *descendant* of
        // the matched element, not an ancestor. An ancestor-only walk could
        // never detect focus for that (extremely common) case.
        bool hasFocus = element.renderObject == focusedRenderObject;
        if (!hasFocus) {
          element.visitAncestorElements((ancestor) {
            if (ancestor.renderObject == focusedRenderObject) {
              hasFocus = true;
              return false;
            }
            return true;
          });
        }
        if (!hasFocus) {
          // Full subtree walk — covers widgets that nest EditableText
          // several levels deep (e.g. TextField -> InputDecorator -> ...
          // -> EditableText).
          void visit(Element e) {
            if (hasFocus) return;
            if (e.renderObject == focusedRenderObject) {
              hasFocus = true;
              return;
            }
            e.visitChildren(visit);
          }
          element.visitChildren(visit);
        }
        return hasFocus ? null : 'does not have focus';
      default:
        return null;
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

      case 'animations':
        await _waitForAnimations(timeoutDur);

      case 'page_load':
      case 'network_idle':
      case 'settled':
        await _sync.waitForSettled(timeout: timeoutDur);
    }
  }

  Future<void> _waitForAnimations(Duration timeout) async {
    final deadline = DateTime.now().add(timeout);
    while (DateTime.now().isBefore(deadline)) {
      final hasFrame = WidgetsBinding.instance.hasScheduledFrame;
      if (!hasFrame) return;
      await Future.delayed(const Duration(milliseconds: 50));
    }
    throw ProbeError(ProbeError.timeout, 'Timed out waiting for animations to finish');
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

    // PT-03: a single giant PointerMoveEvent covering the whole delta in one
    // jump can fail to register as a scroll at all — reproduced against a
    // real iOS simulator (Settings screen: 8 single-jump `scroll down` calls
    // produced zero movement; the same gesture split into incremental steps,
    // matching how a real touch/drag is delivered, scrolled correctly).
    // Likely cause: gesture-arena resolution (and scroll physics, which
    // apply delta per pointer-move event) expect a sequence of small moves
    // building up displacement, not one large jump.
    final gesture = await _createGesture(center);
    const steps = 10;
    for (var i = 1; i <= steps; i++) {
      await gesture.moveBy(delta / steps.toDouble(),
          timeStamp: Duration(milliseconds: (300 * i / steps).round()));
      await Future.delayed(const Duration(milliseconds: 8));
    }
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
    // Wait for the latest frame to be fully rendered before capturing.
    await WidgetsBinding.instance.endOfFrame;

    // Primary path: RenderRepaintBoundary.toImage() — works on both Skia and
    // Impeller. OffsetLayer.toImage() returns a GPU-backed texture on Impeller
    // where toByteData(png) returns null, so we can't rely on it for iOS.
    final pngBytes = await _captureViaRepaintBoundary() ?? await _captureViaLayer();
    if (pngBytes == null) {
      throw ProbeError(ProbeError.internalError, 'Screenshot capture failed: no renderable surface');
    }

    final dir = '${Directory.systemTemp.path}/probe_screenshots';
    final path = '$dir/${name}_${DateTime.now().millisecondsSinceEpoch}.png';
    final file = File(path);
    await file.parent.create(recursive: true);
    await file.writeAsBytes(pngBytes);
    return path;
  }

  /// Finds the largest [RenderRepaintBoundary] in the widget tree and captures
  /// it. Impeller explicitly supports this path, unlike [OffsetLayer.toImage].
  Future<Uint8List?> _captureViaRepaintBoundary() async {
    RenderRepaintBoundary? best;
    double bestArea = 0;

    void visit(Element element) {
      final ro = element.renderObject;
      if (ro is RenderRepaintBoundary) {
        final area = ro.size.width * ro.size.height;
        if (area > bestArea && ro.size.width > 50) {
          bestArea = area;
          best = ro;
        }
      }
      element.visitChildren(visit);
    }

    WidgetsBinding.instance.rootElement?.visitChildren(visit);
    if (best == null) return null;

    final views = RendererBinding.instance.renderViews;
    final pixelRatio = views.isNotEmpty
        ? views.first.flutterView.devicePixelRatio
        : ui.PlatformDispatcher.instance.views.first.devicePixelRatio;
    final image = await best!.toImage(pixelRatio: pixelRatio);
    final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
    return bytes?.buffer.asUint8List();
  }

  /// Fallback capture using the root [OffsetLayer]. Works on Skia; may return
  /// null on Impeller if the GPU texture can't be read back to CPU memory.
  Future<Uint8List?> _captureViaLayer() async {
    // ignore: deprecated_member_use
    final renderView = WidgetsBinding.instance.renderView;
    // ignore: invalid_use_of_protected_member
    final layer = renderView.layer;
    if (layer == null || layer is! OffsetLayer) return null;
    final image = await layer.toImage(renderView.paintBounds, pixelRatio: 2.0);
    final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
    return bytes?.buffer.asUint8List();
  }

  // ---- Open link ----

  Future<void> _openLink(String url) async {
    const channel = MethodChannel('plugins.flutter.io/url_launcher');
    try {
      await channel.invokeMethod<bool>('launch', {
        'url': url,
        'useSafariVC': false,
        'useWebView': false,
        'enableJavaScript': false,
        'enableDomStorage': false,
        'universalLinksOnly': false,
        'headers': <String, String>{},
      });
    } catch (_) {
      // Fallback: record the launch intent so verify_browser can confirm it
      _externalUrlLaunches.add(url);
    }
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

  /// Persists a token to disk so the agent uses it after restart.
  Future<void> _persistNextToken(String token) async {
    try {
      String path;
      if (Platform.isIOS) {
        path = '${Directory.systemTemp.path}/probe/next_token';
      } else if (Platform.isAndroid) {
        final cmdline = File('/proc/self/cmdline').readAsStringSync();
        final pkg = cmdline.split('\x00').first;
        path = '/data/data/$pkg/cache/probe/next_token';
      } else {
        path = '${Directory.systemTemp.path}/probe/next_token';
      }
      final file = File(path);
      await file.parent.create(recursive: true);
      await file.writeAsString(token);
    } catch (e) {
      throw ProbeError(ProbeError.internalError, 'Failed to persist token: $e');
    }
  }
}

// ---- Editable text resolution (PT-04) ----

/// The controller and real FocusNode of an EditableText resolved near a
/// selector — see [ProbeExecutor._findEditableTarget].
class _EditableTarget {
  const _EditableTarget(this.controller, this.focusNode);
  final TextEditingController controller;
  final FocusNode focusNode;
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
