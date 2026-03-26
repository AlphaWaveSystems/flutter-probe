# Troubleshooting

Common issues and solutions when working with FlutterProbe.

## Connection Issues

### "websocket: bad handshake"

**Cause**: The token used to connect doesn't match the agent's current token. This happens when:
- The app restarted and generated a new token
- A stale token file was read
- The agent process crashed and restarted

**Fix**:
1. Restart the app: `adb shell am force-stop <package>` or `xcrun simctl terminate <UDID> <bundleID>`
2. Relaunch and let the CLI read the fresh token
3. Check that no other process is connecting to the same port

### "Address already in use" (port 48686)

**Cause**: Another process is bound to port 48686. Common when switching between iOS and Android testing.

**Fix**:
1. Check what holds the port: `lsof -i :48686`
2. If it's an iOS app: `xcrun simctl terminate <UDID> <bundleID>`
3. If it's a stale adb forward: `adb forward --remove tcp:48686`
4. Use `--port <different-port>` to avoid conflicts

### "probelink: dial ... connection refused"

**Cause**: The agent isn't running or isn't listening on the expected port.

**Fix**:
1. Verify the app is running with ProbeAgent enabled (`--dart-define=PROBE_AGENT=true`)
2. Check the agent log for errors: `adb logcat | grep PROBE` or `xcrun simctl spawn <UDID> log show --last 30s --predicate 'eventMessage CONTAINS "PROBE"'`
3. Verify the port matches: `--port` flag must match `agent.port` in probe.yaml

## iOS-Specific Issues

### Notification Permission Dialog Blocks Tests

**Cause**: `xcrun simctl privacy grant` does not support notification permissions. The app's `UNUserNotificationCenter.requestAuthorization()` triggers a native dialog that blocks Flutter rendering.

**Fix**: Guard notification permission requests in the app:
```dart
const probeEnabled = bool.fromEnvironment('PROBE_AGENT');
if (!probeEnabled) {
  await requestNotificationPermission();
}
```

### "No renderable layer for screenshot"

**Cause**: The Flutter rendering pipeline isn't active. Usually because:
- A native system dialog is covering the Flutter view
- The app hasn't finished initializing
- The app crashed during startup

**Fix**:
1. Dismiss any system dialogs (notification, tracking, etc.)
2. Add longer `wait` steps after app launch
3. Check app logs for crash traces

### `clear app data` Doesn't Clear Login State (iOS)

**Cause**: The app stores credentials in the iOS Keychain, which persists outside the app's data container.

**Fix**: FlutterProbe now resets the simulator Keychain during `clear app data` on iOS. If you still see persisted state, check if the app uses other persistence mechanisms (e.g., shared app groups, iCloud).

### Token Not Found on iOS

**Cause**: The CLI can't find the agent's token file. The token is written to the app container's `tmp/probe/token`, not the device-level tmp.

**Fix**:
1. Verify the app is running: `xcrun simctl list devices booted`
2. Check the container path: `xcrun simctl get_app_container <UDID> <bundleID> data`
3. Read the token: `cat <container>/tmp/probe/token`
4. Ensure `PROBE_AGENT=true` was passed during build

### Physical iOS Device — Connection Drops via USB

**Cause**: USB-C cables cause intermittent drops when the device switches between charging and data transfer modes. This kills the iproxy tunnel.

**Fix**: Use **WiFi mode** instead of USB to eliminate drops entirely:
```bash
# Build with WiFi enabled
flutter build ios --profile --dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true

# Run over WiFi (find token in app console logs)
probe test tests/ --host <device-ip> --token <probe-token>
```

If you must use USB: FlutterProbe v0.5.0+ uses HTTP POST transport (not WebSocket) for physical devices, with auto-reconnect (up to 2 retries per step). Use a USB-A cable if available — no charge/data switching issue.

### Physical iOS Device — "devicectl launch: not installed"

**Cause**: Bundle ID mismatch between `probe.yaml` and the installed app. Debug builds may use a `.dev` suffix.

**Fix**: Check the installed bundle ID: `xcrun devicectl device info apps --device <UDID> | grep <appname>` and update `project.app` in `probe.yaml` to match.

### Physical iOS Device — `tap #key` Not Working

**Cause**: Widgets matched by `Semantics(identifier:)` may not forward gestures properly if pointer IDs collide with real touches.

**Fix**: Update to FlutterProbe v0.5.0+ which uses unique pointer IDs starting at 900 to avoid collisions.

## Android-Specific Issues

### Permission Dialogs Block Tests

**Cause**: The app requests runtime permissions that trigger native Android dialogs.

**Fix**:
1. Use `-y` flag to auto-grant permissions before app launch
2. Or grant specific permissions: `allow permission "camera"` in ProbeScript
3. For CI, FlutterProbe pre-grants all known permissions via `adb shell pm grant`

### `clear app data` Takes Long to Reconnect

**Cause**: After `pm clear`, the app is force-stopped. Relaunching via `monkey` command and reconnecting takes time.

**Fix**:
1. Increase `--reconnect-delay` (default 2s)
2. Increase `--token-timeout` if the app has slow initialization
3. Check if the app has heavy startup work (Firebase, large databases)

## Test Writing Issues

### "Expected to see ... but it was not found"

**Cause**: The text doesn't exist in the widget tree at the time of the check.

**Fix**:
1. Add `wait until "text" appears` before `see "text"` for dynamic content
2. Use `wait N seconds` if the content needs time to load
3. Check the exact text — it's case-sensitive and must match the widget's text exactly
4. Use `dump the widget tree` to see what's actually rendered

### Tags Not Being Filtered

**Cause**: Tags must be on the same line as `test "name"` or on the next indented line.

**Fix**: Correct syntax:
```
test "my test"
  @smoke @critical
  open the app
```
or:
```
test "my test" @smoke @critical
  open the app
```

## CI/CD Issues

### JSON Output Not Parseable When Piped

**Cause**: When using `--format json` without `-o`, status messages mix with JSON on stdout.

**Fix**: Always use `-o` for machine-readable output:
```bash
probe test tests/ --format json -o results.json
```

### Video Not Available After Cloud Session

**Cause**: Video encoding happens after the session is stopped. Fetching artifacts immediately returns no video.

**Fix**: FlutterProbe waits 3 seconds after stopping cloud sessions before fetching artifacts. If video is still missing, increase the wait or retry artifact collection.
