import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/gestures.dart';
import 'package:flutter/cupertino.dart' show CupertinoPageScaffold;
import 'package:flutter/material.dart' show BottomSheet, Dialog, ElevatedButton, GestureDetector, InkResponse, Scaffold, TextButton, OutlinedButton;
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
        final tapWarning = _tapWarning;
        _tapWarning = null;
        return tapWarning == null ? {'ok': true} : {'ok': true, 'warning': tapWarning};

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
        _lastScrollAt = DateTime.now();
        return {'ok': true};

      case ProbeMethods.scroll:
        final dir = req.params['direction'] as String;
        final sel = req.params['selector'] as Map<String, dynamic>?;
        final until = req.params['until'] as Map<String, dynamic>?;
        if (until != null) {
          await _scrollUntil(dir, sel, until);
        } else {
          await _scroll(dir, sel);
        }
        await _sync.waitForSettled();
        _lastScrollAt = DateTime.now();
        return {'ok': true};

      case ProbeMethods.drag:
        await _drag(
          req.params['from'] as Map<String, dynamic>,
          req.params['to'] as Map<String, dynamic>,
        );
        await _sync.waitForSettled();
        _lastScrollAt = DateTime.now();
        return {'ok': true};

      // ---- Device actions ----
      case ProbeMethods.deviceAction:
        _tapWarning = null;
        await _deviceAction(
          req.params['action'] as String,
          req.params['value'] as String? ?? '',
        );
        await _sync.waitForSettled();
        final actionWarning = _tapWarning;
        _tapWarning = null;
        return actionWarning == null ? {'ok': true} : {'ok': true, 'warning': actionWarning};

      case ProbeMethods.close:
        await SystemNavigator.pop();
        return {'ok': true};

      // ---- Diagnostics ----
      case ProbeMethods.screenshot:
        final name = req.params['name'] as String? ?? 'screenshot';
        // PT-16: every other verb calls this before acting/capturing —
        // screenshot didn't, so a capture taken right after navigation
        // could land mid-route-transition (the push/pop AnimationController
        // still ticking) instead of waiting for the new route to settle.
        await _sync.waitForSettled();
        final path = await _screenshot(name);
        // Include base64-encoded PNG data so CLI can save locally (essential for cloud mode
        // where the file is on a remote device and can't be pulled via ADB).
        final fileBytes = await File(path).readAsBytes();
        final b64 = base64Encode(fileBytes);
        return {'path': path, 'data': b64};

      case ProbeMethods.dumpTree:
        final tree = _dumpWidgetTree();
        return {'tree': tree};

      case ProbeMethods.visibleSummary:
        return _finder.visibleSummary();

      case ProbeMethods.selectorBounds:
        final sel = req.params['selector'] as Map<String, dynamic>;
        final bounds = _finder.boundsFor(sel);
        if (bounds == null) {
          throw ProbeError(
            ProbeError.widgetNotFound,
            'Widget not found: ${_selDesc(sel)}',
          );
        }
        return bounds;

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

  /// Set by [_tap] when it had to proceed with a tap that probably will not do
  /// what the test expects; returned to the CLI as a warning instead of the tap
  /// silently doing nothing (FP-19).
  String? _tapWarning;

  /// When the last scroll/swipe/drag finished. A tap shortly after one waits for
  /// its target to become hit-testable (see [_waitUntilHittableAfterScroll]).
  DateTime _lastScrollAt = DateTime.fromMillisecondsSinceEpoch(0);

  /// FP-19 (reported from an Android gate): a tap issued right after
  /// `scroll ... until ... appears` was silently lost, although the target was
  /// fully visible afterwards and a 3-second wait avoided it. At the moment of
  /// the tap a hit test at the target's center did not reach it, so the
  /// synthetic pointer event went nowhere. Whatever transiently blocks hits
  /// after a scroll (overscroll/ballistic activity, an IgnorePointer, an
  /// animating overlay), waiting until the target is hit-testable again is the
  /// reliable fix, so this polls — but only within [window] of the last scroll,
  /// and only until the target is reachable. Costs nothing when no scroll just
  /// happened or the target is already reachable.
  Future<void> _waitUntilHittableAfterScroll(Element element,
      {Duration window = const Duration(seconds: 2)}) async {
    final deadline = _lastScrollAt.add(window);
    while (DateTime.now().isBefore(deadline)) {
      final box = element.renderObject;
      if (box is! RenderBox || !box.attached || !box.hasSize) return;
      if (_hitState(box, box.localToGlobal(box.size.center(Offset.zero))).strict) return;
      await Future.delayed(const Duration(milliseconds: 50));
    }
  }

  /// True while any scrollable that contains [element] is scrolling. Flutter's
  /// Scrollable ignores pointer events for the duration of a scroll activity, so
  /// a tap issued right after a scroll can be dropped without any error.
  bool _scrollingAround(Element element) {
    var scrolling = false;
    try {
      element.visitAncestorElements((a) {
        if (a is StatefulElement && a.state is ScrollableState) {
          final position = (a.state as ScrollableState).position;
          if (position.isScrollingNotifier.value) {
            scrolling = true;
            return false;
          }
        }
        return true;
      });
    } catch (_) {
      return false;
    }
    return scrolling;
  }

  /// Lets a scroll that is still in progress around [element] finish (bounded).
  /// Costs nothing — not even a frame — when nothing is scrolling.
  Future<void> _waitForScrollIdle(Element element, {Duration timeout = const Duration(milliseconds: 1500)}) async {
    if (!_scrollingAround(element)) return;
    final deadline = DateTime.now().add(timeout);
    while (DateTime.now().isBefore(deadline) && _scrollingAround(element)) {
      await Future.delayed(const Duration(milliseconds: 50));
    }
  }

  /// When several widgets match (an exiting SnackBar or route keeps its
  /// widgets in the tree for a moment), prefer the first one a pointer can
  /// actually reach, instead of whichever comes first in tree order.
  Element _preferReachable(Element first, Map<String, dynamic> sel) {
    try {
      final all = _finder.findElements(sel);
      if (all.length < 2) return first;
      Element? related;
      for (final e in all) {
        final b = e.renderObject;
        if (b is! RenderBox || !b.attached || !b.hasSize) continue;
        final state = _hitState(b, b.localToGlobal(b.size.center(Offset.zero)));
        if (state.strict) return e;
        // A text label is often wider than its glyphs, so the hit at its
        // center lands on the enclosing button instead of the paragraph:
        // that still means this copy is the reachable one.
        if (state.related && related == null) related = e;
      }
      if (related != null) return related;
    } catch (_) {}
    return first;
  }

  Future<void> _tap(Map<String, dynamic> sel) async {
    _tapWarning = null;
    // FP-13: a tap issued right after a dialog/bottom sheet closes lands on
    // its modal barrier while the exit animation is still running and is
    // swallowed. Let any in-flight route transition finish first (bounded —
    // never blocks a tap for more than a couple of seconds).
    await _waitForRouteTransitions();
    final element = _preferReachable(_requireElement(sel), sel);
    // FP-19: same idea for a scroll that has not finished (e.g. right after
    // `scroll down until ... appears`).
    await _waitForScrollIdle(element);
    await _waitUntilHittableAfterScroll(element);
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
    // case, invoke onTap directly instead of using pointer events — but
    // only when the target is actually the topmost thing at its own
    // screen position (FP-10). Direct invocation has no relationship to
    // paint order (unlike a real pointer tap, which Flutter's own
    // hit-testing already resolves correctly — see _createGesture below),
    // so without this guard it could fire onTap on a button that's
    // actually hidden behind a modal barrier, loading overlay, or Stack
    // sibling a real user's tap would hit instead.
    if (element.widget is Semantics) {
      final target = _findDirectTapTarget(element);
      final targetBox = target?.renderObject;
      if (target != null && targetBox is RenderBox && _isTopmostAt(targetBox, center)) {
        _invokeOnTap(target);
        return;
      }
      // Not found, or occluded — fall through to a real hit-tested
      // pointer tap, which lands on whatever is actually on top instead.
    }

    // A real pointer tap lands on whatever is on top at the point. If that is
    // a widget unrelated to the target (neither inside it nor around it), say so:
    // otherwise the tap "succeeds" and nothing happens. A hit on a widget that
    // contains the target (a parent GestureDetector/InkWell) is normal and does
    // not warn — an earlier version warned there too (false positive on a
    // snackbar action).
    // An overlay that is still sliding in or out (a SnackBar action, a sheet)
    // can sit over the target for a few frames; give it a moment to settle
    // before calling the tap covered, so a transient state is not reported.
    var warnCenter = center;
    for (var i = 0; i < 12 && box.attached; i++) {
      warnCenter = box.localToGlobal(box.size.center(Offset.zero));
      if (_hitState(box, warnCenter).related) break;
      await Future.delayed(const Duration(milliseconds: 50));
    }
    final covered = !_hitState(box, warnCenter).related;
    final before = covered ? _screenFingerprint() : '';
    final gesture = await _createGesture(warnCenter);
    await gesture.up();
    if (covered) {
      // The hit test says something unrelated is on top, but it can be wrong
      // about overlays (a SnackBar action reported "covered" although tapping
      // it worked). So only report it when the tap also changed nothing on
      // screen — that is the case where a test would otherwise pass silently.
      for (var i = 0; i < 4; i++) {
        await Future.delayed(const Duration(milliseconds: 100));
        if (_screenFingerprint() != before) return;
      }
      _tapWarning = 'tap target ${_selDesc(sel)} is covered by another widget at '
          '(${warnCenter.dx.round()}, ${warnCenter.dy.round()}); the tap lands on whatever is on top '
          'and the screen did not change (topmost hit: ${_hitPathDesc(warnCenter)})'
          '${_keyboardHint()}'
          '${_visibleHint()}';
    }
  }

  /// A soft keyboard that is open can hide a button (a login form's submit
  /// button): say so, since the fix is a `close keyboard` step.
  String _keyboardHint() {
    try {
      final view = WidgetsBinding.instance.platformDispatcher.implicitView;
      if (view != null && view.viewInsets.bottom > 0) {
        return ' — the on-screen keyboard is open and may be covering it; add `close keyboard` before this step';
      }
    } catch (_) {}
    return '';
  }

  String _screenFingerprint() {
    try {
      final s = _finder.visibleSummary(max: 500);
      return '${s['texts']!.join('|')}#${s['keys']!.join('|')}';
    } catch (_) {
      return '';
    }
  }

  /// Walks down from [element] to find the nearest GestureDetector or
  /// InkResponse descendant with a non-null onTap — the same widget a
  /// direct tap would invoke. Returns the Element without invoking
  /// anything, so the caller can check occlusion (FP-10) before firing.
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
  Element? _findDirectTapTarget(Element element) {
    Element? found;
    void visit(Element e) {
      if (found != null) return;
      try {
        final widget = e.widget;
        if (widget is GestureDetector && widget.onTap != null) {
          found = e;
          return;
        }
        if (widget is InkResponse && widget.onTap != null) {
          found = e;
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

  /// Invokes the onTap callback on an Element located by
  /// [_findDirectTapTarget].
  void _invokeOnTap(Element element) {
    final widget = element.widget;
    if (widget is GestureDetector) {
      widget.onTap!();
    } else if (widget is InkResponse) {
      widget.onTap!();
    }
  }

  /// FP-10: returns true if [target] is the render object a real pointer
  /// tap at [position] would actually reach — i.e. nothing else (a
  /// ModalBarrier, a loading overlay, an unrelated Stack sibling) is
  /// painted on top of it at that exact point.
  ///
  /// Uses the same `hitTestInView` call Flutter's own pointer dispatch
  /// makes internally (see `GestureBinding.handlePointerEvent`), but as a
  /// read-only query — no event is actually dispatched, so this has no
  /// side effects on the widget tree.
  /// Hit-test result at [position], relative to [target].
  /// `strict`: the target (or something inside it) is in the hit path.
  /// `related`: the deepest hit render object is the target, inside it, or an
  /// ancestor of it — i.e. NOT an unrelated widget sitting on top of it.
  /// Short description of what a hit test at [position] reaches, deepest first
  /// (render object types), for diagnosing "covered" warnings.
  String _hitPathDesc(Offset position) {
    try {
      final view = WidgetsBinding.instance.platformDispatcher.implicitView;
      if (view == null) return 'no view';
      final result = HitTestResult();
      GestureBinding.instance.hitTestInView(result, position, view.viewId);
      final names = <String>[];
      for (final e in result.path) {
        final t = e.target;
        if (t is RenderObject) names.add('${t.runtimeType}');
        if (names.length >= 14) break;
      }
      return names.isEmpty ? 'nothing' : names.join(' < ');
    } catch (_) {
      return 'unknown';
    }
  }

  bool _handlesPointer(RenderObject r) =>
      r is RenderPointerListener || r is RenderMouseRegion || r is RenderSemanticsGestureHandler;

  ({bool strict, bool related}) _hitState(RenderObject target, Offset position) {
    final view = WidgetsBinding.instance.platformDispatcher.implicitView;
    if (view == null) return (strict: true, related: true);
    final result = HitTestResult();
    GestureBinding.instance.hitTestInView(result, position, view.viewId);
    RenderObject? deepest;
    var strict = false;
    for (final entry in result.path) {
      final t = entry.target;
      if (t is! RenderObject) continue;
      deepest ??= t;
      for (RenderObject? c = t; c != null; c = c.parent) {
        if (identical(c, target)) strict = true;
      }
    }
    var related = deepest == null || strict;
    // A hit that stops at an ancestor is normal only when that ancestor is
    // something that handles taps (a parent GestureDetector/InkWell). Stopping
    // at a passive container or the root view means nothing received the tap
    // at that point (clipped away, off screen, under the keyboard).
    if (!related && _handlesPointer(deepest)) {
      for (RenderObject? c = target; c != null; c = c.parent) {
        if (identical(c, deepest)) related = true;
      }
    }
    return (strict: strict, related: related);
  }

  bool _isTopmostAt(RenderObject target, Offset position) {
    final view = WidgetsBinding.instance.platformDispatcher.implicitView;
    if (view == null) return true; // no view to hit-test against — don't block
    final result = HitTestResult();
    GestureBinding.instance.hitTestInView(result, position, view.viewId);
    for (final entry in result.path) {
      RenderObject? candidate = entry.target is RenderObject ? entry.target as RenderObject : null;
      while (candidate != null) {
        if (identical(candidate, target)) return true;
        candidate = candidate.parent;
      }
    }
    return false;
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
      _enterText(editable, text);
    } else {
      // Fallback: tap to focus, then try to find any focused text field
      await _tap(sel);
      await Future.delayed(const Duration(milliseconds: 200));
      final focused = _findFocusedEditableTarget();
      if (focused != null) {
        _enterText(focused, text);
      } else {
        throw ProbeError(ProbeError.widgetNotFound, 'No text field found for: ${_selDesc(sel)}');
      }
    }
  }

  /// FP-13: replace a field's text the way the platform keyboard does.
  ///
  /// `controller.text = ...` updates the controller (and its listeners) but
  /// never reaches the TextField's `onChanged`, `inputFormatters`, or a
  /// `Form`'s auto-validation — so a dialog that validates on `onChanged`
  /// (e.g. a "value above range" notice) never rebuilt after `type`.
  /// `EditableTextState.userUpdateTextEditingValue` is the entry point real
  /// text input uses, so the whole chain runs. Falls back to setting the
  /// controller value when the state isn't reachable (still notifies
  /// controller listeners, just not `onChanged`).
  void _enterText(_EditableTarget target, String text) {
    final value = TextEditingValue(
      text: text,
      selection: TextSelection.collapsed(offset: text.length),
    );
    final state = target.state;
    if (state != null && state.mounted) {
      state.userUpdateTextEditingValue(value, SelectionChangedCause.keyboard);
    } else {
      target.controller.value = value;
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
        result = _editableTargetOf(e);
        return;
      }
      e.visitChildren(visit);
    }

    Element? current = element;
    for (int i = 0; i < 20 && current != null; i++) {
      if (current.widget is EditableText) {
        return _editableTargetOf(current);
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

  _EditableTarget _editableTargetOf(Element e) {
    final w = e.widget as EditableText;
    final state = e is StatefulElement && e.state is EditableTextState ? e.state as EditableTextState : null;
    return _EditableTarget(w.controller, w.focusNode, state);
  }

  /// Finds any currently focused text field.
  _EditableTarget? _findFocusedEditableTarget() {
    _EditableTarget? result;
    _finder.walkTree((e) {
      if (result != null) return;
      if (e.widget is EditableText && (e.widget as EditableText).focusNode.hasFocus) {
        result = _editableTargetOf(e);
      }
    });
    return result;
  }

  Future<void> _clearText(Map<String, dynamic> sel) async {
    final element = _requireElement(sel);
    final target = _findEditableTarget(element);
    if (target != null) {
      // Same path as `type` so onChanged fires for a cleared field too.
      _enterText(target, '');
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
        // the matched element, not an ancestor, so this only needs a direct
        // match plus a subtree walk.
        //
        // PT-12: this used to also walk *ancestors* looking for a match,
        // but that produces false positives for essentially any on-screen
        // element: once nothing more specific is focused (e.g. right after
        // FocusNode.unfocus()), Flutter falls back to the enclosing
        // ModalRoute's own FocusScopeNode holding primary focus — and that
        // scope is an ancestor of *every* widget on the current screen, so
        // the ancestor walk would report every one of them as "focused".
        // Found while verifying that `close keyboard` (PT-12) actually
        // removes focus: `don't see #field is focused` kept failing right
        // after a real unfocus() because of this ancestor false-positive.
        bool hasFocus = element.renderObject == focusedRenderObject;
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

      case 'idle':
        // FP-13: `wait for idle` — route transitions finished AND the
        // triple-signal sync (frames, animations, HTTP) settled.
        await _waitForRouteTransitions(timeout: timeoutDur);
        await _sync.waitForSettled(timeout: timeoutDur);

      case 'page_load':
      case 'network_idle':
      case 'settled':
        await _sync.waitForSettled(timeout: timeoutDur);
    }
  }

  /// True while any mounted route is mid push/pop transition. A popped
  /// dialog or sheet stays in the tree (inactive, animation reversing) until
  /// its exit animation completes, and its modal barrier keeps absorbing
  /// pointer events for that whole time.
  ///
  /// Routes are discovered through widgets every route body is built from
  /// (page scaffolds, dialogs, sheets) because Navigator keeps its route list
  /// private; `probeRouteOf` then resolves each one's own animation.
  bool _routeTransitionInFlight() {
    final seen = <ModalRoute<dynamic>>{};
    var inFlight = false;
    _finder.walkTree((e) {
      if (inFlight) return;
      final w = e.widget;
      if (w is! Scaffold && w is! Dialog && w is! BottomSheet && w is! CupertinoPageScaffold) return;
      final route = probeRouteOf(e);
      if (route == null || !seen.add(route)) return;
      final status = route.animation?.status;
      if (status == AnimationStatus.forward || status == AnimationStatus.reverse) {
        inFlight = true;
      }
    });
    return inFlight;
  }

  Future<void> _waitForRouteTransitions({Duration timeout = const Duration(seconds: 2)}) async {
    if (!_routeTransitionInFlight()) return;
    final deadline = DateTime.now().add(timeout);
    while (DateTime.now().isBefore(deadline) && _routeTransitionInFlight()) {
      await Future.delayed(const Duration(milliseconds: 50));
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
    // PT-06: WaitStep carries only a raw target string, not a selector kind
    // (unlike Selector/SelectorParam used by tap/type), so an id target must
    // be detected from its '#' prefix here — this previously always built a
    // *text* selector regardless, meaning `wait until #my_button appears`
    // searched for a widget whose visible text literally read "#my_button"
    // and could never match, timing out on indisputably-mounted, visible
    // widgets (icon buttons and other non-text elements) every time. Mirrors
    // the same '#'-prefix check runConditional already uses for `if` steps.
    final sel = text.startsWith('#')
        ? {'kind': 'id', 'text': text}
        : {'kind': 'text', 'text': text};
    final deadline = DateTime.now().add(timeout);
    while (DateTime.now().isBefore(deadline)) {
      final found = _finder.findElements(sel).isNotEmpty;
      if (found == expect) return;
      await Future.delayed(const Duration(milliseconds: 100));
      await _sync.waitForSettled(timeout: const Duration(seconds: 1));
    }
    final desc = expect ? 'appear' : 'disappear';
    throw ProbeError(
      ProbeError.timeout,
      'Timed out waiting for "$text" to $desc${_visibleHint()}',
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
    // PT-15: `scroll`'s job is "reveal more content," unlike `swipe`, which
    // tests a real gesture interaction (swipe-to-dismiss, swipe-to-refresh,
    // etc.) — it doesn't need to go through the gesture arena at all.
    // Simulating scroll as a pointer drag (like swipe does) meant it had to
    // *win* the arena against any competing recognizer along the way, and a
    // `Dismissible`-wrapped row's horizontal drag recognizer could still
    // intercept it even with an axis-pure delta — reproduced against a real
    // iOS simulator (a 50-item list with `Dismissible` rows never scrolled
    // past the first screen, while the same verb worked fine on a plain
    // list). Driving the nearest Scrollable's own ScrollPosition directly
    // sidesteps gesture-arena competition entirely.
    final scrollable = _findScrollable(sel);
    if (scrollable == null) {
      // No Scrollable found (e.g. a custom scroll implementation that
      // doesn't use the standard widget) — fall back to the old
      // gesture-based approach.
      await _swipe(direction, sel);
      return;
    }

    final position = scrollable.position;
    final extent = position.viewportDimension * 0.5;
    // Matches _swipe's verb semantics: 'down'/'right' increase the scroll
    // offset (reveal later content); 'up'/'left' decrease it.
    final signedDelta = switch (direction) {
      'up' || 'left' => -extent,
      _ => extent,
    };
    final target = (position.pixels + signedDelta)
        .clamp(position.minScrollExtent, position.maxScrollExtent);
    // jumpTo (not animateTo) — there's no user-facing transition to make
    // smooth here, and animateTo's Future only resolves once its animation
    // ticks to completion, which needs real frame scheduling that a test
    // harness driving this through dispatch() won't naturally provide.
    //
    // jumpTo schedules a frame but returns before it runs — the RPC
    // dispatcher's own _sync.waitForSettled() call right after this returns
    // isn't enough on its own to guarantee that frame has actually happened
    // yet (schedulerPhase can still read idle in the gap between
    // "requested" and "started"), so a rapid sequence of scroll calls could
    // race ahead of the ListView actually rebuilding to reveal newly
    // visible rows. Wait for the frame explicitly here instead.
    scrollable.position.jumpTo(target);
    await WidgetsBinding.instance.endOfFrame;
  }

  /// FP-13: `scroll <dir> until "X" appears` — scrollIntoView for lazily
  /// built lists. Scrolls half a viewport at a time (rows outside the viewport
  /// aren't built, so the target only exists once enough has been scrolled),
  /// and once found brings it fully on screen: the finder also matches rows
  /// in the list's cache extent, which are mounted but still off-screen.
  /// Stops early when the list can't move any further.
  Future<void> _scrollUntil(
    String direction,
    Map<String, dynamic>? sel,
    Map<String, dynamic> until,
  ) async {
    const maxScrolls = 25;
    var atEnd = false;
    for (var i = 0; ; i++) {
      final found = _finder.findElements(until);
      if (found.isNotEmpty) {
        await Scrollable.ensureVisible(
          found.first,
          alignment: 0.5,
          duration: Duration.zero,
        );
        await WidgetsBinding.instance.endOfFrame;
        return;
      }
      if (atEnd || i >= maxScrolls) break;
      final before = _findScrollable(sel)?.position.pixels;
      await _scroll(direction, sel);
      final after = _findScrollable(sel)?.position.pixels;
      // Can't move: check once more (the last scroll may have built rows)
      // and then give up instead of burning the remaining attempts.
      if (before != null && before == after) atEnd = true;
    }
    throw ProbeError(
      ProbeError.widgetNotFound,
      'scroll $direction until ${_selDesc(until)}: not found after scrolling'
      ' (`scroll down` reveals later content, `scroll up` earlier content)'
      '${_visibleHint()}',
    );
  }

  /// Finds the Scrollable most relevant to a scroll verb: if [sel] resolves
  /// to an element, the nearest enclosing Scrollable (walking up), or —
  /// since a selector commonly targets the list widget itself rather than a
  /// descendant inside it — the nearest one in its own subtree. With no
  /// selector, the first Scrollable belonging to the current (topmost)
  /// route, matching the route-awareness the rest of the finder already
  /// applies (see ProbeFinder._isVisible).
  ScrollableState? _findScrollable(Map<String, dynamic>? sel) {
    if (sel != null) {
      final element = _requireElement(sel);
      final ancestor = Scrollable.maybeOf(element);
      if (ancestor != null) return ancestor;
      ScrollableState? descendant;
      void visit(Element e) {
        if (descendant != null) return;
        if (e is StatefulElement && e.state is ScrollableState) {
          descendant = e.state as ScrollableState;
          return;
        }
        e.visitChildren(visit);
      }
      element.visitChildren(visit);
      return descendant;
    }

    // No selector: several Scrollables can exist on one screen (e.g. a
    // TextField's own internal cursor-scrolling Scrollable, alongside the
    // actual content list) — the first one found in tree order isn't
    // necessarily the one a bare `scroll down` should mean. Pick the one
    // with the largest viewport area instead, since that's virtually always
    // the main scrolling content, not an incidental one inside some other
    // widget.
    //
    // FP-16: "largest" alone can pick a scrollable the user cannot see — an
    // IndexedStack (tabs kept alive) lays out every child but only paints and
    // hit-tests the selected one, so a hidden tab's list can be first in tree
    // order and the same size as the visible one, and `scroll down` then moves
    // nothing on screen. Prefer a scrollable that actually receives touches at
    // its own center; fall back to the largest of all if none does.
    ScrollableState? bestHit;
    double bestHitArea = 0;
    ScrollableState? bestAny;
    double bestAnyArea = 0;
    void visit(Element e) {
      if (probeRouteOf(e)?.isCurrent == false) return;
      if (e is StatefulElement && e.state is ScrollableState) {
        final state = e.state as ScrollableState;
        final box = state.context.findRenderObject();
        if (box is RenderBox && box.hasSize) {
          final area = box.size.width * box.size.height;
          if (area > bestAnyArea) {
            bestAnyArea = area;
            bestAny = state;
          }
          if (area > bestHitArea && box.attached &&
              _isTopmostAt(box, box.localToGlobal(box.size.center(Offset.zero)))) {
            bestHitArea = area;
            bestHit = state;
          }
        }
      }
      e.visitChildren(visit);
    }
    WidgetsBinding.instance.rootElement?.visitChildren(visit);
    return bestHit ?? bestAny;
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
          // At the root route the system Back would leave the app (Android),
          // killing the agent mid-test: every later step then fails with a
          // lost connection. A test saying "go back" never means that — use
          // "close the app" for it — so report it instead of exiting.
          _tapWarning = 'go back: already at the root route, nothing to go back to; '
              'the app was not closed (use "close the app" to leave it)';
        }
      case 'close':
        // PT-12: `close keyboard`/`close the app` (parser.VerbClose) both
        // dispatch here with action='close' — this case never existed, so
        // both were a silent no-op regardless of `value`. For keyboard
        // dismissal specifically, unfocus the current FocusNode directly in
        // the Flutter widget tree rather than an OS-level gesture — the
        // suggested fix in PT-12 to avoid colliding with iOS's Back-swipe
        // gesture recognition, and it also never depends on hit-testing a
        // particular screen location the way a gesture would.
        if (value == 'keyboard') {
          FocusManager.instance.primaryFocus?.unfocus();
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
      // PT-16: Navigator keeps previous routes mounted underneath the
      // current one, and both a pushed route and the one beneath it
      // typically produce a same-size, screen-sized RepaintBoundary — the
      // strict `area > bestArea` comparison below then keeps whichever one
      // is visited first (the previous route, in Overlay insertion order),
      // silently capturing stale content instead of the current screen.
      // Skip anything belonging to a route that isn't current, mirroring
      // ProbeFinder's own route-awareness fix (PT-03).
      if (probeRouteOf(element)?.isCurrent == false) return;
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
        'Widget not found: ${_selDesc(sel)}${_visibleHint()}',
      );
    }
    return elements.first;
  }

  /// FP-13: " — visible: texts [...] keys [...]" suffix for failure messages,
  /// so a failed step says what the screen actually showed instead of only
  /// what was missing. Empty if the summary itself can't be built.
  String _visibleHint() {
    try {
      final s = _finder.visibleSummary(max: 15);
      final texts = s['texts']!;
      final keys = s['keys']!;
      if (texts.isEmpty && keys.isEmpty) return '';
      String fmt(List<String> l) => l.map((v) => '"${v.length > 40 ? '${v.substring(0, 40)}…' : v}"').join(', ');
      return ' — visible texts: [${fmt(texts)}], keys: [${fmt(keys)}]';
    } catch (_) {
      return '';
    }
  }

  String _selDesc(Map<String, dynamic> sel) {
    final kind = sel['kind'] ?? 'text';
    final text = '${sel['text'] ?? ''}';
    // An id selector's text already carries its leading '#' ("#settings_screen"):
    // show it once, as written in the test, not as id("#settings_screen").
    if (kind == 'id') return text.startsWith('#') ? text : '#$text';
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
    if (widget is EditableText) return widget.controller.text;
    // TextField/TextFormField (and design-system wrappers around them) are
    // neither Text nor EditableText themselves — they build an EditableText
    // several layers down. Reuse the same up/down search _findTextController
    // already does for tap-to-focus and clear(), instead of returning '' for
    // every text field (see #field contains "..." always failed silently).
    final controller = _findTextController(e);
    if (controller != null) return controller.text;
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
  const _EditableTarget(this.controller, this.focusNode, [this.state]);
  final TextEditingController controller;
  final FocusNode focusNode;

  /// The EditableText's state, when reachable — the entry point for
  /// keyboard-equivalent edits (see [ProbeExecutor._enterText]).
  final EditableTextState? state;
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
