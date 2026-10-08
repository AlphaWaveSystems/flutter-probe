import 'package:flutter/material.dart' show Tooltip;
import 'package:flutter/widgets.dart';

/// The [ModalRoute] [element] belongs to, found WITHOUT subscribing to it.
///
/// `ModalRoute.of(context)` registers [context] as a dependent of the route's
/// inherited scope. That is only legitimate during a build. The agent calls
/// this from outside any build, on elements of routes that may be closing; when
/// such a route is disposed its scope notifies the dependents it still holds,
/// and the framework asserts that each one is still a descendant
/// (`InheritedElement.notifyClients`, framework.dart) — a red screen in the
/// app after closing a sheet or dialog. This was introduced with the route
/// awareness added in agent 0.10.0 and broke apps in profile/debug test builds
/// (FP-13).
///
/// Walking ancestors reads the same route without registering anything.
/// `_ModalScopeStatus` is private, so it is matched by name and its public
/// `route` field read dynamically; any mismatch (a future Flutter renaming it)
/// degrades to `null`, which every caller already treats as "no route
/// information — assume visible".
ModalRoute<dynamic>? probeRouteOf(Element element) {
  ModalRoute<dynamic>? found;
  try {
    element.visitAncestorElements((ancestor) {
      final widget = ancestor.widget;
      if (widget is InheritedWidget && widget.runtimeType.toString() == '_ModalScopeStatus') {
        final route = (widget as dynamic).route;
        if (route is ModalRoute) found = route;
        return false;
      }
      return true;
    });
  } catch (_) {
    return null;
  }
  return found;
}

/// ProbeFinder translates ProbeLink SelectorParam JSON into Flutter elements
/// by walking the live widget tree. Does NOT use flutter_test finders since
/// those require TestWidgetsFlutterBinding.
class ProbeFinder {
  ProbeFinder._();
  static final ProbeFinder instance = ProbeFinder._();

  /// Returns all [Element]s matching the given selector map.
  /// Only returns elements that are currently visible on screen
  /// (not behind Offstage, Visibility(false), or off-screen routes).
  List<Element> findElements(Map<String, dynamic> sel) {
    final kind = sel['kind'] as String? ?? 'text';
    final text = sel['text'] as String? ?? '';
    final ordinal = (sel['ordinal'] as num?)?.toInt() ?? 1;
    final container = sel['container'] as String? ?? '';
    final relation = sel['relation'] as String? ?? '';
    final anchor = sel['anchor'] as String? ?? '';

    List<Element> raw;
    switch (kind) {
      case 'text':
        raw = _findByText(text);

      case 'id':
        final key = text.startsWith('#') ? text.substring(1) : text;
        raw = _findByKey(key);

      case 'type':
        raw = _findByType(text);

      case 'ordinal':
        // PT-26: "1st #card_id" carries its id with the "#" prefix intact
        // (mirroring the plain 'id' selector kind above) so multiple
        // same-id widgets can be disambiguated by position, not just by
        // matching text.
        final ordinalCandidates = text.startsWith('#')
            ? _findByKey(text.substring(1))
            : _findByText(text);
        final matches = ordinalCandidates.where(_isVisible).toList();
        if (ordinal > 0 && ordinal <= matches.length) {
          return [matches[ordinal - 1]];
        }
        return [];

      case 'positional':
        if (container.isNotEmpty) {
          final containers = _findByText(container).where(_isVisible).toList();
          if (containers.isEmpty) return [];
          final results = <Element>[];
          for (final c in containers) {
            _visitElement(c, (e) {
              if (_matchesText(e.widget, text) && _isVisible(e)) {
                results.add(e);
              }
            });
          }
          return results;
        }
        raw = _findByText(text);

      case 'relational':
        return _findRelational(text, relation, anchor);

      default:
        raw = _findByText(text);
    }
    // Filter to only visible elements
    return raw.where(_isVisible).toList();
  }

  /// Finds elements matching [text] that are spatially positioned relative
  /// to the [anchor] element according to [relation] (below/above/left_of/right_of).
  List<Element> _findRelational(String text, String relation, String anchor) {
    final anchors = _findByText(anchor).where(_isVisible).toList();
    if (anchors.isEmpty) return [];
    final anchorBox = anchors.first.renderObject;
    if (anchorBox is! RenderBox) return [];
    final anchorPos = anchorBox.localToGlobal(anchorBox.size.center(Offset.zero));

    final candidates = _findByText(text).where(_isVisible).toList();
    return candidates.where((e) {
      final ro = e.renderObject;
      if (ro is! RenderBox) return false;
      final pos = ro.localToGlobal(ro.size.center(Offset.zero));
      switch (relation) {
        case 'below':
          return pos.dy > anchorPos.dy;
        case 'above':
          return pos.dy < anchorPos.dy;
        case 'left_of':
          return pos.dx < anchorPos.dx;
        case 'right_of':
          return pos.dx > anchorPos.dx;
        default:
          return false;
      }
    }).toList();
  }

