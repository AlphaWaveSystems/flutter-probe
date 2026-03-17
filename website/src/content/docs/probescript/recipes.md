---
title: Recipes
description: Define reusable step sequences with recipes and import them with use statements.
---

Recipes are reusable step sequences that reduce duplication across tests. They accept parameters and can be imported into any test file.

## Defining a Recipe

A recipe is declared with `recipe` followed by a quoted name and optional parameters in parentheses:

```
recipe "log in as" (email, password)
  open the app
  wait until "Sign In" appears
  tap "Sign In"
  type <email> into "Email"
  type <password> into "Password"
  tap "Continue"
  see "Dashboard"
```

Parameters are referenced with angle brackets (`<email>`, `<password>`) inside the recipe body.

## Using Recipes

Import a recipe file with `use`, then call the recipe by name, passing arguments as quoted strings:

```
use "recipes/auth.probe"

test "logged-in user can view profile"
  log in as "user@test.com" with "secret123"
  tap "Profile"
  see "user@test.com"
```

The `use` path is relative to the project root or the `recipes_folder` defined in `probe.yaml`.

## Recipes Without Parameters

Recipes can also define fixed step sequences with no parameters:

```
recipe "dismiss onboarding"
  if "Get Started" appears
    tap "Get Started"
  if "Skip" appears
    tap "Skip"
  wait for the page to load
```

Called as:

```
test "user reaches home"
  dismiss onboarding
  see "Home"
```

## File Organization

A common pattern is to keep recipes in a dedicated folder:

```
tests/
  recipes/
    auth.probe         # login, signup recipes
    navigation.probe   # menu, tab navigation recipes
    setup.probe        # data setup, cleanup recipes
  smoke/
    login.probe        # imports auth.probe
    profile.probe      # imports auth.probe, navigation.probe
```

Configure the recipe folder in `probe.yaml`:

```yaml
recipes_folder: tests/recipes
```

## Multiple Parameters

Arguments are matched by position using connecting words like "and", "with", or "into":

```
recipe "create account with" (name, email, password)
  tap "Sign Up"
  type <name> into "Name"
  type <email> into "Email"
  type <password> into "Password"
  tap "Create Account"
```

Called as:

```
create account with "Alice" and "alice@test.com" and "pass123"
```
