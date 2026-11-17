import 'dart:convert';

import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';

/// The screens of the widget families (docs/compat/MATRIX.md). Each family is one screen, chosen at build time with
/// `--dart-define=PROBE_FIXTURE_WIDGETS=<family>`; the default surface of the fixture is unchanged. Every element carries both a
/// `Semantics(identifier:)` and a `ValueKey` of the same name (the fixture's convention), and every state a test checks is also shown as
/// text (`Terms: on`), so a test can assert what the widget did, not only that a tap went through.
///
/// A family listed here is a screen to test against, not a claim that FlutterProbe supports it: a matrix cell is changed only by a test
/// that ran (docs/compat/README.md, rule 2).
const widgetFamilies = <String>[
  'selection',
  'textinput',
  'buttons',
  'navigation',
  'display',
  'lists',
  'dialogs',
];

/// The screen of [family], or null when there is none.
Widget? widgetFamilyScreen(String family) => switch (family) {
  'selection' => const SelectionFamilyScreen(),
  'textinput' => const TextInputFamilyScreen(),
  'buttons' => const ButtonsFamilyScreen(),
  'navigation' => const NavigationFamilyScreen(),
  'display' => const DisplayFamilyScreen(),
  'lists' => const ListsFamilyScreen(),
  'dialogs' => const DialogsFamilyScreen(),
  _ => null,
};

/// A widget whose identifier is both its semantics identifier and its key: [build] gets the key to put on the widget itself, so a `#id`
/// selector finds the control and not a wrapper around it.
Widget _s(String id, Widget Function(Key key) build) =>
    Semantics(identifier: id, child: build(ValueKey(id)));

Widget _state(String id, String text) => Semantics(
  identifier: id,
  child: Text(text, key: ValueKey(id)),
);

/// Checkbox, switch, radio group, slider and the Cupertino switch. State texts: `Terms: on|off`, `Notifications: on|off`,
/// `Size: small|medium|large`, `Volume: N`, `Cupertino switch: on|off`.
class SelectionFamilyScreen extends StatefulWidget {
  const SelectionFamilyScreen({super.key});

  @override
  State<SelectionFamilyScreen> createState() => _SelectionFamilyScreenState();
}

class _SelectionFamilyScreenState extends State<SelectionFamilyScreen> {
  bool _terms = false;
  bool _notifications = false;
  bool _cupertino = false;
  String _size = 'small';
  double _volume = 50;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Selection controls')),
      body: ListView(
        children: [
          _s(
            'wf_checkbox',
            (k) => CheckboxListTile(
              key: k,
              title: const Text('Accept terms'),
              value: _terms,
              onChanged: (v) => setState(() => _terms = v ?? false),
            ),
          ),
          _state('wf_checkbox_state', 'Terms: ${_terms ? 'on' : 'off'}'),
          _s(
            'wf_switch',
            (k) => SwitchListTile(
              key: k,
              title: const Text('Notifications'),
              value: _notifications,
              onChanged: (v) => setState(() => _notifications = v),
            ),
          ),
          _state(
            'wf_switch_state',
            'Notifications: ${_notifications ? 'on' : 'off'}',
          ),
          RadioGroup<String>(
            groupValue: _size,
            onChanged: (v) => setState(() => _size = v ?? _size),
            child: Column(
              children: [
                for (final s in const ['small', 'medium', 'large'])
                  _s(
                    'wf_radio_$s',
                    (k) => RadioListTile<String>(
                      key: k,
                      title: Text(s[0].toUpperCase() + s.substring(1)),
                      value: s,
                    ),
                  ),
              ],
            ),
          ),
          _state('wf_radio_state', 'Size: $_size'),
          _s(
            'wf_slider',
            (k) => Slider(
              key: k,
              min: 0,
              max: 100,
              value: _volume,
              onChanged: (v) => setState(() => _volume = v),
            ),
          ),
          _state('wf_slider_state', 'Volume: ${_volume.round()}'),
          ListTile(
            title: const Text('Cupertino switch'),
            trailing: _s(
              'wf_cupertino_switch',
              (k) => CupertinoSwitch(
                key: k,
                value: _cupertino,
                onChanged: (v) => setState(() => _cupertino = v),
              ),
            ),
          ),
          _state(
            'wf_cupertino_switch_state',
            'Cupertino switch: ${_cupertino ? 'on' : 'off'}',
          ),
        ],
      ),
    );
  }
}

