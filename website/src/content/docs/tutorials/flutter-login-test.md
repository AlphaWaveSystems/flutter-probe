---
title: "Write a Flutter login test"
description: "A step-by-step tutorial: add the FlutterProbe agent to a Flutter app, write an end-to-end login test in plain English, add a failure case and share the login steps between tests."
---

This tutorial writes an end-to-end test for a login screen: the happy path, a validation error, and a reusable login step. It takes about ten minutes if the CLI is [installed](/getting-started/installation/).

## 1. Give the widgets keys

Text selectors work, but a key does not change when the copy or the language does. Add a `ValueKey` to the widgets the test touches:

```dart
TextField(key: const ValueKey('email')),
TextField(key: const ValueKey('password'), obscureText: true),
ElevatedButton(key: const ValueKey('loginButton'), onPressed: _submit, child: const Text('Log in')),
```

In a test, `#email` selects the widget with `ValueKey('email')`.

## 2. Add the agent and build with the flag

```yaml
# pubspec.yaml
dev_dependencies:
  flutter_probe_agent: ^0.25.0
```

```dart
// main.dart
const probeEnabled = bool.fromEnvironment('PROBE_AGENT', defaultValue: false);
if (probeEnabled) {
  await ProbeAgent.start();
}
```

Run the app with `flutter run --dart-define=PROBE_AGENT=true`. A normal build without the flag does not start the agent.

## 3. Write the happy path

Run `probe init` in the project, then create `tests/login.probe`:

```
test "a user can log in"
  open the app
  wait until "Log in" appears
  type "user@example.com" into #email
  type "secret123" into #password
  tap #loginButton
  wait until "Dashboard" appears
```

Check it parses, then run it:

```bash
probe lint tests/login.probe
probe device list
probe test tests/login.probe --device <id> -v
```

## 4. Add a failure case

```
test "login shows an error for a wrong password"
  open the app
  wait until "Log in" appears
  type "user@example.com" into #email
  type "wrong" into #password
  tap #loginButton
  wait until "Invalid email or password" appears
  don't see "Dashboard"
```

A failed step reports which line failed, what it was waiting for, and saves a screenshot.

## 5. Share the login steps

Move the repeated steps into a recipe in `tests/recipes/auth.probe`:

```
recipe "log in as" (email, password)
  open the app
  wait until "Log in" appears
  type "<email>" into #email
  type "<password>" into #password
  tap #loginButton
  wait until "Dashboard" appears
```

Use it from any test:

```
use "recipes/auth.probe"

test "a logged-in user sees their profile"
  log in as "user@example.com" with "secret123"
  tap "Profile"
  see "user@example.com"
```

Placeholders must be quoted (`"<email>"`); `probe lint` rejects an unquoted one.

## Tips

- Text selectors match substrings. For a zero-state such as `0 ml`, add an anchored check: `see "0 ml" matching "^0 ml$"`.
- Prefer `wait until "X" appears` after a navigation. A tap returns before the next screen has finished building.
- Run every test file with `probe test tests/ --device <id>`.

## Next

[Run the tests in GitHub Actions](/tutorials/flutter-e2e-github-actions/), or read the [ProbeScript syntax](/probescript/syntax/).

---

The CLI and everything on this page is free. Probe Cloud, hosted report storage, is a [design-partner preview](/probe-cloud/) and is not live yet.
