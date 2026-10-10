---
title: ProbeScript Dictionary
description: Complete reference of every keyword, command, and modifier in the ProbeScript language.
---

Every word recognized by the ProbeScript parser, organized by category.

## Actions

Commands that interact with the app's UI.

| Command | Syntax | Description |
|---|---|---|
| `tap` | `tap "Label"` or `tap #key` | Tap a widget by text label or key |
| `double tap` | `double tap "Label"` | Double-tap a widget |
| `long press` | `long press "Label"` | Long-press a widget (triggers context menus) |
| `type` | `type "text" into "Field"` | Enter text into a text field |
| `clear` | `clear "Field"` | Clear the contents of a text field |
| `swipe` | `swipe up/down/left/right` | Swipe gesture on screen or widget |
| `scroll` | `scroll up/down` | Scroll within a scrollable widget (`down` reveals later content) |
| `scroll until` | `scroll down until "Item" appears` | Keep scrolling until the target is on screen; stops at the end of the list |
| `drag` | `drag #source to #target` | Drag one widget to another |
| `press enter` | `press enter` | Press the keyboard action key (done / go / search / send) on the focused text field; warns if no field has focus |
| `go back` | `go back` | Pop the current route. At the root route it does nothing and warns (it never exits the app; use `close the app`) |
| `open` | `open the app` | Launch the app (CLI-side) and reconnect |
| `close` | `close the app` | Close the app |
| `close` | `close keyboard` | Dismiss the on-screen keyboard |
| `rotate` | `rotate landscape` or `rotate portrait` | Rotate the device orientation |
| `toggle` | `toggle "Dark Mode"` or `toggle #unit_toggle` | Toggle a switch or checkbox — accepts any selector (text, `#id`, ordinal) and dispatches it as a tap on the target |
| `shake` | `shake` | Simulate a device shake gesture |
| `pause` | `pause` | Pause for 1 second |
| `open link` | `open link "https://example.com"` | Open a URL in the external browser via `url_launcher` |
| `open link ... in the app` | `open link "watersip://item/42" in the app` | Route a URL to the app itself via the OS's intent/URL handling (`adb`/`simctl`), not the external browser. Also accepts `into the app` / `in app`. CLI-side only — skipped in cloud mode. On iOS Simulator this reliably works only when the app is already running (see the CHANGELOG for the cold-launch caveat) |
| `add media` | `add media "path/to/photo.jpg"` | Seed a local file into the device's camera roll/gallery (`adb push` + media scan on Android, `simctl addmedia` on iOS). CLI-side only — skipped in cloud mode. Makes a photo available to pick, but does not drive the app's own native image-picker UI to select it |
| `store` | `store "value" as myVar` | Store a literal or `<var>` value for use in later steps |
| `log` | `log "message"` | Emit a message into the run output |
| `tap native` | `tap native "Choose from Gallery"` | Tap a native (non-Flutter) UI element — pickers, share sheets — matched by uiautomator's text or resource-id. Android only; errors clearly on other platforms. CLI-side only — skipped in cloud mode |
| `type native` | `type native "text" into "query"` | Focus a native text field matched by query, then type into it. Android only. If the field is reached via a screen transition (not already visible), add a `wait` step first — the same idiom used for Flutter navigation |

## Assertions

Commands that verify the state of the UI.

