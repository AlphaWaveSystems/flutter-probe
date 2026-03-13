import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart' show find, CommonFinders;

/// ProbeFinder translates ProbeLink SelectorParam JSON into Flutter finders.
class ProbeFinder {
  ProbeFinder._();
  static final ProbeFinder instance = ProbeFinder._();

  /// Returns a [Finder] for the given selector map.
  ///
  /// Selector kinds: text | id | type | ordinal | positional
  Finder forSelector(Map<String, dynamic> sel) {
    final kind = sel['kind'] as String? ?? 'text';
    final text = sel['text'] as String? ?? '';
    final ordinal = (sel['ordinal'] as num?)?.toInt() ?? 1;
    final container = sel['container'] as String? ?? '';

    switch (kind) {
      case 'text':
        return _byText(text);

      case 'id':
        // Strip leading #
        final key = text.startsWith('#') ? text.substring(1) : text;
        return find.byKey(ValueKey(key));

      case 'type':
        // Match by widget type name using a predicate
        return find.byWidgetPredicate(
          (w) => w.runtimeType.toString() == text,
          description: 'widget of type $text',
        );

      case 'ordinal':
        // e.g. 2nd "Add to Cart" button
        final base = _byText(text);
        return _atIndex(base, ordinal - 1); // ordinal is 1-based

      case 'positional':
        // "text" in "container"
        if (container.isNotEmpty) {
          final containerFinder = _byText(container);
          return find.descendant(
            of: containerFinder,
            matching: _byText(text),
          );
        }
        return _byText(text);

      default:
        return _byText(text);
    }
  }

  /// Finds the widget at [index] within [base].
  Finder _atIndex(Finder base, int index) {
    return find.byWidgetPredicate(
      (widget) {
        final matches = base.evaluate().toList();
        if (index >= matches.length) return false;
        final target = matches[index];
        return base.evaluate().any((e) => e == target && e.widget == widget);
      },
      description: 'element at index $index of $base',
    );
  }

  Finder _byText(String text) {
    // Try exact text first, then substring
    return find.byWidgetPredicate(
      (widget) {
        if (widget is Text) {
          return widget.data == text ||
              (widget.data?.contains(text) ?? false);
        }
        if (widget is RichText) {
          return widget.text.toPlainText().contains(text);
        }
        if (widget is EditableText) {
          return widget.controller.text.contains(text);
        }
        return false;
      },
      description: 'text "$text"',
    );
  }

  /// Returns all element info for a given selector (used by dump_tree).
  List<Map<String, dynamic>> findAll(Map<String, dynamic> sel) {
    final finder = forSelector(sel);
    final elements = finder.evaluate().toList();
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
