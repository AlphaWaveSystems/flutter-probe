import 'dart:async';
import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/cupertino.dart' show CupertinoButton, CupertinoSwitch;
import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart'
    show ButtonStyleButton, Checkbox, FloatingActionButton, GestureDetector, IconButton, InkResponse, Radio, Switch, Tooltip;
import 'package:flutter/rendering.dart';
import 'package:flutter/widgets.dart';

import 'finder.dart' show probeRouteOf;
import 'protocol.dart';

// Support for the interactive device view (probe.tap_at, probe.long_press_at,
// probe.swipe_at, probe.dump_tree {bounds}, probe.hit_test and the extra
// probe.screenshot parameters). All coordinates are Flutter logical pixels with
// the origin at the top-left of the view, the same space as RenderBox.localToGlobal.

/// One entry of the flat node list returned by `probe.dump_tree {bounds: true}`
/// and `probe.hit_test`.
class ProbeNode {
  ProbeNode({
    required this.index,
    required this.parent,
    required this.element,
    required this.box,
    required this.type,
    required this.key,
    required this.text,
    required this.label,
    required this.rect,
    required this.interactive,
  });

  final int index;
  final int parent;
  final Element element;
  final RenderBox box;
  final String type;
  final String? key;
  final String? text;
  final String? label;
  final Rect rect;
  final bool interactive;

  Map<String, dynamic> toJson() => {
        'i': index,
        'p': parent,
        'type': type,
        'key': key,
        'text': text,
        'label': label,
        'x': _round(rect.left),
        'y': _round(rect.top),
        'w': _round(rect.width),
        'h': _round(rect.height),
        'interactive': interactive,
      };
}

double _round(double v) => (v * 100).round() / 100;

/// Synthesizes pointer input through [GestureBinding], the same entry point the
/// engine uses, so recognizers, scroll physics and hit testing behave as for a
/// real finger.
class ProbePointerInput {
  ProbePointerInput._();

  // Distinct from the executor's own counter (starts at 900) so the two never collide.
  static int _nextPointer = 1900;

  static int get _viewId => WidgetsBinding.instance.platformDispatcher.implicitView?.viewId ?? 0;

  /// Validates a finite coordinate pair; throws invalidParams otherwise.
  static Offset point(Map<String, dynamic> params, String xk, String yk) {
    final x = params[xk], y = params[yk];
    if (x is! num || y is! num || !x.isFinite || !y.isFinite) {
      throw ProbeError(ProbeError.invalidParams, 'expected finite numbers "$xk" and "$yk"');
    }
    return Offset(x.toDouble(), y.toDouble());
  }

  static Future<void> tap(Offset at) async {
    final s = _Sequence(at);
    s.down();
    s.up();
  }

  static Future<void> longPress(Offset at, {int durationMs = 600}) async {
    final s = _Sequence(at);
    s.down();
    s.advance(durationMs);
    await Future<void>.delayed(Duration(milliseconds: durationMs));
    s.up();
  }

  static Future<void> swipe(Offset from, Offset to, {int durationMs = 300}) async {
    // Gesture arenas and scroll physics expect a build-up of small moves, not one jump.
    final steps = math.max(2, math.min(60, durationMs ~/ 16));
    final stepMs = math.max(1, durationMs ~/ steps);
    final s = _Sequence(from);
    s.down();
    for (var i = 1; i <= steps; i++) {
      s.advance(stepMs);
      s.move(Offset.lerp(from, to, i / steps)!);
      await Future<void>.delayed(Duration(milliseconds: stepMs));
    }
    s.up();
  }
}

/// One pointer's lifetime: added, down, moves, up, removed.
class _Sequence {
  _Sequence(this._position) : _pointer = ProbePointerInput._nextPointer++;

  final int _pointer;
  Offset _position;
  Duration _time = Duration.zero;
  bool _isDown = false;
  final GestureBinding _binding = GestureBinding.instance;
  final int _viewId = ProbePointerInput._viewId;

