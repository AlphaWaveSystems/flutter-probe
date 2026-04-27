import 'package:flutter/widgets.dart';

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
        final matches = _findByText(text).where(_isVisible).toList();
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
  /// Offstage or Visibility widgets.
  bool _isVisible(Element element) {
    final ro = element.renderObject;
    if (ro == null || !ro.attached) return false;
    if (ro is RenderBox) {
      // Zero-size widgets are not visible
      if (ro.size == Size.zero) return false;
      // Check if the widget is actually painted (not behind Offstage etc.)
      if (!ro.hasSize) return false;
    }
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

  /// Returns all element info for a given selector (used by dump_tree).
  List<Map<String, dynamic>> findAll(Map<String, dynamic> sel) {
    final elements = findElements(sel);
    return elements.map((e) => _elementInfo(e)).toList();
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
