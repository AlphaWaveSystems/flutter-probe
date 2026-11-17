/// The application id of the app this agent runs in (the Android package name, the iOS bundle id), for the `probe.ping` handshake and `GET /probe/status`.
///
/// Why: two apps on one device can both host the agent, and the CLI reaches the one that holds the port it forwards, which can be the other app. With the id the
/// CLI can tell which app answered (see the CLI's `project.app`) instead of finding out from a failed test.
///
/// Where it comes from, in this order, and nothing is guessed:
///   1. Android: the process name, the first field of `/proc/self/cmdline`. For an app that does not set `android:process` it is the package name.
///   2. the compile-time define `PROBE_APP_ID` (`--dart-define=PROBE_APP_ID=<id>`), for iOS or an app whose process has another name.
/// A value that is not a plain application id (letters, digits, `_`, `-` in dot-separated segments, at most 255 characters; a process name such as
/// `com.example.app:remote` is not) is left out, never repaired. The text comes from the app itself, so it is a statement of the app, not a proof.
library;

import 'dart:io' show File, Platform;

import 'package:flutter/foundation.dart' show visibleForTesting;

const String? _definedAppId = bool.hasEnvironment('PROBE_APP_ID')
    ? String.fromEnvironment('PROBE_APP_ID')
    : null;

final RegExp _appIdPattern =
    RegExp(r'^[A-Za-z0-9][A-Za-z0-9_-]*(\.[A-Za-z0-9][A-Za-z0-9_-]*)+$');

/// [value] when it is a plain application id, else null.
String? plainAppId(String? value) =>
    value != null && value.length <= 255 && _appIdPattern.hasMatch(value)
        ? value
        : null;

/// The first NUL-terminated field of the bytes of `/proc/self/cmdline` (the process name), or null when there is none.
String? processNameOf(List<int>? cmdline) {
  if (cmdline == null || cmdline.isEmpty) return null;
  var end = cmdline.indexOf(0);
  if (end < 0) end = cmdline.length;
  if (end == 0) return null;
  try {
    return String.fromCharCodes(cmdline.sublist(0, end));
  } catch (_) {
    return null;
  }
}

/// The application id from the process command line bytes ([cmdline], Android) and the build define ([define]); the process name wins.
String? probeAppIdFrom({String? define, List<int>? cmdline}) =>
    plainAppId(processNameOf(cmdline)) ?? plainAppId(define);

String? _cached;
bool _cachedRead = false;

/// The application id of this app, read once.
String? probeAppId() {
  if (_cachedRead) return _cached;
  _cachedRead = true;
  List<int>? cmdline;
  if (Platform.isAndroid) {
    try {
      cmdline = File('/proc/self/cmdline').readAsBytesSync();
    } catch (_) {
      cmdline = null;
    }
  }
  _cached = probeAppIdFrom(define: _definedAppId, cmdline: cmdline);
  return _cached;
}

/// Where the ping and the status endpoint read the id. A test replaces it.
@visibleForTesting
String? Function() probeAppIdReader = probeAppId;
