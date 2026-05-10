/// `flutter_probe_gen` — build_runner code generator for FlutterProbe.
///
/// Reads `@ProbeSuite`, `@ProbeTest`, and `@ProbeRecipe` annotations from
/// `flutter_probe_annotation` and emits matching `.probe` test files into
/// `tests/generated/`.
///
/// You don't import anything from this library directly. Instead, add this
/// package as a `dev_dependency` in your Flutter app's `pubspec.yaml` and
/// run `dart run build_runner build`.
///
/// See <https://flutterprobe.dev> for the language reference.
library flutter_probe_gen;

// The Builder factory used by build_runner is exposed via lib/builder.dart
// (referenced from the package's build.yaml). This file exists primarily so
// pub.dev's "library name matches package name" rule is satisfied; nothing
// from it is meant to be imported by user code.
export 'builder.dart' show probeBuilder;