  List<Element> _findByText(String text) {
    final results = <Element>[];
    walkTree((e) {
      if (_matchesText(e.widget, text)) {
        results.add(e);
      }
    });
    if (results.isNotEmpty) return results;
    // Nothing shows this text: fall back to what the app labels for assistive
    // technology and hover help — a Semantics label or a Tooltip message (an
    // icon button has no Text, only a tooltip). Only when no visible text matches, so
    // a Semantics wrapper around a Text never doubles a count.
    walkTree((e) {
      final w = e.widget;
      if (w is Tooltip && (w.message?.contains(text) ?? false)) {
        results.add(e);
      } else if (w is Semantics && (w.properties.label?.contains(text) ?? false)) {
        results.add(e);
      }
    });
    return results;
  }

  List<Element> _findByKey(String key) {
    final results = <Element>[];
    final targetKey = ValueKey(key);
    walkTree((e) {
      if (e.widget.key == targetKey) {
        results.add(e);
        return;
      }
      // Also match Semantics.identifier
      if (e.widget is Semantics) {
        final sem = e.widget as Semantics;
        if (sem.properties.identifier == key) {
          results.add(e);
        }
      }
    });
    return results;
  }

  List<Element> _findByType(String typeName) {
    final results = <Element>[];
    walkTree((e) {
      if (e.widget.runtimeType.toString() == typeName) {
        results.add(e);
      }
    });
    return results;
  }

  bool _matchesText(Widget widget, String text) {
    if (widget is Text) {
      return widget.data == text || (widget.data?.contains(text) ?? false);
    }
    if (widget is RichText) {
      return widget.text.toPlainText().contains(text);
    }
    if (widget is EditableText) {
      return widget.controller.text.contains(text);
    }
    return false;
  }

  /// Returns true if the element is currently visible on screen.
  /// Checks that the render object is painted and not hidden behind
  /// Offstage or Visibility widgets, and — PT-03 — belongs to the current
  /// (topmost) route of its nearest Navigator.
  ///
  /// Flutter's Navigator keeps previous routes mounted underneath the
  /// current one by default (no Offstage wrapper), so a screen reached via
  /// a stacked push can have several live Scrollables/widgets matching the
  /// same selector simultaneously — one per mounted route. Without this
  /// check, every selector-based verb (tap, see, wait, scroll, swipe, ...)
  /// could resolve to a widget on a route the user can no longer see,
  /// producing an action that "succeeds" with no visible effect (see the
  /// scroll/swipe symptom in IMPROVEMENT_TASKS.md PT-03) or a false-positive
  /// `see`/`wait until` match against stale content underneath the current
  /// screen. `ModalRoute.of(element)` returns null for content with no
  /// Navigator ancestor at all (e.g. the root scaffold) — that case is
  /// treated as visible, since there's no ambiguity to resolve.
  bool _isVisible(Element element) {
    // An element that is mid-rebuild or unmounted has no render object yet; in debug builds the
    // `renderObject` getter then asserts instead of returning null. Not visible, not an error.
    final RenderObject? ro;
    try {
      ro = element.renderObject;
    } catch (_) {
      return false;
    }
    if (ro == null || !ro.attached) return false;
    if (ro is RenderBox) {
      // Zero-size widgets are not visible
      if (ro.size == Size.zero) return false;
      // Check if the widget is actually painted (not behind Offstage etc.)
      if (!ro.hasSize) return false;
      // A widget laid out entirely outside the screen is not visible to the user: the
      // neighbouring page of a PageView, a tab kept alive off to the side, a list item in the
      // cache area beyond the viewport. Without this `don't see X` counted such copies.
      if (_isOffScreen(ro)) return false;
    }
    final route = probeRouteOf(element);
    if (route != null && !route.isCurrent && !_routeOnScreen(route)) return false;
    // Walk up the tree to check for Offstage / Visibility ancestors
    Element? current = element;
    while (current != null) {
      final widget = current.widget;
      if (widget is Offstage && widget.offstage) return false;
      if (widget is Visibility && !widget.visible) return false;
      current = _parentElement(current);
    }
    return true;
  }

  bool _isOffScreen(RenderBox ro) {
    try {
      final view = WidgetsBinding.instance.platformDispatcher.implicitView;
      if (view == null || view.devicePixelRatio <= 0) return false;
      final screen = Offset.zero & (view.physicalSize / view.devicePixelRatio);
      final rect = ro.localToGlobal(Offset.zero) & ro.size;
      return !rect.overlaps(screen);
    } catch (_) {
      return false; // cannot tell: assume visible
    }
  }

  // The routes of the Navigator stack, bottom to top, as the tree lists them.
  // Cached for a few milliseconds: _isVisible runs for every candidate element of
  // one lookup, and the stack cannot change between them.
  List<ModalRoute<dynamic>>? _stackCache;
  int _stackStamp = -1;

