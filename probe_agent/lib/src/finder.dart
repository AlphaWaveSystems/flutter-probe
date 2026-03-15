import 'package:flutter/semantics.dart';
import 'package:flutter/widgets.dart';

/// ProbeFinder translates ProbeLink SelectorParam JSON into Flutter elements
/// by walking the live widget tree. Does NOT use flutter_test finders since
/// those require TestWidgetsFlutterBinding.
class ProbeFinder {
  ProbeFinder._();
  static final ProbeFinder instance = ProbeFinder._();

  /// Returns all [Element]s matching the given selector map.
  List<Element> findElements(Map<String, dynamic> sel) {
    final kind = sel['kind'] as String? ?? 'text';
    final text = sel['text'] as String? ?? '';
    final ordinal = (sel['ordinal'] as num?)?.toInt() ?? 1;
    final container = sel['container'] as String? ?? '';

    switch (kind) {
      case 'text':
        return _findByText(text);

      case 'id':
        final key = text.startsWith('#') ? text.substring(1) : text;
        return _findByKey(key);

      case 'type':
        return _findByType(text);

      case 'ordinal':
        final matches = _findByText(text);
        if (ordinal > 0 && ordinal <= matches.length) {
          return [matches[ordinal - 1]];
        }
        return [];

      case 'positional':
        if (container.isNotEmpty) {
          final containers = _findByText(container);
          if (containers.isEmpty) return [];
          // Find text within the container element's subtree
          final results = <Element>[];
          for (final c in containers) {
            _visitElement(c, (e) {
              if (_matchesText(e.widget, text)) {
                results.add(e);
              }
            });
          }
          return results;
        }
        return _findByText(text);

      default:
        return _findByText(text);
    }
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

  void walkTree(void Function(Element) visitor) {
    final rootElement = WidgetsBinding.instance.rootElement;
    if (rootElement == null) return;
    _visitElement(rootElement, visitor);
  }

  void _visitElement(Element element, void Function(Element) visitor) {
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
