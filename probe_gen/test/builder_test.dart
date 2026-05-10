/// Golden tests for [ProbeBuilder] — runs the builder against in-memory
/// Dart sources and compares the emitted `.probe` output to a checked-in
/// expected string.
library;

import 'dart:io';

import 'package:build/build.dart';
import 'package:build_test/build_test.dart';
import 'package:flutter_probe_gen/builder.dart';
import 'package:test/test.dart';

void main() {
  // Annotation package source — bridged into the test asset graph so the
  // analyzer can resolve `package:flutter_probe_annotation/...` imports
  // appearing in fixture files.
  Future<Map<String, String>> annotationPackageAssets() async {
    final dir = Directory('../probe_annotation');
    final assets = <String, String>{};
    await for (final entity in dir.list(recursive: true)) {
      if (entity is File && entity.path.endsWith('.dart')) {
        final relative = entity.path.replaceFirst('../probe_annotation/', '');
        if (relative.startsWith('lib/')) {
          assets['flutter_probe_annotation|$relative'] =
              await entity.readAsString();
        }
      }
    }
    return assets;
  }

  Future<void> runGolden(String fixtureName) async {
    final annotation = await annotationPackageAssets();
    await testBuilder(
      probeBuilder(BuilderOptions.empty),
      {
        ...annotation,
        'pkg|lib/$fixtureName.dart':
            await File('test/fixtures/$fixtureName.dart').readAsString(),
      },
      outputs: {
        'pkg|tests/generated/$fixtureName.probe':
            await File('test/fixtures/$fixtureName.probe.golden')
                .readAsString(),
      },
      rootPackage: 'pkg',
    );
  }

  test('emits a basic test from @ProbeSuite',
      () => runGolden('login_screen'));

  test('emits hooks from @ProbeSuite',
      () => runGolden('suite_with_hooks'));

  test('emits recipes and recipe invocations',
      () => runGolden('recipe_and_call'));

  test('emits loops and conditionals with nested bodies',
      () => runGolden('loops_and_conditionals'));

  test('skips files with no FlutterProbe annotations', () async {
    final annotation = await annotationPackageAssets();
    await testBuilder(
      probeBuilder(BuilderOptions.empty),
      {
        ...annotation,
        'pkg|lib/plain_widget.dart': '''
import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

// No annotations — builder should produce no output.
class PlainWidget {}
''',
      },
      // Empty `outputs` means we expect zero generated files.
      outputs: const {},
      rootPackage: 'pkg',
    );
  });
}
