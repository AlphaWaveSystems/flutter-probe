/// Selector types used by steps that target a widget. Every selector class
/// has a fully `const` constructor so it can be used inside annotations.
library;

/// Base type for all widget selectors.
sealed class Selector {
  const Selector();
}

/// Selects a widget by its visible text content.
///
/// Emits ProbeScript: `"value"`.
class TextSel extends Selector {
  final String value;
  const TextSel(this.value);
}

/// Selects a widget by its `ValueKey` or `Semantics.identifier`.
///
/// Emits ProbeScript: `#key`.
class IdSel extends Selector {
  final String key;
  const IdSel(this.key);
}

/// Selects a widget by its Dart type name (e.g. "ElevatedButton").
///
/// Emits ProbeScript: `WidgetType`.
class TypeSel extends Selector {
  final String widgetType;
  const TypeSel(this.widgetType);
}

/// A convenience selector accepted by every step that takes a target.
/// Provide one of `text` or `id` (or pass a `Selector` instance directly).
class Field extends Selector {
  final String? text;
  final String? id;
  const Field({this.text, this.id})
      : assert(text != null || id != null,
            'Field requires either text: or id:');
}

/// Selects the Nth occurrence of a text-matched widget, optionally inside a
/// container.
///
/// Emits: `1st "Item"` or `2nd "Item" in "List"`.
class Ordinal extends Selector {
  final int n;
  final String text;
  final String? container;
  const Ordinal(this.n, this.text, {this.container})
      : assert(n >= 1, 'Ordinal n must be >= 1');
}

/// Selects a widget positioned spatially relative to another anchor widget.
///
/// Emits: `"target" below "anchor"`.
class Below extends Selector {
  final String target;
  final String anchor;
  const Below(this.target, {required this.anchor});
}

/// Emits: `"target" above "anchor"`.
class Above extends Selector {
  final String target;
  final String anchor;
  const Above(this.target, {required this.anchor});
}

/// Emits: `"target" left of "anchor"`.
class LeftOf extends Selector {
  final String target;
  final String anchor;
  const LeftOf(this.target, {required this.anchor});
}

/// Emits: `"target" right of "anchor"`.
class RightOf extends Selector {
  final String target;
  final String anchor;
  const RightOf(this.target, {required this.anchor});
}

/// Selects a widget by text within a named container ("Email" in "LoginForm").
class InContainer extends Selector {
  final String text;
  final String container;
  const InContainer(this.text, {required this.container});
}
