---
title: Data-Driven Tests
description: Run the same test with multiple data sets using Examples blocks and variable substitution.
---

Data-driven tests let you run the same test logic with different input values. ProbeScript uses `with examples:` blocks with `<variable>` substitution.

## Basic Example

```
test "login validation"
  open the app
  type <email> into "Email"
  type <password> into "Password"
  tap "Continue"
  see <expected>

with examples:
  email              password     expected
  "user@test.com"    "pass123"    "Dashboard"
  ""                 "pass123"    "Email is required"
  "user@test.com"    ""           "Password is required"
```

The test runs once for each row in the examples table. Each column header becomes a variable that can be referenced with `<variable>` syntax in the test body.

## How It Works

1. The parser reads the `with examples:` block and identifies column headers from the first row
2. Each subsequent row creates a separate test instance
3. All `<variable>` references in the test body are replaced with the corresponding column value
4. Each instance is reported as a separate test result (e.g., "login validation [row 1]", "login validation [row 2]")

## Column Formatting

Column values are separated by whitespace. Quoted strings can contain spaces:

```
with examples:
  name            email                 expected
  "Alice Smith"   "alice@test.com"      "Welcome, Alice"
  "Bob Jones"     "bob@test.com"        "Welcome, Bob"
```

## Combining with Tags

Tags apply to all rows:

```
test "search results"
  @regression
  type <query> into "Search"
  tap "Go"
  see <result>

with examples:
  query          result
  "flutter"      "Flutter SDK"
  "dart"         "Dart Language"
  "probe"        "FlutterProbe"
```

## Combining with Recipes

Data-driven tests work seamlessly with recipes:

```
use "recipes/auth.probe"

test "role-based access"
  log in as <email> with <password>
  see <page>

with examples:
  email               password     page
  "admin@test.com"    "admin123"   "Admin Panel"
  "user@test.com"     "user123"    "Dashboard"
  "guest@test.com"    "guest123"   "Read Only"
```
