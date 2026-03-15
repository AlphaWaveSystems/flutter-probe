import 'dart:async';

import 'package:flutter/gestures.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/widgets.dart';

import 'protocol.dart';

typedef SendFn = void Function(String message);

/// ProbeRecorder intercepts user gestures and text input on the live app
/// and streams recorded events back to the CLI via JSON-RPC notifications.
class ProbeRecorder {
  SendFn? _send;
  bool _recording = false;

  // Pointer tracking for gesture classification
  final Map<int, _PointerSession> _pointers = {};

  // Text input tracking
  Timer? _fieldScanTimer;
  final Map<int, _TrackedField> _trackedFields = {};
  int _nextFieldId = 0;

  // Thresholds
  static const double _swipeThreshold = 20.0;
  static const Duration _longPressThreshold = Duration(milliseconds: 500);
  static const Duration _textDebounce = Duration(milliseconds: 300);

  void start(SendFn send) {
    if (_recording) return;
    _send = send;
    _recording = true;
    GestureBinding.instance.pointerRouter.addGlobalRoute(_onPointerEvent);
    _startTextTracking();
  }

  void stop() {
    if (!_recording) return;
    _recording = false;
    GestureBinding.instance.pointerRouter.removeGlobalRoute(_onPointerEvent);
    _stopTextTracking();
    _pointers.clear();
    _send = null;
  }

  bool get isRecording => _recording;

  // ---- Pointer event handling ----

  void _onPointerEvent(PointerEvent event) {
    if (!_recording) return;

    if (event is PointerDownEvent) {
      _pointers[event.pointer] = _PointerSession(
        startPosition: event.position,
        startTime: event.timeStamp,
      );
    } else if (event is PointerMoveEvent) {
      final session = _pointers[event.pointer];
      if (session != null) {
        session.lastPosition = event.position;
      }
    } else if (event is PointerUpEvent) {
      final session = _pointers.remove(event.pointer);
      if (session == null) return;

      final endPosition = event.position;
      final displacement = endPosition - session.startPosition;
      final distance = displacement.distance;
      final duration = event.timeStamp - session.startTime;

      if (distance < _swipeThreshold) {
        if (duration >= _longPressThreshold) {
          _emitGesture('long_press', session.startPosition);
        } else {
          _emitGesture('tap', session.startPosition);
        }
      } else {
        _emitSwipe(displacement);
      }
    }
  }

  void _emitGesture(String action, Offset position) {
    final selector = _identifyWidget(position);
    _emit({
      'action': action,
      if (selector != null) 'selector': selector,
      'timestamp': DateTime.now().millisecondsSinceEpoch ~/ 1000,
    });
  }

  void _emitSwipe(Offset displacement) {
    final String direction;
    if (displacement.dx.abs() > displacement.dy.abs()) {
      direction = displacement.dx > 0 ? 'right' : 'left';
    } else {
      direction = displacement.dy > 0 ? 'down' : 'up';
    }
    _emit({
      'action': 'swipe',
      'direction': direction,
      'timestamp': DateTime.now().millisecondsSinceEpoch ~/ 1000,
    });
  }

  // ---- Widget identification from screen coordinates ----

  /// Framework-internal widget types that should not be used as selectors.
  static const _frameworkTypes = <String>{
    'RenderObjectToWidgetAdapter',
    'NotificationListener',
    'InheritedElement',
    'RepaintBoundary',
    'Semantics',
    'MergeSemantics',
    'MediaQuery',
    'Builder',
    'Listener',
    'RawGestureDetector',
    'FadeTransition',
    'SlideTransition',
    'AnimatedBuilder',
    'KeyedSubtree',
    'Offstage',
    'Positioned',
    'Padding',
    'SizedBox',
    'Align',
    'Center',
    'Container',
    'DecoratedBox',
    'ConstrainedBox',
    'LimitedBox',
    'ColoredBox',
    'Expanded',
    'Flexible',
  };

  Map<String, dynamic>? _identifyWidget(Offset position) {
    try {
      final hitResult = HitTestResult();
      RendererBinding.instance.renderViews.first
          .hitTest(hitResult, position: position);

      // Collect all elements from the hit test path
      Map<String, dynamic>? bestId;
      Map<String, dynamic>? bestText;
      Map<String, dynamic>? bestSemantics;
      Map<String, dynamic>? bestType;

      for (final entry in hitResult.path) {
        final renderObject = entry.target;
        if (renderObject is! RenderBox) continue;

        final element = _findElementForRenderObject(renderObject);
        if (element == null) continue;

        // Walk this element and its ancestors looking for selectors
        Element? current = element;
        for (int i = 0; i < 10 && current != null; i++) {
          final widget = current.widget;

          // ValueKey → highest priority
          if (bestId == null && widget.key is ValueKey) {
            final keyValue = (widget.key as ValueKey).value;
            if (keyValue is String && keyValue.isNotEmpty) {
              bestId = {'kind': 'id', 'text': '#$keyValue'};
            }
          }

          // Text widget
          if (bestText == null && widget is Text && widget.data != null && widget.data!.isNotEmpty) {
            bestText = {'kind': 'text', 'text': widget.data!};
          }

          // RichText
          if (bestText == null && widget is RichText) {
            final plain = widget.text.toPlainText();
            if (plain.isNotEmpty) {
              bestText = {'kind': 'text', 'text': plain};
            }
          }

          // Semantics label
          if (bestSemantics == null && widget is Semantics && widget.properties.label != null) {
            final label = widget.properties.label!;
            if (label.isNotEmpty) {
              bestSemantics = {'kind': 'text', 'text': label};
            }
          }

          // User-meaningful widget type (skip framework internals)
          if (bestType == null) {
            final typeName = widget.runtimeType.toString();
            if (!_frameworkTypes.contains(typeName) &&
                !typeName.startsWith('_') &&
                !typeName.contains('NotificationListener') &&
                !typeName.contains('RenderObject')) {
              bestType = {'kind': 'type', 'text': typeName};
            }
          }

          // Move up
          Element? parent;
          current.visitAncestorElements((ancestor) {
            parent = ancestor;
            return false;
          });
          current = parent;
        }

        // Short-circuit: if we have an id or text, that's good enough
        if (bestId != null) return bestId;
        if (bestText != null) return bestText;
      }

      // Return best available selector by priority
      return bestId ?? bestText ?? bestSemantics ?? bestType;
    } catch (_) {
      // Hit testing can fail during transitions
    }
    return null;
  }