/// A plain field, a validated form field, a password field, a multi-line field and a Cupertino field, each with a text that mirrors what
/// it holds (the password's mirror shows only its length). State texts: `Plain: <text>`, `Form: <text>`, `Form error: none|Required`,
/// `Password length: N`, `Lines: N`, `Cupertino: <text>`.
class TextInputFamilyScreen extends StatefulWidget {
  const TextInputFamilyScreen({super.key});

  @override
  State<TextInputFamilyScreen> createState() => _TextInputFamilyScreenState();
}

class _TextInputFamilyScreenState extends State<TextInputFamilyScreen> {
  final _formKey = GlobalKey<FormState>();
  String _plain = '';
  String _form = '';
  String _formError = 'none';
  String _password = '';
  String _multi = '';
  String _cupertino = '';

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Text input')),
      body: ListView(
        padding: const EdgeInsets.all(12),
        children: [
          _s(
            'wf_textfield',
            (k) => TextField(
              key: k,
              decoration: const InputDecoration(hintText: 'Plain text'),
              onChanged: (v) => setState(() => _plain = v),
            ),
          ),
          _state(
            'wf_textfield_state',
            'Plain: ${_plain.isEmpty ? '(empty)' : _plain}',
          ),
          Form(
            key: _formKey,
            child: Column(
              children: [
                _s(
                  'wf_formfield',
                  (k) => TextFormField(
                    key: k,
                    decoration: const InputDecoration(hintText: 'Form text'),
                    validator: (v) =>
                        (v == null || v.isEmpty) ? 'Required' : null,
                    onChanged: (v) => setState(() => _form = v),
                  ),
                ),
                _state(
                  'wf_formfield_state',
                  'Form: ${_form.isEmpty ? '(empty)' : _form}',
                ),
                _s(
                  'wf_validate',
                  (k) => ElevatedButton(
                    key: k,
                    onPressed: () => setState(
                      () => _formError = _formKey.currentState!.validate()
                          ? 'none'
                          : 'Required',
                    ),
                    child: const Text('Validate'),
                  ),
                ),
                _state('wf_form_error', 'Form error: $_formError'),
              ],
            ),
          ),
          _s(
            'wf_password',
            (k) => TextField(
              key: k,
              obscureText: true,
              decoration: const InputDecoration(hintText: 'Password'),
              onChanged: (v) => setState(() => _password = v),
            ),
          ),
          _state('wf_password_state', 'Password length: ${_password.length}'),
          _s(
            'wf_multiline',
            (k) => TextField(
              key: k,
              maxLines: 3,
              decoration: const InputDecoration(hintText: 'Several lines'),
              onChanged: (v) => setState(() => _multi = v),
            ),
          ),
          _state(
            'wf_multiline_state',
            'Lines: ${_multi.isEmpty ? 0 : _multi.split('\n').length}',
          ),
          _s(
            'wf_cupertino_field',
            (k) => CupertinoTextField(
              key: k,
              placeholder: 'Cupertino text',
              onChanged: (v) => setState(() => _cupertino = v),
            ),
          ),
          _state(
            'wf_cupertino_state',
            'Cupertino: ${_cupertino.isEmpty ? '(empty)' : _cupertino}',
          ),
        ],
      ),
    );
  }
}

