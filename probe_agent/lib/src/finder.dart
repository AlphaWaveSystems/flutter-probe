import 'package:flutter/cupertino.dart' show CupertinoTextField;
import 'package:flutter/material.dart' show TextField, Tooltip;
import 'package:flutter/rendering.dart' show RenderIndexedStack;
import 'package:flutter/widgets.dart';

import 'textfold.dart';

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

/// The position of the child an [IndexedStack] shows, when [element] is the stack's own element; null for any other element.
/// -1 means it shows nothing (an index of null, or out of range of the children).
///
/// What hides the other children is the render object: `RenderIndexedStack` paints, hit-tests and reports semantics for the
/// child at its `index` only, and that is the same in every Flutter version. Some versions (3.44) also wrap each child in a
/// `Visibility(visible: false)`, which the rules below recognise; others (3.50) do not, and the child stays mounted, laid out
/// at its size and attached, so no other check sees it as hidden. The `DropdownButton` keeps its items in an `IndexedStack`, so
/// a closed one hides them the same way. An ordinary `Stack` shows all its children and is not touched.
int? probeIndexedStackShown(Element element) {
  if (element is! MultiChildRenderObjectElement) return null;
  final RenderObject ro;
  try {
    ro = element.renderObject;
  } catch (_) {
    return null;
  }
  if (ro is! RenderIndexedStack) return null;
  return ro.index ?? -1;
}

/// Whether [child] (a child element of [stack]) is under the child [stack] shows. True when [stack] is no IndexedStack.
bool probeIsShownChild(Element stack, Element child) {
  final shown = probeIndexedStackShown(stack);
  if (shown == null) return true;
  var i = 0;
  var position = -1;
  stack.visitChildren((c) {
    if (identical(c, child)) position = i;
    i++;
  });
  return position == shown;
}

/// ProbeFinder translates ProbeLink SelectorParam JSON into Flutter elements
/// by walking the live widget tree. Does NOT use flutter_test finders since
/// those require TestWidgetsFlutterBinding.
class ProbeFinder {
  ProbeFinder._();
  static final ProbeFinder instance = ProbeFinder._();

  /// Text fields seen obscured, keyed by controller so an entry does not keep a
  /// field alive. A password field that a show-password toggle reveals stays
  /// secret for the rest of the run.
  final Expando<bool> _everObscured = Expando<bool>('probe obscured field');

  /// The most keys remembered. A screen has a few password fields, so the cap is
  /// not reached in practice. Once it is, every keyed field counts as secret: the
  /// memory stays bounded, and the failure is toward hiding a value.
  static const int _maxObscuredKeys = 256;

  /// The keys (ValueKey values) of the fields seen obscured. A field that the app
  /// rebuilds under the same key, with a new controller, is still secret.
  final Set<String> _obscuredKeys = {};

  /// The number of keys remembered. The cap test reads it to pin the bound.
  @visibleForTesting
  int get obscuredKeyCount => _obscuredKeys.length;

  /// True for an [EditableText] that is obscured now, or was obscured at any
  /// earlier read, by its controller or by the key of its field. Its value is
  /// then never read into a summary or a message.
  ///
  /// [element] is the EditableText's element, because the field's key is not on
  /// the EditableText: a TextField gives it a GlobalKey, so the key is read from
  /// the field above it (see [_fieldKeyOf]).
  ///
  /// Limits, so they are not taken for guarantees: a field that is visible at
  /// every read and typed into is ordinary text, because nothing ever saw it
  /// obscured; a Text widget that mirrors a controller's value cannot be told
  /// apart from any other Text, so it is reported as text; two fields that share
  /// a key share this state; and Semantics identifiers are not used. Unkeyed
  /// fields, and fields whose key is not a ValueKey<String> (a GlobalKey, for
  /// example), are tracked per controller only, so such a field rebuilt with a
  /// new controller is not covered. Once [_maxObscuredKeys] keys are remembered,
  /// every keyed field counts as secret, whether or not it was seen obscured.
  bool isSecretEditable(Element element) {
    final w = element.widget as EditableText;
    final key = _fieldKeyOf(element);
    if (w.obscureText) {
      _everObscured[w.controller] = true;
      if (key != null && _obscuredKeys.length < _maxObscuredKeys) _obscuredKeys.add(key);
    }
    if (_everObscured[w.controller] == true) return true;
    return key != null && (_obscuredKeys.contains(key) || _obscuredKeys.length >= _maxObscuredKeys);
  }