| Command | Syntax | Description |
|---|---|---|
| `see` | `see "Text"` | Assert that text is visible on screen |
| `don't see` | `don't see "Text"` | Assert that text is NOT visible (`dont see` is accepted too) |
| `see native` | `see native "..."` | Assert a native (non-Flutter) UI element matching the query is present — matched by uiautomator's text or resource-id. Android only; treated as "not found" on other platforms |
| `don't see native` | `don't see native "..."` | Assert a native UI element matching the query is NOT present |
| `see exactly` | `see exactly 3 "Item"` | Assert exact count of matching widgets |
| `see enabled` | `see "Submit" is enabled` | Assert widget is enabled |
| `see disabled` | `see "Submit" is disabled` | Assert widget is disabled |
| `see checked` | `see "Agree" is checked` | Assert the switch, checkbox, radio (or their list tiles, filter or choice chips) is on; "not a checkable control" for anything else. `don't see "Agree" is checked` asserts it is off |
| `see focused` | `see "Email" is focused` | Assert the widget currently holds keyboard focus |
| `see contains` | `see "Price" contains "$"` | Assert widget text contains substring |
| `see any of` | `see any of "Create Your Account", "Confirm Your Details"` | Assert that any one of the texts is on screen; `don't see any of "A", "B"` asserts none is |
| `see matching` | `see "Email" matching ".*@.*"` | Assert that some widget found by the text selector has text matching the regex. Text selectors match substrings, so use an anchored pattern for exact text: `see "0 ml" matching "^0 ml$"` |

State suffixes compose: `see "Field" is enabled contains "y" matching "z"` checks all three at once.

## Wait

Commands that pause execution until a condition is met.

| Command | Syntax | Description |
|---|---|---|
| `wait N seconds` | `wait 5 seconds` | Wait for a fixed duration |
| `wait until appears` | `wait until "Dashboard" appears` | Wait until text becomes visible |
| `wait until disappears` | `wait until "Loading" disappears` | Wait until text is no longer visible |
| `wait until appears matching` | `wait until "0 ml" appears matching "^0 ml$"` | Wait until a matched widget's text also matches the regular expression (exact text) |
| `wait until any of` | `wait until any of "Got it", "Login", "Home" appears` | Wait until any one of the texts is visible (screens that vary, e.g. an optional consent dialog) |
| `wait for idle` | `wait for idle` | Wait for route transitions, frames, animations and HTTP requests to settle |
| `tap in system dialog` | `tap "Allow" in system dialog` | Tap a button in an OS system dialog (outside the Flutter app) |
| `type into system field` | `type "$PW_ENV" into system field "Password"` | Type a value (from an env var; always masked) into a system dialog field |
| `see system dialog` | `see system dialog "Title"` | Assert a system dialog is showing (`don't see system dialog` asserts none) |
| `wait for system dialog` | `wait for system dialog "Title" appears` | Wait for a system dialog to appear or disappear |
| `dismiss system dialog` | `dismiss system dialog` | Tap Cancel / Don't Allow / Not Now; no-op when nothing is showing |
| `sign in sandbox tester` | `sign in sandbox tester` | Sign the StoreKit sandbox tester in (`PROBE_SANDBOX_USER` / `PROBE_SANDBOX_PASSWORD`); no-op if no sheet |
| `wait for page to load` | `wait for page to load` | Wait for the UI to settle (triple-signal sync) |
| `wait for network idle` | `wait for network idle` | Wait for pending HTTP requests to complete |
| `wait for animations to end` | `wait for animations to end` | Wait until no scheduled animation frames remain |

## App Lifecycle

Commands that control the app process.

| Command | Syntax | Description |
|---|---|---|
| `restart the app` | `restart the app` | Force-stop and relaunch (preserves data) |
| `kill the app` | `kill the app` | Force-stop without relaunching |
| `open the app` | `open the app` | Launch the app and reconnect |
| `clear app data` | `clear app data` | Wipe all app data and relaunch (skipped on physical iOS). On iOS `--yes` cannot answer the notification alert (simctl cannot grant notifications): use `--grant notifications` for that. Asks for confirmation on a terminal; without one (CI, scripts) pass `probe test --yes` or the step fails |

## Permissions

Commands that manage OS-level app permissions.

| Command | Syntax | Description |
|---|---|---|
| `allow permission` | `allow permission "camera"` | Grant a specific permission |
| `deny permission` | `deny permission "location"` | Revoke a specific permission |
| `grant all permissions` | `grant all permissions` | Grant all known runtime permissions |
| `revoke all permissions` | `revoke all permissions` | Revoke all runtime permissions |

Supported permission names: `camera`, `microphone`, `location`, `storage`, `notifications`, `contacts`, `phone`, `calendar`, `sms`, `bluetooth`, `photos`.

