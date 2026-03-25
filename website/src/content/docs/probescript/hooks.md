---
title: Hooks
description: Run setup and teardown steps with before all, after all, before each, after each, and on failure hooks.
---

Hooks let you define steps that run automatically around your tests. They are defined at the file level and apply to all tests in that file.

FlutterProbe supports two levels of hooks:
- **Suite-level**: `before all` and `after all` — run once per file
- **Test-level**: `before each`, `after each`, and `on failure` — run around every test

## before all

Runs once before the first test in the file. If it fails, all tests in the file are skipped:

```
before all
  open the app
  tap "Accept Terms"
  wait for the page to load
```

Use this for expensive one-time setup like accepting onboarding flows, seeding data, or logging in.

## after all

Runs once after the last test in the file, regardless of whether tests passed or failed:

```
after all
  take screenshot "suite_final"
  call DELETE "https://api.example.com/test-data"
```

Use this for suite-level teardown like cleaning up test data or capturing final state.

## before each

Runs before every test in the file:

```
before each
  open the app
  wait for the page to load
```

Use this for common setup like launching the app, navigating to a screen, or logging in.

## after each

Runs after every test in the file, regardless of whether the test passed or failed:

```
after each
  take screenshot "after_test"
```

Use this for cleanup or final screenshots.

## on failure

Runs only when a test fails. Useful for capturing debugging information:

```
on failure
  take screenshot "failure"
  save logs
  dump tree
```

## Combining Hooks

You can use all five hooks in the same file:

```
before all
  open the app
  tap "Accept Terms"

after all
  take screenshot "suite_final"

before each
  see "Home"

after each
  take screenshot "after_test"

on failure
  take screenshot "failure_state"
  save logs
  dump tree

test "user can view settings"
  tap "Settings"
  see "Account"
  go back

test "user can view profile"
  tap "Profile"
  see "Email"
  go back
```

## Execution Order

Full execution order for a file with two tests:

1. `before all` steps (once)
2. `before each` steps
3. Test 1 steps
4. `after each` steps
5. `before each` steps
6. Test 2 steps
7. `after each` steps
8. `after all` steps (once)

For a failing test:
1. `before each` steps
2. Test steps (until failure)
3. `on failure` steps (best-effort)
4. `after each` steps (best-effort)

If `before all` fails, all tests in the file are marked as failed and skipped. `after all` always runs, even if tests failed.

## Hooks with Recipes

Hooks can call recipes just like regular test steps:

```
use "recipes/auth.probe"

before each
  log in as "test@example.com" with "password"

test "user can update name"
  tap "Profile"
  tap "Edit"
  type "New Name" into "Name"
  tap "Save"
  see "New Name"
```

## Scope

Hooks are file-scoped. Each `.probe` file can define its own set of hooks. There are no global hooks — if you need the same hooks across multiple files, define them in a recipe and call it from each file's `before each`.

`before all` and `after all` share a separate executor from the per-test hooks, so state set in `before all` (like variables) does not carry into individual tests.
