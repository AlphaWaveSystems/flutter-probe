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
don't see "Error"                  # text is NOT visible
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
drag "Item A" to "Item B"
```

## Wait Commands

```
wait 5 seconds
wait until "Dashboard" appears
wait until "Loading" disappears
wait for the page to load
wait for network idle
```

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
go back                            # device back button
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