## Screenshots & Visual

Commands for capturing and comparing screenshots.

| Command | Syntax | Description |
|---|---|---|
| `take screenshot` | `take screenshot "name"` | Save a PNG screenshot |
| `compare screenshot` | `compare screenshot "baseline"` | Compare against a visual regression baseline |
| `compare screenshot ... of` | `compare screenshot "price_tag" of "Price Label"` | Scope the comparison to a single widget's on-screen bounds instead of the full screen |
| `dump tree` | `dump tree` | Dump the widget tree for debugging |
| `save device logs` | `save device logs` | Save device logs to the report folder |

## Clipboard

Commands for clipboard interaction.

| Command | Syntax | Description |
|---|---|---|
| `copy to clipboard` | `copy "text" to clipboard` | Copy text to the device clipboard |
| `paste from clipboard` | `paste from clipboard` | Read clipboard contents (stored in `<clipboard>` variable) |

## Device

Commands for device-level operations.

| Command | Syntax | Description |
|---|---|---|
| `set location` | `set location 37.7749, -122.4194` | Set GPS coordinates (emulator/simulator only) |
| `l10n "key"` | `tap l10n "saveButton"` · `see l10n "title"` | A string from the app's ARB files in the current app language (needs `l10n.dir`). Works wherever a quoted string does |
| `set language` | `set language "de"` · `"pt-BR"` · `"ar"` · `"system"` | Run the app in another language and relaunch it (Android 13+, iOS simulator). `"system"` removes the override. The device language and system dialogs are unchanged |
| `travel to` | `travel to` (indented waypoints) `over N seconds` | Simulate GPS movement through an ordered list of waypoints over a duration (emulator/simulator only) — see example below |
| `verify external browser` | `verify external browser opened` | Assert that `url_launcher` was called |

`travel to` example — an ordered, indented list of `lat, lng` waypoints followed by a sibling
`over N seconds` clause at the same indent level as `travel to`:

```probescript
travel to
  37.7749, -122.4194
  37.7849, -122.4094
  37.7949, -122.3994
over 10 seconds
```

The device's GPS location moves through the waypoints in order, interpolated at roughly 1-second
intervals so it looks like continuous movement rather than instant jumps — useful for maps,
delivery, rideshare, or fitness flows that react to location changes over time. Reuses the same
`set location` primitive under the hood, so it shares its emulator/simulator-only limitation. The
`over N seconds` clause is optional; omitting it defaults to about 1 second per leg of the route.

## HTTP Calls

Make HTTP requests from the CLI (not the device).

| Command | Syntax | Description |
|---|---|---|
| `call GET` | `call GET "https://api.example.com/health"` | Send a GET request |
| `call POST` | `call POST "url" with body "{...}"` | Send a POST request with JSON body |
| `call PUT` | `call PUT "url" with body "{...}"` | Send a PUT request |
| `call DELETE` | `call DELETE "url"` | Send a DELETE request |

Response variables: `<response.status>` (HTTP status code), `<response.body>` (response body).

## Control Flow

Commands that control test execution flow.

| Command | Syntax | Description |
|---|---|---|
| `if appears` | `if "Dialog" appears` | Execute indented block only if widget is visible |
| `otherwise` | `otherwise` | Else branch for `if` block |
| `repeat N times` | `repeat 5 times` | Loop an indented block N times |
| `retry N times` | `retry 3 times` | Re-run an indented block from the top on failure, up to N attempts, stopping at the first success |

## Conditional Actions (if visible)

Modifier that silently skips an action when the target widget is not found.

| Command | Syntax | Description |
|---|---|---|
| `tap if visible` | `tap "OK" if visible` | Tap only if present, skip otherwise |
| `type if visible` | `type "text" into "Field" if visible` | Type only if field exists |
| `clear if visible` | `clear "Field" if visible` | Clear only if field exists |
| `long press if visible` | `long press "Item" if visible` | Long press only if present |
| `double tap if visible` | `double tap "Item" if visible` | Double tap only if present |