/// Elevated, filled, outlined and text buttons, an icon button with a tooltip, a floating action button, a Cupertino button, a button with an icon, a disabled
/// button and one with both a tap and a long press. State texts (`Elevated: N`, ...) count the taps; labels are chosen so that no label is part of a state text.
class ButtonsFamilyScreen extends StatefulWidget {
  const ButtonsFamilyScreen({super.key});

  @override
  State<ButtonsFamilyScreen> createState() => _ButtonsFamilyScreenState();
}

class _ButtonsFamilyScreenState extends State<ButtonsFamilyScreen> {
  final _counts = <String, int>{
    'elevated': 0,
    'filled': 0,
    'outlined': 0,
    'text': 0,
    'icon': 0,
    'fab': 0,
    'cupertino': 0,
    'withicon': 0,
    'disabled': 0,
    'tap': 0,
    'long': 0,
  };

  void _bump(String k) => setState(() => _counts[k] = _counts[k]! + 1);

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Buttons')),
      floatingActionButton: _s(
        'wf_fab',
        (k) => FloatingActionButton(
          key: k,
          tooltip: 'Compose',
          onPressed: () => _bump('fab'),
          child: const Icon(Icons.edit),
        ),
      ),
      body: ListView(
        padding: const EdgeInsets.all(8),
        children: [
          _s(
            'wf_elevated',
            (k) => ElevatedButton(
              key: k,
              onPressed: () => _bump('elevated'),
              child: const Text('Press elevated'),
            ),
          ),
          _state('wf_elevated_state', 'Elevated: ${_counts['elevated']}'),
          _s(
            'wf_filled',
            (k) => FilledButton(
              key: k,
              onPressed: () => _bump('filled'),
              child: const Text('Press filled'),
            ),
          ),
          _state('wf_filled_state', 'Filled: ${_counts['filled']}'),
          _s(
            'wf_outlined',
            (k) => OutlinedButton(
              key: k,
              onPressed: () => _bump('outlined'),
              child: const Text('Press outlined'),
            ),
          ),
          _state('wf_outlined_state', 'Outlined: ${_counts['outlined']}'),
          _s(
            'wf_text_button',
            (k) => TextButton(
              key: k,
              onPressed: () => _bump('text'),
              child: const Text('Press text'),
            ),
          ),
          _state('wf_text_state', 'Text button: ${_counts['text']}'),
          _s(
            'wf_icon_button',
            (k) => IconButton(
              key: k,
              tooltip: 'Add item',
              icon: const Icon(Icons.add),
              onPressed: () => _bump('icon'),
            ),
          ),
          _state('wf_icon_state', 'Icon button: ${_counts['icon']}'),
          _state('wf_fab_state', 'FAB: ${_counts['fab']}'),
          _s(
            'wf_cupertino_button',
            (k) => CupertinoButton(
              key: k,
              onPressed: () => _bump('cupertino'),
              child: const Text('Press cupertino'),
            ),
          ),
          _state('wf_cupertino_state', 'Cupertino: ${_counts['cupertino']}'),
          _s(
            'wf_with_icon',
            (k) => ElevatedButton.icon(
              key: k,
              onPressed: () => _bump('withicon'),
              icon: const Icon(Icons.send),
              label: const Text('Send now'),
            ),
          ),
          _state('wf_with_icon_state', 'With icon: ${_counts['withicon']}'),
          _s(
            'wf_disabled',
            (k) => ElevatedButton(
              key: k,
              onPressed: null,
              child: const Text('Cannot press'),
            ),
          ),
          _state('wf_disabled_state', 'Disabled: ${_counts['disabled']}'),
          _s(
            'wf_long_press',
            (k) => ElevatedButton(
              key: k,
              onPressed: () => _bump('tap'),
              onLongPress: () => _bump('long'),
              child: const Text('Hold me'),
            ),
          ),
          _state(
            'wf_long_press_state',
            'Hold: tap ${_counts['tap']}, long ${_counts['long']}',
          ),
          const SizedBox(height: 80),
        ],
      ),
    );
  }
}

