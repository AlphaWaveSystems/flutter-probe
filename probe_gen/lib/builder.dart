/// Public entry point for the `flutter_probe_gen` build_runner builder.
library;

import 'package:build/build.dart';

import 'src/probe_builder.dart';

/// Factory function referenced by `build.yaml` — build_runner calls this with
/// the user's [BuilderOptions] (from their own `build.yaml` if any) and
/// expects a [Builder] back.
Builder probeBuilder(BuilderOptions options) => ProbeBuilder();