  Element? _findElementForRenderObject(RenderObject target) {
    Element? result;
    void visit(Element element) {
      if (result != null) return;
      if (element.renderObject == target) {
        result = element;
        return;
      }
      element.visitChildren(visit);
    }
    WidgetsBinding.instance.rootElement?.visitChildren(visit);
    return result;
  }

  // ---- Text input tracking ----

  void _startTextTracking() {
    _scanForTextFields();
    _fieldScanTimer = Timer.periodic(
      const Duration(milliseconds: 500),
      (_) => _scanForTextFields(),
    );
  }

  void _stopTextTracking() {
    _fieldScanTimer?.cancel();
    _fieldScanTimer = null;
    for (final tracked in _trackedFields.values) {
      tracked.controller.removeListener(tracked.listener);
      tracked.debounceTimer?.cancel();
    }
    _trackedFields.clear();
  }

  void _scanForTextFields() {
    final rootElement = WidgetsBinding.instance.rootElement;
    if (rootElement == null) return;

    final seen = <TextEditingController>{};
    _visitElement(rootElement, (element) {
      if (element.widget is EditableText) {
        final controller = (element.widget as EditableText).controller;
        seen.add(controller);

        // Already tracking?
        final alreadyTracked = _trackedFields.values
            .any((t) => t.controller == controller);
        if (!alreadyTracked) {
          _trackController(controller, element);
        }
      }
    });

    // Remove stale trackers
    _trackedFields.removeWhere((id, tracked) {
      if (!seen.contains(tracked.controller)) {
        tracked.controller.removeListener(tracked.listener);
        tracked.debounceTimer?.cancel();
        return true;
      }
      return false;
    });
  }

  void _trackController(TextEditingController controller, Element element) {
    final id = _nextFieldId++;
    String lastText = controller.text;
    Timer? debounceTimer;

    void listener() {
      if (!_recording) return;
      final currentText = controller.text;
      if (currentText == lastText) return;
      lastText = currentText;

      debounceTimer?.cancel();
      debounceTimer = Timer(_textDebounce, () {
        if (!_recording || currentText.isEmpty) return;
        // Find the label for this text field
        final fieldSelector = _fieldLabel(element);
        _emit({
          'action': 'type',
          'text': currentText,
          if (fieldSelector != null) 'selector': fieldSelector,
          'timestamp': DateTime.now().millisecondsSinceEpoch ~/ 1000,
        });
      });
    }

    controller.addListener(listener);
    _trackedFields[id] = _TrackedField(
      controller: controller,
      listener: listener,
      debounceTimer: debounceTimer,
    );
  }

  /// Try to find a label for a text field by walking ancestors.
  Map<String, dynamic>? _fieldLabel(Element element) {
    Element? current = element;
    for (int i = 0; i < 15 && current != null; i++) {
      final widget = current.widget;

      if (widget.key is ValueKey) {
        final keyValue = (widget.key as ValueKey).value;
        if (keyValue is String && keyValue.isNotEmpty) {
          return {'kind': 'id', 'text': '#$keyValue'};
        }
      }

      // Look for hint text or label in TextField decoration
      if (widget is EditableText) {
        // Check siblings for label text — walk parent's children
      }

      // Semantics label
      if (widget is Semantics && widget.properties.label != null) {
        final label = widget.properties.label!;
        if (label.isNotEmpty) {
          return {'kind': 'text', 'text': label};
        }
      }

      Element? parent;
      current.visitAncestorElements((ancestor) {
        parent = ancestor;
        return false;
      });
      current = parent;
    }
    return null;
  }

  void _visitElement(Element element, void Function(Element) visitor) {
    visitor(element);
    element.visitChildren((child) => _visitElement(child, visitor));
  }

  // ---- Event emission ----

  void _emit(Map<String, dynamic> event) {
    _send?.call(
      ProbeNotification('probe.recorded_event', event).encode(),
    );
  }
}

// ---- Internal data classes ----

class _PointerSession {
  final Offset startPosition;
  final Duration startTime;
  Offset lastPosition;

  _PointerSession({
    required this.startPosition,
    required this.startTime,
  }) : lastPosition = startPosition;
}

class _TrackedField {
  final TextEditingController controller;
  final VoidCallback listener;
  Timer? debounceTimer;

  _TrackedField({
    required this.controller,
    required this.listener,
    this.debounceTimer,
  });
}
