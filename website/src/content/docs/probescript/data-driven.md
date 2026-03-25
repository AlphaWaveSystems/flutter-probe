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

## Loading Data from CSV

For large data sets, load examples from an external CSV file instead of inline tables:

```
test "login with CSV data"
  open the app
  type "<email>" into "Email"
  type "<password>" into "Password"
  tap "Sign In"
  see "<expected>"

with examples from "fixtures/users.csv"
```

The CSV file should have headers as the first row:

```csv
email,password,expected
user@test.com,pass123,Dashboard
admin@test.com,admin456,Admin Panel
guest@test.com,guest1,Limited Access
```

CSV paths are resolved relative to the `.probe` file's directory. Absolute paths are also supported.

## Random Data Generators

Use built-in generators to create random test data on each run:

```
test "registration with random data"
  open the app
  type "<random.email>" into "Email"
  type "<random.name>" into "Full Name"
  type "<random.phone>" into "Phone"
  type "<random.uuid>" into "Reference ID"
  type "<random.number(18,65)>" into "Age"
  type "<random.text(8)>" into "Invite Code"
  tap "Register"
  see "Success"
```

Available generators:

| Generator | Example Output | Description |
|---|---|---|
| `<random.email>` | `user_x7k2m@test.probe` | Random email address |
| `<random.name>` | `Alice Johnson` | Random first + last name |
| `<random.phone>` | `+1-555-042-7831` | US-format phone number |
| `<random.uuid>` | `550e8400-e29b-...` | UUID v4 |
| `<random.number(min,max)>` | `42` | Random integer in range |
| `<random.text(length)>` | `aB3kM9xQ` | Random alphanumeric string |

Random generators are expanded before variable substitution, so they work in both inline and CSV-based data-driven tests.
