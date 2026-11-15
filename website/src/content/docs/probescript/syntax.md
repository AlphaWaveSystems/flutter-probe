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
swipe up on "Card" over 60 percent      # travel 60 percent of the card instead of the default 50
press backspace                         # delete the character before the caret in the focused field
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

## Matching text in several languages

Prefer `#key` selectors (locale-independent). When you select by visible text, matching is a case-sensitive substring by
default. Run with `--match-loose` (or `defaults.match: loose`) to fold case, accents/diacritics, typographic
apostrophes and dashes, full-width forms and whitespace on both sides, so one script tolerates `Änderungen speichern` /
`anderungen SPEICHERN`, `Don’t Allow` / `Don't allow`, and full-width digits. Scripts that differ per language use
`see any of "Save", "Speichern", "Guardar"`. Scripts with Indic or Thai text are not stripped of their vowel signs.

## Targets that are off screen

`tap`, `type`, `long press`, `double tap` and `clear` scroll a target that exists in the widget tree into view first, like a
user would. A lazy list only builds the children near its viewport, so a target beyond that (a chip far to the left of a
horizontal list) does not exist yet: scroll toward it with `scroll left until "All" appears`, then act. `see` and
`don't see` only count what is on screen.

## Text matching is by substring

A quoted text selector (`see "0 ml"`, `tap "Save"`, `wait until "Done" appears`) matches any widget whose text
**contains** it: `see "0 ml"` passes while only "250 ml" is on screen. For an exact check use an anchored pattern,
`see "0 ml" matching "^0 ml$"` (some matched widget's text must match the regular expression), or select by key
(`see #counter_text`). Before 0.16.8 the `matching` pattern was accepted but never applied.

For zero-state and count checks ("0 ml", "3 items", "No items") prefer the anchored form, because a substring match also
passes on longer text such as "10 ml". `probe lint` warns, without failing, about a `see` or `wait until ... appears` on
such a text that has no `matching` pattern. A `matching` pattern anchored at both ends (`^...$`) silences the warning; any other pattern is still a substring match and is still flagged.

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
when the app calls GET "/api/orders"
  respond with 200 and body "[]" after 3 seconds       # slow
when the app calls PATCH "/api/profile"
  respond with network failure                          # connection dropped
```

Mocks are effective (agent 0.22+): the app's `dart:io` HTTP client receives them. They last for one test and survive `restart the app`.

## Performance

```
start measuring "checkout"
tap "Pay"
wait for response POST "/api/pay" status 200 within 10 seconds
stop measuring
see memory below 300 MB
see memory growth below 20 MB
see cpu below 60 percent
see slow frames below 5 percent
see data transferred below 500 KB
see response POST "/api/pay" below 2000 ms
```

Measures memory, frame timings and HTTP traffic (agent 0.23+) and CPU (Android, iOS simulator) between `start measuring` and `stop measuring`.
See [Performance testing](/advanced/performance/).

## Time Budgets

```
tap "Export" within 5 seconds
wait until "Report ready" appears within 90 seconds
wait for response "/api/export" status 200 within 120 seconds
```

`within N seconds|ms` gives that one step its own timeout and implicit-wait window. See [Timeouts, waiting and retries](/advanced/timeouts-and-retries/).

## Backend Responses

```
wait for response GET "/api/orders" status 200
see response "/api/me" json "data.plan" equals "pro"
see response "/api/me" contains "premium"
store response "/api/me" json "data.plan" as plan      # then <plan>
if response "/api/me" json "data.plan" equals "pro"
  see "Premium"
see exactly 1 request GET "/api/orders"
see no requests "/api/analytics/*"
clear recorded requests
```

See [Testing against backend data](/advanced/backend-data/).

## Utility Commands

```
take screenshot "checkout_page"    # save PNG to screenshots folder
take screenshot "checkout_<timestamp>"  # a name that carries the time of the step
compare screenshot "baseline"      # compare against visual regression baseline
dump tree                          # dump widget tree for debugging
save logs                          # save app logs
press enter                        # keyboard action key on the focused field (Maestro pressKey: Enter)
go back                            # pop the current route (no-op + warning at the root; use "close the app" to exit); takes no arguments, so words after it on the same line are a separate step
rotate landscape                   # rotate device
shake                              # simulate device shake gesture
log "checkpoint reached"           # print to test output
pause                              # 1-second pause
```

### Screenshot names

The app saves a screenshot as the name you give it plus a millisecond stamp, so two screenshots with the same name do not replace each other; a placeholder lets the name itself say what the screenshot shows. The name of `take screenshot` is expanded first: `<var>`, `${ENV}` and `<random.*>` work as in every other step, and three built-in placeholders are added:

| Placeholder | Value |
| --- | --- |
| `<timestamp>` | the time of the step, `20261113-141502-123` (local time, to the millisecond) |
| `<run>` | one stamp shared by every screenshot of this `probe test` run, `20261113-141502` |
| `<test>` | the name of the running test, letters and digits of any script stay (UTF-8 is kept); spaces, `/`, emoji, punctuation and invalid bytes become `_` |

```
take screenshot "F14_<test>_s02_<run>"
```

A variable with the same name as a built-in (`<run>`) wins. A placeholder that is neither is left as written. The built-ins are expanded in the name as you wrote it, before variables are substituted, so a variable whose value contains `<test>` keeps that text. Two steps in the same millisecond get the same `<timestamp>`; the app's own stamp still keeps their files apart. `compare screenshot` expands variables but not the built-ins: its name is the key of the baseline, and a name that changes every run would never find it.

The finished name is cleaned, whatever a variable, an environment value or `<random.*>` put in it: `/` and `\` become `_`; control characters (NUL included), direction-changing characters (such as U+202E), the byte order mark and line or paragraph separators are dropped; every `..` is removed; the name is cut at 120 bytes (at a whole character; a long name in a script like Chinese or Japanese can reach that at about 40 characters); and a name with nothing left becomes `screenshot`. Joiners (U+200C, U+200D) are kept, so Persian, Indic and emoji-sequence names stay as written. A name with none of these is used as it is; note that `checkout/step1` becomes `checkout_step1` and `loading...` becomes `loading.`. The cleaning above is all probe does: the app appends its millisecond stamp to the name and does not clean it further, so names that Windows does not accept (reserved names such as `CON`, characters like `:` `?` `*`, a trailing dot or space) are not handled. An `${ENV}` value that ends up in the name is part of the file name; unlike printed lines it is not masked. `compare screenshot` keys its baseline by the name as typed, not by the cleaned name.


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

## Localized text from ARB files

```
tap l10n "saveButton"        # the text of key saveButton in the app's current language
see l10n "title"
```

`l10n "key"` is a string wherever a string works. It needs `l10n.dir` in probe.yaml; a key that does not resolve fails the run before
it starts. See [Testing localized apps](/advanced/multi-language/#your-arb-files-l10n-key).

## App Language

```
set language "de"                  # relaunch the app in German (also "pt-BR", "ja", "ar", ...)
see "Einstellungen"
set language "system"              # back to the device language
```

The app is force-stopped and relaunched, so app state is lost (data on disk is kept). Android 13+ uses per-app locales; an iOS
simulator gets the language as a launch argument. Right-to-left languages (`ar`, `he`, `fa`, `ur`) flip the layout because
Flutter follows the locale. Not supported on physical iOS devices. See [Testing localized apps](/advanced/multi-language/).

## Device Appearance

```
set appearance dark                # the device's dark mode (also light; auto on Android only)
see "Settings"
set appearance light               # back to light
```

The device's own light or dark mode, not one app's. The app is not relaunched; an app whose theme follows the system setting changes with it. An iOS simulator takes
`light` or `dark`; `auto` is refused there when the step runs. A physical iOS device is refused: change its appearance in
Settings. See [Android](/platform/android/#appearance) and [iOS](/platform/ios/).

Text after the value is not rejected. The line ends after the value, and the rest is read as the next statement, the same as after `set language`.

## Device Airplane Mode

```
set airplane mode on               # the emulator's radios off
see "No connection"
set airplane mode off              # nothing turns it back on or off by itself
```

`set airplane mode on` and `off` change the airplane mode of the Android emulator the run targets, through
`adb shell cmd connectivity airplane-mode enable` or `disable`. Nothing restores the state at the end of a test, so a test that
turns it on must turn it off, in the same test or a later one. An iOS simulator has no airplane mode command, so the step fails there.
Only an Android emulator with a serial of the form `emulator-NNNN` takes the step. A physical device (over USB or wireless debugging,
change its airplane mode in its settings) is refused, and so is a device that adb reaches over the network (a `host:port` address):
Genymotion, other network emulators and cloud farm devices. Airplane mode would cut the connection adb uses to them. The check reads
the serial alone, so an `emulator-NNNN` serial is accepted wherever the device runs, and a run that marks a device physical refuses it
whatever its serial. Text after the value is read as the next statement, the same as
after `set appearance`.

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

The text of `type native` goes to the device through `adb shell input text`, which has limits: it cannot type control characters, characters outside ASCII (for example accented letters or Chinese text), a `%` (adb reads `%s` as a space) or a text that starts with `-`. Such text is refused with a message before anything is tapped. Everything else is passed to the device as one quoted word, so characters like `;`, `&`, `$` and quotes are typed as text and never run as commands. To type non-ASCII text into a Flutter field, use `type "..." into ...`, which does not use this path.

Android only (dispatched via `uiautomator`, no new dependencies). On iOS, `tap native`/`type
native` error clearly rather than silently no-op'ing; `see native` reports "not found". If the
native field is reached via a screen transition, add a `wait` step first — the same idiom used
after Flutter navigation.

## WebView content (Android)

A WebView shows a web page inside the app, and the Flutter widget tree cannot see into it. Add `in webview` to
`tap`, `type`, `see`, `don't see` and `wait until ... appears` to act on that page instead:

```
tap "Contact support" in webview
type "hello" into css:"#message" in webview
see "Getting Started" in webview
don't see css:".error" in webview
wait until "Message sent" appears in webview
tap "Pay" in webview "checkout"          # name the page when the app has more than one
```

- A **quoted string is text**: the visible text of an element (for an input without text, its value), with runs of whitespace
  collapsed. An element whose whole text equals yours is looked for first; if there is none, an element whose text contains
  yours. The match is case-sensitive. An element is skipped when one of its children has exactly the same text, but for a
  part of a text the first visible element in document order wins, which can be a container that holds the text.
  **`css:"..."` is a CSS selector** (`css:"#submit"`). The syntax decides, so a text such as `#1 Item` or `.5 off` is always text.
- The optional quoted name after `in webview` is a part of the page's title or its address, without regard to case. Without
  a name the app must have exactly one page; with several, the step lists their titles and asks for a name. A name that
  matches no page, or more than one, is an error that lists the pages (use a longer part of the title or address).
- The step acts on the first **visible** element that matches, scrolled into view. `tap` presses and releases at its
  centre. `type` focuses the element, checks that it holds the focus and then inserts the text; if it did not take the
  focus, or cannot take typed text, nothing is typed and the step fails with the reason: `(disabled)`, `(read-only)`, `(an input of
  type "checkbox")` and so on, or `(not a text field: select)`. Text, search, email, url, tel, password and number inputs, textareas
  and content-editable elements are typed into; checkbox, radio, file, button, range, color and the date and time inputs are not.
  A text or selector
  longer than 2048 characters, or a text to type longer than 8192, is refused before anything is sent. `don't see` passes when no element matches or
  the match is hidden, and fails when the WebView itself cannot be reached.
- There is no step that runs JavaScript, and none can be written: the lookup is a fixed piece of probe's own code, and your
  text only ever travels as data to it.
- Variables and `${ENV}` values work in the selector, the page name and the typed text, and are masked in step lines and
  messages like the text of a `type` step.

### A short example

An app shows its help page in a WebView with a contact form. The test fills the form, sends it and checks the answer:

```
test "contact support from the help page"
  tap "Help"
  wait until "Contact support" appears in webview
  type "My order has not arrived" into css:"#message" in webview
  tap "Contact support" in webview
  wait until "Message sent" appears in webview within 10 seconds
  don't see css:".error" in webview
```

`tap "Help"` is an ordinary step on the Flutter screen; the four steps after the first `wait` act on the page.

### What to know first

- **The app's WebView must have debugging on**: `WebView.setWebContentsDebuggingEnabled(true)` in the app (a debug or profile
  build usually has it; a release build with it off has nothing to reach, and the step says so). probe only ever uses the
  debugging socket of the app under test, over a loopback port it removes again after the step.