  void advance(int ms) => _time += Duration(milliseconds: ms);

  void down() {
    _binding.handlePointerEvent(PointerAddedEvent(
      viewId: _viewId, timeStamp: _time, device: _pointer, position: _position));
    _binding.handlePointerEvent(PointerDownEvent(
      viewId: _viewId, timeStamp: _time, pointer: _pointer, device: _pointer, position: _position));
    _isDown = true;
  }

  void move(Offset to) {
    _binding.handlePointerEvent(PointerMoveEvent(
      viewId: _viewId,
      timeStamp: _time,
      pointer: _pointer,
      device: _pointer,
      position: to,
      delta: to - _position,
    ));
    _position = to;
  }

  void up() {
    if (!_isDown) return;
    _binding.handlePointerEvent(PointerUpEvent(
      viewId: _viewId, timeStamp: _time, pointer: _pointer, device: _pointer, position: _position));
    _binding.handlePointerEvent(PointerRemovedEvent(
      viewId: _viewId, timeStamp: _time, device: _pointer, position: _position));
    _isDown = false;
  }
}

/// Builds the flat node list (preorder, parent by index) for the current screen.
/// Only widgets a user or a selector cares about are listed: keyed widgets,
/// interactive widgets, text, images, icons, editable text and scrollables.
List<ProbeNode> collectProbeNodes() {
  final nodes = <ProbeNode>[];
  final root = WidgetsBinding.instance.rootElement;
  if (root == null) return nodes;

  void visit(Element e, int parent) {
    final widget = e.widget;
    if (widget is Offstage && widget.offstage) return;
    if (widget is Visibility && !widget.visible) return;
    var childParent = parent;
    try {
      // Routes underneath the current one stay mounted; they are not on screen.
      final include = _isListed(widget);
      if (include && probeRouteOf(e)?.isCurrent != false) {
        final ro = e.renderObject;
        if (ro is RenderBox && ro.hasSize && ro.attached) {
          final topLeft = ro.localToGlobal(Offset.zero);
          final index = nodes.length;
          nodes.add(ProbeNode(
            index: index,
            parent: parent,
            element: e,
            box: ro,
            type: widget.runtimeType.toString(),
            key: _keyOf(widget.key),
            text: _textOf(e),
            label: _labelOf(widget),
            rect: topLeft & ro.size,
            interactive: _isInteractive(widget),
          ));
          childParent = index;
        }
      }
    } catch (_) {
      // Element may be disposed or mid-build during the walk: skip it.
    }
    e.visitChildren((c) => visit(c, childParent));
  }

  root.visitChildren((c) => visit(c, -1));
  return nodes;
}

bool _isListed(Widget w) =>
    _keyOf(w.key) != null ||
    w is Text ||
    w is EditableText ||
    w is Image ||
    w is Icon ||
    w is Scrollable ||
    _isInteractive(w);

/// Widgets that react to a tap or accept text, from the widget type (the
/// semantics tree is not always enabled).
bool _isInteractive(Widget w) {
  if (w is ButtonStyleButton ||
      w is IconButton ||
      w is FloatingActionButton ||
      w is CupertinoButton ||
      w is Checkbox ||
      w is Radio ||
      w is Switch ||
      w is CupertinoSwitch ||
      w is EditableText) {
    return true;
  }
  if (w is GestureDetector) {
    return w.onTap != null ||
        w.onTapDown != null ||
        w.onDoubleTap != null ||
        w.onLongPress != null ||
        w.onPanUpdate != null ||
        w.onHorizontalDragUpdate != null ||
        w.onVerticalDragUpdate != null;
  }
  if (w is InkResponse) return w.onTap != null || w.onLongPress != null || w.onDoubleTap != null;
  if (w is Semantics) {
    final p = w.properties;
    return p.onTap != null || p.onLongPress != null || p.textField == true;
  }
  return false;
}

String? _keyOf(Key? key) {
  if (key is ValueKey<String>) return key.value;
  return null;
}

