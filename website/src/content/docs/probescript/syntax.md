---
title: ProbeScript Syntax
description: Complete reference for ProbeScript — tests, selectors, assertions, gestures, waits, conditionals, and loops.
---

ProbeScript is a natural language test syntax with indent-based blocks (like Python). Test files use the `.probe` extension.

## Tests

Every test starts with `test` followed by a quoted name, with steps indented below:

```
test "user sees welcome screen"
  open the app
  wait 3 seconds
  see "Welcome"
  don't see "Error"
```

### Tags

Add tags with `@` after the test declaration:

```
test "critical login flow"
  @smoke @critical
  open the app
  tap "Sign In"
  see "Dashboard"
```

Run tagged tests with `probe test tests/ --tag smoke`.

## Selectors

ProbeScript supports multiple strategies for identifying widgets:

| Selector | Syntax | Example |
|----------|--------|---------|
| Text match | `"text"` | `tap "Submit"` |
| Widget key | `#keyName` | `tap #loginButton` |
| Widget type | `<TypeName>` | `tap <ElevatedButton>` |
| Ordinal | `1st "Item"`, `2nd "Item"` | `tap 2nd "Add"` |
| Positional | `"text" in "Container"` | `tap "Edit" in "Settings"` |

## Text Input

```
type "hello@world.com" into "Email"
type "secret123" into the "Password" field
```

## Assertions

```
see "Dashboard"                    # text is visible
don't see "Error"                  # text is NOT visible (dont see "Error" also works)
see 3 "Item"                       # exactly 3 matches
see "Submit" is enabled            # widget state
see "Terms" is checked             # checkbox state
see "Price" contains "$9.99"       # partial text match
```

## Gestures

```
tap "Button"
double tap "Image"
long press "Item"
swipe left
swipe up on "Card"
scroll down
scroll up on "ListView"
scroll down until "Rate this app" appears      # keep scrolling until it is on screen
scroll until #share_button is visible           # direction defaults to down
drag "Item A" to "Item B"
```

`scroll down` reveals **later** content (the list offset grows); `scroll up` goes back toward the top.
`scroll ... until <target> appears` scrolls half a viewport at a time — lists build rows lazily, so
the target only exists once enough has been scrolled — and then brings the target fully on screen.
It stops early when the list can't move further, and fails with the visible texts if the target never
shows up. It is the equivalent of Maestro's `scrollUntilVisible`. Use `on "List"` to pick which
scrollable when a screen has several.

## Text matching is by substring

