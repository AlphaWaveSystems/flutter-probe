/// What the app's own build says about the Flutter and Dart that built it, for the `probe.ping` handshake.
///
/// The Flutter tool puts these values into every app build as compile-time defines (`FLUTTER_VERSION`, `FLUTTER_CHANNEL`,
/// `FLUTTER_FRAMEWORK_REVISION`, `FLUTTER_DART_VERSION`); the framework exposes them as `FlutterVersion`. They are read here
/// directly, without that class, so this package compiles on every Flutter it supports and a build that does not set them
/// simply has none. They describe the APP's build, not the Flutter that published this package.
///
/// Nothing is guessed: a value that is missing, empty or not plain version text is left out, never replaced by a stand-in.
/// The framework's own documentation notes that the values can be unreliable for a fork or a custom embedder.
library;

import 'dart:io' show Platform;

const String? _flutterVersion = bool.hasEnvironment('FLUTTER_VERSION')
    ? String.fromEnvironment('FLUTTER_VERSION')
    : null;
const String? _flutterChannel = bool.hasEnvironment('FLUTTER_CHANNEL')
    ? String.fromEnvironment('FLUTTER_CHANNEL')
    : null;
const String? _frameworkRevision =
    bool.hasEnvironment('FLUTTER_FRAMEWORK_REVISION')
        ? String.fromEnvironment('FLUTTER_FRAMEWORK_REVISION')
        : null;
const String? _dartVersion = bool.hasEnvironment('FLUTTER_DART_VERSION')
    ? String.fromEnvironment('FLUTTER_DART_VERSION')
    : null;

/// Letters, digits, dots, dashes and plus signs, at most 64 characters: what a version, a channel or a short revision looks like.
final RegExp _plainVersionText = RegExp(r'^[A-Za-z0-9.+-]{1,64}$');

/// [value] when it is plain version text, else null.
String? _plain(String? value) =>
    value != null && _plainVersionText.hasMatch(value) ? value : null;

/// The Dart version a `Platform.version` string starts with ("3.12.2 (stable) (...) on "macos_arm64""), or null.
String? dartVersionOf(String? platformVersion) {
  if (platformVersion == null) return null;
  final first = platformVersion.trim().split(RegExp(r'\s+')).first;
  return _plain(first);
}

/// The fields `probe.ping` adds for the build: `flutter_version`, `flutter_channel`, `framework_revision` and `dart_version`, each only when
/// it is known and plain. The Dart version falls back to the running VM's own ([platformVersion]) when the build did not set it.
Map<String, String> probeBuildInfoFrom({
  String? flutterVersion,
  String? flutterChannel,
  String? frameworkRevision,
  String? dartVersion,
  String? platformVersion,
}) {
  final out = <String, String>{};
  void put(String key, String? value) {
    final v = _plain(value);
    if (v != null) out[key] = v;
  }

  put('flutter_version', flutterVersion);
  put('flutter_channel', flutterChannel);
  put('framework_revision', frameworkRevision);
  put('dart_version', _plain(dartVersion) ?? dartVersionOf(platformVersion));
  return out;
}

/// The build info of this app: the compile-time defines of its build and the running VM.
Map<String, String> probeBuildInfo() => probeBuildInfoFrom(
      flutterVersion: _flutterVersion,
      flutterChannel: _flutterChannel,
      frameworkRevision: _frameworkRevision,
      dartVersion: _dartVersion,
      platformVersion: Platform.version,
    );
