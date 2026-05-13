/// The build_runner [Builder] that scans annotated Dart libraries for
/// `@ProbeSuite` / `@ProbeTest` / `@ProbeRecipe` / `@ProbeCompositeTest`
/// and emits matching `.probe`
/// files into `tests/generated/`.
library;

import 'package:analyzer/dart/constant/value.dart';
import 'package:build/build.dart';
import 'package:source_gen/source_gen.dart';

import 'probe_emitter.dart';

class ProbeBuilder implements Builder {
  /// Maps `lib/<anything>.dart` → `tests/generated/<anything>.probe`.
  /// The `{{}}` capture is a build_runner pattern: it preserves whatever
  /// path component matched.
  @override
  final Map<String, List<String>> buildExtensions = const {
    'lib/{{}}.dart': ['tests/generated/{{}}.probe'],
  };

  @override
  Future<void> build(BuildStep buildStep) async {
    // Cheap source-text pre-check: skip files that don't even mention our
    // annotations. Avoids invoking the analyzer for every Dart file in the
    // user's project.
    final source = await buildStep.readAsString(buildStep.inputId);
    if (!source.contains('@ProbeSuite') &&
        !source.contains('@ProbeTest') &&
        !source.contains('@ProbeRecipe') &&
        !source.contains('@ProbeCompositeTest')) {
      return;
    }

    if (!await buildStep.resolver.isLibrary(buildStep.inputId)) {
      return;
    }
    final library = await buildStep.inputLibrary;
    final reader = LibraryReader(library);

    final emitter = ProbeEmitter();
    emitter.emitHeader(buildStep.inputId.path);

    for (final element in reader.allElements) {
      // analyzer 8+ wraps annotations in a Metadata object; pre-8 returned
      // a List<ElementAnnotation> directly. We're pinned to >=8.
      // ignore: deprecated_member_use
      for (final annotation in element.metadata.annotations) {
        final value = annotation.computeConstantValue();
        if (value == null) continue;
        final typeName = _typeNameOf(value);
        switch (typeName) {
          case 'ProbeSuite':
            emitter.emitSuite(value);
            break;
          case 'ProbeTest':
            emitter.emitTest(value);
            break;
          case 'ProbeRecipe':
            emitter.emitRecipe(value);
            break;
          case 'ProbeCompositeTest':
            emitter.emitCompositeTest(value);
            break;
        }
      }
    }

    if (emitter.isEmpty) {
      return;
    }

    final outputId = _outputIdFor(buildStep.inputId);
    await buildStep.writeAsString(outputId, emitter.toString());
  }

  /// `lib/screens/login.dart` → AssetId `tests/generated/screens/login.probe`.
  AssetId _outputIdFor(AssetId input) {
    final path = input.path;
    assert(path.startsWith('lib/') && path.endsWith('.dart'),
        'unexpected input path: $path');
    final relative = path.substring(4, path.length - 5);
    return AssetId(input.package, 'tests/generated/$relative.probe');
  }
}

/// Builder factory entry point referenced by `build.yaml`.
Builder probeBuilder(BuilderOptions _) => ProbeBuilder();

/// Returns the simple class name of a constant value's type, working across
/// the analyzer 7→8+ element-model migration. Returns the empty string if
/// the type name can't be determined.
String _typeNameOf(DartObject value) {
  final type = value.type;
  if (type == null) return '';
  // toString() yields e.g. "ProbeSuite" or "ProbeSuite*" depending on
  // analyzer version. Strip suffix decorations and any generic args.
  final repr = type.getDisplayString();
  final lt = repr.indexOf('<');
  final base = lt < 0 ? repr : repr.substring(0, lt);
  return base.replaceAll('*', '').replaceAll('?', '');
}