## Optional Steps

Modifier that attempts a step, but treats a failure as non-fatal instead of failing the test.
Unlike `if visible` above (a pre-check that never even attempts the step), `optional` always runs
the step and only changes what happens after it fails.

| Command | Syntax | Description |
|---|---|---|
| `tap optional` | `tap "Maybe There" optional` | Attempt the tap; log a warning and continue if it fails |
| `type optional` | `type "text" into "Field" optional` | Attempt to type; continue if the field isn't found |
| `clear optional` | `clear "Field" optional` | Attempt to clear; continue if not found |
| `long press optional` | `long press "Item" optional` | Attempt a long press; continue if not found |
| `double tap optional` | `double tap "Item" optional` | Attempt a double tap; continue if not found |
| `see optional` | `see "Maybe There" optional` | Attempt the assertion; continue if it doesn't hold |

## Data Generators

Dynamic placeholders that generate random data at runtime.

| Placeholder | Example Output | Description |
|---|---|---|
| `<random.email>` | `user_a7b3@test.com` | Random email address |
| `<random.name>` | `Alice Johnson` | Random full name |
| `<random.phone>` | `+1-555-0142` | Random phone number |
| `<random.uuid>` | `550e8400-e29b-41d4...` | Random UUID v4 |
| `<random.number>` | `42` | Random integer (0-9999) |
| `<random.number(1,100)>` | `73` | Random integer in range |
| `<random.text(8)>` | `xK4mP2qR` | Random alphanumeric string |

## Test Structure

Keywords for organizing tests.

| Keyword | Syntax | Description |
|---|---|---|
| `test` | `test "name"` | Define a test case |
| `recipe` | `recipe "name" (param1, param2)` | Define a reusable recipe |
| `use` | `use "path/to/recipe.probe"` | Import a recipe file |
| `@tag` | `@smoke @critical` | Tag a test for filtering with `--tag` |
| `with examples` | `with examples:` | Start a data-driven example table |
| `with examples from` | `with examples from "file.csv"` | Load examples from a CSV file |

## Hooks

Lifecycle hooks that run around tests.

| Hook | Syntax | Description |
|---|---|---|
| `before each` | `before each` | Run before every test in the file |
| `after each` | `after each` | Run after every test in the file |
| `before all` | `before all` | Run once before all tests in the file |
| `after all` | `after all` | Run once after all tests in the file |
| `on failure` | `on failure` | Run when a test fails (for cleanup/screenshots) |

## Dart Escape Hatch

Execute arbitrary Dart code on the device.

| Command | Syntax | Description |
|---|---|---|
| `run dart` | `run dart: print('hello')` | Execute inline Dart code |

## HTTP Mocking

Mock API responses for the app.

| Command | Syntax | Description |
|---|---|---|
| `when` | `when the app calls GET "/api/users"` | Define a mock rule |
| `respond` | `respond with 200 and body "[]" after 2 seconds` · `respond with network failure` | Define the mock response; `after N seconds` delays it, `network failure` drops the connection |

## Performance

| Command | Syntax | Description |
|---|---|---|
| `start measuring` | `start measuring "checkout"` | Open a measuring window (CPU, memory, frames, network) |
| `stop measuring` | `stop measuring` | Close it and print the numbers |
| `see memory below` | `see memory below 300 MB` · `see memory growth below 20 MB` | Peak memory / growth over the window |
| `see cpu below` | `see cpu below 60 percent` · `see cpu peak below 90 percent` | Average / busiest-second CPU (percent of one core) |
| `see slow frames below` | `see slow frames below 5 percent` · `see frame time below 16 ms` · `see slowest frame below 100 ms` | Frame timings |
| `see data transferred below` | `see data transferred below 500 KB` | Request + response bytes |
| `see response ... below` | `see response "/api/x" below 800 ms` | Response time |

See [Performance testing](/advanced/performance/).

## Time budget

