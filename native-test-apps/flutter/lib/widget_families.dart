import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';

/// The screens of the widget families (docs/compat/MATRIX.md). Each family is one screen, chosen at build time with
/// `--dart-define=PROBE_FIXTURE_WIDGETS=<family>`; the default surface of the fixture is unchanged. Every element carries both a
/// `Semantics(identifier:)` and a `ValueKey` of the same name (the fixture's convention), and every state a test checks is also shown as
/// text (`Terms: on`), so a test can assert what the widget did, not only that a tap went through.
///
/// A family listed here is a screen to test against, not a claim that FlutterProbe supports it: a matrix cell is changed only by a test
/// that ran (docs/compat/README.md, rule 2).
const widgetFamilies = <String>['selection', 'textinput'];

/// The screen of [family], or null when there is none.
Widget? widgetFamilyScreen(String family) => switch (family) {
  'selection' => const SelectionFamilyScreen(),
  'textinput' => const TextInputFamilyScreen(),
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
