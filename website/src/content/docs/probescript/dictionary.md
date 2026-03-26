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
| `scroll` | `scroll up/down` | Scroll within a scrollable widget |
| `drag` | `drag #source to #target` | Drag one widget to another |
| `go back` | `go back` | Press the device back button |
| `open` | `open the app` | Launch the app (CLI-side) and reconnect |
| `close` | `close the app` | Close the app |
| `rotate` | `rotate landscape` or `rotate portrait` | Rotate the device orientation |
| `toggle` | `toggle "Dark Mode"` | Toggle a switch or checkbox |
| `shake` | `shake` | Simulate a device shake gesture |
| `pause` | `pause` | Pause for 1 second |

## Assertions

Commands that verify the state of the UI.

| Command | Syntax | Description |
|---|---|---|
| `see` | `see "Text"` | Assert that text is visible on screen |
| `don't see` | `don't see "Text"` | Assert that text is NOT visible |
| `see exactly` | `see exactly 3 "Item"` | Assert exact count of matching widgets |
| `see enabled` | `see "Submit" is enabled` | Assert widget is enabled |
| `see disabled` | `see "Submit" is disabled` | Assert widget is disabled |
| `see checked` | `see "Agree" is checked` | Assert checkbox/toggle is checked |
| `see contains` | `see "Price" contains "$"` | Assert widget text contains substring |
| `see matching` | `see "Email" matching ".*@.*"` | Assert widget text matches regex pattern |

## Wait

Commands that pause execution until a condition is met.

| Command | Syntax | Description |
|---|---|---|
| `wait N seconds` | `wait 5 seconds` | Wait for a fixed duration |
| `wait until appears` | `wait until "Dashboard" appears` | Wait until text becomes visible |
| `wait until disappears` | `wait until "Loading" disappears` | Wait until text is no longer visible |
| `wait for page to load` | `wait for page to load` | Wait for the UI to settle (triple-signal sync) |
| `wait for network idle` | `wait for network idle` | Wait for pending HTTP requests to complete |

## App Lifecycle

Commands that control the app process.

| Command | Syntax | Description |
|---|---|---|
| `restart the app` | `restart the app` | Force-stop and relaunch (preserves data) |
| `kill the app` | `kill the app` | Force-stop without relaunching |
| `open the app` | `open the app` | Launch the app and reconnect |
| `clear app data` | `clear app data` | Wipe all app data and relaunch (skipped on physical iOS) |

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
| `dump tree` | `dump tree` | Dump the widget tree for debugging |

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
| `verify external browser` | `verify external browser opened` | Assert that `url_launcher` was called |

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

## Conditional Actions (if visible)

Modifier that silently skips an action when the target widget is not found.

| Command | Syntax | Description |
|---|---|---|
| `tap if visible` | `tap "OK" if visible` | Tap only if present, skip otherwise |
| `type if visible` | `type "text" into "Field" if visible` | Type only if field exists |
| `clear if visible` | `clear "Field" if visible` | Clear only if field exists |
| `long press if visible` | `long press "Item" if visible` | Long press only if present |
| `double tap if visible` | `double tap "Item" if visible` | Double tap only if present |

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
| `respond` | `respond with 200 and body "[]"` | Define the mock response |

## Selectors

How ProbeScript locates widgets in the Flutter widget tree.

| Selector | Syntax | Matches |
|---|---|---|
| Text | `"Login"` | Widget whose text contains "Login" |
| Key | `#sign_in_button` | Widget with `Key('sign_in_button')` or `Semantics(identifier: 'sign_in_button')` |
| Ordinal | `2nd "Item"` | The 2nd widget matching "Item" |
| Positional | `"Price" in "Product Card"` | "Price" text within a "Product Card" container |

## Filler Words

These words are ignored by the parser — they make tests more readable but have no effect.

`the`, `a`, `an`, `in`, `at`, `of`, `from`, `is`, `are`, `that`, `this`, `it`, `for`

Example: `tap the "Login" button` is equivalent to `tap "Login"`.
