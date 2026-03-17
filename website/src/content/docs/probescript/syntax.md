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
dump tree                          # dump widget tree for debugging
save logs                          # save app logs
go back                            # device back button
rotate landscape                   # rotate device
log "checkpoint reached"           # print to test output
pause                              # 1-second pause
```

## App Lifecycle

```
clear app data                     # wipe data and relaunch
restart the app                    # force-stop and relaunch (preserves data)
```

## Permissions

```
allow permission "notifications"
deny permission "camera"
grant all permissions
revoke all permissions
```

See [App Lifecycle](/platform/app-lifecycle/) for details on how these work across platforms.