  List<ModalRoute<dynamic>> _routeStack() {
    final now = DateTime.now().microsecondsSinceEpoch;
    final cached = _stackCache;
    if (cached != null && now - _stackStamp < 5000) return cached;
    final routes = <ModalRoute<dynamic>>[];
    final root = WidgetsBinding.instance.rootElement;
    void visit(Element e) {
      final w = e.widget;
      if (w is InheritedWidget && w.runtimeType.toString() == '_ModalScopeStatus') {
        final r = (w as dynamic).route;
        if (r is ModalRoute && !routes.contains(r)) routes.add(r);
      }
      e.visitChildren(visit);
    }
    if (root != null) {
      try {
        visit(root);
      } catch (_) {}
    }
    _stackCache = routes;
    _stackStamp = now;
    return routes;
  }

  /// A route that is not current is still on screen when every route above it
  /// is see-through (a dialog, a bottom sheet, a popup): the page underneath
  /// stays painted, and so does what it shows — a SnackBar, say. Routes under
  /// an opaque page are hidden and stay excluded (PT-03).
  bool _routeOnScreen(ModalRoute<dynamic> route) {
    final stack = _routeStack();
    final i = stack.indexOf(route);
    if (i < 0) return false;
    for (var j = i + 1; j < stack.length; j++) {
      if (stack[j].opaque) return false;
    }
    return true;
  }

  Element? _parentElement(Element element) {
    Element? parent;
    element.visitAncestorElements((e) {
      parent = e;
      return false; // stop after first ancestor
    });
    return parent;
  }

  void walkTree(void Function(Element) visitor) {
    final rootElement = WidgetsBinding.instance.rootElement;
    if (rootElement == null) return;
    _visitElement(rootElement, visitor);
  }

  void _visitElement(Element element, void Function(Element) visitor) {
    // Skip subtrees rooted at Offstage or Visibility(visible: false)
    final widget = element.widget;
    if (widget is Offstage && widget.offstage) return;
    if (widget is Visibility && !widget.visible) return;
    visitor(element);
    element.visitChildren((child) => _visitElement(child, visitor));
  }

  /// FP-13: a short snapshot of what a user could currently see — visible
  /// text strings and string-valued ValueKeys — for failure diagnostics.
  /// A timed-out step used to report only "context deadline exceeded", leaving
  /// the failure screenshot as the sole clue to what the screen showed.
  /// Bounded by [max] per list so the message stays one readable line.
  Map<String, List<String>> visibleSummary({int max = 25}) {
    final texts = <String>[];
    final keys = <String>[];
    walkTree((e) {
      if (texts.length >= max && keys.length >= max) return;
      final widget = e.widget;
      final key = widget.key;
      if (key is ValueKey<String> && keys.length < max && !keys.contains(key.value) && _isVisible(e)) {
        keys.add(key.value);
      }
      if (texts.length < max && (widget is Text || widget is EditableText)) {
        final text = widget is Text
            ? (widget.data ?? widget.textSpan?.toPlainText() ?? '')
            : (widget as EditableText).controller.text;
        final trimmed = text.trim();
        if (trimmed.isNotEmpty && !texts.contains(trimmed) && _isVisible(e)) {
          texts.add(trimmed);
        }
      }
    });
    return {'texts': texts, 'keys': keys};
  }

  /// Returns all element info for a given selector (used by dump_tree).
  List<Map<String, dynamic>> findAll(Map<String, dynamic> sel) {
    final elements = findElements(sel);
    return elements.map((e) => _elementInfo(e)).toList();
  }

  /// Returns the on-screen pixel bounding box of the first element matching
  /// [sel] ({x, y, width, height}, device pixels, top-left origin), or null
  /// if the selector matches nothing visible. Used to redact a widget's
  /// region from a screenshot before it's sent to an AI provider.
  Map<String, double>? boundsFor(Map<String, dynamic> sel) {
    final elements = findElements(sel);
    if (elements.isEmpty) return null;
    final renderObject = elements.first.renderObject;
    if (renderObject is! RenderBox) return null;
    final origin = renderObject.localToGlobal(Offset.zero);
    final size = renderObject.size;
    return {
      'x': origin.dx,
      'y': origin.dy,
      'width': size.width,
      'height': size.height,
    };
  }

  Map<String, dynamic> _elementInfo(Element e) {
    final rect = e.renderObject is RenderBox
        ? (e.renderObject as RenderBox)
            .localToGlobal(Offset.zero)
            .translate(
              (e.renderObject as RenderBox).size.width / 2,
              (e.renderObject as RenderBox).size.height / 2,
            )
        : Offset.zero;
    return {
      'type': e.widget.runtimeType.toString(),
      'key': e.widget.key?.toString(),
      'x': rect.dx,
      'y': rect.dy,
    };
  }
}
