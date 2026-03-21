# Lessons Learned — E2E CLI Parameter Testing Session

Date: 2025-03-20 / 2025-03-21

## Overview

Comprehensive E2E testing of every FlutterProbe CLI parameter locally against Android emulator (`emulator-5554`) and iOS simulator (iPhone 16 Pro, `909F49AD-EE6A-4263-AFED-BAC0FC5C8B40`). 84 test cases across 6 phases, plus fixes for tag-based filtering, SauceLabs artifacts, iOS permission handling, and iOS reconnect flow.

---

## Key Findings

### 1. iOS Notification Permission Cannot Be Pre-Granted

**Problem**: `xcrun simctl privacy grant` does not support the `notifications` service. Apple's notification permission (`UNUserNotificationCenter.requestAuthorization`) is handled entirely by the app at runtime, and there is no simctl mechanism to pre-authorize it.

**Impact**: The native notification dialog blocks the Flutter rendering pipeline. When the dialog is on screen, the ProbeAgent can connect (WebSocket works), but `see` commands fail with "No renderable layer for screenshot" because the Flutter view is behind the system dialog.

**Fix**: The app must check `bool.fromEnvironment('PROBE_AGENT')` and skip notification permission requests when running under FlutterProbe. Example:
```dart
const probeEnabled = bool.fromEnvironment('PROBE_AGENT');
if (!probeEnabled) {
  await FirebaseMessagingHandler.instance.initialize();
}
```

**Takeaway**: Document this in onboarding guides for new apps. Any app integrating ProbeAgent should guard notification permission requests behind the `PROBE_AGENT` flag.

### 2. iOS Keychain Survives App Data Clearing

**Problem**: `simctl get_app_container` + deleting `Documents/`, `Library/`, `tmp/` does NOT clear Keychain entries. Apps that store login credentials in the Keychain (common with `flutter_secure_storage`) remain logged in after `clear app data`.

**Fix**: Added `xcrun simctl keychain <UDID> reset` to the iOS `ClearAppData` flow. This resets the entire simulator Keychain, which is acceptable for testing. Note: this affects ALL apps on the simulator, not just the target app.

**Takeaway**: iOS data clearing requires Keychain reset for tests that depend on a clean login state. Android's `pm clear` handles everything in one command; iOS needs the extra step.

### 3. iOS Token File Lives in the App Container, Not Device-Level tmp

**Problem**: The Dart ProbeAgent writes its token file to the app's sandbox (`<container>/tmp/probe/token`), not the device-level path (`~/Library/Developer/CoreSimulator/Devices/<UDID>/data/tmp/probe/token`). The CLI was looking at the wrong path.

**Fix**: Use `xcrun simctl get_app_container <UDID> <bundleID> data` to resolve the correct container path. Re-resolve periodically during polling in case the container changes (e.g., after data clear).

**Takeaway**: Always use `get_app_container` for app-specific paths. Never hardcode or assume the container UUID.

### 4. `simctl privacy grant` Terminates the Running App

**Problem**: Calling `xcrun simctl privacy grant <UDID> <service> <bundleID>` on a running app terminates it. If permissions are granted AFTER the app starts, the WebSocket connection drops and the CLI must reconnect.

**Fix**: Grant permissions BEFORE launching the app. The CLI now grants all supported iOS permissions (camera, microphone, location, photos, contacts-limited, calendar) before the first `simctl launch`.

**Takeaway**: Order matters: grant permissions -> launch app -> read token -> connect.

### 5. `log show` Fallback Returns Stale Tokens

**Problem**: When `ReadToken()` couldn't find the token file after 10 attempts, it fell back to parsing `simctl spawn ... log show --last 30s`. This could return tokens from PREVIOUS app instances, causing "bad handshake" errors on reconnect.

**Fix**: Removed the `log show` fallback entirely. The file-based token reading now polls continuously until timeout (up to 30s by default). Re-resolves the container path every 5 attempts to handle container changes.

**Takeaway**: Token reading should use the file as the single source of truth. Log parsing is unreliable for tokens because logs from previous app instances persist.

### 6. Port 48686 Conflict Between iOS and Android

**Problem**: When running iOS tests followed by Android tests, the iOS Digacel app holds port 48686 on localhost. Android's `adb forward tcp:48686 tcp:48686` fails with "Address already in use".

