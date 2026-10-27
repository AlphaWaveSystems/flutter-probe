import 'dart:convert';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_probe_agent/src/device_view.dart';
import 'package:flutter_probe_agent/src/executor.dart';
import 'package:flutter_probe_agent/src/protocol.dart';

// probe.tap_at / long_press_at / swipe_at / type_text / dump_tree {bounds} /
// hit_test / probe.screenshot scale: the agent side of the device view.

Future<Map<String, dynamic>> _rpc(WidgetTester tester, String method, Map<String, dynamic> params) async {
  String? raw;
  final ex = ProbeExecutor((s) => raw = s);
  var done = false;
  final call = ex
      .dispatch(ProbeRequest(jsonrpc: '2.0', id: 1, method: method, params: params))
      .whenComplete(() => done = true);
  for (var i = 0; i < 100 && !done; i++) {
    await tester.pump(const Duration(milliseconds: 50));
  }
  await call;
  return jsonDecode(raw!) as Map<String, dynamic>;
}

Map<String, dynamic> _node(List nodes, String key) =>
    (nodes.firstWhere((n) => (n as Map)['key'] == key) as Map).cast<String, dynamic>();

void main() {
  testWidgets('tap_at presses the button under the point', (tester) async {
    var count = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Center(
          child: ElevatedButton(
            key: const ValueKey('inc'),
            onPressed: () => count++,
            child: const Text('Increment'),
          ),
        ),
      ),
    ));
    final c = tester.getCenter(find.byKey(const ValueKey('inc')));
    final res = await _rpc(tester, ProbeMethods.tapAt, {'x': c.dx, 'y': c.dy});
    expect(res['error'], isNull);
    expect(res['result'], {'ok': true});
    expect(count, 1);

    // A point on empty background taps nothing.
    await _rpc(tester, ProbeMethods.tapAt, {'x': 2, 'y': 2});
    expect(count, 1);
  });

  testWidgets('tap_at rejects missing coordinates', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold()));
    final res = await _rpc(tester, ProbeMethods.tapAt, {'x': 1});
    expect(res['error'], isNotNull);
  });

  testWidgets('long_press_at triggers onLongPress', (tester) async {
    var long = 0, short = 0;
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Center(
          child: GestureDetector(
            key: const ValueKey('lp'),
            onTap: () => short++,
            onLongPress: () => long++,
            child: const SizedBox(width: 100, height: 100, child: ColoredBox(color: Colors.red)),
          ),
        ),
      ),
    ));
    final c = tester.getCenter(find.byKey(const ValueKey('lp')));
    final res = await _rpc(tester, ProbeMethods.longPressAt, {'x': c.dx, 'y': c.dy, 'durationMs': 600});
    expect(res['error'], isNull);
    expect(long, 1);
    expect(short, 0);
  });

  testWidgets('swipe_at scrolls a ListView', (tester) async {
    final controller = ScrollController();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: ListView.builder(
          controller: controller,
          itemCount: 100,
          itemBuilder: (_, i) => SizedBox(height: 50, child: Text('row $i')),
        ),
      ),
    ));
    expect(controller.offset, 0);
    // Finger moves up: the list scrolls down.
    final res = await _rpc(tester, ProbeMethods.swipeAt, {'x1': 200, 'y1': 400, 'x2': 200, 'y2': 100, 'durationMs': 300});
    expect(res['error'], isNull);
    expect(controller.offset, greaterThan(100));
  });

  testWidgets('type_text inserts into the focused field', (tester) async {
    final a = TextEditingController(), b = TextEditingController(text: 'ab');
    final changes = <String>[];
    final focusB = FocusNode();
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          TextField(controller: a),
          TextField(controller: b, focusNode: focusB, onChanged: changes.add),
        ]),
      ),
    ));
    focusB.requestFocus();
    await tester.pump();
    b.selection = const TextSelection.collapsed(offset: 1);

    final res = await _rpc(tester, ProbeMethods.typeTextFocused, {'text': 'XY'});
    expect(res['error'], isNull);
    expect(b.text, 'aXYb');
    expect(a.text, isEmpty);
    expect(changes, ['aXYb']);
  });

  testWidgets('type_text without a focused field fails clearly', (tester) async {
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: TextField(controller: TextEditingController()))));
    final res = await _rpc(tester, ProbeMethods.typeTextFocused, {'text': 'x'});
    expect(jsonEncode(res['error']), contains('no text field has focus'));
  });

  testWidgets('dump_tree {bounds:true} returns a flat node list with logical bounds', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Align(
          alignment: Alignment.topLeft,
          child: Padding(
            padding: const EdgeInsets.all(20),
            child: Column(mainAxisSize: MainAxisSize.min, children: [
              ElevatedButton(key: const ValueKey('go'), onPressed: () {}, child: const Text('Go')),
              const SizedBox(key: ValueKey('box'), width: 40, height: 30),
              const SizedBox(width: 200, child: TextField(key: ValueKey('name'))),
            ]),
          ),
        ),
      ),
    ));
    final res = await _rpc(tester, ProbeMethods.dumpTree, {'bounds': true});
    expect(res['error'], isNull);
    final nodes = (res['result'] as Map)['nodes'] as List;
    expect(nodes, isNotEmpty);

    final go = _node(nodes, 'go');
    final goRect = tester.getRect(find.byKey(const ValueKey('go')));
    expect(go['type'], 'ElevatedButton');
    expect(go['interactive'], true);
    expect(go['text'], 'Go');
    expect(go['x'], closeTo(goRect.left, 0.01));
    expect(go['y'], closeTo(goRect.top, 0.01));
    expect(go['w'], closeTo(goRect.width, 0.01));
    expect(go['h'], closeTo(goRect.height, 0.01));

    final box = _node(nodes, 'box');
    expect(box['w'], 40);
    expect(box['h'], 30);
    expect(box['interactive'], false);

    expect(_node(nodes, 'name')['type'], 'TextField');

    // Parent indices point at an earlier node (or -1) and indices are sequential.
    for (var i = 0; i < nodes.length; i++) {
      final n = nodes[i] as Map;
      expect(n['i'], i);
      expect(n['p'] as int, lessThan(i));
    }

    // Without bounds the text dump is unchanged.
    final text = await _rpc(tester, ProbeMethods.dumpTree, {});
    expect((text['result'] as Map)['tree'], isA<String>());
  });

  testWidgets('hit_test returns the keyed button, null on empty space', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Align(
          alignment: Alignment.topLeft,
          child: ElevatedButton(key: const ValueKey('login'), onPressed: () {}, child: const Text('Log in')),
        ),
      ),
    ));
    final c = tester.getCenter(find.byKey(const ValueKey('login')));
    final res = await _rpc(tester, ProbeMethods.hitTest, {'x': c.dx, 'y': c.dy});
    expect(res['error'], isNull);
    final node = (res['result'] as Map)['node'] as Map;
    expect(node['key'], 'login');
    expect(node['type'], 'ElevatedButton');
    expect(node['interactive'], true);

    final empty = await _rpc(tester, ProbeMethods.hitTest, {'x': 700, 'y': 500});
    expect((empty['result'] as Map)['node'], isNull);
  });

  testWidgets('hit_test does not report a widget covered by a dialog barrier', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Builder(
        builder: (context) => Scaffold(
          body: Align(
            alignment: Alignment.topLeft,
            child: ElevatedButton(
              key: const ValueKey('under'),
              onPressed: () => showDialog<void>(
                context: context,
                builder: (_) => const AlertDialog(content: Text('hi')),
              ),
              child: const Text('open'),
            ),
          ),
        ),
      ),
    ));
    final c = tester.getCenter(find.byKey(const ValueKey('under')));
    await tester.tap(find.byKey(const ValueKey('under')));
    await tester.pumpAndSettle();
    final res = await _rpc(tester, ProbeMethods.hitTest, {'x': c.dx, 'y': c.dy});
    final node = (res['result'] as Map)['node'] as Map?;
    expect(node?['key'], isNot('under'));
  });

  testWidgets('collectProbeNodes gives an obscured field no text and a plain field its text', (tester) async {
    final email = TextEditingController(text: 'dev@example.invalid');
    final password = TextEditingController(text: 'quorvex-secret-123');
    addTearDown(email.dispose);
    addTearDown(password.dispose);
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: Column(children: [
          TextField(key: const ValueKey('email'), controller: email),
          TextField(key: const ValueKey('password'), controller: password, obscureText: true),
        ]),
      ),
    ));

    final fields = collectProbeNodes().where((n) => n.type == 'EditableText').toList();
    expect(fields, hasLength(2));
    expect(fields.where((n) => n.text == 'dev@example.invalid'), hasLength(1),
        reason: 'a plain field keeps its text in the node list');
    expect(fields.where((n) => n.text == null), hasLength(1),
        reason: 'an obscured field has no text in the node list');
    final dump = jsonEncode([for (final n in collectProbeNodes()) n.toJson()]);
    expect(dump, isNot(contains('quorvex-secret-123')));
  });

  testWidgets('probeScaledPng reads the size and downscales; metrics are logical', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: SizedBox()));
    final m = probeViewMetrics();
    expect(m.logicalWidth, 800);
    expect(m.logicalHeight, 600);
    expect(m.dpr, 3.0);

    // The full screenshot pipeline needs real frames, which a widget test does not
    // produce; the encode/scale step is exercised on a generated image instead.
    await tester.runAsync(() async {
      final recorder = ui.PictureRecorder();
      Canvas(recorder).drawRect(const Rect.fromLTWH(0, 0, 100, 60), Paint()..color = Colors.red);
      final img = await recorder.endRecording().toImage(100, 60);
      final png = (await img.toByteData(format: ui.ImageByteFormat.png))!.buffer.asUint8List();

      final full = await probeScaledPng(png, 1.0);
      expect((full.width, full.height), (100, 60));
      expect(identical(full.bytes, png), true);

      final half = await probeScaledPng(png, 0.5);
      expect(half.width, 50);
      expect(half.height, 30);
      expect(half.bytes.sublist(1, 4), [0x50, 0x4E, 0x47]);

      // Out-of-range scales are clamped to 0.1..1.0.
      expect((await probeScaledPng(png, 0.0)).width, 10);
      expect((await probeScaledPng(png, 5.0)).width, 100);
    });
  });
}
