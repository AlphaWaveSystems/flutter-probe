---
title: probe-convert
description: Convert tests from Maestro, Gherkin, Robot Framework, Detox, and Appium to ProbeScript.
---

`probe-convert` is a standalone multi-format test converter that translates tests from 7 source formats into ProbeScript with **100% construct coverage** across all supported languages.

## Supported Formats

| Source Framework | Extensions | Constructs | Full | Partial |
|------------------|-----------|------------|------|---------|
| Maestro | `.yaml`, `.yml` | 26 | 24 | 2 |
| Gherkin (Cucumber) | `.feature` | 34 | 34 | 0 |
| Robot Framework | `.robot` | 29 | 28 | 1 |
| Detox | `.js`, `.ts` | 22 | 22 | 0 |
| Appium (Python) | `.py` | 14 | 13 | 1 |
| Appium (Java) | `.java`, `.kt` | 12 | 12 | 0 |
| Appium (JS/WebdriverIO) | `.js` | 13 | 13 | 0 |

## Installation

```bash
make build-convert    # from repo root, outputs bin/probe-convert
```

## Usage

### Convert a single file

```bash
bin/probe-convert tests/login.yaml
```

Format is auto-detected from file extension and content markers.

### Convert a directory

```bash
bin/probe-convert -r maestro_tests/ -o probe_tests/
```

### Force a source format

```bash
bin/probe-convert --from maestro flow.yml
```

### Preview without writing

```bash
bin/probe-convert --dry-run tests/login.yaml
```

### Convert and validate

```bash
# Validate with probe lint
bin/probe-convert --lint tests/login.yaml -o output/

# Validate with probe test --dry-run
bin/probe-convert --verify tests/login.yaml -o output/
```

## Conversion Levels

- **Full** — Lossless 1:1 mapping (e.g., `tapOn` becomes `tap on`)
- **Partial** — Lossy but valid ProbeScript with guidance comments (e.g., `evalScript` becomes a `run dart:` block)

No constructs remain at Manual level — all have been promoted to Full or Partial.

## Examples

### Maestro YAML to ProbeScript

```yaml
# login.yaml (Maestro)
appId: com.example.app
---
- launchApp
- tapOn: "Sign In"
- inputText: "user@test.com"
- assertVisible: "Dashboard"
```

Becomes:

```
test "login"
  open the app
  tap on "Sign In"
  type "user@test.com"
  see "Dashboard"
```

### Gherkin to ProbeScript

```gherkin
Feature: Login
  Background:
    Given the app is launched
  Scenario: Valid login
    When I tap on "Sign In"
    Then I should see "Dashboard"
```

Becomes:

```
before each
  open the app

test "Valid login"
  tap on "Sign In"
  see "Dashboard"
```

### Detox JS to ProbeScript

```js
describe('Login', () => {
  it('should sign in', async () => {
    await element(by.id('email')).typeText('user@test.com');
    await element(by.text('Sign In')).tap();
    await expect(element(by.text('Dashboard'))).toBeVisible();
  });
});
```

Becomes:

```
test "should sign in"
  type "user@test.com" into #email
  tap on "Sign In"
  see "Dashboard"
```

## Grammar Catalog

View the formal construct catalog for any language:

```bash
bin/probe-convert catalog            # summary table
bin/probe-convert catalog maestro    # full Maestro catalog
bin/probe-convert catalog gherkin    # full Gherkin catalog
bin/probe-convert catalog --markdown # Markdown output
```

## CLI Flags

| Flag | Description |
|------|-------------|
| `--from, -f` | Force source format (maestro, gherkin, robot, detox, appium) |
| `--output, -o` | Output directory or file |
| `--dry-run` | Preview to stdout |
| `--recursive, -r` | Recurse into subdirectories |
| `--lint` | Validate with `probe lint` after conversion |
| `--verify` | Validate with `probe test --dry-run` after conversion |
| `--probe-path` | Path to probe binary (auto-detected) |

## Format Auto-Detection

The converter guesses format from file extension and content:

- `.feature` maps to Gherkin, `.robot` maps to Robot Framework
- `.yaml`/`.yml` maps to Maestro if it contains `appId`, `tapOn`, `launchApp`, etc.
- `.js`/`.ts` maps to Detox if it contains `element(by.` or `device.launchApp`, otherwise Appium JS
- `.py` maps to Appium Python
- `.java`/`.kt` maps to Appium Java
