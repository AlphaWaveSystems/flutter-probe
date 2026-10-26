---
title: CLI Reference
description: Complete reference for all probe CLI commands and flags.
---

## Commands

| Command | Description |
|---------|-------------|
| `probe init` | Scaffold `probe.yaml` and `tests/`, with a sample test that only opens the app. Set `project.app` in `probe.yaml` first |
| `probe validate` | Check `probe.yaml` and the `probe_version` pin without connecting to a device. Exits non-zero when a check fails; `--format json` prints the report. Run it in CI before `probe test` |
| `probe doctor` | Check that `probe.yaml`, the `tests/` folder, the Flutter SDK, the platform tools (`adb`, `xcrun`) and the visible devices are ready for `probe test`. Says how to fix what is missing and changes nothing. Exits non-zero only when `probe.yaml` or `tests/` is missing. `--format json` prints the report |
| `probe test [path]` | Run `.probe` test files |
| `probe lint [path]` | Validate `.probe` files without running them: syntax errors, steps and recipe calls that resolve to nothing, and selectors that can never match fail the command; unresolved placeholders and count or zero-state text checks warn |
| `probe perf trend` / `probe perf compare` | Show a measured number over past runs, or compare two baseline files. See [Performance testing](/advanced/performance/) |
| `probe device list` | List connected devices/simulators |
| `probe device view` | Interactive device view over a local WebSocket (preview) |
| `probe device start` | Start an emulator/simulator. `--name` names the iOS simulator it boots (created when missing); Android emulators run under their AVD name |
| `probe record` | Record user interactions as ProbeScript |
| `probe report` | Generate HTML report from test results |
| `probe migrate` | Convert Maestro YAML flows to ProbeScript. A flow pulled in with `runFlow` is converted to a recipe file (`recipe "flow <file name>"`) and the caller gets `use "<file>.probe"` plus a call to it. Maestro regex selectors (`A\|B`, `.*x.*`), `evalScript` and `${ENV}` placeholders are not translated: they get a `# TODO` comment and a warning |
| `probe generate` | AI-assisted test generation (any `ai:` provider, including a local model) |
| `probe system-dialog` | Drive OS system dialogs (permission alerts, sign-in sheets): `tap`, `type`, `see`, `wait`, `dismiss`, `list`, `sign-in-sandbox`. See [System dialogs](/tools/system-dialogs/) |
| `probe ios-driver` | Manage the iOS system-dialog driver: `install`, `status`, `stop` |
| `probe ai doctor` | Check the configured `ai:` provider: endpoint, model, text, vision. Optional — see [AI & local models](/tools/ai/) |
| `probe triage` | Explain failures from a JSON results file with the configured model. Advisory; never re-runs or changes results |
| `probe studio` | Launch interactive test studio |

