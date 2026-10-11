---
title: Using FlutterProbe results in other analytics tools
description: What `--format junit` contains, which fields flaky-test and test-analytics services read, and which of them were checked against FlutterProbe's output only offline.
---

`probe test --format junit -o results.xml` writes JUnit XML that flaky-test and test-analytics services can read. This page says what is in
the file, what three services document about the fields they need, and what has **not** been verified against a live service.

> **Status: checked offline only.** The output was compared with each vendor's public documentation (read 2026-10-11). Nothing was
> uploaded to any service and no account was used. Treat "accepted" as unproven until you upload a report yourself; the Trunk CLI can
> validate a file locally without an account (below).

## What the file contains

```xml
<testsuites tests="2" failures="1" errors="0" skipped="0" time="3.200">
  <testsuite name="tests/a.probe" file="tests/a.probe" tests="2" failures="1" errors="0" skipped="0" time="3.200" timestamp="2026-10-11T12:00:00">
    <properties>
      <property name="probe_version" value="0.25.0"/> <property name="platform" value="android"/>
      <property name="os_version" value="Android 15"/> <property name="app_id" value="dev.example.app"/>
      <property name="app_version" value="1.2.3"/> <property name="device_name" value="Pixel 7"/>
      <property name="device_class" value="emulator"/> <property name="run_label" value="shard-1"/>
    </properties>
    <testcase name="login" classname="tests/a.probe" file="tests/a.probe" time="1.200"/>
    <testcase name="pay [row 2]" classname="tests/a.probe" file="tests/a.probe" time="2.000">
      <failure message="expected to see &quot;Done&quot;">expected to see "Done"</failure>
    </testcase>
  </testsuite>
</testsuites>
```

- **One `<testsuite>` per `.probe` file**, sorted by name so two runs diff cleanly. `time` is the sum of its cases, `timestamp` the start of its
  first test (UTC, no zone, as in Codecov's documented example).
- **Paths are relative to the project directory, with forward slashes**, in `name`, `classname` and `file`; a file outside the project keeps
  only its name. No home directory or machine path reaches the report, and the same test has the same identity on every machine.
- **`<failure>` is a failed assertion; `<error>` is an infrastructure problem** (the agent connection dropped, a `before all`/`before each`
  hook failed). The suite and root `errors` attributes count them.
- **`<properties>`** on each suite: the probe version, platform, OS version, app id and version, device name and class
  (`emulator`, `simulator`, `physical`, `cloud`), provider, locale and, with `--junit-label`, a `run_label`.
- A data-driven test is one case per row, named `name [row N]`, with a `row` property. **Inserting or reordering rows renames the tests**, which
  starts a new history in any tool that keys on the name; append new rows at the end.
- A re-run test (`retry_failed_tests`) has an `attempts` property on its case.

## Retries and flaky tests

By default a test that failed and then passed on a retry is **one passing case** (with the `attempts` property): a CI summary shows a green run.
A flaky-test service learns about flakiness by seeing both a failure and a pass of the same test; Trunk's pass-on-retry monitor, for example, is
documented as detecting "a test fails, is retried, and passes on the same commit" (https://docs.trunk.io/flaky-tests/detection/pass-on-retry-monitor).
Add `--junit-attempts` to write **one `<testcase>` per attempt** with the same `classname` and `name`: the failed attempts carry `<failure>` or
`<error>`, the last one carries the final outcome, and each has an `attempt` property such as `1/2`. It is off by default because the extra failed
cases change the counts a CI summary shows for a run that finally passed.

None of the three services documents the Maven-style `<flakyFailure>` / `<rerunFailure>` elements, so FlutterProbe does not write them; repeated
test cases are the representation the Trunk page above describes (each attempt as its own result of the same test).

## Same test on several devices

Two devices produce two cases with the same `classname` and `name`. Give each run its own identity with `--junit-suffix -ios` (appended to
every `classname`) and/or `--junit-label ios-15` (a `run_label` property), or upload each device's report separately.

## What each service documents

| Service | Fields it names | Checked | Not verified |
|---|---|---|---|
| **Trunk** (Flaky Tests) | "the file name, test name, and stack trace of each test result"; test names must not be randomized; flakiness is judged from "each test case's results over time" ([other test frameworks](https://docs.trunk.io/flaky-tests/get-started/frameworks/other-test-frameworks)) | `trunk-analytics-cli validate --junit-paths results.xml` runs **locally, no account** ([uploader](https://docs.trunk.io/flaky-tests/uploader)) | its exact required attributes and test identity key; whether a report with repeated cases is read as retries |
| **Codecov** (Test Analytics) | `classname`, `name`, `time`; its example shows `errors`, `timestamp` on the suite ([test analytics](https://docs.codecov.com/docs/test-analytics)) | the suite attributes above match that example | how repeated cases are treated; any offline validation |
| **Currents** (JUnit upload) | per-attempt artifacts through `<properties>` in a `<testcase>`; `--framework` is required ([convert](https://docs.currents.dev/resources/reporters/currents-cmd/currents-convert)) | the `file` attribute gives it a spec file | whether a `--framework` value for Flutter is accepted; how attempts map from JUnit |

To try it yourself with no account: `probe test tests/ --format junit -o results.xml --junit-attempts`, then
`./trunk-analytics-cli validate --junit-paths results.xml` (download the CLI from the Trunk docs page above).