  /// The string value of the ValueKey that names the field [element] belongs to:
  /// its own key, else the key of the nearest TextField, TextFormField or
  /// CupertinoTextField above it. Other keyed ancestors are containers, not the
  /// field, so they are not read: a keyed Column does not make its plain fields
  /// secret.
  String? _fieldKeyOf(Element element) {
    final own = element.widget.key;
    if (own is ValueKey<String>) return own.value;
    String? found;
    try {
      element.visitAncestorElements((ancestor) {
        final w = ancestor.widget;
        final isField = w is TextField || w is FormField<String> || w is CupertinoTextField;
        final key = w.key;
        if (isField && key is ValueKey<String>) {
          found = key.value;
          return false;
        }
        return true;
      });
    } catch (_) {
      // An element that is no longer in the tree cannot be walked: in debug builds
      // visitAncestorElements asserts. Its field then counts by controller only.
      return null;
    }
    return found;
  }

  /// Returns all [Element]s matching the given selector map.
  /// Only returns elements that are currently visible on screen
  /// (not behind Offstage, Visibility(false), or off-screen routes).
  bool _includeOffScreen = false;

  /// Like [findElements], but also returns widgets that are built yet laid out entirely
  /// off screen (a chip beyond the end of a horizontal list). Used by actions, which scroll
  /// such a target into view; assertions must not see them.
  List<Element> findElementsIncludingOffScreen(Map<String, dynamic> sel) {
    _includeOffScreen = true;
    try {
      return findElements(sel);
    } finally {
      _includeOffScreen = false;
    }
  }

  List<Element> findElements(Map<String, dynamic> sel) {
    _loose = sel['loose'] == true;
    try {
      return _findElements(sel);
    } finally {
      _loose = false;
    }
  }