- **Android only.** On iOS the steps fail with a message that says so; in a cloud run (no adb) they are skipped like the other
  device-only steps.
- **Only the top document of a page is searched.** Content inside a shadow DOM or an iframe is not found.
- **A field that cannot take typed text is refused, with the reason, and nothing is typed**: a disabled or read-only field, a
  checkbox, a date input and the like, a select, a button. (The page is asked what kind of element it is, so a custom widget
  that is not an input, a textarea or content-editable is refused too.) The focus is checked once: a page that moves
  the focus away right after can still receive the text elsewhere. The text is inserted as text, not as key presses, so a
  page that only reacts to key events may not notice it.
- **Positions are in CSS pixels**, tested at a device pixel ratio of 1; other ratios, scrolled pages and iframes are not yet
  tested.
- A page is opened for each step and closed again; a step never keeps a connection open (one run on one emulator took about 70 ms per step).
- There is no implicit waiting inside the step: use `wait until ... in webview` when the page is still loading.
- `probe lint` warns about `see "0 items" in webview` and `wait until "3 items" appears in webview`: a count or
  zero-state text can match a longer element text, and these steps have no `matching` form, so check the element with
  `css:"..."` instead.

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
dismiss system dialog                                # Cancel / Don't Allow / Not Now / Ask App Not to Track; no-op if none
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