/// The app bar and the routes (docs/compat/NEXT.md, item 2): a `Scaffold` whose `AppBar` is titled "Inbox", with a search action (an
/// `IconButton` with the tooltip "Search"), and a button that pushes a second route titled "Message". State texts: `Queries: N` on the
/// first route, `From: Ada` on the second (no label is part of a state text: text selectors match substrings). The second route is a `MaterialPageRoute` (opaque), so the first one is under it.
class NavigationFamilyScreen extends StatefulWidget {
  const NavigationFamilyScreen({super.key});

  @override
  State<NavigationFamilyScreen> createState() => _NavigationFamilyScreenState();
}

class _NavigationFamilyScreenState extends State<NavigationFamilyScreen> {
  int _searches = 0;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Inbox'),
        actions: [
          _s(
            'wf_search',
            (k) => IconButton(
              key: k,
              tooltip: 'Search',
              icon: const Icon(Icons.search),
              onPressed: () => setState(() => _searches++),
            ),
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(12),
        children: [
          _state('wf_searches', 'Queries: $_searches'),
          _s(
            'wf_open_message',
            (k) => ElevatedButton(
              key: k,
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute<void>(
                  builder: (_) => Scaffold(
                    appBar: AppBar(title: const Text('Message')),
                    body: Padding(
                      padding: const EdgeInsets.all(12),
                      child: _state('wf_message_body', 'From: Ada'),
                    ),
                  ),
                ),
              ),
              child: const Text('Open message'),
            ),
          ),
        ],
      ),
    );
  }
}

/// Text and display (docs/compat/NEXT.md, items 1 and 3). The text part: a plain `Text`, a one-line `Text` that is cut with an ellipsis, a
/// `Text.rich` of two spans ("Hello " and a bold "world") and a `RichText` of two spans ("Painted " and "rich text"). The part without
/// text: an `Icon` with the semantic label "Favorite", an `Image` with the semantic label "Logo" (an inline PNG through `Image.memory`, so
/// the fixture needs no asset), a `CircleAvatar` showing "AB", and a `FlutterLogo` inside a `GestureDetector` that counts taps. State
/// text: `Taps: N` (the logo taps; no state text holds "Logo", "Favorite" or "AB", so a match on those comes from the widget).
class DisplayFamilyScreen extends StatefulWidget {
  const DisplayFamilyScreen({super.key});

  /// The text of the ellipsis line: chosen to be longer than a phone screen is wide, so it is painted cut (an inference from its length on
  /// the two devices used; the agent does not report the painted width).
  static const longLine =
      'Shipping address: 1600 Amphitheatre Parkway, Mountain View, California, United States of America';

  /// A 1×1 PNG, drawn at 48×48.
  static final pixel = base64Decode(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==',
  );

  @override
  State<DisplayFamilyScreen> createState() => _DisplayFamilyScreenState();
}

class _DisplayFamilyScreenState extends State<DisplayFamilyScreen> {
  int _logoTaps = 0;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Display')),
      body: ListView(
        padding: const EdgeInsets.all(12),
        children: [
          _s('wf_text', (k) => Text('Order total: 42.00', key: k)),
          _s(
            'wf_ellipsis',
            (k) => Text(
              DisplayFamilyScreen.longLine,
              key: k,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
          _s(
            'wf_rich',
            (k) => Text.rich(
              key: k,
              const TextSpan(
                text: 'Hello ',
                children: [
                  TextSpan(
                    text: 'world',
                    style: TextStyle(fontWeight: FontWeight.bold),
                  ),
                ],
              ),
            ),
          ),
          _s(
            'wf_richtext',
            (k) => RichText(
              key: k,
              text: TextSpan(
                style: DefaultTextStyle.of(context).style,
                text: 'Painted ',
                children: const [TextSpan(text: 'rich text')],
              ),
            ),
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              _s(
                'wf_icon',
                (k) => Icon(Icons.favorite, key: k, semanticLabel: 'Favorite'),
              ),
              const SizedBox(width: 12),
              _s(
                'wf_image',
                (k) => Image.memory(
                  DisplayFamilyScreen.pixel,
                  key: k,
                  width: 48,
                  height: 48,
                  fit: BoxFit.fill,
                  semanticLabel: 'Logo',
                ),
              ),
              const SizedBox(width: 12),
              _s(
                'wf_avatar',
                (k) => CircleAvatar(key: k, child: const Text('AB')),
              ),
              const SizedBox(width: 12),
              GestureDetector(
                onTap: () => setState(() => _logoTaps++),
                child: _s('wf_logo', (k) => FlutterLogo(key: k, size: 48)),
              ),
            ],
          ),
          _state('wf_logo_taps', 'Taps: $_logoTaps'),
        ],
      ),
    );
  }
}