  List<Element> _findElements(Map<String, dynamic> sel) {
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
              if (_matchesText(e, text) && _isVisible(e)) {
                results.add(e);
              }
            });
          }
          return _oncePerText(results);
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
      if (_matchesText(e, text)) {
        results.add(e);
      }
    });
    if (results.isNotEmpty) return _oncePerText(results);
    // Nothing shows this text: fall back to what the app labels for assistive
    // technology and hover help — a Semantics label or a Tooltip message (an
    // icon button has no Text, only a tooltip). Only when no visible text matches, so
    // a Semantics wrapper around a Text never doubles a count.
    walkTree((e) {
      final w = e.widget;
      if (w is Tooltip && w.message != null && _textHit(w.message!, text)) {
        results.add(e);
      } else if (w is Semantics && w.properties.label != null && _textHit(w.properties.label!, text)) {
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

  /// Text comparison: a plain substring by default; with `loose` matching (selector flag
  /// `loose: true`, from `--match-loose` / `defaults.match: loose`) case, accents, typographic
  /// apostrophes/dashes, full-width forms and whitespace are folded on both sides.
  bool _textHit(String haystack, String needle) =>
      _loose ? foldedContains(haystack, needle) : haystack.contains(needle);

  bool _loose = false;

  /// One element per visible text (issue #800): a [Text] builds a [RichText] inside it, and both match the same text, so `see exactly 1`
  /// found 2 and an ordinal could pick the inner one. A [RichText] whose nearest [Text] above it (the one that built it) is also in
  /// [matches] is left out; the [Text] stays (its render object is the same paragraph). A [RichText] of its own, and a `Text.rich` (which
  /// matches only through its [RichText]), are kept.
  List<Element> _oncePerText(List<Element> matches) {
    final texts = matches.where((e) => e.widget is Text).toSet();
    if (texts.isEmpty) return matches;
    return matches.where((e) {
      if (e.widget is! RichText) return true;
      Element? builtBy;
      e.visitAncestorElements((a) {
        if (a.widget is Text) {
          builtBy = a;
          return false;
        }
        return a.widget is! RichText; // stop at another RichText: this one is not the inner paragraph of a Text above it
      });
      return builtBy == null || !texts.contains(builtBy);
    }).toList();
  }

  /// Whether [e] is a widget that shows [text]. The value of an obscured field (a password) is never a match target: a selector that could
  /// find a field by the text it holds would let a test learn the value by asking about it, one prefix at a time. The field is still found by its
  /// key, its label or its hint, and a Text that mirrors the value is just text (see [isSecretEditable] for the limits).
  bool _matchesText(Element e, String text) {
    final widget = e.widget;
    if (widget is Text) {
      final data = widget.data;
      return data != null && (data == text || _textHit(data, text));
    }
    if (widget is RichText) {
      return _textHit(widget.text.toPlainText(), text);
    }
    if (widget is EditableText) {
      if (isSecretEditable(e)) return false;
      return _textHit(widget.controller.text, text);
    }
    return false;
  }

  /// True when an obscured field is on the screen. It does not depend on what was asked, so a failure message can say that the value of such a
  /// field is never searched without telling whether it would have matched.
  bool hasSecretField() {
    var found = false;
    walkTree((e) {
      if (!found && e.widget is EditableText && isSecretEditable(e)) found = true;
    });
    return found;
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
      if (!_includeOffScreen && _isOffScreen(ro)) return false;
    }
    final route = probeRouteOf(element);
    if (route != null && !route.isCurrent && !_routeOnScreen(route)) return false;
    // Walk up the tree to check for Offstage / Visibility ancestors
    // An IndexedStack shows one child: a widget under any other child is hidden (its render object paints nothing for it).
    Element? current = element;
    Element? below;
    while (current != null) {
      final widget = current.widget;
      if (widget is Offstage && widget.offstage) return false;
      if (widget is Visibility && !widget.visible) return false;
      if (below != null && !probeIsShownChild(current, below)) return false;
      below = current;
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

  /// [walkTree], except that it does not descend below an obscured
  /// [EditableText] (a password field). The field itself is still visited, so
  /// its own key can be reported, but nothing inside it is.
  void _walkTreePruningObscuredFields(void Function(Element) visitor) {
    final rootElement = WidgetsBinding.instance.rootElement;
    if (rootElement == null) return;
    _visitElement(rootElement, visitor, _isSecretField);
  }

  bool _isSecretField(Element e) {
    final w = e.widget;
    return w is EditableText && isSecretEditable(e);
  }

  /// Visits [element] and its descendants. When [skipChildrenOf] returns true
  /// for an element, that element is still visited but its children are not.
  void _visitElement(
    Element element,
    void Function(Element) visitor, [
    bool Function(Element)? skipChildrenOf,
  ]) {
    // Skip subtrees rooted at Offstage or Visibility(visible: false)
    final widget = element.widget;
    if (widget is Offstage && widget.offstage) return;
    if (widget is Visibility && !widget.visible) return;
    visitor(element);
    if (skipChildrenOf != null && skipChildrenOf(element)) return;
    // An IndexedStack walks only the child it shows.
    final shown = probeIndexedStackShown(element);
    var position = 0;
    element.visitChildren((child) {
      if (shown == null || position == shown) _visitElement(child, visitor, skipChildrenOf);
      position++;
    });
  }

  /// FP-13: a short snapshot of what a user could currently see — visible
  /// text strings and string-valued ValueKeys — for failure diagnostics.
  /// A timed-out step used to report only "context deadline exceeded", leaving
  /// the failure screenshot as the sole clue to what the screen showed.
  /// Bounded by [max] per list so the message stays one readable line.
  ///
  /// An obscured text field (an [EditableText] with `obscureText: true`, such as
  /// a password) contributes no text: neither its controller value nor anything
  /// beneath it. Every caller reads this one summary (the `visible_summary` RPC
  /// and the failure hints the executor builds), so the rule lives here.
  Map<String, List<String>> visibleSummary({int max = 25}) {
    final full = visibleSummaryFull(max: max);
    return {'texts': full['texts'] as List<String>, 'keys': full['keys'] as List<String>};
  }

  /// The `visible_summary` RPC answer: [visibleSummary]'s `texts` and `keys`, unchanged, and keys an older CLI ignores:
  /// - `editable_texts`: the text of every visible field that is not obscured, recorded whether or not a [Text] with the same text is
  ///   already in `texts`. `texts` is de-duplicated, so a [Text] "Sign In" first hid a field holding "Sign In", which a text selector
  ///   also matches.
  /// - `rich_texts`: the texts of visible [RichText]s that no [Text] and no [Icon] built (a [Text]'s own paragraph is already in
  ///   `texts`; an [Icon]'s is its glyph); a text selector matches them too.
  /// - `max`: the cap of each list; `counts`: how many distinct items each list saw before the cap; `truncated`: true when any list
  ///   saw more than `max`, so a reader knows a list is incomplete.
  /// Each list is de-duplicated by trimmed text, in tree order, and holds no empty text. An obscured field adds nothing, in any list.
  Map<String, dynamic> visibleSummaryFull({int max = 25}) {
    final texts = <String>{};
    final keys = <String>{};
    final editable = <String>{};
    final rich = <String>{};
    _walkTreePruningObscuredFields((e) {
      final widget = e.widget;
      final key = widget.key;
      if (key is ValueKey<String> && !keys.contains(key.value) && _isVisible(e)) {
        keys.add(key.value);
      }
      if (widget is Text) {
        final trimmed = (widget.data ?? widget.textSpan?.toPlainText() ?? '').trim();
        if (trimmed.isNotEmpty && !texts.contains(trimmed) && _isVisible(e)) texts.add(trimmed);
      } else if (widget is EditableText && !isSecretEditable(e)) {
        // An obscured field's value is never read, so it cannot reach the summary.
        final trimmed = widget.controller.text.trim();
        if (trimmed.isNotEmpty && _isVisible(e)) {
          texts.add(trimmed); // a set: a text already there stays once
          editable.add(trimmed);
        }
      } else if (widget is RichText && !_builtByText(e) && !_builtByIcon(e)) {
        final trimmed = widget.text.toPlainText().trim();
        if (trimmed.isNotEmpty && !rich.contains(trimmed) && _isVisible(e)) rich.add(trimmed);
      }
    });
    final lists = {'texts': texts, 'keys': keys, 'editable_texts': editable, 'rich_texts': rich};
    return {
      for (final entry in lists.entries) entry.key: entry.value.take(max).toList(),
      'max': max,
      'counts': {for (final entry in lists.entries) entry.key: entry.value.length},
      'truncated': lists.values.any((l) => l.length > max),
    };
  }

  /// Whether an [Icon] built [e] (a [RichText] holding the icon's glyph, a private-use character): the glyph is not a text a person reads
  /// or a test names, and a screen of icons would otherwise fill `rich_texts`.
  bool _builtByIcon(Element e) {
    var built = false;
    e.visitAncestorElements((a) {
      if (a.widget is Icon) {
        built = true;
        return false;
      }
      return a.widget is! RichText;
    });
    return built;
  }

  /// Whether a [Text] built [e] (a [RichText]): its nearest [Text] above it, with no other [RichText] between.
  bool _builtByText(Element e) {
    var built = false;
    e.visitAncestorElements((a) {
      if (a.widget is Text) {
        built = true;
        return false;
      }
      return a.widget is! RichText;
    });
    return built;
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