A quoted text selector (`see "0 ml"`, `tap "Save"`, `wait until "Done" appears`) matches any widget whose text
**contains** it: `see "0 ml"` passes while only "250 ml" is on screen. For an exact check use an anchored pattern,
`see "0 ml" matching "^0 ml$"` (some matched widget's text must match the regular expression), or select by key
(`see #counter_text`). Before 0.16.8 the `matching` pattern was accepted but never applied.

## Wait Commands

```
wait 5 seconds
wait until "Dashboard" appears
wait until "Loading" disappears
wait until any of "Got it", "Login", "Home" appears   # whichever shows first
wait until "0 ml" appears matching "^0 ml$"            # exact text (selectors match substrings)
wait for the page to load
wait for network idle
wait for idle
```

`wait for idle` waits until route transitions (dialogs, sheets, pages closing or opening) have finished
and no frames, animations or HTTP requests are pending. Use it after closing a dialog or bottom sheet
instead of a fixed `wait 2 seconds`. `tap` also waits (up to 2 seconds) for an in-flight route transition
before it fires.

When a step times out, the error names the line, the step, the timeout and what was on screen
(`visible texts: [...], keys: [...]`), so a failure no longer reduces to `context deadline exceeded`.

## Conditionals

```
if "Accept Cookies" appears
  tap "Accept Cookies"
```

With an else branch:

```
if "Welcome Back" appears
  tap "Continue"
else
  tap "Sign In"
```

## Loops

```
repeat 3 times
  swipe left
  wait 1 second
```

## Retry Blocks

`retry N times` re-runs its whole block from the top on failure, up to N total attempts,
stopping at the first success — unlike `repeat`, which always runs every iteration:

```
retry 3 times
  tap "Submit"
  see "Success"
```

## Optional Steps

A trailing `optional` on `tap`/`type`/`long press`/`double tap`/`clear`/`see` attempts the step,
but logs a warning and continues instead of failing the test when it errors. Unlike `if visible`
(a pre-check that skips the step entirely when the target isn't found), `optional` always tries:

```
tap "Rate this app" optional
see "Promo banner" optional
```

## Dart Escape Hatch

For anything ProbeScript doesn't cover natively, use a `dart:` block:

```
dart:
  final prefs = await SharedPreferences.getInstance();
  await prefs.clear();
```

## HTTP Mocking

```
when the app calls POST "/api/auth/login"
  respond with 503 and body "{ \"error\": \"Service Unavailable\" }"
```

## Utility Commands

```
take screenshot "checkout_page"    # save PNG to screenshots folder
compare screenshot "baseline"      # compare against visual regression baseline
dump tree                          # dump widget tree for debugging
save logs                          # save app logs
press enter                        # keyboard action key on the focused field (Maestro pressKey: Enter)
go back                            # pop the current route (no-op + warning at the root; use "close the app" to exit)
rotate landscape                   # rotate device
shake                              # simulate device shake gesture
log "checkpoint reached"           # print to test output
pause                              # 1-second pause
```

## App Lifecycle

```
clear app data                     # wipe data and relaunch
restart the app                    # force-stop and relaunch (preserves data)
kill the app                       # force-stop only (no relaunch)
open the app                       # launch the app (CLI-side) and reconnect
```

## Clipboard

```
copy "user@example.com" to clipboard
paste from clipboard               # stores result in <clipboard> variable
type "<clipboard>" into "Email"    # use the pasted value
```

## Device Location

```
set location 37.7749, -122.4194    # set GPS coordinates (lat, lng)
```

Simulate movement through an ordered route of waypoints over a duration:

```
travel to
  37.7749, -122.4194
  37.7849, -122.4094
over 10 seconds
```

The device's location moves through the waypoints in order, interpolated at roughly 1-second
intervals. `over N seconds` is optional (defaults to ~1 second per leg). Emulator/simulator only,
same as `set location` — skips with a warning on physical devices.

## Device Media

```
add media "fixtures/photo.jpg"     # seed a file into the camera roll/gallery
```

Uses `adb push` + a media-scanner broadcast on Android and `simctl addmedia` on iOS simulators —
the file becomes visible to image pickers. CLI-side only; skipped with a warning in cloud mode.
Note: this makes a photo *available* to pick — driving the native picker UI itself to select it
is `tap native` (Android, below).

## External Browser and Deep Links

```
open link "https://example.com"              # external browser via url_launcher
verify external browser opened               # assert url_launcher was called
open link "myapp://profile/42" in the app    # route via OS intent handling to the
                                             # app's own registered scheme instead
```

`in the app` (also `into the app` / `in app`) dispatches through the OS (`am start -a VIEW` /
`simctl openurl`), so a custom scheme or App/Universal Link registered by the app under test is
delivered to *that app*. On iOS Simulator this reliably works when the app is already running —
`simctl openurl` cannot cold-launch a terminated app. No such caveat on Android.

## Native UI (Android)

Reach outside the Flutter widget tree into native, OS-owned UI — pickers, share sheets — matched
against uiautomator's text or resource-id:

```
tap native "Choose from Gallery"
see native "IMG_0001.jpg"
don't see native "Error"
type native "wifi" into "Search settings"
```

Android only (dispatched via `uiautomator`, no new dependencies). On iOS, `tap native`/`type
native` error clearly rather than silently no-op'ing; `see native` reports "not found". If the
native field is reached via a screen transition, add a `wait` step first — the same idiom used
after Flutter navigation.

## Biometric Authentication

Drive Face ID / Touch ID / fingerprint prompts on the simulator or
emulator. Skipped on physical devices.

```
enroll biometric                   # mark the device as having an enrolled face/finger
biometric match                    # simulate a successful capture (unblocks a pending prompt)
biometric no match                 # simulate a failed capture (triggers the failure path)
```

Typical pattern — wraps a Face ID prompt with a happy and unhappy path:

```
before all tests
  enroll biometric

test "matching face unlocks"
  open the app
  tap "Sign in with Face ID"
  biometric match
  wait until "Dashboard" appears

test "non-matching face is rejected"
  open the app
  tap "Sign in with Face ID"
  biometric no match
  see "Authentication failed"
```

On iOS, this posts the `BiometricKit_Sim.faceCapture.match` / `.no-match`
Darwin notifications (and the `fingerTouch.*` equivalents for Touch ID
devices), then sends `probe.biometric_signal` to the agent. On Android,
this calls `adb -s <serial> emu finger touch <id>` — fingerprint ID `1`
is matching by convention (must be pre-enrolled in Settings before tests
run); any unregistered ID is no-match.

:::caution[iOS 26+ — use `awaitBiometricResult()` in your app]
On iOS 26+ simulator the `no-match` notification no longer resolves
`LAContext.evaluatePolicy`. Use `awaitBiometricResult()` from
`flutter_probe_agent` in PROBE_AGENT builds — the CLI resolves it via
`probe.biometric_signal`. See the [iOS platform guide](/platform/ios/#biometric-authentication-face-id--touch-id) for the code pattern.
:::

## HTTP Calls

Make real HTTP requests to APIs (runs on the CLI, not the device):

```
call GET "https://api.example.com/health"
call POST "https://api.example.com/seed" with body "{\"env\":\"test\"}"
call PUT "https://api.example.com/users/1" with body "{\"name\":\"updated\"}"
call DELETE "https://api.example.com/sessions"
```

Responses are stored in variables:
- `<response.status>` — HTTP status code (e.g., `200`)
- `<response.body>` — response body as a string

## Data Generators

Generate random data for form-heavy tests:

```
type "<random.email>" into "Email"          # e.g., user_x7k2m@test.probe
type "<random.name>" into "Name"            # e.g., Alice Johnson
type "<random.phone>" into "Phone"          # e.g., +1-555-042-7831
type "<random.uuid>" into "Reference"       # UUID v4
type "<random.number(1,100)>" into "Age"    # random int in range
type "<random.text(8)>" into "Code"         # random alphanumeric string
```

## Permissions

```
allow permission "notifications"
deny permission "camera"
grant all permissions
revoke all permissions
```

See [App Lifecycle](/platform/app-lifecycle/) for details on how these work across platforms.

To grant permissions once, before the first test, instead of in every test, pass `--grant`:

```bash
probe test tests/ --grant notifications,camera,location
```

Android grants via `adb shell pm grant` (including `POST_NOTIFICATIONS`); the iOS simulator grants the
services `xcrun simctl privacy` supports (camera, location, microphone, photos, contacts, calendar, sms).
**iOS notifications** have no simctl service. Since 0.16.0 probe answers the SpringBoard alert itself:
`--grant notifications` and `allow permission "notifications"` tap **Allow** when it appears (needs Xcode,
simulators only; see [System dialogs](#system-dialogs)).

## System dialogs

OS-level dialogs (permission alerts, the StoreKit "Sign in to Apple Account" sheet, Android permission
dialogs) live outside the Flutter widget tree. These steps drive them:

```
tap "Allow" in system dialog
tap "OK" in system dialog "Apple Account"            # optional title filter
type "$PROBE_SANDBOX_PASSWORD" into system field "Password"
see system dialog "Sign in to Apple Account"
don't see system dialog "Notifications"
wait for system dialog "Notifications" appears       # or: disappears
dismiss system dialog                                # Cancel / Don't Allow / Not Now; no-op if none
sign in sandbox tester                               # StoreKit sandbox account; no-op if no sheet appears
tap "Allow" in system dialog optional                # any step: don't fail when no dialog shows up
```

Values typed into a system field come from an environment variable (`$NAME` or `${NAME}`) or a literal, and are
**always masked** in step output, reports and error messages. See [System dialogs](/tools/system-dialogs/) for
setup, the standalone `probe system-dialog` command and troubleshooting.

## Conditional Actions

Skip an action silently when the target widget is not found:

```
tap "Aceptar" if visible           # tap only if present, skip otherwise
tap "Cerrar" if visible            # useful for dismissing optional dialogs
clear "Search" if visible          # clear field only if it exists
type "text" into "Field" if visible
long press "Item" if visible
double tap "Element" if visible
```

The `if visible` suffix works with `tap`, `type`, `clear`, `long press`, and `double tap`. If the widget is not found, the step is silently skipped (no error). Connection errors are still propagated.