**Fix**: Terminate the iOS app before running Android tests when both platforms share the same port. In the health check script, iOS tests run first, then the iOS app is terminated before Android tests.

**Takeaway**: For parallel testing on multiple platforms, use different ports via `--port` flag. Sequential testing requires explicit cleanup between platforms.

### 7. `--format json` Without `-o` Mixes Status and JSON on stdout

**Problem**: When `--format json` is used without `-o <file>`, status messages ("Waiting for ProbeAgent token...", "Connected to ProbeAgent...") go to stdout along with the JSON output, making it unparseable by pipe consumers.

**Status**: Known limitation. Use `--format json -o <file>` for machine-readable output. Fixing this requires routing all status output to stderr when structured output targets stdout — a larger refactor.

### 8. `--format xml` and `--port 0` Are Silently Accepted

**Problem**: The CLI accepts `--format xml` (unknown format) without error — it falls back to terminal output. `--port 0` is accepted and the OS assigns a random port, which never matches the agent's port.

**Status**: Accepted as permissive behavior. These are edge cases that don't cause data loss. Adding strict validation is a future enhancement.

### 9. Tag-Based Test Selection Required Lexer + Parser Changes

**Problem**: The `@tag` syntax wasn't tokenized by the lexer (the `@` character was silently skipped). Additionally, when tags appeared on the next indented line after `test "name"`, consuming the INDENT for tag parsing left `parseBody()` without its expected INDENT token.

**Fix**:
- Added `lexTag()` method to tokenize `@tag` as IDENT tokens with the `@` prefix
- Added `indentConsumed` flag in `parseTestDef()` — when tags consume the INDENT, the body is parsed inline instead of via `parseBody()`

**Takeaway**: ProbeScript's indent-based syntax requires careful handling of token consumption. Any feature that looks ahead or consumes structural tokens (INDENT/DEDENT) must coordinate with downstream parsers.

### 10. SauceLabs Uses Separate Job IDs for RDC vs Appium

**Problem**: SauceLabs Real Device Cloud (RDC) assigns its own job UUIDs that are completely unrelated to Appium session IDs. Artifacts (video, screenshots) are available via the RDC API using the RDC job ID, not the Appium session ID.

**Fix**: After the Appium session completes, query `/v1/rdc/jobs` filtered by the Appium session ID to find the corresponding RDC job ID. Then fetch artifacts from `/v1/rdc/jobs/<rdcJobID>`.

**Takeaway**: Cloud provider artifact collection must happen AFTER stopping the session, not before. Video encoding takes a few seconds after session stop.

---

## Test Results Summary

| Phase | Description | Tests | Result |
|---|---|---|---|
| 1 | Offline (no device) | 20/20 | All pass |
| 2 | Android emulator | 31/31 | All pass |
| 3 | iOS simulator | 8/8 | All pass (after fixes) |
| 4 | Parameter combinations | 9/9 | All pass (after fixes) |
| 5 | Edge cases | 7/9 | 2 known limitations (JSON pipe, format validation) |
| 6 | Subcommands | 8/8 | All pass |
| **Total** | | **83/85** | **97.6% pass rate** |

## Files Changed

| File | Change |
|---|---|
| `internal/parser/lexer.go` | Added `lexTag()` for `@tag` tokenization |
| `internal/parser/parser.go` | Fixed tag parsing with `indentConsumed` flag |
| `internal/cli/test.go` | Pre-grant iOS/Android permissions before token read |
| `internal/device/permissions.go` | Removed `notifications` from iOS services |
| `internal/ios/simctl.go` | Fixed token path to use app container, added `KeychainReset()`, removed stale log fallback |
| `internal/runner/device_context.go` | Fixed `iosTokenPath()` to use container, added Keychain reset to `ClearAppData` |
| `internal/cloud/saucelabs.go` | Added screenshot URLs from RDC job detail |

## Recommendations

1. **App onboarding docs**: Require guarding notification permissions behind `PROBE_AGENT` flag
2. **JSON stdout fix**: Route status to stderr when `--format json` targets stdout (future PR)
3. **Format validation**: Add `--format` enum validation (terminal, json, junit) — reject unknown values
4. **Parallel port management**: Document multi-device port assignment in CI/CD guide
5. **iOS Keychain behavior**: Add a note in the ProbeScript docs that `clear app data` on iOS also resets the simulator Keychain