`probe version` prints the installed version; a canary ends in `-next.N` (see [Try a canary build](/getting-started/installation/#try-a-canary-build)).

## probe test

Run `.probe` test files against a connected device.

### Usage

```bash
probe test [path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--device <serial>` | auto | Target device UDID or serial |
| `--host <ip>` | `127.0.0.1` | Agent host IP (use device IP for WiFi testing) |
| `--token <string>` | — | Agent auth token (skip auto-detection; for WiFi testing) |
| `--parallel` | `false` | Run tests in parallel across all connected devices |
| `--devices <serials>` | — | Comma-separated device serials for parallel execution |
| `--tag <tag>` | — | Run only tests matching this tag |
| `--timeout <duration>` | `30s` | Per-step timeout |
| `--format <fmt>` | `terminal` | Output format: `terminal`, `junit`, `json` |
| `--junit-attempts` | `false` | With `--format junit`: one `<testcase>` per attempt of a test that was re-run (failed attempts as `<failure>`/`<error>`), so flaky tests are visible to tools that track them. See [Results in other tools](/ci-cd/analytics-tools/) |
| `--junit-suffix <text>` | — | With `--format junit`: appended to every `classname` (for example `-ios`) so the same test on several devices has distinct identities |
| `--junit-label <name>` | — | With `--format junit`: written as the `run_label` property of every suite |
| `-o, --output <path>` | — | Output file path for reports |
| `-v, --verbose` | `false` | Verbose output |
| `--locale <tag>` | — | Run the app in this language (`de`, `pt-BR`, `ar`, `system`) from the start: sets the per-app language and relaunches before the agent connects. Android 13+ and iOS simulators; the device language and system dialogs are unchanged. The tag is recorded in the JSON/HTML report |
| `--locales <a,b,c>` | — | Run the whole suite once per language (`--locales de,ja,ar`), each in its own run with `--locale`; `-o report.json` becomes `report.de.json`, `report.ja.json`, ... and a pass/fail table per language is printed. Exit code is non-zero if any language fails |
| `--match-loose` | `false` | Match text selectors loosely: fold case, accents and diacritics, typographic apostrophes and dashes, full-width forms and whitespace on both sides (`Änderungen` matches `anderungen`, `Don't Allow` matches `Don’t Allow`). `defaults.match: loose` in probe.yaml sets it permanently. Ids (`#key`) are never folded |
| `--timeout` | `30s` | Per-step timeout. A single step can override it: `... within 90 seconds`. See [Timeouts, waiting and retries](/advanced/timeouts-and-retries/) |
| `--upload-report` | `false` | Upload the scrubbed JSON report to the hosted service (Probe Cloud, preview; needs a `probe_cloud` license; also `hosted.upload_report`). Warnings only, never fails the run. See [Probe Cloud (preview)](/tools/probe-cloud-preview/) |
| `--upload-label NAME` | — | Label for this upload (also `hosted.label`): a suite, shard or matrix leg such as `shard-2-of-4`; two uploads of one CI job must differ in it |
| `--upload-dry-run` | `false` | Print the exact payload `--upload-report` would send and send nothing (no license needed) |
| `--record` | `false` | Record each test step locally; recordings are never uploaded |
| `--record-dir DIR` | `reports_folder/recordings` | Recording base directory; used with `--record` (usually `reports/recordings`) |
| `--upload-local` | `false` | Send this run as a local run (Probe Cloud, preview; needs a `probe_cloud` license and a team that enabled local uploads). Git facts instead of CI facts; unverified evidence that never counts toward a pull request's verdict. Never implied by `--upload-report`, and `hosted.upload_report` does not apply to it. See [Probe Cloud (preview)](/tools/probe-cloud-preview/) |
| `--measure-all` | `false` | Measure every test: open a performance window named after the test around its body (also `perf.measure_all` in probe.yaml). Tests that use `start measuring` themselves are left alone. See [Performance testing](/advanced/performance/) |
| `--perf-baseline FILE` | — | Compare every `start measuring` window with this baseline; a test whose numbers got worse than the tolerance fails. See [Performance testing](/advanced/performance/) |
| `--perf-metrics LIST` | memory,cpu,slow_frames,frame_time,data on physical devices; memory,cpu,data on emulators and simulators | Metrics a baseline comparison fails on (comma separated, or `all`). `memory_growth`, `cpu_peak` and `frame_max` are single-sample extremes and only gate when listed; `cpu` only for windows of 5 s or more |
| `--perf-ignore LIST` | — | Metrics to leave out of the comparison |
| `--perf-key-base DIR` | cwd | Directory baseline keys are relative to, so the same tests match from another checkout |
| `--perf-update-baseline` | `false` | Write this run's measurements of passing tests to the `--perf-baseline` file (default `perf-baseline.json`) |
| `--perf-tolerance` | `20` | Percent a number may exceed its baseline by (small absolute changes are ignored) |
| `--perf-history FILE` / `--no-perf-history` | `reports/perf-history.jsonl` | Where measurements are appended for `probe perf trend`, or turn it off |
| `--retry-failed` | `0` (off) | Re-run a failing test up to N extra times before reporting it failed (also `defaults.retry_failed_tests`). A pass on a retry is reported with its attempt count |
| `--implicit-wait` | `0` (off) | Retry `tap`, `type`, `long press`, `double tap`, `clear`, `drag` and plain `see` steps whose target is not on screen yet for up to this long before failing, like Maestro's implicit waiting (`--implicit-wait 7s`). `defaults.implicit_wait` in probe.yaml sets it permanently. Steps with `if visible` / `optional`, `don't see` and explicit waits are never retried |
| `--no-grant-on-clear` | `false` | After `clear app data` keep runtime permissions revoked even with `--yes`, so a test can see the first-run permission dialog |
| `-y, --yes` | `false` | Auto-confirm destructive ops + auto-grant permissions (on an iOS simulator notifications cannot be granted this way: pass `--grant notifications` to answer the alert) |
| `--watch` | `false` | Watch mode — re-run on file changes |
| `--video` | `false` | Enable video recording per test |
| `--video-resolution` | `720x1280` | Android screenrecord resolution |
| `--video-framerate` | `2` | Video framerate (fps) |
| `--visual-threshold` | `0.5` | Max allowed pixel diff % for visual regression |
| `--visual-pixel-delta` | `8` | Pixel color delta tolerance |
| `--port <int>` | `48686` | Agent WebSocket port |
| `--agent-port <int>` | — | Alias of `--port`. Pair with the app's `--dart-define=PROBE_PORT=<n>` so two simulators (or a stale process on 48686) can't collide. A failed dial on loopback names the process holding the port. |
| `--ai-triage` | `false` | After the run, ask the configured `ai:` model to explain failures (advisory, needs `ai:` in `probe.yaml`). Runs after results are final; cannot change results, reports or the exit code. Writes `<reports>/triage.md` |
| `--grant <list>` | — | Pre-grant OS permissions once before the first test, e.g. `--grant notifications,camera,location`. Android: `adb shell pm grant`; iOS simulator: `simctl privacy`. iOS notifications have no simctl service: a watcher taps Allow when the alert appears (needs Xcode, see [System dialogs](/tools/system-dialogs/)). |
| `--dial-timeout` | `30s` | WebSocket connection timeout |
| `--token-timeout` | `30s` | Agent auth token wait timeout |
| `--reconnect-delay` | `2s` | Post-restart reconnect delay |
| `--app-path <path>` | — | Install app before testing |
| `--adb <path>` | `adb` | Override ADB binary path |
| `--flutter <path>` | `flutter` | Override Flutter binary path |
| `--config <path>` | `probe.yaml` | Config file path |
| `--shard <N/M>` | — | Run shard N of M (for parallel CI) |
| `--fail-on-warning` | `false` | Treat agent warnings as step failures: a tap that provably did nothing (covered, screen unchanged), `press enter` with no focused field, `go back` at the root route |
| `--dry-run` | `false` | Parse and validate without executing; also fails on any step that is not built in and matches no recipe (typos, unknown verbs), including inside recipes |

### Examples

```bash
# Run all tests verbosely
probe test tests/ -v

# Run with custom timeout and auto-confirm
probe test tests/ --device emulator-5554 --timeout 60s -y

# Generate JUnit report for CI
probe test tests/ --format junit -o reports/results.xml

# Parallel CI sharding
probe test tests/ --shard 1/3
probe test tests/ --shard 2/3
probe test tests/ --shard 3/3

# WiFi testing on physical iOS device
probe test tests/ --host 192.168.1.100 --token abc123... --device <UDID>

# Parallel across multiple devices
probe test tests/ --parallel
probe test tests/ --devices emulator-5554,<iOS-UDID>
```

### Local step recordings

`probe test --record` writes a separate bundle for each test attempt beneath
`<reports_folder>/recordings/<UTC run timestamp>/<device>/`. Use `--record-dir DIR`
to choose another base directory. Each bundle includes a manifest, a step timeline
with source file and line, screenshots before and after each step, a widget tree
after each step, the app's `dart:io` HTTP exchanges with credential redaction, and
an offline HTML viewer. Capture failures produce warnings and do not change test
results or exit codes.

A step that taps, double taps, long presses, types, clears, toggles, or swipes or
scrolls with a selector also records that element: its selector, and its bounds and tap point when the element is
on screen as the step starts. Text typed into a field is never recorded.

Native SDK and WebView traffic and screenshot redaction rules are not yet recorded.
Widget trees are not captured before steps.
Screenshots are not redacted. Recordings can contain screens and request or
response bodies; keep them out of version control. Recording sends nothing to a
hosted service, including when report upload is enabled.

## probe lint

Validate `.probe` files without executing them.

```bash
probe lint tests/
probe lint tests/smoke/login.probe
```

Lint checks the references a run would resolve, without running a step, and fails the command (exit status 1) for an error:

- A step or recipe call that matches no built-in step and no recipe in the file, its `use` statements or the recipes folder. This is the check `probe test --dry-run` makes. A `use` statement inside a test is reported too, because a file reads `use` only before its first test. A `go back` followed by words on its line is reported with that reason, because `go back` takes no arguments.
- A selector that can never match: an ordinal selector without a number and text, an `#id` without a name, a `below`, `above`, `left of` or `right of` selector without anchor text, or an `in` with an empty container.

It warns, without failing, when:

- A `<name>` placeholder is not an example column, a recipe parameter, a value stored with `store` or `read ... with ai`, one of the runtime values `clipboard`, `response.status` or `response.body`, or a `<random.*>` generator. A run leaves such a placeholder in the text as written.
- A `use` names a file that does not exist. A run only needs that file when a call uses one of its recipes, and such a call is reported as an unknown step.
- A `see` or `wait until ... appears` whose text is a count or a zero state (`see "0 items"`, `wait until "No items" appears`) and has no `matching` pattern, because a substring match also passes on "10 items". Anchor the pattern for an exact check: `see "0 items" matching "^0 items$"`. A `matching` pattern anchored at both ends (`^...$`) silences the warning; any other pattern is still a substring match and is still flagged.

Environment variables (`${NAME}`) are not checked, because lint does not know the environment that will run the tests.

## probe record

Record user interactions and generate ProbeScript.

```bash
probe record --device <UDID> --output tests/my_flow.probe
probe record --timeout 60s -o tests/flow.probe
```

## probe report

Generate an HTML report from JSON test results.

```bash
probe report --input reports/results.json -o reports/report.html
probe report --input reports/results.json -o reports/report.html --open
```

## probe device

Manage connected devices and emulators.

```bash
probe device list
probe device start --platform android
probe device start --platform ios
probe device start --platform ios --name probe-checkout   # boot (or create) a simulator called probe-checkout
probe device view --device <id>                            # interactive device view over a local WebSocket (preview)
probe device set appearance dark --device <id>             # dark mode, font scale, app language, orientation, deep link
```

`probe device view` starts a hub on `127.0.0.1` with a one-time token and prints one JSON line with `url`, `token` and `device`. It shows the Flutter layer from the on-device agent (not the status bar or system dialogs), accepts tap, long press, swipe and typed text, and writes no files. The protocol is described in [`docs/studio-device-protocol.md`](https://github.com/AlphaWaveSystems/flutter-probe/blob/main/docs/studio-device-protocol.md). The VS Code command "FlutterProbe: Open Device View" uses it.

### Device settings: `probe device set`

`probe device set <setting> <value> --device <id>` changes an OS setting of one named emulator or simulator, so a test can be tried in dark mode, with a large font or in another language without opening the system settings. `--device` is required (a serial, a UDID or a unique device name); `booted` is not accepted, so a change never lands on whichever device is running. Values are checked before anything runs.

| Setting | Android (`adb`) | iOS simulator (`simctl`) |
|---|---|---|
| `appearance <light\|dark\|auto>` | `cmd uimode night` (`auto` is Android only) | `simctl ui appearance` |
| `font_scale <value>` | a factor from 0.5 to 3.0, such as `1.3` (`settings put system font_scale`) | a content size category, such as `accessibility-large` (`simctl ui content_size`) |
| `locale <tag> --app <id>` | `cmd locale set-app-locales` (Android 13 or later; the app under test) | relaunches the app with `-AppleLanguages` and `-AppleLocale` (the app restarts) |
| `orientation <name>` | `portrait`, `landscape`, `reverse-portrait`, `reverse-landscape` (turns auto-rotate off) | not available: `simctl` has no rotate command |
| `open_url <link> [--app <id>]` | `am start -a VIEW`, restricted to `--app` when given | `simctl openurl` |

Physical iOS devices are refused (these settings use `simctl`). The settings persist on the device until you change them back, so set `appearance light`, `font_scale 1.0` or `medium`, and `orientation portrait` when you are done. The same operations are the `set_device_setting` MCP tool.

### Named devices

Every simulator or emulator probe opens has a name, and every test result records the device it ran on (name and id/serial; physical devices by model or serial). `probe device list` shows running emulators by AVD name, so `probe-checkout` or `my-avd` appears in JUnit/JSON/HTML reports instead of a bare serial. `--name` on iOS boots the simulator with that name or, when none exists, creates one with the default simulator's device type and runtime. Android emulators are named by their AVD (create it with Android Studio or `avdmanager`, then `probe device start --avd <name>`).

## probe-convert

Standalone tool for converting tests from other frameworks. See [probe-convert](/tools/probe-convert/) for full documentation.

```bash
probe-convert <file|dir> [flags]
probe-convert catalog [lang]
probe-convert formats
```