/// Lists and scrolling (docs/compat/NEXT.md, items 4 and 5). The first screen is a `ListView.builder` of 200 rows ("Row 0" to "Row 199"), so
/// a far row is not built until the list is scrolled to it, with four buttons that push a page each: a `GridView.count` of 60 cells, a
/// `CustomScrollView` with a `SliverList` of 200 items, a `SingleChildScrollView` of 40 lines (not lazy: every line is built), and tiles
/// (`Card`s holding `ListTile`s, and a `CupertinoListTile`). Each page keeps its state text above the scrolling part, so it is on screen after
/// a scroll. State texts: `Tapped row: N|none`, `Tapped cell: N|none`, `Tapped item: N|none`, `Tapped bottom: yes|no`, `Tapped: none|Alice|Bob|Carol`,
/// `Long press: none|Alice|Bob`.
class ListsFamilyScreen extends StatefulWidget {
  const ListsFamilyScreen({super.key});

  @override
  State<ListsFamilyScreen> createState() => _ListsFamilyScreenState();
}

class _ListsFamilyScreenState extends State<ListsFamilyScreen> {
  String _row = 'none';

  void _open(String title, Widget body) => Navigator.of(context).push(
    MaterialPageRoute<void>(
      builder: (_) => Scaffold(
        appBar: AppBar(title: Text(title)),
        body: body,
      ),
    ),
  );

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Lists')),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 12),
            child: _state('wf_list_state', 'Tapped row: $_row'),
          ),
          Wrap(
            spacing: 8,
            children: [
              _s(
                'wf_open_grid',
                (k) => TextButton(
                  key: k,
                  onPressed: () => _open('Grid', const _GridPage()),
                  child: const Text('Open grid'),
                ),
              ),
              _s(
                'wf_open_sliver',
                (k) => TextButton(
                  key: k,
                  onPressed: () => _open('Sliver list', const _SliverPage()),
                  child: const Text('Open sliver list'),
                ),
              ),
              _s(
                'wf_open_scrollview',
                (k) => TextButton(
                  key: k,
                  onPressed: () =>
                      _open('Scroll view', const _ScrollViewPage()),
                  child: const Text('Open scroll view'),
                ),
              ),
              _s(
                'wf_open_tiles',
                (k) => TextButton(
                  key: k,
                  onPressed: () => _open('Tiles', const _TilesPage()),
                  child: const Text('Open tiles'),
                ),
              ),
            ],
          ),
          Expanded(
            child: _s(
              'wf_list',
              (k) => ListView.builder(
                key: k,
                itemCount: 200,
                itemBuilder: (context, i) => _s(
                  'wf_row_$i',
                  (rk) => ListTile(
                    key: rk,
                    title: Text('Row $i'),
                    onTap: () => setState(() => _row = '$i'),
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _GridPage extends StatefulWidget {
  const _GridPage();

  @override
  State<_GridPage> createState() => _GridPageState();
}

class _GridPageState extends State<_GridPage> {
  String _cell = 'none';

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        _state('wf_grid_state', 'Tapped cell: $_cell'),
        Expanded(
          child: _s(
            'wf_grid',
            (k) => GridView.count(
              key: k,
              crossAxisCount: 3,
              children: [
                for (var i = 0; i < 60; i++)
                  _s(
                    'wf_cell_$i',
                    (ck) => InkWell(
                      key: ck,
                      onTap: () => setState(() => _cell = '$i'),
                      child: Center(child: Text('Cell $i')),
                    ),
                  ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class _SliverPage extends StatefulWidget {
  const _SliverPage();

  @override
  State<_SliverPage> createState() => _SliverPageState();
}

class _SliverPageState extends State<_SliverPage> {
  String _item = 'none';

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        _state('wf_sliver_state', 'Tapped item: $_item'),
        Expanded(
          child: _s(
            'wf_sliver',
            (k) => CustomScrollView(
              key: k,
              slivers: [
                const SliverToBoxAdapter(
                  child: Padding(
                    padding: EdgeInsets.all(12),
                    child: Text('Sliver header'),
                  ),
                ),
                SliverList(
                  delegate: SliverChildBuilderDelegate(
                    (context, i) => _s(
                      'wf_item_$i',
                      (ik) => ListTile(
                        key: ik,
                        title: Text('Item $i'),
                        onTap: () => setState(() => _item = '$i'),
                      ),
                    ),
                    childCount: 200,
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class _ScrollViewPage extends StatefulWidget {
  const _ScrollViewPage();

  @override
  State<_ScrollViewPage> createState() => _ScrollViewPageState();
}

class _ScrollViewPageState extends State<_ScrollViewPage> {
  bool _bottom = false;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        _state(
          'wf_scrollview_state',
          'Tapped bottom: ${_bottom ? 'yes' : 'no'}',
        ),
        Expanded(
          child: _s(
            'wf_scrollview',
            (k) => SingleChildScrollView(
              key: k,
              child: Column(
                children: [
                  for (var i = 0; i < 40; i++)
                    Padding(
                      padding: const EdgeInsets.all(12),
                      child: Text('Line $i'),
                    ),
                  _s(
                    'wf_bottom',
                    (bk) => ElevatedButton(
                      key: bk,
                      onPressed: () => setState(() => _bottom = true),
                      child: const Text('Bottom button'),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class _TilesPage extends StatefulWidget {
  const _TilesPage();

  @override
  State<_TilesPage> createState() => _TilesPageState();
}

class _TilesPageState extends State<_TilesPage> {
  String _tapped = 'none';
  String _long = 'none';

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(12),
      children: [
        _state('wf_tiles_tapped', 'Tapped: $_tapped'),
        _state('wf_tiles_long', 'Long press: $_long'),
        Card(
          child: _s(
            'wf_alice',
            (k) => ListTile(
              key: k,
              title: const Text('Alice'),
              subtitle: const Text('Online'),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => setState(() => _tapped = 'Alice'),
              onLongPress: () => setState(() => _long = 'Alice'),
            ),
          ),
        ),
        Card(
          child: _s(
            'wf_bob',
            (k) => ListTile(
              key: k,
              title: const Text('Bob'),
              subtitle: const Text('Away'),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => setState(() => _tapped = 'Bob'),
              onLongPress: () => setState(() => _long = 'Bob'),
            ),
          ),
        ),
        CupertinoListSection.insetGrouped(
          children: [
            _s(
              'wf_carol',
              (k) => CupertinoListTile(
                key: k,
                title: const Text('Carol'),
                subtitle: const Text('Busy'),
                trailing: const Icon(CupertinoIcons.chevron_right),
                onTap: () => setState(() => _tapped = 'Carol'),
              ),
            ),
          ],
        ),
      ],
    );
  }
}

/// Dialogs and sheets (docs/compat/NEXT.md, items 8 and 9): an `AlertDialog` ("Delete item?", actions "Cancel" and "Delete"), a
/// `CupertinoAlertDialog` ("Discard changes?", actions "Keep editing" and "Discard"), a `CupertinoActionSheet` ("Share", actions "Copy link" and
/// "Send", cancel button "Cancel"), a modal bottom sheet ("Option one", "Option two") and a `SnackBar` ("Saved", action "Undo", 4 seconds). A button
/// of the page opens a Cover page: a "Page button" at the exact centre of the page, a "Top button" at its top and a tall `AlertDialog` ("Tall dialog", a 240 dp body, action "Close") that
/// the page opens. The dialog is centred, so its non-interactive body spans the centre of the page whatever the screen is, and the Top button is above it: a tap on the first lands on
/// the dialog, a tap on the second on the barrier. State texts: `Dialog: none|cancelled|deleted`,
/// `Cupertino dialog: none|kept|discarded`, `Sheet: none|copy|send|cancelled`, `Bottom sheet: none|one|two`, `Snack: none|saved|undone`; on the Cover page `Page taps: N`, `Top taps: N`, `Cover dialog: none|closed`.
class DialogsFamilyScreen extends StatefulWidget {
  const DialogsFamilyScreen({super.key});

  @override
  State<DialogsFamilyScreen> createState() => _DialogsFamilyScreenState();
}

class _DialogsFamilyScreenState extends State<DialogsFamilyScreen> {
  String _dialog = 'none';
  String _cupertino = 'none';
  String _sheet = 'none';
  String _bottom = 'none';
  String _snack = 'none';

  Future<void> _openDialog() async {
    final result = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Delete item?'),
        content: const Text('This cannot be undone.'),
        actions: [
          _s(
            'wf_dialog_cancel',
            (k) => TextButton(
              key: k,
              onPressed: () => Navigator.of(context).pop('cancelled'),
              child: const Text('Cancel'),
            ),
          ),
          _s(
            'wf_dialog_delete',
            (k) => TextButton(
              key: k,
              onPressed: () => Navigator.of(context).pop('deleted'),
              child: const Text('Delete'),
            ),
          ),
        ],
      ),
    );
    if (mounted) setState(() => _dialog = result ?? 'none');
  }

  Future<void> _openCupertino() async {
    final result = await showCupertinoDialog<String>(
      context: context,
      builder: (context) => CupertinoAlertDialog(
        title: const Text('Discard changes?'),
        content: const Text('Your edits are lost.'),
        actions: [
          _s(
            'wf_cupertino_keep',
            (k) => CupertinoDialogAction(
              key: k,
              onPressed: () => Navigator.of(context).pop('kept'),
              child: const Text('Keep editing'),
            ),
          ),
          _s(
            'wf_cupertino_discard',
            (k) => CupertinoDialogAction(
              key: k,
              isDestructiveAction: true,
              onPressed: () => Navigator.of(context).pop('discarded'),
              child: const Text('Discard'),
            ),
          ),
        ],
      ),
    );
    if (mounted) setState(() => _cupertino = result ?? 'none');
  }

  Future<void> _openSheet() async {
    final result = await showCupertinoModalPopup<String>(
      context: context,
      builder: (context) => CupertinoActionSheet(
        title: const Text('Share'),
        actions: [
          CupertinoActionSheetAction(
            onPressed: () => Navigator.of(context).pop('copy'),
            child: const Text('Copy link'),
          ),
          CupertinoActionSheetAction(
            onPressed: () => Navigator.of(context).pop('send'),
            child: const Text('Send'),
          ),
        ],
        cancelButton: CupertinoActionSheetAction(
          onPressed: () => Navigator.of(context).pop('cancelled'),
          child: const Text('Cancel'),
        ),
      ),
    );
    if (mounted) setState(() => _sheet = result ?? 'none');
  }

  Future<void> _openBottom() async {
    final result = await showModalBottomSheet<String>(
      context: context,
      builder: (context) => Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          ListTile(
            title: const Text('Option one'),
            onTap: () => Navigator.of(context).pop('one'),
          ),
          ListTile(
            title: const Text('Option two'),
            onTap: () => Navigator.of(context).pop('two'),
          ),
        ],
      ),
    );
    if (mounted) setState(() => _bottom = result ?? 'none');
  }

  void _save() {
    setState(() => _snack = 'saved');
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: const Text('Saved'),
        duration: const Duration(seconds: 4),
        persist: false,
        action: SnackBarAction(
          label: 'Undo',
          onPressed: () => setState(() => _snack = 'undone'),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Dialogs')),
      body: ListView(
        padding: const EdgeInsets.all(12),
        children: [
          _state('wf_dialog_state', 'Dialog: $_dialog'),
          _state('wf_cupertino_state', 'Cupertino dialog: $_cupertino'),
          _state('wf_sheet_state', 'Sheet: $_sheet'),
          _state('wf_bottom_state', 'Bottom sheet: $_bottom'),
          _state('wf_snack_state', 'Snack: $_snack'),
          _s(
            'wf_open_dialog',
            (k) => ElevatedButton(
              key: k,
              onPressed: _openDialog,
              child: const Text('Open dialog'),
            ),
          ),
          _s(
            'wf_open_cupertino',
            (k) => ElevatedButton(
              key: k,
              onPressed: _openCupertino,
              child: const Text('Open Cupertino dialog'),
            ),
          ),
          _s(
            'wf_open_sheet',
            (k) => ElevatedButton(
              key: k,
              onPressed: _openSheet,
              child: const Text('Open action sheet'),
            ),
          ),
          _s(
            'wf_open_bottom',
            (k) => ElevatedButton(
              key: k,
              onPressed: _openBottom,
              child: const Text('Open bottom sheet'),
            ),
          ),
          _s(
            'wf_save',
            (k) => ElevatedButton(
              key: k,
              onPressed: _save,
              child: const Text('Save'),
            ),
          ),
          _s(
            'wf_open_cover',
            (k) => ElevatedButton(
              key: k,
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute<void>(builder: (_) => const _CoverPage()),
              ),
              child: const Text('Open cover page'),
            ),
          ),
        ],
      ),
    );
  }
}

class _CoverPage extends StatefulWidget {
  const _CoverPage();

  @override
  State<_CoverPage> createState() => _CoverPageState();
}

class _CoverPageState extends State<_CoverPage> {
  int _pageTaps = 0;
  int _topTaps = 0;
  String _dialog = 'none';

  Future<void> _openTall() async {
    final result = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Tall dialog'),
        content: const SizedBox(
          height: 240,
          child: Text(
            'A dialog with a tall body, so it covers the middle of the page.',
          ),
        ),
        actions: [
          _s(
            'wf_tall_close',
            (k) => TextButton(
              key: k,
              onPressed: () => Navigator.of(context).pop('closed'),
              child: const Text('Close'),
            ),
          ),
        ],
      ),
    );
    if (mounted) setState(() => _dialog = result ?? 'none');
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Cover')),
      body: Stack(
        children: [
          Align(
            alignment: Alignment.topCenter,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                _s(
                  'wf_top_button',
                  (k) => ElevatedButton(
                    key: k,
                    onPressed: () => setState(() => _topTaps++),
                    child: const Text('Top button'),
                  ),
                ),
                _state('wf_top_taps', 'Top taps: $_topTaps'),
                _state('wf_page_taps', 'Page taps: $_pageTaps'),
                _state('wf_cover_state', 'Cover dialog: $_dialog'),
              ],
            ),
          ),
          Center(
            child: _s(
              'wf_page_button',
              (k) => ElevatedButton(
                key: k,
                onPressed: () => setState(() => _pageTaps++),
                child: const Text('Page button'),
              ),
            ),
          ),
          Align(
            alignment: Alignment.bottomCenter,
            child: _s(
              'wf_open_tall',
              (k) => ElevatedButton(
                key: k,
                onPressed: _openTall,
                child: const Text('Open tall dialog'),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