| Command | Syntax | Description |
|---|---|---|
| `within` | `wait until "Done" appears within 90 seconds` · `tap "Go" within 2 seconds` · `see "Y" within 500 ms` | Per-step timeout and implicit-wait window. See [Timeouts, waiting and retries](/advanced/timeouts-and-retries/) |

## Backend Responses

React to what the backend returned (the app's `dart:io` HTTP traffic; agent 0.22+). See [Testing against backend data](/advanced/backend-data/).

| Command | Syntax | Description |
|---|---|---|
| `wait for response` | `wait for response GET "/api/orders" status 200` | Wait for a new matching response |
| `see response` | `see response "/api/me" status 200` · `contains "x"` · `json "data.plan" equals "pro"` · `json "a.b" exists` | Check the newest matching response |
| `store response` | `store response "/api/me" json "data.plan" as plan` | Keep a value from the body in `<plan>` |
| `if response` | `if response "/api/me" json "data.plan" equals "pro"` | Branch on the newest matching response |
| `see exactly N requests` | `see exactly 2 requests GET "/api/orders"` · `see no requests "/x"` | Count the requests the app made |
| `clear recorded requests` | `clear recorded requests` | Forget what was recorded so far |

## AI-Powered Assertions

Natural-language, LLM-backed checks for things that are hard to express structurally. Require an
`ai:` block in `probe.yaml` (`provider: openai | anthropic | local`, your own API key) — every
call goes directly from the CLI to the provider you configure, never through a FlutterProbe relay.

| Command | Syntax | Description |
|---|---|---|
| `see with ai` | `see "the checkout total looks correct" with ai` | Natural-language visual assertion |
| `assert no visual defects with ai` | `assert no visual defects with ai` | Fixed smoke check for cut-off/overlapping/mis-centered elements |
| `read with ai into` | `read "the 6-digit OTP code" with ai into otp` | Extract text off the current screen into a variable |

## Biometrics

Simulate Face ID / Touch ID / fingerprint prompts on a simulator or emulator (skipped with a
warning on physical devices).

| Command | Syntax | Description |
|---|---|---|
| `enroll biometric` | `enroll biometric` | Mark the sim/emulator as having an enrolled face/finger |
| `biometric match` | `biometric match` | Simulate a successful biometric capture |
| `biometric no match` | `biometric no match` | Simulate a failed biometric capture |

## Signals

A generic escape hatch for resolving in-app prompts that don't map to a built-in verb (native
push-permission dialogs, payment sheets, ATT prompts, custom deep-link handlers).

| Command | Syntax | Description |
|---|---|---|
| `deliver signal` | `deliver signal "payment_ready" "true"` | Resolve a pending `awaitSignal(name)` call in the app; value defaults to `"true"` |

## Composite (Multi-Device) Tests

Run coordinated steps across multiple devices in a single test — chat apps, referrals, admin/user
pairs.

| Keyword | Syntax | Description |
|---|---|---|
| `composite test` | `composite test "name"` | Define a multi-device test |
| `devices` | `devices:` / `A: <device>` | Declare device aliases inside a composite test |
| `sync` | `sync "label"` | Cross-device barrier — every device's steps block here until all arrive |

## Selectors

How ProbeScript locates widgets in the Flutter widget tree.

| Selector | Syntax | Matches |
|---|---|---|
| Text | `"Login"` | Widget whose text contains "Login" |
| Key | `#sign_in_button` | Widget with `Key('sign_in_button')` or `Semantics(identifier: 'sign_in_button')` |
| Ordinal | `2nd "Item"` | The 2nd widget matching "Item" (also composes with a key: `1st #card_id`) |
| Positional | `"Price" in "Product Card"` | "Price" text within a "Product Card" container |
| Relational | `"Submit" below "Email"` | "Submit" positioned below the "Email" widget (also `above`, `left of`, `right of`) |

## Filler Words

These words are ignored by the parser — they make tests more readable but have no effect.

`the`, `a`, `an`, `on`, `in`, `into`, `at`, `of`, `from`, `is`, `are`, `that`, `this`, `it`, `for`

Example: `tap the "Login" button` is equivalent to `tap "Login"`.