String? _labelOf(Widget w) {
  if (w is Semantics) return w.properties.label;
  if (w is Text) return w.semanticsLabel;
  if (w is Icon) return w.semanticLabel;
  if (w is Tooltip) return w.message;
  return null;
}

String? _textOf(Element e) {
  final w = e.widget;
  if (w is Text) return w.data ?? w.textSpan?.toPlainText();
  if (w is EditableText) return w.obscureText ? null : w.controller.text;
  if (!_isInteractive(w) && _keyOf(w.key) == null) return null;
  // A button or keyed container: its first visible label.
  String? found;
  void visit(Element c, int depth) {
    if (found != null || depth > 80) return;
    final cw = c.widget;
    if (cw is Text) {
      found = cw.data ?? cw.textSpan?.toPlainText();
      return;
    }
    if (cw is EditableText) return;
    c.visitChildren((x) => visit(x, depth + 1));
  }

  e.visitChildren((c) => visit(c, 0));
  return found;
}

/// `probe.hit_test`: the deepest interactive-or-keyed widget at [at], taken
/// from the real hit-test path so widgets under a modal barrier are not reported.
ProbeNode? probeHitTest(Offset at) {
  final view = WidgetsBinding.instance.platformDispatcher.implicitView;
  if (view == null) return null;
  final nodes = collectProbeNodes();
  final result = HitTestResult();
  GestureBinding.instance.hitTestInView(result, at, view.viewId);

  final byBox = <RenderObject, List<ProbeNode>>{};
  for (final n in nodes) {
    if (n.interactive || n.key != null) (byBox[n.box] ??= []).add(n);
  }
  for (final entry in result.path) {
    final candidates = byBox[entry.target];
    if (candidates == null) continue;
    var best = candidates.last;
    // Several widgets can share one render object (a keyed wrapper around a
    // button); prefer the keyed one, else the outermost.
    final keyed = candidates.where((n) => n.key != null);
    best = keyed.isNotEmpty ? keyed.last : candidates.first;
    // A button's InkResponse sits below the button itself: climb to the
    // enclosing interactive widget so the node is the one a test would select.
    while (best.parent >= 0 && nodes[best.parent].interactive && best.key == null) {
      best = nodes[best.parent];
    }
    return best;
  }
  return null;
}

/// Layout facts of the view for screenshot replies.
({double logicalWidth, double logicalHeight, double dpr}) probeViewMetrics() {
  final views = RendererBinding.instance.renderViews;
  final view = views.isNotEmpty
      ? views.first.flutterView
      : ui.PlatformDispatcher.instance.views.first;
  final dpr = view.devicePixelRatio;
  return (
    logicalWidth: view.physicalSize.width / dpr,
    logicalHeight: view.physicalSize.height / dpr,
    dpr: dpr,
  );
}

/// Optionally downscales PNG [png] by [scale] (0.1..1.0) and describes the
/// result. JPEG is not offered: dart:ui has no JPEG encoder and the agent has
/// no image dependency, so the reply always says `format: png`.
Future<({Uint8List bytes, int width, int height})> probeScaledPng(Uint8List png, double scale) async {
  final s = scale.isNaN ? 1.0 : scale.clamp(0.1, 1.0);
  final data = ByteData.sublistView(png);
  // PNG IHDR: width and height are big-endian uint32 at offsets 16 and 20.
  final width = data.getUint32(16), height = data.getUint32(20);
  if (s >= 1.0) return (bytes: png, width: width, height: height);
  final codec = await ui.instantiateImageCodec(
    png,
    targetWidth: math.max(1, (width * s).round()),
  );
  final frame = await codec.getNextFrame();
  final out = await frame.image.toByteData(format: ui.ImageByteFormat.png);
  final w = frame.image.width, h = frame.image.height;
  frame.image.dispose();
  codec.dispose();
  if (out == null) return (bytes: png, width: width, height: height);
  return (bytes: out.buffer.asUint8List(), width: w, height: h);
}
