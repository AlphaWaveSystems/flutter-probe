---
title: Hooks
description: Run setup and teardown steps with before each, after each, and on failure hooks.
---

Hooks let you define steps that run automatically around your tests. They are defined at the file level and apply to all tests in that file.

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

You can use all three hooks in the same file:

```
before each
  open the app
  wait for the page to load

after each
  take screenshot "final_state"

on failure
  take screenshot "failure_state"
  save logs
  dump tree

test "user can view settings"
  tap "Settings"
  see "Account"
  see "Notifications"

test "user can view profile"
  tap "Profile"
  see "Email"
  see "Name"
```

## Execution Order

For a passing test:
1. `before each` steps
2. Test steps
3. `after each` steps

For a failing test:
1. `before each` steps
2. Test steps (until failure)
3. `on failure` steps
4. `after each` steps

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
