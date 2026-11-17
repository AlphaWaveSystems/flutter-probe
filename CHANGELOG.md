# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

> At release time the version in `probe_agent/pubspec.yaml` and the one in `probe_agent/lib/src/agent_version.dart` are bumped in the same commit, and never in a feature change. A step is allowed by the capabilities an agent reports, not by its version number.

### Added
- **A plan for the next compatibility tests (`docs/compat/NEXT.md`).** It ranks the widgets that the matrix has not assessed by how many of the 36 packages of `flutter/samples` (at a fixed commit) construct them. `Text` comes first, then the app bar and routes, widgets without text, scrolling lists, tiles, progress indicators, gestures, dialogs, snack bars, the Cupertino bars and overlapping widgets. For each it gives the fixture screen, the ProbeScript steps and the variants that must fail. It names the open questions each test settles, among them a device run of the route rule (`see` does not match a route under an opaque one, which an agent unit test covers; under a dialog the page stays matched by design) and whether a lazy list's unbuilt rows count as absent. The follow-ups for the partly assessed cells are listed, and the families the source cannot rank (pickers, menus, tabs, drawers, sheets) are listed without a usage claim. `docs/compat/widget-usage.py` is the count, to run again on any checkout. Documentation only; no matrix cell changes.
- **A design partner application form, and one way to apply on every page.** `.github/ISSUE_TEMPLATE/design-partner.yml` is a GitHub issue form ("Design partner application", label `design-partner`): team name, GitHub owner of the repository, CI provider (GitHub Actions or other), device types, what you test today, and an optional contact email, under a warning that the issue is public with the email path beside it. `config.yml` offers that email path in the template chooser. The pricing page, the Probe Cloud page, the design-partner guide and the Probe Cloud preview docs now all offer the same two ways: **Apply on GitHub** (`/issues/new?template=design-partner.yml`) and **Apply by email** (support@alphawavesystems.com). The pricing page's "Apply as a design partner" button opens the form, and the plain "open an issue" links are gone. `scripts/site_apply_links_test.go` checks the form against GitHub's issue-form rules and the fields above, and checks that every link on the site whose text says "apply" goes to one of the two targets, that no page opens a new issue any other way, that each offer page gives both, and that every "waitlist" link goes to the email or to `#waitlist`. Website and repository files only.
- **The WebView suite reads the page's result text, not only its existence, and its negative variants are in the repository.** `see css:"#result" in webview` only shows that an element exists; the suite now reads its text with `store text` and checks it through the Flutter echo. The 9 variants that must fail are `native-test-apps/flutter-webview/probe-tests/negative/webview.probe.expect-fail` (named so that a directory run never picks them up). The case rule of the app id warning is one tested method, `Platform.AppIDIsCaseInsensitive`. Fixture, tests and docs only.
- **The Flutter fixture app has a buttons screen and a suite for it, and the matrix has its Buttons results.** `PROBE_FIXTURE_WIDGETS=buttons` builds elevated, filled, outlined, text, icon (with a tooltip), floating action, Cupertino, with-icon, disabled and long-press buttons; `probe-tests/widgets/buttons.probe` (7 tests) passes on builds by Flutter 3.44.8 and 3.50.0 on an Android 15 emulator, and its 8 negative variants (`probe-tests/negative/buttons.probe.expect-fail`, which no directory run picks up) all fail for the stated reason. Findings in `docs/compat/MATRIX.md`: a tap on a disabled button raises no error (the fixture cannot show whether the tap did anything: the button has no handler); a held press on a button with no long-press handler ends as a tap (Flutter's own behavior, measured on 3.44.8 and 3.50.0); icon and floating action buttons are found by their tooltip. Fixture and docs only.
- **A Flutter app with a WebView, to run the WebView steps through `probe test`, and the matrix row for it.** `native-test-apps/flutter-webview` hosts the agent and shows the page of the native WebView fixture through `webview_flutter` 4.14.1 (pinned, with its lock file) from its own assets, with Flutter widgets next to it; `probe-tests/webview.probe` (5 tests) passes on builds by Flutter 3.44.8 and 3.50.0 on an Android 15 emulator, and variants of it that assert something wrong or cross the boundary all fail. `docs/compat/MATRIX.md` records what it shows: find, tap, type (read back), read and wait work in the page; the Dart steps do not see page content and the WebView steps do not see Flutter widgets; scroll is not assessed. `docs/compat/RELEASES.md` has the package entry. Fixture and docs only.
- **The app id warning compares the id without regard to case on iOS only.** An Android package name is case-sensitive (`com.Example.app` is another app), an iOS bundle id is not. `AppIDMismatchWarning` takes `ignoreCase`; the CLI passes it for an iOS device.
- **The agent reports the application id of its app, and the CLI warns when it is not `project.app`.** `probe.ping` and `GET /probe/status` carry `app_id` (on Android the agent's own process name, which is the package name; or `--dart-define=PROBE_APP_ID=<id>`), left out when it is not a plain id. The CLI validates it like the build-info fields (plain text only, dropped otherwise) and, after the handshake of `probe test`, of each lane of a parallel run and of a reconnect, prints a warning when the agent's id differs from `project.app`. A warning, not a refusal: the id is the app's own statement, and a suffixed build id would otherwise break runs. It lets a run see that another app's agent answered; the port-following of #741 already avoids that case when it can. Needs a new `probe_agent` release; an older agent sends no id and nothing changes.
- **On Android the CLI follows the agent to the port the app under test really listens on, when another app with the agent holds the configured one.** Two apps on one device cannot both bind the agent port: the second app's agent moves to a port above it, and the CLI, which forwarded the configured port, reached the first app (a 401 with the second app's token) or nothing. After a failed dial (a 401, or no answer for 3 seconds) the CLI now looks, read-only, at `/proc/net` and `pm list packages -U` for the one port in the 10 above the configured one that the app's own package uid listens on, moves the `adb forward` there and prints `agent port N on the device is held by <package>; using M for <app>`. It needs `project.app`. It does nothing when the app listens on none or several of those ports, when the package shares its uid, or when the files are unreadable (the usual hint is then printed). A token that was not read through `run-as` does not prove the app (it can be the other app's), so then the port is checked before the first dial, and again when a token is read again after the agent rejected one (on a cold start the run-as read can fall back to a shared source): when another app holds it, the CLI looks for the app's own port for the whole dial window (the app may still be starting), and when it finds none it **refuses to connect** and says which app holds the port (`agent port N on the device is held by <package>, and <app> has not opened an agent port; not connecting ...`) instead of dialing the other app. With a `run-as` token (the usual case when `project.app` is set) a run with one agent makes no extra adb call; with another token source it makes the three read-only calls of the port check before the first dial; an iOS simulator is untouched; each device of a parallel run looks for itself. New API: `ReadTokenAndroidSource` (the old `ReadTokenAndroid` delegates), `device.PortFollower`, `probelink.DialOptions.OnPortSuspect`. Not covered (they keep forwarding or dialing the configured port): Studio, Studio's Wi-Fi target (it dials a host and port, with no adb forward), `probe record`, the device view, the cloud path of `probe test` (the provider forwards the port) and the first connect of a composite test. Still unprotected: `/proc/net` that cannot be read (nobody can say who holds the port, so nothing is refused) together with an agent too old to move its port and an app that is not debuggable (no `run-as`): the token and the port can then both belong to another app. The agent needs no change.
- **The Flutter fixture app has widget family screens and tests, and the compatibility matrix has its first results.** `native-test-apps/flutter` builds the screen of one family with `--dart-define=PROBE_FIXTURE_WIDGETS=<family>` (`selection`: checkbox, switch, radio group, slider, Cupertino switch; `textinput`: plain, form, obscured, multi-line and Cupertino fields), with a ProbeScript test per family in `probe-tests/widgets/`; both pass on builds by Flutter 3.50.0 and 3.44.8 on an Android 15 emulator. `docs/compat/MATRIX.md` records what each test shows and what it does not, including one finding: the agent answers `is checked` "not checked" for a radio that is on, under the `RadioGroup` API (a fix is a separate change). Fixture app and docs only.
- **Clear the app's data before every test (`defaults.clear_data_before_each_test`, `--clear-data-before-each-test`, and a Studio box).** With it on, every test (and every retry and every data-driven row) starts after the app's data was wiped and the app relaunched, so each begins from a clean state without a `clear app data` line in the file. The wipe is the same one `clear app data` does, and it goes through the one gate every wipe uses (`DeviceContext.AskClearData`): it is asked **once for each device of the run** (a parallel run: once in each lane) (the question names the package and the device, cleaned to one line, and says it happens before every test) and is answered by `--yes` or by the confirmation; with `--yes` nobody is shown the question, so the log says, once for each device (a parallel run prints it for each lane, with that lane's device), `--yes: all data of <package> on <device> will be wiped BEFORE EVERY TEST on this device`; on a simulator or an emulator the person's own setting `probe prefs set clear-data-without-asking on` (or Studio's box) answers it, with the line that names the package and the device, so the choice is yours: automatic on a simulator or an emulator, or asked. A physical phone still asks, and a physical iPhone cannot be cleared (the existing skip). **A parallel or composite run (`--devices`) answers it the way it answers every wipe: with `--yes` only.** Its lanes cannot ask, so without `--yes` every test of the run fails with the hint to pass `--yes` and nothing is wiped. A no, or nobody to ask in CI, fails every test the same way (the message says to pass `--yes`) without running a test body or its before-each hooks; the on-failure and after-each hooks and the failure screenshot still run, on the app that was not cleared. The flag overrides probe.yaml in both directions (`--clear-data-before-each-test=false`). With no local device (cloud mode, a dry run) the setting does nothing and a line says so once. The clear and relaunch count against `--test-timeout` of each attempt. Studio: "Clear the app's data before every test" in Workspace settings writes the probe.yaml key like the other settings. The answer to the per-run question does not become permission for a `clear app data` step inside a test, which still asks.
- **`clear app data` can run without the question on a simulator or an emulator, in `probe` and in Studio, as your own setting.** `probe prefs set clear-data-without-asking on` (or the box under "Studio on this computer" in Workspace settings) keeps the choice in a file in your user configuration folder (`<config dir>/flutter-probe/prefs.json`, private to you, replaced atomically). It is never read from a project, so a `probe.yaml` or a cloned repository cannot turn it on; the Studio automation endpoint and the headless bridge cannot press it. It applies only to a device that is verified to be a simulator or an emulator, checked when a step is about to ask: the device list says so (an Android serial that starts with `emulator-`, an iOS device with no USB or network transport) **and** an Android device says so itself (`ro.kernel.qemu` or `ro.boot.qemu` is 1; any error, empty answer or timeout is no, because a serial can be given any name). A physical phone, a WiFi connection and anything unclassified always ask (or, in CI, need `--yes`). The skipped question is named: `cleared data of <package> on <device> (no question: your setting ...)` in `probe`, and the same in a Studio note. A preferences file that is a link, or that other users can read or write (a mode wider than 0600), or whose folder is a link or can be written by group or others (a mode with 020 or 002; modes are not checked on Windows), is not trusted: the setting is off and one line says why (`probe test` prints it, Settings shows it). A file that is not exactly the preferences is the defaults, everything off. In `probe test` it answers only the question, so unlike `--yes` it does not grant permissions. New: `internal/userprefs`, `probe prefs`, bound methods `GetStudioPrefs` and `SetClearDataWithoutAsking` (decision-table category `userPrefs`). **Not covered (existing behaviour, not changed here):** a parallel run (`--devices`) answers every `clear app data` question yes on every device, physical ones included, as it did before; the setting neither adds to nor limits that.
- **Studio shows what the app reports about its own build.** When the agent sends them (`flutter_version`, `flutter_channel`, `dart_version`, `framework_revision` in the `probe.ping` result), the Device pane shows one line under its title, `App built with Flutter 3.44.8 (stable), Dart 3.12.2 · framework revision 1a2b3c4d5e`, with a hover that says a fork or a custom embedder of Flutter can report other values. Each part is shown only when it was reported: an older agent, or a build that did not set the values, shows nothing (no placeholder, no guess), and the line goes with the connection. The text is set as text, comes from the Go side's validated fields, and is not sent to an AI provider. Studio draws no conclusion from it (no compatibility verdict). New: `probelink.CheckHandshakeResult` (the whole handshake result; `CheckHandshakeVersion` is built on it), and `ConnectionStatus.appBuild`.
- **The compatibility catalog and release log are current to Flutter 3.50.0 (Dart 3.14.0), and the catalog no longer records the SDK's channel.** `docs/compat/catalog.json` is regenerated from a verified 3.50.0 SDK: 572 widgets, two added (`SliverClipRect`, `SliverClipRRect`), none removed or changed. `docs/compat/RELEASES.md` has entries for 3.50.0, 3.47.0 to 3.47.9 and Dart 3.13 and 3.14, with the tool requirements read from the tool source (3.50.0 raises the minimum Gradle to 9.3.1, AGP to 9.1.1 and Kotlin to 2.4.10, above the Flutter fixtures' versions). `tools/flutter-catalog` stops recording the channel, which says how an SDK was installed and not which release it is, so the weekly `-check` can match a CI-installed stable release.
- **The agent reports the Flutter and Dart that built the app, and the CLI carries them (`probe.ping`).** New optional fields `flutter_version`, `flutter_channel`, `framework_revision` and `dart_version` in the ping answer, read from the compile-time defines the Flutter tool puts into every app build (so they are the app's build, not the Flutter that published `probe_agent`); the Dart version falls back to the running VM's. `probelink.HandshakeResult` has the four fields, filled only with plain version text (letters, digits, dots, dashes and plus signs, at most 64 characters) because the text comes from the app; anything else, or a field of the wrong type, reads as empty and never fails the connect. An older agent sends none and an older CLI ignores them. Nothing is shown yet: Studio's display and a note when the app's Flutter is newer than the version the compatibility matrix was assessed on come next.
- **Studio: Settings writes `test_timeout`, `retry_failed_tests` and `implicit_wait`** (Test time limit, Retry a failed test, Wait for a target, under `defaults:` in `probe.yaml`). Validated in Go with the CLI's own rules for the limit (at least 1 s, 0 = off) plus the form's bounds (retries 0 to 10, wait up to 5 m; a value beyond them that is already in the file stays). **Settings now edits `probe.yaml` in place:** the previous writer decoded and re-encoded the whole file, which dropped comments and blank lines and re-ordered keys, for the form and for the AI provider section. Both now change only the lines whose value changed, leave everything else byte for byte, replace the file atomically (temp file in the same folder, then a rename) with its mode kept, refuse a `probe.yaml` that does not parse, that the engine cannot load after the edit, or that is a link, and a save for any folder but the workspace that is open (`SaveWorkspaceSettings` and `SaveAISettings` check it, so a crafted call cannot write another folder's `probe.yaml`), and never write a field that was not changed. The edited text is read back with the engine's parser before the write and refused unless it reads as the original plus exactly the edited keys; the form sends the values it was loaded with, and a change to a field that `probe.yaml` changed under the form is refused with a reload message; two saves never run at once; line endings are kept per line (a file with mixed endings gets new lines in its dominant one); the file is replaced by a rename, so a hard link to it is not kept. A new `probe.yaml` created by Settings is mode 0600 (it was 0644 for the non-AI form).
- **Studio: export the results of a run as a JSON or JUnit report.** **Export JSON** and **Export JUnit** in the Results header write the same report `probe test --format json|junit` writes, from the results the engine returned (not the window's copy), to a path chosen in the native save dialog. The bound method `ExportRunResults(format)` takes no path: the path is the dialog's, checked (absolute, the format's extension, not a folder or link, an existing folder) and written atomically; cancelling saves nothing. It is a new decision-table category (`writesOutside`) that automation may never hold, refused over the headless bridge, and kept out of every AI path. The report has the same content as the CLI's (error messages are written as the engine produced them and can contain local paths: check it before sharing); a run that was stopped leaves nothing to export; opening another workspace drops the old results; the buttons follow Studio's own state (`CanExportRunResults`), also after a reload.
- **The Flutter widget catalog and the compatibility rules (`docs/compat/`).** `catalog.json` lists the 570 public widgets of Flutter 3.44.8 (Dart 3.12.2), generated by the new `tools/flutter-catalog` from an installed SDK. The tool only reads the SDK, records names only, and gives the same bytes on every run. `MATRIX.md` starts with every cell "not assessed" and `RELEASES.md` with the first entry. Documentation and a tool only; no behaviour changes.
- **A weekly compatibility check (`.github/workflows/compat-drift.yml`).** Every Monday, and on demand, it reads Flutter's official release manifest, fails when the newest stable release is not in `docs/compat/RELEASES.md`, and runs `tools/flutter-catalog -check` against that SDK. A failure opens or updates one issue labelled `compat-drift`. It is CI only: no product behaviour changes.
- **When the app has died, a failed reconnect says so, and no longer blames an `adb forward` that is the CLI's own (Android).** After a step lost its connection and the auto-reconnect failed, the error used to end with hints that could point the wrong way, such as an adb port forward "left over from an earlier run" when the forward was the one probe had made for the app that had just died. Now the reconnect's own dial treats the host port as probe's own forward on Android, and after a failed reconnect the error names the cause: the app is not running (after the test's own `kill the app`: the test stopped it, with `open the app` or `restart the app` to start it again; otherwise: it ended during this step, and with `--crash-evidence` the crash evidence for the test says why, without it a hint to use the flag), or, when the app runs, that the agent port is forwarded to another device (named, with the command that removes that forward). It makes at most three read-only adb calls (the device's clock, `pidof`, and `forward --list` when the app runs), only after the reconnect has failed, and changes no reconnect behaviour. A forward counts as another device's when its serial differs from the one probe uses, so a device over Wi-Fi that `adb forward --list` shows under another form of its serial (an mDNS name instead of ip:port, say) can be named as another device; a serial that is not plain (a letter or digit first, then letters, digits and `. _ : -`) is never printed. `open link` now counts as a launch like `open the app`, so a link that starts a killed app is not mistaken for a stopped one. **A deliberate false positive:** an app that was stopped by the system or the user while the test did nothing has no process and is reported as ended during the step, since that cannot be told from a death.
- **Studio says how long the running step has waited, and when it is still waiting.** A step that waits for a text that never appears holds the run for the step timeout (30 s by default), and the toolbar only said "step N of M": a run that was just slow looked hung. The progress now adds the running step's elapsed time after three seconds ("step 2 of 5 · 7 s"); after ten seconds on one step a line under the toolbar says "Still waiting for tap \"Pay\": Cancel stops the run." (it does not change as the seconds pass, so a screen reader hears it once; the progress text is no longer a live region); and when the device stops sending its screen, "no signal" shows next to Cancel at once and the waiting line says the device has stopped sending its screen. A step that finishes, or the end of the run, takes them away. Text only.

- **Studio warns when a step uses a `${NAME}` that its environment does not have.** `probe test` reads `${NAME}` from the shell it runs in. A Studio opened from the Dock or Finder does not have that shell's variables, so the engine left `${USER_EMAIL}` as written (with a warning on stderr, which a window never shows) and the test typed or asserted the literal text and failed with a confusing "not found". The Problems tab now lists each unset variable as a warning on its line, naming the variable and the way out (start Studio from a terminal where it is set). It reads the strings the parser holds for the steps of tests, hooks, recipes and composite tests, so a `${X}` in a comment is not one, and only asks whether a variable exists: no value is read, shown or kept. Studio has no place to set variables (they often hold secrets). New `runner.UnsetEnvRefs`; `probe test` and `probe lint` are unchanged.
- **Studio says in the Workspace pane when `probe.yaml` will trip a first run.** A workspace with no `probe.yaml`, one without `project.app`, or one that does not parse gave no sign until a step failed with a raw system error (`The request to open "" failed`) or, for a file that does not parse, until a run silently used other settings. A one-line note now says it: "No probe.yaml in this folder: runs use the defaults (agent port 48686) and no app id", "probe.yaml has no project.app: steps that restart or clear the app ... will fail until it is set", or "probe.yaml cannot be read, so Studio is not using it: <reason>" (one line, no path). It reads the file with the loader runs use, is plain text, and the refresh button reads it again. New bound method `WorkspaceNote` (reads one file in the workspace).
- **Studio shows a retried test: "passed on attempt N" and the earlier failures.** With `defaults.retry_failed_tests` set, a test that failed and then passed looked like a clean pass in Studio. Its row now has a note "Passed on attempt 3, after 2 failed attempts (defaults.retry_failed_tests re-ran it): the test is flaky", and a test that failed on every attempt says "Failed on all 2 attempts"; under either, one line per earlier failed attempt ("Attempt 1 failed: ...", at most five, each cleaned to one line with no absolute path). Like the crash line and the timeout they are for the window only and are not sent to an AI provider (a test checks that).
- **Studio shows why the app died, and that a test timed out.** Under a failed test the Results pane now has a note: the one line that says why the app died (`defaults.crash_evidence: true`, Android) in the same words `probe test` prints; "Timed out: the test ran past its 1 min 30 s limit (defaults.test_timeout in probe.yaml)" for a test that hit its limit; and, when a test failed with a lost connection on Android with crash evidence off, a one-line hint to turn it on. Studio already ran with these settings from `probe.yaml` but dropped what the engine reported. The crash line goes through the same last guard as other engine text (no absolute path, one line, no control or invisible character) and is shown as text. It and the timeout are for the window only: they are not added to what an Explain, a Suggest a fix or the chat sends to a provider (a test checks that). New exported `runner.IsConnectionLost`.
- **Studio asks in a dialog before `clear app data`.** The step wipes the app's data. In `probe test` it asks on the terminal or needs `--yes`; since the shared device context (#643) Studio refused it outright (with a message that sent the user to `probe test --yes`). Now Studio shows a dialog that names the app (the app id of the workspace's `probe.yaml`, labelled so) and the device, with Cancel selected: Escape and Cancel say no wherever the focus is (a click on the dark area or the page behind does not take the keys), Enter never presses **Clear data** (a click or Space does, and only after half a second), and a second `clear app data` in the same run asks again. The question is about the device the step would wipe: if the window has moved to another connection the step is not run, and **Connect is refused while a run is in progress** ("cancel it or wait for it to finish"). With no answer in 60 seconds, no window to ask (a headless Studio with no page, the automation endpoint), no run, or a cancelled run, the step is not run and a note says so. The request id is random (128 bits), bound to the pending question and to the run, and used once; the new `AnswerConfirm` method is in a category the automation endpoint can never reach, and in the desktop window only a person can press the button (the headless test build, which a local script can reach with its bridge token, refuses `AnswerConfirm` unless its environment says `PROBE_STUDIO_E2E_ALLOW_CONFIRM_OVER_HTTP=1`, which only the E2E script sets); the dialog text is built by Studio from the workspace's app id and the device name, never from engine text, and shown as text. `probe test` is unchanged.
- **Studio: a Problems tab with the lint findings of the open file.** Studio only marked parse errors and a test with no steps. The new Problems tab under the editor lists what `probe lint` finds (a `use` recipe or a data file that is not there, placeholders, program rules), marks each on its line in the editor (errors and warnings), counts them on the tab, and a click on a row or Enter goes to the line. It lints the text in the editor with `runner.LintSource`, so unsaved changes are checked, against the open workspace's `probe.yaml` (a relative recipes folder is read from the workspace, not from the folder Studio was started in). It waits 400 ms after typing, drops an answer for text that has changed, never blocks Run, and shows a line ("Not checked: ...") when the text does not parse yet. New bound method `LintProblems`: it reads only files inside the open workspace (the lint is fenced with `runner.LintWithRoot`): a `use` target, a CSV source, a recipe file or the `recipes_folder` of `probe.yaml` that is outside it, or a link that leads out of it, is not read and gives one fixed warning that names no file (an outside recipes folder: "the recipes folder is outside the workspace; recipes were not read ...", at line 0, and the lint goes on without flooding the list with unknown-step errors). No finding quotes the text of an outside file, shows a folder outside the workspace or confirms a CSV column name; an absolute path in any finding is replaced. A path outside the workspace is refused like `ReadFile`; a text over 1 MiB is not checked; requests queued behind a running lint are dropped for the newest.
- **`--crash-evidence` also looks at a test that ran past `--test-timeout`.** A timed-out attempt gets one look at the device for a dead app, on its own context of up to 10 seconds derived from the run's, before its cleanup starts, so the look can add up to 10 seconds to the cleanup. A cancelled run stops the look at once, and without the flag the limit makes no device call. The report line and the crash files are written as for any failed test, and the evidence is kept even when the cleanup is abandoned.
- **`--crash-evidence` also explains a death that shows up as a timeout, not only a lost connection.** When the app was killed during a `wait` or while a `see` was waiting, the step used to fail with only `timed out ... context deadline exceeded` and no evidence. Now any step that fails (not a restart, kill or clear step, not an optional step that goes on, not a cancelled run) gets one look at whether the app is dead, with the same rules as before: a gone process or an unexpected exit entry inside this test's window counts, a slow app that still runs with no new exit entry reports nothing, and the one line is printed for the failed test. An app that has no process after the test's own `kill the app` (and no launch since) is no longer taken for a death; after a restart, clear or open step an app with no process and no exit entry yet is. The look is read-only and changes neither the step's error, its timeout nor any reconnect attempt; with the flag off nothing changes at all.
- **A test that ran past `test_timeout` is reported as a timeout, and a `wait N seconds` at or above the limit fails at once.** A JUnit report gives the test a `<failure type="test_timeout" message="test exceeded 90s">` whose text keeps the step and the cause (an infra error stays an `<error>`); the HTML report marks a timed-out row with the limit: every row of a run with a limit carries `time_limit_seconds` in its data, and only a timed-out row carries `timed_out`. A literal `wait N seconds` at or above `test_timeout` fails as soon as it starts, with a message that names the wait and the limit: the limit would only end the wait. A wait below the limit, and a run without a limit, are unchanged. The rule covers a test's own steps and its before-each hooks. A wait in a cleanup hook (after each, on failure) is not refused: the cleanup runs outside the test's budget, and a cleanup hook's error is shown rather than dropped. A composite test's device steps run through the composite runner's executors, which have no wait limit, so the rule does not apply to them.
- **`probe lint` warns about a literal `wait N seconds` at or above `defaults.test_timeout` where the run refuses it.** A wait in a test body or a before-each hook is warned, and so is a wait in a recipe that one of those calls, directly or through another recipe. A wait directly in an after-each or on-failure hook, or in a before-all or after-all hook, is not warned, since the runtime lets it run to its end. A recipe that no test body or before-each hook of its own file reaches is warned with its condition, because a test in another file may call it under the limit. A composite test is not checked, since its steps run without the limit. A config without `test_timeout` gets no such warning, so `probe lint` output is unchanged for it.
- **`defaults.test_timeout` and `probe test --test-timeout` set a wall-clock limit per test attempt.** A test that runs past the limit fails with `test exceeded <limit>` and the run goes on. The cleanup (the failure screenshot, the failure and after-each hooks, the video stop and the perf close) runs within a 30 second total budget: a cleanup call that does not return in time is given up, and the attempt goes on. Then the app is restarted so the next test starts clean, also after a cleanup that was given up; a restart is skipped only when the run itself was cancelled. A `before each` hook or a body step that ignores its context blocks the attempt as it does without a limit: the limit cuts a call only when the call honours its context, as step timeouts already require. The reconnect and the step-timeout annotation are skipped only when the limit ended the step (a deadline), so a run without the limit, and a Ctrl-C with the limit on, do both as before. The scrcpy recording stop is killed when the cleanup's deadline ends; the screenrecord and simctl stops still wait for their recording process, so an abandoned one of those lasts until that process exits. The flag wins over the file; zero (the default) is off, and a value below 1s or negative stops the run before it starts. Each retry and each data-driven row gets its own limit; a composite test has one budget for all its device blocks, whose cleanup is not yet run. The JSON report carries `timed_out` and `time_limit_seconds` only when the limit is set. Measured on an Android emulator (API 35) with a 20 second limit: the slow test failed at 22.7 s (the cleanup took about 2.7 s) and the next test passed; without the limit the same test passed in 32.7 s; `--test-timeout 10s` overrode the file's 20 s. The JUnit and HTML reports and the wait rule come in later changes.
- **Studio says what to do next on a first run, and why a primary button is disabled.** A fresh Studio (no workspace) showed `(empty)` in the Workspace pane, an editor note that sent the person to "the left" where nothing was listed, and Run, Record and Connect disabled with no stated reason (Connect had a hover text only). Now a line under the toolbar names the one next step (open your project folder, open a `.probe` file, no device found, every device shut down, pick a device, press Connect) and is hidden once the step is done; an empty Workspace pane says "No workspace open" with an **Open workspace** button (the same call as the folder button), or "No .probe files here" for an open folder with none; and Run, Record, Save and Connect each say why they are disabled, as their hover text and as a description they point to with `aria-describedby`, so a screen reader reads it. Nothing is shown while the workspace is still loading, the device list is not called empty before its first listing (or when listing failed), and the text is set as plain text. The editor's scratch note no longer points to a list that may be empty. Frontend only: no new bound method.
- **With `--crash-evidence`, the JSON report carries a `crash` object for a failed test whose app died, and two small files are written.** The object (also in the `--stream` events) has `kind` (`crash`, `native_crash`, `anr`, ...), `gone`, `pid`, `process`, `exit_time` (the device's own clock, no zone), `summary`, `exit_description`, `truncated` and `files`; every text in it is one line of plain text (control, bidi and hidden characters become spaces). The files are `<reports folder>/crash/<folder>/exit.txt` (one line per exit entry of the app) and `crash.txt` (the crash text of the process that died), cleaned line by line and already masked with the run's secrets; the report lists them relative to its own folder. `<folder>` is the test's name reduced to ASCII letters and digits (a name with none, such as a Japanese one, becomes `test`), a dash and eight hex digits of a hash of the file, the full result name (the data row included) and the attempt, so two tests, two rows or two attempts never share one, and the folder is never a path or a name Windows reserves; the report's `files` point at what was written. The reports folder is trusted like it is for every other report file: nothing checks it for symbolic links. The `crash` object is never sent in the hosted upload. Without the flag, or for a test that did not lose its app, the report is byte for byte what it was; JUnit and HTML do not show the evidence yet.
- **`--crash-evidence` (`defaults.crash_evidence`) says why the app died when a test fails with a lost connection (Android, opt-in).** After a step loses its connection and before anything reconnects or relaunches, probe reads the device once: whether the app still has a process, the app's recent exit entries (`dumpsys activity exit-info`) and the crash buffer, with a few short looks when the process is gone and the entry has not appeared yet. An unexpected exit that happened during the test (a crash, a native crash, a kill by a signal, for memory or for resources, an ANR kill, an initialization failure, a dead dependency, or the app exiting itself) counts as a death even if the app has been started again since; a force-stop, a change of permission or package, freezing and "other kills by system" do not, and an exit that an earlier test of the run already had collected is not blamed on the next test, and an app that still runs with no recent exit is taken as slow, not dead. A failed test then prints one line, for example `the app died during this test: native_crash (pid 3644): Fatal signal 11 (SIGSEGV)`. The printed line is always one line of plain text (every control, hidden and bidi character becomes a space, runs of spaces collapse, a length cap). Every text from the device is cleaned, masked with the run's known secrets and the credential shapes, and cut. Off unless asked for; it adds no adb call to a run in which no step lost its connection; the JSON report and files are in the next entry; it does not cover iOS, cloud runs or composite runs, and JUnit and HTML do not show it yet. Entries of other Android users, and entries stamped after the device's clock, are not used; the collector's cap on entries now applies after that filtering and sorting.
- **The Flutter fixture has a forced-crash screen for `--crash-evidence`.** Built with `PROBE_FIXTURE_CRASH=true`, its one button (`#fixture_crash`, "Crash the app") aborts the process from native code through `dart:ffi` (the C library's `abort()`, no plugin). `probe-tests/crash/forced_crash.probe` taps it, so the step after the tap loses its connection; the test fails on purpose and is run by hand, not in CI. Verified on an Android emulator: with `--crash-evidence` the failed test prints `the app died during this test: native_crash (pid …): Fatal signal 6 (SIGABRT)` and carries a `crash` object (`kind` `native_crash`, `summary` `Fatal signal 6 (SIGABRT)`); without the flag the run reports only the lost connection. The default screen and the list screen are unchanged.
- **The Flutter fixture has a load screen for the sustained-load reproduction (PT-25).** Built with `PROBE_FIXTURE_LOAD=true` (and `PROBE_FIXTURE_LOAD_MODE=ui`, the default, or `isolate`), it runs background CPU work that never stops while the screen is up. `probe-tests/load/sustained_load.probe` waits and checks the screen for about five minutes; a reproduction is a `Connection lost` line with `close 1006`. It is a reproduction only, with no change to the agent. Not in CI. The default screen, the list screen and the crash screen are unchanged.
- **Studio: the Explain notes and the Suggest a fix card name where the failure is, and the same click opens it.** Each Explain note carries the file and line of its test's first failed step (only the file when no failed step is known), and a suggested fix carries the failed step's file and line; the cards show a small `L12` button (the file's name as its hover text, never its folder; the file's name alone when there is no line) that sends the same jump a failing row in Results sends, so the same rules apply (another file is opened after the question about unsaved changes, a name with `..`, a path outside the workspace, a link out of it or a file that is gone is refused with a sentence, and a line past the end shows the last one). The place goes to the window only: it is not in the request to a provider (a test checks that no workspace path is sent), and the preview does not show it. `ExplainNote` and `SelectorFix` gain `file` and `line`.
- **Studio: a click on a failing row opens the failing line, in the right file.** A step row in Results sent only its line number, so a click moved the editor to that line of whatever file was open, whether or not the failure was in it; the failed test row had no jump at all. A click on a failed step, or on a failed test (its first failed step, or the test's file when none is known), now asks for the step's file and line: the same file is not read again, another file of the workspace is opened first (through `ReadFile`, after the existing "Discard unsaved changes?" question; cancelling, or a file Studio refuses, leaves the editor and the cursor as they were), and a line past the end of the file shows the last one and says so. A file name with `..` in it (either slash) or with a control character, a path outside the workspace (a name that only starts like it is outside too), a file that is not a `.probe` file, a link out of the workspace, and a file that is gone are refused with a sentence. Only a click moves the editor: a run that fails does not. The failed test row is focusable and opens on Enter. No new bound method and no write; the decisions are in a pure module (`goto-line.ts`), and `ReadFile`'s checks have a new test for a deleted file, a link to nothing, a folder with a `.probe` name and a name written with `..`.
- **Studio: Suggest a fix again, after a re-run that still failed; at most three suggestions per failed step.** When Re-run the failed test shows `Still failing`, the card offers **Suggest a fix again** (a button; nothing is asked automatically). It is the same Suggest a fix as before: the preview and the confirmation Studio asks for (every time for a destination that is not this machine, once per session for one that is), then one request. Studio counts the attempts, in Go, for the failed step (file, line and test): the preview and the send both refuse a fourth (`3 suggestions were already asked for this step in this session: edit the line by hand`), the cap is checked before anything is built, so the device is not asked for its widget tree past it, an attempt is counted when its request goes out whatever the provider answers (a refused send is not one), a preview taken before the cap cannot be sent past it, and opening another workspace starts the count again. `SelectorFixPreview` and `SelectorFix` carry `attempt` and `maxAttempts`; the preview dialog says which suggestion it would be, the card is numbered from the second suggestion (`Suggested fix (2 of 3)`), and the offer says how many are left from Studio's live count (a new read-only method, `SelectorFixAttempts`), so a suggestion that went out and failed to arrive still counts. A new suggestion replaces the card of the replacement before it, so it goes when the Explain offer goes.
- **Studio: Re-run the failed test, after Replace the line.** After a replacement the suggestion card stays and shows a **Re-run the failed test** button and, after the run, the verdict (`Passed: name`, or `Still failing: name: error`, the error cut to 300 characters). It runs the test that holds the replaced line, once per click, through the existing scoped run, and it never saves: the run reads the file on disk, so the button is disabled until the person saves the file (the existing Save), and the reason is shown. It is also disabled while a run is going, with no device connected, in another file than the one that was changed, and after the replaced line was edited, or after lines were added or removed or the test header around it moved (the same text on the same line number could then be in another test, so the line count and the header of the test must be what they were at the replacement); a file that changes while the test runs gets a note that the verdict may not be the replaced line's. No new bound method, no write: browser specs rr-01 to rr-05 assert zero `WriteFile` calls from Replace and exactly one, from the Save click, and no run call from a disabled button; 20 node tests cover the gate and the verdict. The verdict's error text and test name are cleaned before they are shown: control, bidirectional, zero-width and other hidden characters are dropped, a line break is a space, a home folder (`/Users/x`, `/home/x`, `C:\\Users\\x`) becomes `~`, and the cut at 300 characters is on a character boundary (never inside a surrogate pair); a skipped test next to a passed one does not make the verdict "skipped". Device specs (dg-01, dg-02) run the whole round trip on a named simulator, and a double click and a held key on the button start exactly one run. The card is kept when a passing re-run makes the Explain offer go away (the device spec found that the offer's refresh closed it; rr-04 pins it). A new run started elsewhere still closes the card as before; only the re-run that the card started keeps it. Not in this version: a second suggestion from the card after a failed re-run (next).
- **`press tab`, `press escape`, `press backspace`, `press space` and the four arrow keys run on iOS.** The runner no longer refuses them there: the agent does the key in Flutter's focus system and the focused field's own state, with no platform branch, and each of the eight keys was run once on an iOS 26.5 simulator against a fixture with two text fields (tab and down moved the focus to the second field, up moved it back, left and right warned that there was nothing to move to, escape left the field, backspace and space edited it and reached `onChanged`). Only the simulator was tried; a physical iPhone was not.
- **The `store text of ... in webview` step is documented.** The dictionary has a row, the syntax guide a section "Reading text from a WebView" (what is stored, the 1000-character cap, hidden characters, the taint and the steps that refuse the value, masking by variable name, names, where it runs), the Android page a paragraph, and the MCP `write_test` text, `llms.txt`, and the VS Code snippet (`webviewread`), completions and README list it. The guide says plainly that for a name without a secret word, later steps may print the resolved page text (in a `type` step line, or an error that quotes a selector): name a variable like a secret when the page text is one.
- **`store text of css:"#total" in webview as total` reads text from a WebView into a variable (Android).** The text is the element's visible text with runs of whitespace collapsed (the same text `see` matches against); hidden characters (control and format characters, bidi controls, zero-width characters, line separators, non-characters, invalid UTF-8) are dropped (also the variation selectors, the Hangul fillers, the braille blank and similar characters that show nothing), and at most 1000 characters are kept (the step says `cut from N` when it cut). An element that is missing or hidden fails the step with the usual messages and sets nothing; an element with no text stores an empty string. The step prints the variable's name and a length, never the text. **The variable holds text from a page, so it is tainted:** later steps may type it, look for it (`see`, `wait until`) and use it in selectors, but `open link`, `add media`, `set language`, `type native`, `take screenshot` and `compare screenshot` names, `call`, `mock`, `start measuring` and the system dialog steps refuse it, naming the variable; a copy through `store`, a recipe parameter or the clipboard stays tainted, and a clean `store` clears it. A variable whose name has a secret word (`token`, `password`, `pin`, `key`, ...) is also masked wherever it could be printed: the whole stored text is masked, when it has 3 or more characters (a part of it that shows up elsewhere is not masked on its own); any other name is not, so an ordinary page text is not masked. The name may not be empty or one a step manages (`clipboard`, `response`, `response.*`). The step needs a local Android device and is an error in a cloud run (a skipped read would leave the variable unset). In one run by the author on one Android emulator (API 35, WebView 124) against a test page with a total, a password-like text, hidden characters, an empty element and a 3000-character text, the step behaved as described; other devices and WebView versions were not checked. The step is documented in the syntax guide, the dictionary, the Android page and the editor surfaces (see the entry above). Changed: lines of the form `store text of css:"..." in webview` used to parse as a store of the word `text` into a variable named `css` followed by stray steps; they are now this step, or a parse error that names what is missing when the form is cut short, has a quoted text instead of `css:"..."`, or has words after the name (84 other lines, `store` lines, recipe names and the words `text`, `of`, `css` and `webview` used as ordinary words, were compared against the previous parser and parse as before).
- **Studio: Replace the line puts a suggested fix into the editor.** The suggestion card has a **Replace the line** button (only when cleaning changed nothing in the suggestion) that replaces the failed step's line in the editor with the suggested one, as one undo step. It saves nothing and runs nothing. Studio decides, in Go, from its own record of the suggestion and the file as the window holds it (`CheckReplaceLine`, a new bound method that reads and writes nothing): the open file is the file the failed step ran from; line N still has the text it had when the suggestion was asked for; the suggestion is one line of plain characters (a line break, control, hidden, bidirectional, line-separator or non-ASCII space character refuses it); and the file parses as it is and with the line replaced into exactly the same steps at the same lines, the step keeping its profile: the same verb and the same flags (`optional`, `if visible`, `with ai`, ...), from a closed list of what a selector fix may be and may replace (`tap`, `long press`, `double tap`, `type ... into`, `see`, `don't see`, `wait until ... appears` or `disappears`). Every action verb, wait kind and step type the parser has is classified in a test, so a new one is refused until someone lists it on purpose. Only the selector may change: a `type` keeps what it types and its target (`type "bob" into "Email"` is never `type "bob"` or `type "pw" into "Email"`), and a `contains "b"` keeps its value. A suggestion that is `open link`, `clear app data`, `kill`, `restart`, `store`, a native or webview action, `add media`, `see ... with ai`, a second step, a recipe call or a block is refused, so is one that adds `optional` or `if visible` the line did not have, and one that brings a comment, a `;` or a `${ENV}` or `<variable>` the line did not have. A line with a comment, a step that holds others and any step in a composite test are not replaced. The window applies the edit only if the editor still holds exactly the text Studio checked, and says why when it does not. Refusals name what did not hold. Browser specs sf-13 to sf-17 (the replacement and its one undo step, a refused kind, a line edited after the suggestion, a changed suggestion, and typing before the replacement staying after an undo). Not in this version: running the step ("Try it"), saving, and replacing a step in a composite test. Also in this change: a suggested fix carries an `id`, and a bound method `CheckReplaceLine` is added (`SelectorFix.id`, `ReplaceLineCheck`).
- **`store text of css:"#total" in webview as total` reads text from a WebView into a variable (Android).** The text is the element's visible text with runs of whitespace collapsed (the same text `see` matches against); hidden characters (control and format characters, bidi controls, zero-width characters, line separators, non-characters, invalid UTF-8) are dropped (also the variation selectors, the Hangul fillers, the braille blank and similar characters that show nothing), and at most 1000 characters are kept (the step says `cut from N` when it cut). An element that is missing or hidden fails the step with the usual messages and sets nothing; an element with no text stores an empty string. The step prints the variable's name and a length, never the text. **The variable holds text from a page, so it is tainted:** later steps may type it, look for it (`see`, `wait until`) and use it in selectors, but `open link`, `add media`, `set language`, `type native`, `take screenshot` and `compare screenshot` names, `call`, `mock`, `start measuring` and the system dialog steps refuse it, naming the variable; a copy through `store`, a recipe parameter or the clipboard stays tainted, and a clean `store` clears it. A variable whose name has a secret word (`token`, `password`, `pin`, `key`, ...) is also masked wherever it could be printed: the whole stored text is masked, when it has 3 or more characters (a part of it that shows up elsewhere is not masked on its own); any other name is not, so an ordinary page text is not masked. The name may not be empty or one a step manages (`clipboard`, `response`, `response.*`). The step needs a local Android device and is an error in a cloud run (a skipped read would leave the variable unset). In one run by the author on one Android emulator (API 35, WebView 124) against a test page with a total, a password-like text, hidden characters, an empty element and a 3000-character text, the step behaved as described; other devices and WebView versions were not checked. The documentation of the step in the syntax guide, the dictionary and the editor surfaces follows in a separate change. Changed: lines of the form `store text of css:"..." in webview` used to parse as a store of the word `text` into a variable named `css` followed by stray steps; they are now this step, or a parse error that names what is missing when the form is cut short, has a quoted text instead of `css:"..."`, or has words after the name (84 other lines, `store` lines, recipe names and the words `text`, `of`, `css` and `webview` used as ordinary words, were compared against the previous parser and parse as before).
- **Internal: variable expansion can carry a "taint" flag, and the steps that send a string out of the UI take their strings through one guard (`forSink`).** Nothing is tainted yet, so every script behaves as before (the expansion of every string of the 37 example scripts is byte-identical). It prepares the WebView read step (`docs/proposals/webview-read-and-taint.md`): a value read from a page is inserted as it is, never expanded again, and refused by `open link`, `add media`, `set language`, `type native`, `take screenshot`, `compare screenshot`, `call`, `mock`, `start measuring` and the steps of a system dialog (whose typed text can name an environment variable), and a value the script copies to the clipboard stays tainted when it is pasted, in the same test or a later one, until a clean copy; a copy that fails never clears it. A new run (a new `probe test` process, or a new device context) starts clean even if the device clipboard still holds a page value. Every write to a variable goes through one function, and a test requires that every verb and step type of the parser is classified (sink, literal only, UI only, propagates, no string), that the sinks use the guard, and that the literal-only steps expand nothing.
- **`--junit-artifacts` lists each test's artifacts in the JUnit report.** With `--format junit`, a testcase gets a `<system-out>` with one `artifact: <path>` line per screenshot, video or diff image of the test (the diff image of a failed `compare screenshot` now reaches the artifacts, see the entry above). A path inside the directory of the report file (the `-o` file; otherwise the working directory) is written relative to it. A path outside it is written as its file name alone, a `..` path and a Windows or UNC path from another machine too, so a report that CI uploads never carries an absolute path of the machine that ran the tests. At most 50 lines per test, then `artifacts: N more not listed`; a path is cut at 300 characters, repeated paths are listed once, names are masked for the secrets the run knows and made safe to write (control characters, line separators and XML-illegal characters become spaces; the XML encoder escapes the rest), and a test with no artifacts gets no element. It is off by default and the output is byte-identical without it. With `--junit-attempts` the lines go on the case that carries the final outcome. The lines are plain text, the same for every CI system: Jenkins' JUnit attachments plugin reads a `[[ATTACHMENT|path]]` form in `system-out`, which this does not write (not verified here against that plugin; GitLab and Azure Pipelines show `system-out` as text).
- **The WebView steps are documented and in the editor surfaces.** `tap`, `type`, `see`, `don't see` and `wait until ... appears` with `in webview` (Android; the runner part shipped in the previous entries) now have: dictionary rows (and the Studio dictionary generated from them), a "WebView content (Android)" section in the syntax guide with a short example and what to know first (the app's WebView needs debugging on, only the top document is searched with no shadow DOM or iframes, an element that cannot take typed text is refused, the focus is checked once, positions were tested at a device pixel ratio of 1, no implicit waiting inside the step), a section on the Android platform page, a "not supported yet" section on the iOS page, the MCP `write_test` reference text, the README, `llms.txt`, and in the VS Code extension a snippet (`webview`), completions, the grammar's `in webview` keyword and the README list.
- **The WebView steps run on Android.** `tap`, `type`, `see`, `don't see` and `wait until ... appears` with `in webview` now act on the page inside the app's own WebView, over the DevTools socket of the app's process (the app must have WebView debugging on; only the app under test's own socket is used, over a loopback port that is removed after every step). A page is opened for each step and always closed. A text operand matches visible element text (exact first, then as a part of it); `css:"..."` is a CSS selector; an optional page name picks one of several pages. `see` needs a visible element; `don't see` passes when there is none or it is hidden, and fails when the WebView cannot be reached at all (it never answers "not seen" for a page it did not look at). `type` refuses when the element does not take the focus, and the typed text appears in the step line the way the text of a `type` step does (known secrets masked), and never in an error from the page. Variables and `${ENV}` values in the selector, the page name and the text are expanded and masked like the text of a `type` step, in the step line and in every error; page text in messages is masked with the run's secret masking too. On iOS the steps fail with a message that says only Android is supported, and in a cloud run they are skipped like the other device-only steps. `device.OpenWebView` now refuses a nil `Redact` instead of running without masking, and removes DevTools addresses from any text the page supplied (a loopback host with a port, with or without a scheme and with spaces in it, `0.0.0.0`, IPv4-mapped forms, percent-encoded forms, and everything after a `/devtools/` path). An error from a WebView step is told apart from an agent error by its type, never by the words in its message: a page title, an element text or a selector that holds `EOF` or `broken pipe` cannot make the run reconnect the agent or retry the step, and a WebView step never asks the Flutter screen what it shows, also inside a recipe, `retry` or `repeat`. A run with no agent connection never panics on a reconnect. A target or page name in a step line or message is cut at 200 characters. A `wait until ... in webview` that the step's deadline ends says what it waited for and the page's name. A WebView step with no app id says to set `app` under `project` in probe.yaml. `probe lint` warns about `see "0 items" in webview` and `wait until "3 items" appears in webview` (a count or zero-state text can match a longer element text; a WebView step has no `matching` form, so the warning suggests checking the element with `css:"..."`). Not covered yet: shadow DOM and iframes (only the top document is searched), implicit waiting and retries inside the step, scrolled pages and a device pixel ratio other than 1. Checked on a named Android emulator (Android 15, API 35) against a one-WebView test app: see, don't see, type, tap and wait until all passed and left no adb forward behind.
- **A report carries the run's properties and each test's tags.** `probe test --property key=value` (repeatable) adds a property to the run: a `<property>` on every suite in `--format junit` (after `run_label`, sorted by key), a `properties` object in `--format json`, and a line under the filters in the HTML report made by `probe report`. A test's `@tags` are written as a `tags` property on its `<testcase>` (comma-joined), as `tags` in the JSON result and as chips in the HTML report. Nothing changes for a run with no `--property` and no tagged test: the JUnit and JSON output are byte-identical to before (the existing JUnit golden files are the check). Limits: a key is 1 to 64 letters, digits, `_`, `.` or `-` and may not be a name the report writes itself (`run_label`, `tags`, `platform`, `device`...), at most 32 properties, a value of at most 1 KB, no key twice; a bad entry stops the run before it starts with exit code 2 (exit code 2 is now used for the usage errors of `--property` only; every other error, including an unknown command or flag, still exits 1, so a script that treats any non-zero code as a failure is unaffected) and the message never repeats the value. At most 32 tags are written per test, each cut to 64 characters. Values and tags are masked like other printed text when the report is written (so a secret the run learned later is masked too), and every control character, line separator, character XML 1.0 cannot carry and invalid UTF-8 becomes a space; the JUnit file is written with `encoding/xml` and the tests parse it back. In the HTML page the values reach the script as escaped JSON and are put on the page as text nodes, never as markup. Not changed: the compare-screenshot diff image is not linked from the report yet.
- **WebView steps parse (`tap`, `type`, `see`, `don't see` and `wait until ... appears` with `in webview`); the runner does not run them yet.** `tap "Contact support" in webview`, `tap css:"#submit" in webview "checkout"`, `type "hello" into css:"#message" in webview`, `see "Getting Started" in webview`, `don't see css:".error" in webview` and `wait until "Done" appears in webview`: a quoted string is the visible text, `css:"..."` is a CSS selector, an optional quoted page name follows, and a trailing `within N seconds` works as on other steps. There is no form that takes JavaScript. The grammar (EBNF and conformance test) and the dictionary generated from it are updated. The steps run on Android (next entry) and are documented (the entry after it). Changed: lines with `in webview` after a text or a `css:` operand, which the parser used to read as the step followed by a call to a recipe named `webview` (`probe lint` already flagged that call), are now these steps; where something other than a page name follows `in webview` (`if visible`, `optional`, a second string, another word) the line is now a parse error that names the rule. Every other line parses as before (a 70-case comparison of parse trees against the previous parser).
- **`press tab`, `press escape`, `press backspace`, `press space`, `press up`, `press down`, `press left` and `press right` press a key on the focused field (ProbeScript).** Tab and the arrow keys move the focus with the focus system; escape leaves the focused field; backspace deletes the selection or the character before the caret, and space inserts a space, both through the field's own update, so `onChanged` runs. With nothing focused, tab moves the focus to the first focusable widget and escape does nothing, and neither warns; an arrow moves the focus into a field in its direction when one exists, and does nothing otherwise, without a warning; backspace and space warn that no text field has focus, as `press enter` does. With a field focused, an arrow with nothing in its direction warns that there is nothing to move the focus to. The step reaches the agent as the `press_key` device action, the same path as `press enter`; nothing runs in the device's shell. `press enter` is unchanged. On iOS the step is refused with a message, because the key path is not verified on a simulator. The grammar (EBNF `press-key-step` and its conformance test), the dictionary and the Studio module generated from it, the syntax page, the VS Code snippet, completions and README, and the MCP `write_test` description are updated. The step needs an agent that reports the `press_key` capability in its handshake: the CLI refuses the step for an agent that does not report it, with a message to update probe_agent (and rebuild the app). The gate is the capability, not the version number.
- **`swipe up on "Card" over 40 percent` sets how far the swipe travels.** The number is a whole number from 1 to 100, a percent of the element the swipe starts on or of the screen, and it ends the line; anything else after `over` is a parse error that names the range. A swipe without it still travels 50 percent. The step sends the distance to the agent as `percent` in the `probe.swipe` request, which is left out when the step does not set it, so an existing test sends the same request as before; the agent clamps the number to 1 to 100 as well. The grammar (EBNF and conformance test), the dictionary and the Studio module generated from it, the MCP `write_test` description, the VS Code snippet, completions and README, and the syntax and grammar pages are updated. `probe lint` warns about a word where the direction goes (`swipe sideways`), unless a recipe with that name exists: the swipe still runs down and the word is still read as the next step, as before. Making that an error waits for a release that has shipped the warning. The swipe starts at the centre of the element or screen, so above 50 percent the finger ends outside it (50 percent ends at the edge, 100 percent one half-size past it). The travelled distance has not been measured on a device yet.
- **`swipe right on "Menu" from 2 percent` starts a swipe in from the edge it starts at (an edge swipe for a drawer).** `from N percent` (a whole number from 1 to 100) sets where a swipe on a target starts, counted from the edge it starts at: the bottom for `swipe up`, the top for `swipe down`, the right for `swipe left` and the left for `swipe right`. It comes after the target, written in quotes, and before `over N percent`; a bare swipe with `from`, or a name without quotes before it, is a parse error. Without it the swipe starts at the centre, as before. The step sends `start_percent` in the `probe.swipe` request, which is left out when the step does not set it. An agent that does not report the `swipe_from` capability in its handshake refuses the step with a message to update probe_agent, so an older agent never swipes from the centre by mistake. An agent that does not report the capability fails the step with a message to update probe_agent; the gate is the capability, not the version number. A start is computed in the target's own coordinates, so a target under a scale transform starts at the wrong point (under a 2x scale, `from 50 percent` lands at 50 percent of the unscaled size), and rotated targets are untested, as for `over N percent`. Only the start point changed: the distance and the travel are as before. The grammar (EBNF `swipe-start` and its conformance row), the dictionary and the Studio module generated from it, the MCP `write_test` description, the syntax guide, the VS Code snippet, completions, TextMate grammar and README are updated.
- **`probe lint` warns about a word after a hook header, which the parser ignores.** `before each tests` or `on failure now` is read as the header alone, as it always was; the lint names the words at the header's line: "unexpected word(s) after the hook header are ignored: tests". The steps under such a header are never the hook's: with the word `test` among the words they run as a test with no name (`on failure test`); with other words they are dropped and do not run (`before each tests`). The warning says which, so put the header on its own line.

### Changed
- **`probe-mcp --http` writes its token to a file for each address and port: `http-token-<address>-<port>`, not `http-token`.** Two servers of one user used to overwrite each other's token file, and the first one's token was then no longer on disk (the server only warned). Now each address and port has its own file (`http-token-127.0.0.1-7391`; `::1` is written `__1`, as a colon is not allowed in a file name everywhere, and an IPv6 zone is part of the name, in hexadecimal after `-z` (a zone longer than 32 characters is written as a hash after `-h`, so a name stays short): `fe80::1%en0` and `fe80::1%en1` are two files, and two servers never share a file name), a server removes only its own, and two servers do not interfere, also the same port on `127.0.0.1` and `::1`. A start on an address and port that is taken still fails before any file is touched. The path is still printed on start; a script that read the fixed `http-token` path must read the file of its address and port, or take the path from the start-up line.
- **The device list, lint and the device view of the VS Code extension no longer inherit the three secret variables, and Test Explorer runs get the secrets from the settings.** Programs the extension starts inherit VS Code's environment. `PROBE_CLOUD_KEY`, `PROBE_CLOUD_SECRET` and `PROBE_RELAY_CLI_TOKEN` are now removed from it, in any casing (Windows reads `Probe_Cloud_Key` as `PROBE_CLOUD_KEY`), for the children that do not run tests (`probe device list`, `probe lint`, the device view's hub). The other terminals the extension opens (Studio, record, convert and the like) still inherit VS Code's own environment, so a variable VS Code itself was started with reaches them. Test Explorer runs now also get the values from `flutterprobe.cloudKey`, `cloudSecret` and `relayToken`, as terminal runs do (they inherited only VS Code's own environment before). Nothing else is removed from the environment: a `${NAME}` in a test reads any variable by design, and the boundary is which binaries the extension runs and in which workspaces. Not breaking. New: `childEnv` in `vscode/src/secretArgs.ts`; documented in the extension README and the VS Code docs page.
- **The VS Code extension runs only the binaries you chose (breaking for a workspace that sets their paths, and for `bin/probe` users).** `flutterprobe.probePath`, `flutterprobe.convertPath`, `flutterprobe.adbPath` and `flutterprobe.flutterPath` are now machine-scoped: VS Code reads them only from user or remote settings (on SSH, WSL or a container, the remote settings), so a repository's `.vscode/settings.json` can no longer point the extension at a binary of its own (a value there is now ignored: move it to your user settings). The extension resolves the binary itself and runs it by its absolute path: the configured path (an absolute path that is a regular file, or a command name looked up on PATH; a relative path is refused), else `probe` on PATH. Only the absolute folders of PATH are searched: on Windows, Node's `spawn` (libuv) and cmd.exe look in the current folder before PATH, and the extension starts its children in the workspace, so a `probe.exe` in a repository could run instead of yours. On Windows, when `probe` is on no folder of PATH, the extension says so instead of running a bare name. **`bin/probe` and `bin/probe-convert` inside the workspace are no longer used on their own**: only with the new machine-scoped setting `flutterprobe.useWorkspaceBinaries` (off by default) and in a trusted workspace, since trusting a folder is not consent to run its binaries. The manifest declares `capabilities.untrustedWorkspaces.supported: false` with its reason; that is what VS Code already did for an extension that declared nothing, so an untrusted workspace was not exposed before. The terminal command is now quoted like its arguments (not on Windows, where the shell is not known). Not changed: `probe test` reads the workspace's `probe.yaml`, whose `tools:` section can name `adb` and `flutter`. New: `vscode/src/toolPath.ts`.
- **The VS Code extension no longer puts the cloud key, the cloud secret or the relay token on a command line (breaking for a workspace that sets them).** A command line is visible to other users in the process list and stays in shell history; the extension typed these values into its terminal as `--cloud-key`, `--cloud-secret` and `--relay-token`. It now creates the terminal with them in its environment, and `probe test` reads `PROBE_CLOUD_KEY`, `PROBE_CLOUD_SECRET` and `PROBE_RELAY_CLI_TOKEN` when the flag is not given (a flag that is given, even empty, wins, so existing scripts are unchanged). The relay variable is not `PROBE_RELAY_TOKEN`, which is the agent's dart-define for a different token of the same relay session. `runInTerminal` refuses a command line that carries one of the three flags, and a terminal is replaced when a run's secrets differ from those it was made with. **The settings `flutterprobe.cloudKey`, `flutterprobe.cloudSecret` and `flutterprobe.relayToken`, and the two that decide where they are sent, `flutterprobe.relayUrl` (it receives the relay token) and `flutterprobe.cloudProvider` (it receives the key and the secret), are now machine-scoped**: VS Code reads them only from user or remote settings, so a value in a workspace's `.vscode/settings.json` is ignored (move it to your user settings, or set the environment variable; the extension says so once for that workspace), a repository cannot set them, and Settings Sync leaves them out. This covers the extension's settings only: `probe test` still reads the workspace's `probe.yaml`, whose `cloud:` section can name a provider and a relay service URL, and which binary the extension runs is a separate set of settings (tool paths, machine-scoped too: see the entry above). The secrets stay in the FlutterProbe terminal's environment for its whole life, inherited by every command typed there and by the programs `probe` starts; whether VS Code restores a terminal's environment after a restart was not verified. The settings are still stored in plain text by VS Code. New: `internal/cli/secret_flags.go`, `vscode/src/secretArgs.ts`, `buildTestRun`.
- **The agent no longer matches the value of an obscured (password) field (breaking for suites that assert it).** A text selector matched the typed value of an obscured field, so `see "<value>"`, a prefix of it, `wait until "<value>" appears`, `tap` and `selector_bounds` found the field: a test could learn the value one prefix at a time. Measured on the agent's own responses: `dump_tree`, `visible_summary`, `hit_test` and the failure messages never carried it. The value is now never a match target (Material and Cupertino fields; the field is still found by its id, key, label or hint), and a failure on a screen with an obscured field says that its value is never searched. Migration: a suite that asserts a password field's value with `see "<value>"` now fails; assert what the field does not hide instead: a `Text` the app shows about it (the Flutter test app shows `Password length: 6` and its test asserts `see "Password length: 6"`), the field by its id (`see #pw`), or what the app does once the value is accepted; `see #pw is filled` and `is empty` are planned. A suite that taps "show password" and then asserts the value fails too: a field seen obscured stays secret after the app shows it. Limits: a field never seen obscured is ordinary text, fields sharing a key share the state, a `Text` that mirrors the value is just text, and the value is still readable through `run_dart`, screenshots and the app's own logs, which this rule does not cover. A `type "..." into <field>` step still prints its literal text in `-v` output (a separate change). Agent change: it takes a new `probe_agent` release.
- **The Android fixture apps (`native-test-apps/flutter`, `native-test-apps/studio-fixture`) use Gradle 9.5.0, AGP 9.3.1 and Kotlin 2.4.20, so they build on Flutter 3.50.0.** Flutter 3.50.0 refuses a build below Gradle 9.3.1, AGP 9.1.1 and Kotlin 2.4.10 (measured: the unchanged fixture stops with "Your project's Gradle version (9.1.0) is lower than Flutter's minimum supported version of 9.3.1"). The raised fixtures build on 3.50.0 and on 3.44.8, and the Flutter fixture suite passes 5 of 5 on an Android 15 emulator with either build. Test apps only; `docs/compat/RELEASES.md` records the measured results.
- **Breaking: `deny permission "notifications"` on an iOS simulator asks before it reinstalls the app.** With the app not running, the step reinstalls the app to reset its notification decision. That deletes the app's data (its keychain items are kept), so it now asks first, as a clear does. A parallel or composite lane without `--yes` refuses it with the hint, and `probe test --yes` lets it through. **Upgrade note:** a CI job that denies notifications in a parallel run, with the app not running, needs `--yes`.
- **An iOS simulator's clear says what its keychain reset takes with it.** The question and the refusal say that `simctl keychain reset` also resets the keychain of ALL apps on that simulator, and the line printed when the setting answers without asking says the same. Measured on a scratch simulator: a container wipe leaves the app's keychain items, so the reset stays.
- **Breaking: a parallel or composite run no longer clears app data without `--yes`.** `probe test --parallel` (also `--devices`, composite devices and several devices) used to clear `clear app data` on every device it ran on, physical devices included, without asking. A composite run refused a clear even with `--yes`. Now a device refuses the clear unless the run has `--yes`, or the device is a simulator or emulator that the person has allowed to be wiped without a question in their settings (`probe prefs`) and FlutterProbe verifies as one. The refused step fails with the hint to pass `--yes` (`-y`) to `probe test`. Composite runs honour `--yes`. The MCP tools `run_tests`, `start_tests` and `run_script` refuse a `-y` or `--yes` in any argument, so an AI-driven run cannot carry that consent, and their parallel and composite runs refuse clears. **Upgrade note:** a CI job that clears app data in a parallel run needs `--yes` (one flag); nothing else changes for it.
- **A token where a swipe's direction goes is a parse error, not a silent down swipe.** `swipe sideways`, `swipe 5`, `swipe tap` and a word followed by a target (`swipe sideways on "Menu"`) ran a down swipe, and the token was read as the next step; `probe lint` only warned about some of them. Now the line is refused at parse with the four directions listed, so `probe test` stops before it runs. A suite that relied on it: write `swipe down` (or `swipe` on its own, which still runs down) for the down swipe it got, and quote an element name, `swipe up on "Menu"`. A recipe name on the same line as `swipe` (`swipe login`) was a recipe call after a down swipe and is refused the same way: put the call on its own line after `swipe`. The EBNF is unchanged, since its `direction` rule already lists only these four words. A word after the direction is not checked: `swipe up sideways` still reads `sideways` as the next step, and a quoted `swipe "up"` is read as the up direction, not as an element named up.
- **The docs say that WebView steps need the probe agent.** `tap`, `type`, `see`, `don't see`, `wait until ... appears` and `store text of` with `in webview` run through the agent connection like every other step, so the app must host `probe_agent`; a plain native Android app cannot be driven by `probe test` (it fails at connect with "agent token not found") and is only for the Go tests of `internal/runner`. Stated in the ProbeScript syntax page, the Android platform page, the dictionary rows and the fixture README. Documentation only.
- **A parallel run refuses devices of more than one platform, at start.** `--devices` with an Android serial and an iOS UDID (or auto-discovered devices of both platforms) used to start one run whose lanes share one agent port and one app id: the Android device did not connect while the iOS device passed, and the run exited 0 with one device error. Now the run stops before it connects with "`--devices` must list devices of one platform; run one probe test per platform". Each `--devices` serial is looked up among the connected devices and takes the platform that device reports, so a physical iPhone and an Android wireless-debugging serial are classified correctly; a serial that is not connected is refused with "`--devices`: device … is not connected". Discovered devices of two platforms, with `--parallel` and no `--devices`, are refused with a message that names the platforms and says to choose one platform with `--devices`. A parallel run now also needs the device listing to work: a listing that fails (adb not found, say) stops the run at start with its error, where it used to be taken for no devices. Without `--parallel` a run is unchanged: it still uses the first connected device.
- **`--shard` takes a file's shard from its path inside the repository, so the spelling of the path and the checkout's location no longer change the shard.** A file's shard was a hash of the path as typed: a CI job that spelled the same file another way (an absolute path, `./`, another checkout root, or a symlink to the checkout) could run a file in two shards and drop another. Now the key is the path relative to the repository root (the nearest directory above the config that holds a `.git`), or relative to the config's directory when there is no repository. A typed relative path is taken from the working directory. Symbolic links are resolved, the path is cleaned, uses forward slashes, is in Unicode NFC (macOS names files in NFD), and is lower-cased (case-insensitive file systems); two files can collide, and then only share a shard. **An existing shard job can move files once:** a file not spelled as its repository-relative path (an absolute path, `./`, a path through a symlink, or any upper-case letter in its path) can land in another shard. `--verbose` now prints each shard's files, so the old and new lists can be compared to see which files moved. **Limitation:** a file outside both the repository and the config's directory keeps its absolute path as its key, so its shard depends on where its checkout is; `--verbose` marks such a file with a note, naming it by its base name only. **Monorepos:** with the config in a subfolder (`e2e/probe.yaml` beside `tests/` in a repository root), the key is relative to the repository root, so a git checkout and a tarball of the same tree disagree when the tarball has no `.git`: in the reviewer's sample 17 of 30 files moved between them. Pin the base with `--shard-root <dir>` (or `PROBE_SHARD_ROOT`) to the same directory in every shard job, or make every job a git checkout; see the CLI reference and the CI docs. A `.git` file (a worktree or a submodule) marks the repository as a `.git` directory does.
- **The Studio docs say what a Studio run does and does not do.** The Studio page, `studio/README.md`, the root README and the wiki now describe the features added since 0.27.0 (Problems, Replace the line and Re-run, the failing-line jump, first-run guidance, a run that reads `probe.yaml` like `probe test`) and the limits: `clear app data` is not run in Studio (no confirmation yet; the step fails with a message, and `probe test --yes` runs it), the toolbar scrolls sideways a little between 1040 and about 1120 px, and the lint stops at 1 MiB. The root README no longer says "v0.6.0 ships" Studio.
- **The VS Code completion words and the grammar's keyword rule come from the ProbeScript docs.** `vscode/scripts/gen-keywords.mjs` writes `PROBESCRIPT_KEYWORDS` (the lead word of each command in dictionary.md, and the reserved words of grammar.md, without the English function words) and the grammar's `generated-keywords` rule, from the same parsers the Studio dictionary uses. The completion list offers each command form of the docs (the Command cell of a row, and the plain phrase of each of its examples, such as `see no requests` or `with examples:`) with its arguments as tab stops, and the lead word of each command and the reserved words. Forms the parser accepts that the docs do not list (`tap on`, `toggle wifi`) come from `scripts/extra-forms.json`, each with its reason. Two phrases the old hand-written list had are no longer offered: `pause for` (`pause` takes no arguments; `probe lint` reports `pause for 2 seconds` as `unknown step "2 seconds"`) and `Examples:` (the docs write `with examples:`). A drift test (`npm test`) fails when the docs change and the lists are not regenerated (`npm run gen:keywords`), when a command form is not offered and not listed in `scripts/keyword-gaps.json`, and when a command has no completion word.
- **The VS Code extension has a skeleton snippet for each documented command that takes an argument.** `vscode/scripts/gen-snippets.mjs` writes `snippets/probescript.generated.json` from the first example of each command in dictionary.md: the example's quoted texts and numbers are tab stops, and its prefix is the example, so `tap "OK" if visible` or `wait 5 seconds` offers the whole form. A command that a hand-written snippet already has (`tap on "…"`, `wait until "…" appears`) gets no skeleton. A command without an argument is left out, since the keyword completion already offers it, and so is a syntax template (`swipe up/down/left/right […]`) and the `respond` line under `when the app calls`. A skeleton stands in a context: a step inside a test, a file-level line (`composite test`), a line after a test (`with examples from`) or a line inside a composite test (`sync`); `snippets/probescript.generated.contexts.json` says which. The snippet file is listed under `contributes.snippets`; the contexts file is for the Go gate. `internal/runner/skeleton_snippets_test.go` expands every skeleton in its context and lints it with the `probe lint` check, so a snippet that does not parse fails `go test`. A drift test (`npm test`) fails when the files are out of date, when a skeleton does not reproduce its example, and when a documented command with an argument has no snippet and no reason.
- **The completion inserts of a command take its documented example.** `open link "myapp://item/42" in the app` inserts with its argument as a tab stop; the previous insert dropped the argument (`open link in the app`). `press <key>` keeps its placeholder.
- **The TextMate grammar highlights the bare hooks as hooks.** `before each`, `after each`, `before all`, `after all` and `on failure` start a line and are highlighted as hooks, as the parser reads them. Before, `before each test`, `after each test` and `on failure` were highlighted, and the bare `before each` and `after each`, and `before all` and `after all`, were not. An indented hook is still not highlighted, since the parser rejects it inside a test. A drift test checks that every hook of dictionary.md is matched.
- **The README, the troubleshooting wiki and the VS Code snippets use the dictionary spellings `take screenshot "name"` and `dump tree`.** They used `take a screenshot called` and `dump the widget tree`, which the parser accepts, so no behavior changes. The website grammar page still lists both spellings on purpose, as the accepted alternates. The skeleton for `take screenshot "name"` goes, since the hand-written `Take screenshot` snippet now covers it. A drift test fails when the README, the wiki or the hand-written snippets use the old spellings again. The converter (`probe-convert`) and `probe migrate` still emit the old spelling; changing that is a code change with golden-file updates, and it is not in this entry.
- **A Studio run reads `probe.yaml` like `probe test` does.** Studio built its device context by hand and set only the agent port, so a step that works on the device by itself (`restart the app`, `kill the app`, permissions, `clear app data`) ran without the app id and failed (`restart: launch: ... The request to open "" failed`), and the timings, the HTTP fallback of a physical phone and `defaults.crash_evidence` were never read. `probe test` and Studio now build the context with one constructor, `runner.NewConfiguredDeviceContext`: Studio now uses the app id (`project.app`), the reconnect, restart, token and dial timings, the driver port, `defaults.crash_evidence` (so a Studio run prints the same death line a CLI run does) and, for a Wi-Fi device, its host. Tests keep it that way: a context field has to be set by the constructor or named with a reason, the CLI and Studio results are compared field by field, and a Studio run still refuses `clear app data` (nothing in the window confirms it yet, so it is denied whatever `probe.yaml` says).
- **A JWT that is cut inside its payload is redacted too.** The JWT rule needed all three parts, so a 500 or 1,000 character JWT that a bound or a clip cut left `header.partial-payload` (the header and the first claims) in what was sent. The third part is now optional, in the gate and in the network redactor: a part that starts with `eyJ` and has at least 7 characters, a dot, and at least 4 characters of the next part are redacted. What can still be sent when a JWT is cut: the header alone, with or without its dot, and the header followed by fewer than 4 characters of the payload (`eyJ`), none of which holds a claim. Ordinary text such as `eyJab.cd` is not changed.
- **A field that is bounded before the gate no longer ends inside a token.** `boundRunes` (the bound of Explain's fields, the chat's 150,000-character field bound, the suggested fix's fields and the network redactor's raw bounds) drops the run with no space at its cut and a fixed 200 characters, and that second step could end inside a token that began before it, so the first part of a token the gate cannot see (up to 21 characters of a `ghp_` token, 9 of `sk-`, 16 of an `AKIA` key, 45 of a JWT, measured in spaced text within about 250 characters of the cut) was sent. The text now ends where no token character (an ASCII letter or digit, or one of `_ - . + / ~ %`) is left at its end, by dropping at most 200 more characters: the last token that began before the cut goes whole. A token longer than that is cut anyway, and its first part is then long enough for the long-token rules. Fields over their limit end up to 200 characters shorter than before. Tested with ghp, `sk-`, AKIA, JWT, AWS-secret, hex and a 180-character token at every 7th position around the cut, in a spaced, a line-broken and a minified text.
- **`scripts/ci-local.sh` runs the Studio frontend check when a change touches what the generated ProbeScript dictionary is made from, and the Studio workflow now starts for those files.** A change to `website/src/content/docs/probescript/*.md`, the dictionary generator or `studio/frontend/src/probescript/` adds the Studio frontend check (`npm run check:probescript`, `npm run test:probescript` and the rest of that job) to a default run; `--all`, `--job` and the release dry run are unchanged, and the run says which file added it. A failure is FAIL, and a missing `node` or `npm` is PARTIAL (exit 5), never a silent pass. `--plan` prints the checks a run would select and exits without running anything. The `studio-ci.yml` triggers (push and pull request) also list `website/src/content/docs/probescript/**`: before, a change to those docs alone did not start the workflow, so a stale generated dictionary reached review unnoticed.
- **A recipe whose name is exactly one of `press tab`, `press escape`, `press backspace`, `press space`, `press up`, `press down`, `press left` or `press right` no longer runs: the line runs the key instead.** The keys are tab, escape, backspace, space, up, down, left and right. Every other `press <word>` line still calls a recipe, as before, and a word close to a key (such as `press tba`) draws a lint warning that names the keys. This is the only change to existing suites.
- **probe_agent answers a device action it does not know with an error instead of ok, and reports its capabilities in the `probe.ping` handshake (`capabilities`, now `press_key`).** Before, an unknown action was answered with ok and did nothing, so a step it could not run passed. The CLI sends only actions the agent knows, so no existing suite changes. An agent built before the capability list reports none.
- **The converter (`probe-convert`, and `probe migrate`) writes a screenshot in the dictionary spelling: `take screenshot "name"`, not `take a screenshot called "name"`.** The converter's help table and its golden files show the same spelling. Both spellings parse, so no existing suite changes; the old spelling still works in a hand-written suite.
- **The program rules of `probe lint` (the empty test, the dart share, the count and zero-state text checks, WebView included) live in `runner.LintFile`, next to the reference rules, so Studio can show the same findings.** Nothing changes for `probe lint`: the same findings, messages, output and exit codes, checked by a corpus of 114 files and 3 directories with and without a recipes folder, and a golden test with one case per message. `LintFile` takes its config as given; a caller must load it the way `probe lint` does, from `--config` or the `probe.yaml` in the working directory.
- **`runner.LintSource(cfg, path, content)` lints text the caller holds, such as an editor buffer, and gives the same report as `LintFile` for the same content.** `path` only resolves `use` targets and `with examples from` CSVs relative to the file's folder; the file need not exist. The program findings carry their `Line` (the test's line for a test with no steps and for a dart share, the step's line for a count check), and `probe lint` prints the same text as before. `LintSource` writes no package state and is safe for concurrent calls (tested under `-race`).

### Fixed
- **Every message of the wipe gates now shows the app id and the device as one clean line.** The question of a `clear app data` step, the reinstall question and its hint, the "Clearing all data" line and the notifications reinstall line printed the app id and the device serial as they were written; they now go through the same one-line cleaning (control and direction characters, 80 characters) as the before-every-test question and the refusal hint. Tests cover the questions, the hints and the "Clearing all data" line; the notifications reinstall line ("iOS cannot revoke notifications: reinstalling ...") is changed the same way but has no unit test, since it needs a simulator.
- **Studio no longer offers the results of a run for export in a workspace it did not run in.** The results kept for Export were attributed to whichever workspace was open when the run finished, so opening another folder while a run was going gave a report whose JUnit names were relative to the wrong place. A run now keeps its results only if the workspace it took its run slot in is still the open one (the same folder spelled another way, with a trailing slash, a dot segment or through a link, is the same workspace; the check and the keep happen under one lock, so a workspace change cannot fall between them) (found by new direct tests of the run slot and run state: one claim wins a race, a cancel keeps the slot until the run ends, a run that cannot start gives its slot back, a finish after a cancel is finished with the reason, a new run starts clean).
- **Three wordings of the clear-data messages are true now.** The refusal hint (`clear app data: this is a destructive operation that wipes all app data for ...`) printed the app id as it was written, and now cleans it like the question does (control and direction characters, length). The line that says a wipe was answered without a question (the CLI's and Studio's note) now says when the wipe happens when it is the before-every-test one ("Will clear data before every test of this run, of ..." in Studio). The `--yes` line is per device, not per run, and the docs now say so.
- **`see "..." is checked` works for radios under Flutter's `RadioGroup` API.** The agent read such a radio as never checked (it compared the radio's value with its own `groupValue`, which the `RadioGroup` API leaves null), so `see "Medium" is checked` failed for a radio that was on and `don't see "Medium" is checked` passed for it. Found by the widget family test of the selection controls (#732) and reproduced on builds by Flutter 3.50.0 and 3.44.8; fixed in `probe_agent` for `Radio`, `RadioListTile` and `CupertinoRadio`, both the older `groupValue:` form and the `RadioGroup` form. When the group cannot be found (for example a build that renames classes) or read, the answer is "not a checkable control", never a wrong yes or no. Agent change: it takes a new `probe_agent` release.
- **A Windows console gets its prompt back.** The terminal check asks `GetConsoleMode` (kernel32, loaded with the standard library, no new dependency), so a clear in a Windows console shows the `[y/N]` prompt again, as it did before `/dev/null` was refused. Pipes, NUL and other systems without a terminal query still get no prompt and the hint to pass `--yes`. Compiled and vetted for Windows; not yet run on a Windows console.
- **With stdin on `/dev/null`, a clear is refused with the hint to pass `--yes`, not read as "cancelled by user".** `/dev/null` is a character device, so the prompt counted it as a terminal, read end of file, and reported a refusal by the person when nobody was asked. A terminal is now checked with the terminal driver itself. A pipe and `/dev/null` get no prompt; a real terminal still asks. On Windows, FreeBSD and other systems without the terminal check, stdin is never taken for a terminal: no prompt is shown and a clear is refused with the hint, so a person there passes `--yes`.
- **A parallel or composite run grants permissions after `clear app data` the way a single-device run does.** A parallel lane read `grant_permissions_on_clear` from `probe.yaml` only, so `--yes` did not re-grant after a clear and `--no-grant-on-clear` was ignored. A composite device never re-granted. Every lane now follows one rule: granted when `--yes` or `grant_permissions_on_clear` asks, and never with `--no-grant-on-clear`. **Upgrade note:** a parallel run with `--yes` now grants permissions after each clear; pass `--no-grant-on-clear` to keep the old behaviour. A composite device with `grant_permissions_on_clear: true` now grants too.
- **`probe record` and Studio's recorder write text with a no-break space, a soft hyphen, a zero-width or bidirectional character or a control character so that it matches at replay.** Recorded text was quoted with Go's `%q`, which writes `\u00a0`, `\xNN`, `\a` and `\r` for such characters; the ProbeScript lexer reads only `\"`, `\\`, `\n` and `\t` as escapes and any other backslash pair as its second character, so a button labelled `Sign\u00a0in` was recorded as `tap "Signu00a0in"` and never matched, and the same for typed text. The shared mapping now has its own quote function: only `\"`, `\\`, newline and tab are escaped, everything else (no-break space, soft hyphen, zero-width and bidirectional characters, emoji, any script) is written as it is; a line break in screen text becomes a space and spaces, tabs and carriage returns around the text are trimmed (a no-break space at an edge is kept, a carriage return inside the text is refused, not deleted). An id or a widget type that holds a control character or a line separator is not written either: the lookup falls back to the next one. **The claim covers text without a placeholder:** the runner expands `${NAME}` and `<name>` / `<random.name>` in a step's text when it runs, and there is no escape for it, so a recorded label such as `${HOME}` or `<email>` changes meaning at replay (it did before); a step with such text is now followed by a comment, `# the text above contains a placeholder (${...} or <...>): the runner expands it, so the step may not mean what was recorded`, in `probe record` and in Studio. A character a script line cannot hold (a control character other than tab and newline, a line or paragraph separator, text that is not valid UTF-8) is not written wrong: a selector text with one falls back to the next lookup (the flat text or id, else `the element`), and typed text becomes a comment, `# type into #field was not recorded: the text has a character a script cannot hold. Add the step by hand.`, that does not carry the text. A property test runs 20,000 random texts through the real parser.
- **`probe record` and Studio's recorder write the same steps, from one mapping (`internal/scriptgen`), and `probe record` no longer panics on an event whose selector has no text.** The two had separate copies of the event-to-step mapping and of the rule for a wait after a pause of more than two seconds. The shared mapping keeps what both wrote for normal events byte for byte and settles the places where they differed: a selector with no text (the CLI panicked) or an `id` selector with empty text (both wrote a bare `#`) is no selector and the step falls back to the flat text or id, else `the element`; a `type` event with no target writes `type "hi"` (the CLI wrote `type "hi" into the "hi" field`, using the typed text as the field); an unknown action is written as a comment of its sanitized name only (`# pinch`; the CLI wrote `# unknown event: pinch`, and an event with no action wrote a comment, now skipped), so no other value of the event can reach a comment and an action with a line break can no longer start a new step; an action that is not text is skipped (the old CLI wrote `%!s(float64=3)`); the name of an unknown action and the direction of a swipe or scroll get the engine's one-line cleaning (every kind of line or paragraph separator, control, invisible and bidirectional character becomes one space, trimmed) and are cut at 48 runes, the cut marked with an ellipsis. The text of an obscured field is dropped from the event for every action, so it can never become a selector or a comment.
- **A failed connect names another app on the device that holds the agent's port (Android).** Every app on a device shares one loopback, so two apps that host the probe agent on the same port cannot both bind it; the second one moves to the next port or fails, and `restart the app` then waited out its 30 seconds and said nothing. After a failed connect or a failed token read, probe now reads (as the shell user, read-only, three adb calls at most, five seconds at most) `/proc/net/tcp6` and `/proc/net/tcp` for the app that listens on the agent's device port and `pm list packages -U` for its package, and says `agent port N on the device is held by <package> (another app on this device)` with the ways out (stop it, or give the app under test another `PROBE_PORT` and a matching `agent.port` / `agent.device_port`). It names only an app (never the system or the shell), never the app under test, and at most three packages when they share a user id. When the owner cannot be read, a failed token read after a restart says only that the agent did not answer and that another app using the same port could be the cause. A forward to another device, when `adb forward --list` shows one, still comes first. This only names the other app: the agent already falls back to the next port when its port is busy and prints `PROBE_PORT=<actual>`, and the CLI does not read that yet, so following the port is the next change. Measured on an Android 15 phone image and an Android 14 TV image; other Android versions and vendor builds are not measured, and an unreadable table simply gives no names.
- **Studio: the AI provider form no longer reverts an edit made to `probe.yaml` under it.** Like the other Settings fields, the AI form now sends the values it was loaded with; Save writes only the fields the person changed from them, takes every other field from what the file says now, and refuses a change to a field that `probe.yaml` changed on disk since the form was loaded (an editor, git, another window) with a message to reopen Settings. The key source and its variable name count as one field. A change the file already has (an editor set the value the person is setting) is accepted, the refusal names the field and never a value, and after a save the form shows what the file holds, so an edit made under it is shown and not reverted.
- **The Flutter fixture suite's before-each restart runs.** `before each test:` had a trailing colon, which made the parser drop the hook's body, so the restart the suite asks for never ran (`probe lint` now reports the header). The colon is gone: every test of `native-test-apps/flutter/probe-tests/flutter_suite.probe` starts with a restart, about 2.7 s more per test. Run on a local Android 15 emulator before and after: 5 of 5 passed both times.
- **A failed connect no longer blames "an adb port forward" that is the adb server, and names a forward only when `adb forward --list` shows it (Android).** When the agent port could not be dialled, the message said the port was "held by an adb port forward (pid N)" whenever `lsof` found the adb server listening on it, which is the case for the CLI's own forward too, so it named a pid that meant nothing and a forward that might not exist. The adb server is no longer named at all. The message now says "agent port N is forwarded to another device (SERIAL)", with the command that removes it, only when `adb forward --list` shows that port mapped to another device; with no such mapping it says nothing about forwards. The same check serves the parallel runs, the single-device Android run, composite runs and the reconnect, and a failed iOS connect never runs it. The name, pid and simulator id of a process that does hold the port are cleaned before they are printed. It is one read-only adb call, made only after a failed connect. A cancelled run no longer makes a crash look at all (it used to try one call that was refused, and used up one of its three tries).
- **Studio: the redaction of secret text fields before the widget tree goes to an AI provider covers every text-entry widget.** The rule matched only `EditableText`, `TextField`, `TextFormField` and `CupertinoTextField`, so a password in a `CupertinoSearchTextField`, `CupertinoTextFormFieldRow`, `SearchBar`, `SearchAnchor`, `DropdownMenu`, `Autocomplete` or a subclass kept its key in the tree that was sent. It now matches by class name (the TextField, TextFormField, SearchField, SearchBar, SearchAnchor, EditableText, Autocomplete and DropdownMenu families, private `_` prefixes and subclasses included), and a test reads `docs/compat/catalog.json` and fails when a catalog widget that looks like a text input is neither covered nor reviewed as not one, so a new Flutter version cannot silently add an uncovered input.
- **Studio finds `adb` in the standard SDK places, and says when it cannot find it.** The device list ignores an `adb` error, so for a Studio opened from the Dock or Finder (no shell `PATH`) a missing `adb` just made Android emulators and phones absent, and the engine's "adb not found" message only appears at connect, which a device that never lists cannot reach. Studio now looks for the default `adb` on `PATH` and then in the standard Android SDK places (`ANDROID_HOME` / `ANDROID_SDK_ROOT`, `~/Library/Android/sdk` on macOS, `~/Android/Sdk` on Linux, `%LOCALAPPDATA%\Android\Sdk` on Windows; an executable file only), picks one up that appears later while no device is connected, and when none is found and no device is usable the first-run line says what was searched and what to do. **Opening a folder never runs a program it names:** a `tools.adb` in `probe.yaml` is part of the repository and is not used by any passive action (device list, refresh, first-run line, device settings). It is used only when you press Connect, as `probe test` uses it only when asked to run tests, and only if it is an absolute path to an executable file called `adb` outside the repository the workspace belongs to (the folder above it that holds a `.git`; with none, outside the workspace folder; symlinks resolved), after which Studio says "Using adb from probe.yaml: adb"; one that fails the checks refuses Connect with the reason. `tools.flutter` is not used by Studio. New bound method `DeviceListNote`.

- **`config.Default()` is guarded against sharing.** It copies the built-in default with a plain assignment, which is a full copy while that default holds no map, slice or pointer. A test now fails if a default ever gains one (so it must be deep-copied then), and another checks that changing a returned config does not change the next. No behaviour change.
- **Studio no longer runs with another folder's `probe.yaml` when the workspace's cannot be loaded.** When the open workspace's `probe.yaml` did not parse, or had an invalid app id, Studio quietly read the `probe.yaml` of the folder it was started in instead, and connected and ran with that project's app id, port, timeouts and `tools.adb`: a `clear app data` could have been asked about, and run on, another project's app. Studio now refuses: Connect, ConnectWiFi and a run give one line, "probe.yaml cannot be read: <reason>: fix it or remove it to use the defaults" (no path), the Problems tab says it was not checked, and the Workspace note shows the same line. A workspace with no `probe.yaml`, and no workspace at all, use the built-in defaults (agent port 48686, no app id) and never a config from the folder Studio was started in. Behaviour change: a workspace whose `probe.yaml` was silently ignored before now needs the file fixed or removed.
- **Studio: a Disconnect while the question before `clear app data` is open says no and closes the dialog.** The step used to keep waiting (up to 60 s) for an answer about a device that was gone. It is now denied at once, the dialog closes, and a note says "Clear app data was not run: the device was disconnected".
- **Studio: the lint says so when the workspace is the root of the file system, and keeps URLs in what it shows.** A workspace of `/` (or a drive) makes the fence around the lint meaningless, so the Problems tab says "Not checked: the open workspace is the root of the file system ..." and reads nothing; a home folder is fine. The guard that removes absolute paths from finding text no longer changes `https://example.com/a/b` into `https:<path>`; it replaces real file system paths and `file://` URLs only.
- **Studio: Enter on a failed test row no longer also types a new line into the file.** The row jumps to the failing line and moves focus to the editor; the same Enter then typed a line break into it and marked the file as having unsaved changes. The Results rows now stop the key from going on (`preventDefault`), like the new Problems rows. The three tabs under the editor (Results, Network, Problems) are also marked up as a tablist with `aria-controls`, `aria-selected` and labelled panels.
- **Studio: a recording survives a reconnect cleanly.** When a run restarts the app while a recording is on, the recording moves to the new connection. The check and the move are now one step, so a Stop that comes at that moment can no longer leave the recorder on the new connection with the new app still recording (it is told to stop); a recording is marked active only once its handler exists; the recording is taken over before the old live view is stopped (up to 2 s of taps were lost); and no `wait N seconds` line is written for the time the restart took.
- **Studio: the live view and every later call follow a run that restarted the app.** A `restart the app`, `open the app` or `clear app data` step kills the app and reconnects the run to the new agent, but the connection Studio holds (the live device view, the widget tree, Record, the request log) stayed on the old one. After such a run the device label said "no signal" for good (`use of closed network connection`) until the person disconnected and connected again. `runner.RunOptions.OnClient` now reports each new client, and Studio takes it, restarts the live view on it and closes the old one. The other holders of the old client follow too: with the optional shared device view on, its hub resolves the connection's client on each call; a recording that is on when the app restarts moves to the new client (and `StopRecording` clears the handler on the client it set it on); and every read of the connection's client goes through one accessor that takes the lock. Found by driving the real window on a simulator, after the shared device context let the restart step run in Studio.

- **Studio: a remembered folder that is gone, or has no `.probe` files, is named as the next step.** Studio reopens the last workspace at start. When that folder had moved or been deleted, the Workspace pane stayed blank and the line under the toolbar still said "Open a .probe file from the Workspace pane", with nothing to open. Now the pane says "This folder cannot be read (it may have moved or been deleted). Open another folder." with an **Open workspace** button, an empty folder says it has no `.probe` files, and the line under the toolbar names the folder as the next step in both cases. Found by driving the real window.
- **Studio: Run keeps saying why it is disabled while a run is in flight.** The first-run guidance read the button's own state, so typing in the editor during a run or a re-run left Run disabled with no reason (`aria-describedby` gone, the title back to "Run the open file"). The reason is now derived from the window's state: "A run is in progress." while a run or a re-run is in flight, and it goes when the run ends. The guidance is also re-read when a run starts and ends.
- **`before all test` and `after all test` (singular) are a parse error, and the message names the plural.** The lexer reads `before all` as one word, so the singular used to end the hook at once, with no body, and its indented steps then ran as a test with no name, with no error. The parse now fails with `did you mean "before all tests"?`. **Migration:** a file that uses the singular form parsed before and now fails to parse; rename the header to `before all tests` or `after all tests`. Other words after a hook header are still accepted as before (see the Added entry on `probe lint`).

- **A lane error fails a parallel run, and the files the lane did not run are reported as failures.** A lane stops on an error that is not one test's: a file that cannot be read or parsed, a CSV or recipe that cannot be loaded, a `--grant` that names no permission the run knows, or a device that never connects. (On Android a grant the device refuses is a warning, as before; on an iOS simulator a grant the simulator refuses is an error that stops the lane.) Before, a run could exit 0 with those files missing from the results. Now the completed tests of the file the error came from are kept, that file is a failed result, every file after it is a failed "not run" result that says where the lane stopped (`not run: lane stopped at <file>`; the cause is on the stopping file's result, once), and the run exits non-zero. A file that ran with no test left (no tests, or none a tag let through) has no result, as it did before, and is not reported as not run. A lost connection in the middle of a test is none of these: it becomes that test's failed result, as before. A lane is retried only when it cannot connect (the port forward, the token, the dial or the handshake), before any of its tests runs, so no test runs twice.
- **Cleaning a printed name can no longer panic on a limit below 1.** `oneline.Cut` (behind `device.CleanOneLine`) cut a non-empty text to `n-1` runes plus an ellipsis, which panicked for `n` 0 or below; it now returns an empty string. Every caller passes 20 or more, so no run hit it. The hidden characters in `internal/oneline` are written as `\u` escapes, and it has its own tests.

### Security
- **The Studio automation discovery file is checked like the MCP token file, by the same code.** The file holds the automation token. Studio created its folder 0700 and wrote the file 0600, but did not re-check an existing folder or file, did not refuse a link at either name, and fell back to the system temp folder when the user had no config folder. It now goes through one writer for every secret file (the new `internal/secretfile`, which `probe-mcp --http` uses for its token file as well): a folder or file that is a link, a file that is not a regular file, one that belongs to another user or that others can reach is refused and not repaired; the file is written to a new 0600 file and renamed into place, so it is never written through a link; with a file named by `PROBE_STUDIO_AUTOMATION_FILE` the folder is the person's own, so it only has to be a real folder of theirs that others cannot write to; and with no user config folder and no named file the endpoint does not start (the shared temp folder is not a fallback). On exit a server removes the file only while it holds its own document, and never waits on what is at the name (a pipe planted there made removing it wait for a writer, and with it Studio's start-up). A write that fails removes its temporary file, and the next write removes the ones an earlier crash left: only a file named exactly like the ones this code makes (the file's name, a dot and a decimal number of at most ten digits), a regular 0600 file of this user, older than ten seconds. A file such as `automation.json.bak` next to it is never touched, whatever the folder. A file of the person's own that is named exactly like a temporary file (`automation.json.` and one to ten digits), is a regular 0600 file and is older than ten seconds is taken for a stale one and removed. Two limits, on purpose: only the last component of the path is checked, so a link in a folder above (such as `~/.config` pointing elsewhere) is followed, as the person set it up; and on Windows the mode and owner checks are skipped, because file modes there do not say who can read a file.
- **A device or simulator id no longer shapes a file path.** Reading a physical iPhone's process list named devicectl's output file after the id (`devicectl-procs-<first 8 characters>.json` in the temporary folder) and removed it afterwards, so an id beginning with `/../../` (only its first 8 characters were used) named, and then deleted, a `.json` file one folder above the temporary folder, an id shorter than 8 characters crashed probe, and the predictable name in a shared folder could be replaced by a link. The output now goes to `processes.json` in a new private folder (`os.MkdirTemp`, mode 0700, removed after the call), and the id must be a device UDID (8 hex digits, a dash and exactly 16 hex digits; or 40 hex digits) or a UUID before anything runs. A simulator's data path, the token search under it (a glob) and the token file removed before a reconnect are built only for a simulator UDID (a UUID); for anything else there is no path, so no token is read from, or removed at, a place the id or the working directory would choose. New: `ios.ValidDeviceUDID`, `ios.ValidSimulatorUDID`.
- **The test-only headless bridge no longer serves its token.** The token was written into `/__headless/bridge.js`, which any local process could fetch without it, and the event stream took it in the address (`?token=`), where it is logged and kept in history. The bridge now needs `PROBE_STUDIO_HEADLESS_TOKEN` from the program that starts Studio (at least 32 characters; without it Studio's headless mode does not start), takes it out of its own environment so the programs it starts do not inherit it (this does not hide it from a process of the same user, which can still read it with `ps eww` or `/proc/<pid>/environ`), refuses a token with spaces or control characters in it, and accepts it only in the `X-Headless-Token` header on calls and on the event stream; a token in the query string is refused. The page script and the page hold no secret. The Playwright suite makes a token per run and sends it with every request of every page; its traces after a failure record request headers, so they hold that run's token, which stops working when the run's server stops. Headless mode is test-only and off by default; this affects only the people and scripts that start it.
- **Studio no longer sends a saved WiFi token to a device just because it announces the right name.** A saved token was looked up by the device's announced name, and any machine on the network can announce any name, so selecting a look-alike at another address with the token field left blank sent it the saved token. Now a token is saved bound to the address (host and port) of the connection that accepted it, and a saved token goes only to that address. For another address, and for a token saved before this change (it has no address), the window asks first: "Send the saved token?", naming the announced name, the address it would go to and the one the token was saved for, with Cancel focused, Enter not pressing the send button, Escape a no, and the send button armed after half a second, like the clear-data question. No answer, no window (the headless bridge cannot answer) and a timeout are all a no, and nothing is dialled. A public address is refused before anyone is asked. After a confirmed connection the token is bound to the new address, so a phone whose address changed asks once. Forgetting a token forgets its address. The address that is compared is the address that is dialled: the host is normalised (no brackets, no trailing dot, no surrounding space, lower case) before both, so a spelling that compares equal cannot be resolved differently. A confirmed connection that fails binds nothing, and a token replaced while the question was open is not bound to the address that was approved for the old one. A token saved with no address is asked about in words that say so. The typed-token path is unchanged. Studio only; the CLI keeps no saved tokens.
- **A lint can be fenced to a workspace, and then it reads nothing outside it.** `runner.LintSource` and `runner.LintFile` take the new `LintWithRoot(root)` option (Studio's lint over an editor's text is the caller). Under it a `use` target, a CSV source or the recipes folder outside the root is not read, and the report says so once for it with fixed text that names no file; while a recipe source is refused (the recipes folder, a recipe file or a use target), an unresolved recipe call is not reported as an unknown step, since the refused source may define it. A file inside the root that does not parse is reported by its relative name and the parser's line, never by its text, which the parser's message quotes. A symlink inside the root is refused when its target is outside the root or does not resolve (a dangling link), and the refusal is the same whether the target exists or not. A file inside the root that cannot be read is an error named by its relative path, and the lint goes on. An empty root is an error. The check and the read are two steps: the read opens the path the check resolved, but a symlink replaced between the two is not caught, and a hard link to a file outside the root cannot be told from one inside, so it is read. Without the option the lint reads exactly as it did: `probe lint` and the corpus output are unchanged. The `with examples from` line of a CSV source is now the line of that statement; it used to be the line after it.
- **The MCP `take_screenshot` file check is stricter in four small ways.** A picture whose file time is up to two seconds before the capture started still counts (file systems with a coarse clock no longer make a fresh capture "not found"); the file is opened without waiting, so a named pipe put in its place cannot block the server; a picture that grew past the 64 MB read limit after it was measured is refused with its own message; and the exact-name match is tested against a name that is only the end of another name.
- **The MCP `take_screenshot` tool no longer trusts the name it is given.** The name went unchecked into the script that takes the picture, so a name containing a quote and a new line could add steps of the caller's choosing (for example `restart the app`, `clear app data` or a `call`), and into the file pattern that finds the picture, so a name with `../` or `*` could return a PNG from outside `reports/screenshots`. The name is now cleaned once (path separators, control and direction characters, `..`, characters that mean something in a script or a file pattern, at most 120 bytes; an empty name is `mcp_capture`), the same cleaned name is used for the script, the pattern and the answer, and only a regular file called exactly `<name>_<digits>.png`, whose real path is inside `reports/screenshots` and which the capture just wrote, is returned (the newest by the number in its name); a failed capture is "not found", not an older picture. A name made of plain characters behaves as before.
- **Every Android device command that carries text from a script, a file name or the configuration now quotes it for the device's shell.** `adb shell` hands its words to the device's shell as one command line, so a file name such as `x;touch y.jpg` in `add media`, or an app id or language tag with `;`, `&`, `$(...)` or a quote, ran as a command on the device. The quoting is now one helper, `internal/shellquote` (the three earlier copies are gone), used for `add media`, `set language`, permission grants and revokes, deep links, package queries, the launch, restart, clear-data and kill commands, the app-token read, the CPU measurement (`pidof`), the screenshot read with `run-as` and the dialog diagnosis. Plain words (package names, permissions, paths) are passed unchanged. Two inputs are also checked now: the video resolution in `probe.yaml` must be `WIDTHxHEIGHT` (otherwise recording falls back to screencap with a message), and a screenshot path from the agent must be a file in the app's own private storage (otherwise it is refused).
- **A failed `type native`, or a failed type into a system dialog, no longer prints the typed text.** The error of a failed adb call quotes the whole command line, which held the typed text (it can be a password); the text is now masked in that error, and the `type native` step line now shows the text with its variables expanded and known secret values masked, the way the `type` step line does (it showed the placeholder, for example `<pw>`, before). Typed text of fewer than four characters is not masked in its bare forms, because that would mask every occurrence of those characters in the message; its quoted form is.
- **`type native` no longer lets typed text run as a command on the Android device.** The text was passed to `adb shell input text` with only spaces escaped, and adb hands the line to the device's shell, so text such as `a;reboot` ran `reboot` there (checked on an emulator). The text is now passed as one single-quoted word. Text that cannot be typed this way is refused with a clear message, before the tap: control characters (a new line included), anything outside ASCII (`input text` cannot type it), a `%` (`input text` reads `%s` as a space) and a text that starts with `-`. Typing non-ASCII text into a native field is not supported until another path exists.
- **Device names, serials, UDIDs, app ids and process names that come from `adb`, `simctl` or `lsof` are printed as plain text on one line.** A device name can hold anything (a simulator name in simctl's JSON can hold a new line, an `adb` model name an escape sequence), and so can a process name, so these lines could print a terminal escape, a bidi or zero-width character, or a second line that looked like probe's own ("pass --yes"): `probe device list` (serial, state, name), `probe device view` (the connect line, the listening line in every output mode, the "several devices are ready" choice), `probe device set` (the done line and both refusals), the install, grant and launch lines on Android and iOS (the app path, the serial and the app id; each cleaned and cut to 80 characters), "device not found", "Created simulator" and the simulator-mismatch error, the app-not-running error of a token read (`TokenAppNotRunningError`), "versionName not found", the `adb` error that carries `adb`'s stderr and the `uiautomator dump` error that carries the device's output (cut to 400), the port-following notice, the WebView forward errors, the simctl boot and reinstall errors, and the port-holder hint (the `lsof` process name). They now go through one cleaner for display only: the values given to `adb`, `simctl` and `devicectl` are unchanged (the tests check the real serial, udid and app id reach each call). The cleaner moved to the new leaf package `internal/oneline` so `internal/probelink` and `internal/ios` can use it; `device.CleanOneLine` delegates to it. An agent's handshake app id was already limited to a plain application id, so it never reached the mismatch warning raw (now tested). First of three small changes; the run's device context and the parallel run's device labels follow.
- **A parallel run names each lane's device as plain text on one line.** The lane list, the `[device]` labels of the progress and warning lines, the connect, token and handshake errors and the run's summary printed the device's id and name as `adb` or `simctl` reported them, so a name could carry a terminal escape, a bidi or zero-width character, or a second line that looked like probe's own. They now go through `device.CleanOneLine` for display only: the lane's id, the one given to `adb` and `simctl` and written to the reports, is unchanged. The 12-character short label is now cut by characters, not bytes, so it no longer splits a multi-byte character; a plain serial reads as before.
- **A parallel lane's error is printed as plain text on one line.** The error of a lane that failed (a refused connection or handshake, adb's output) can carry text from the device or the agent; the retry line, the "all attempts failed" line, the device summary and the run's `device errors` now show it cleaned (control characters, escapes, bidi and zero-width characters become spaces), as the lane's device name already was.
- **A screenshot path from the agent is no longer used as a host path as it is.** When an agent answered a screenshot request without image data, the CLI used the path in its answer as a host file: it read it (and handed the bytes on, for example to an AI provider as the screenshot), attached it to the run, and with `ai.redact` rules wrote the redacted image back over it, so an agent could make probe read, copy or overwrite another host image. Every current agent sends the image itself; it is now saved as a new file in `reports/screenshots` (named after the base of the agent's path, always ending in `.png`, `screenshot.png` for a name Windows reserves such as `CON` or `COM1` or one ending in a dot or space; created exclusively, so an existing file or a planted link is never written through), and undecodable data is an error. An answer without data, which is what an older agent on an iOS simulator sends, is read only when the file is a regular `.png` file with a single name (not a link, not a hard link, not a FIFO) whose real path, asked of the open file, is in a simulator's data folder (`~/Library/Developer/CoreSimulator/Devices/<UUID>/data/`); the file is opened once, checked and read from that open file, and its bytes are saved as a new file in `reports/screenshots` like data, so no later step opens the agent's path and a link swapped in after the check is never followed. Anything else is an error, which an older agent on Android or a physical iPhone now gets. A redacted screenshot is written to a new file (`reports/screenshots/<name>-redacted-*.png`, mode 0600), never over the file it came from.
- **A device id that is not one is refused where it would act, not later.** `device.ios_device_id` in `probe.yaml` must be a simulator or device UDID: anything else (a device name, a path) stops the config load with a message that names the key (breaking for a `probe.yaml` that put a device name there; it was used as the `--device` serial, which a name with a space already failed). Reading a simulator's token with an id that is not a simulator UDID (a physical device's, or not an id) fails at once instead of polling until the timeout. On a physical iPhone, a kill whose process lookup fails (the id is refused, or devicectl cannot list the processes) now says `the app was not stopped: ...` in one line; it used to pass as "not running", so a relaunch could start beside a running app. The token paths of a simulator are built in one place (`SimCtl.tokenFileCandidates`), and with an id that is not a simulator UDID there is none, the legacy device-level file included.

### Added
- **The MCP `take_screenshot` picture is size-bounded.** A PNG above 2 MB or above 2000 px on a side is scaled down before it is sent as base64 (the answer says so); a file that is too large to read, or whose header claims more than 25 megapixels or about 256 MB of decoded memory, is refused with a clear message instead of being sent whole.
- **`set airplane mode on` (ProbeScript) turns an Android emulator's airplane mode on or off, and `probe migrate maestro` converts Maestro's `setAirplaneMode: enabled` and `disabled` to it.** The step takes `on` or `off`, bare or quoted, with an optional `to`, and runs `adb shell cmd connectivity airplane-mode enable` or `disable` on the device the run targets. Only an Android emulator takes it (serial `emulator-NNNN`): an iOS simulator (`simctl` has no command for it, and the step does not fake the change), a physical device over USB or wireless debugging (not verified on one) and a network adb address (`host:port`) are refused, because turning airplane mode on would cut the connection adb uses. That covers Genymotion, other network emulators and cloud farm devices that adb reaches over the network. The check reads the serial alone: a device that adb reports as `emulator-NNNN` is accepted wherever it runs, and a run that marks a device physical refuses it whatever its serial. Nothing restores the state at the end of a test: a test that turns it on must turn it off. Migration: before, the converter read the Maestro value as a boolean, so `setAirplaneMode: enabled` wrote a TODO comment that told the user to run `airplane-mode disable`; `enabled` and `disabled` now become the step, and any other value keeps a TODO comment with a warning. The grammar (EBNF and conformance test), the dictionary and the Studio module generated from it, the MCP `write_test` description, the VS Code grammar, snippet, completions and README, `llms.txt`, the root README, the roadmap line, and the Android, iOS, syntax and MCP pages are updated. `probe-convert`, a separate tool, still writes a TODO comment for `setAirplaneMode`. Checked on a named Android emulator (API 35): `on` and `off` read back with `settings get global airplane_mode_on`.
- **`set appearance dark` (ProbeScript) sets the device's light or dark mode.** The step takes `light`, `dark` or `auto` (Android only; iOS refuses `auto` when the step runs), bare or quoted, with an optional `to`. It runs the same commands as `probe device set appearance`, through the shared `devsettings` package, on the device the run targets, and it does not relaunch the app. A physical iOS device is refused with a message that names Settings. The parser refuses any other value before a device is touched. The grammar (EBNF and conformance test), the dictionary and the Studio module generated from it, the MCP `write_test` description, the VS Code snippet and grammar, and the Android, iOS, syntax and MCP pages are updated. Checked on a named Android emulator (`dark`, `light` and `auto` read back with `cmd uimode night`, and the app's process did not restart) and on a named iOS simulator (`dark` and `light` read back with `simctl ui`, `auto` refused). A user recipe named `set appearance dark`, `set appearance light` or `set appearance auto` is now shadowed by this step: a call with that name runs the step instead. Text after the value is not rejected; it is read as the next statement, as after `set language`.
- **The name of `take screenshot` accepts placeholders, so a name can carry what makes it unique.** The name is expanded like other steps (`<var>`, `${ENV}`, `<random.*>`), and three built-ins are new: `<timestamp>` (the time of the step to the millisecond, `20261113-141502-123`), `<run>` (one stamp for the whole `probe test` run) and `<test>` (the running test's name, made safe for a file name; letters and digits of any script and UTF-8 are kept). The built-ins are expanded in the name as written, before variables, so a variable value that contains `<test>` stays text. The finished name is cleaned (`/` and `\` become `_`, control characters, direction controls and the byte order mark are dropped, `..` is removed, at most 120 bytes; joiners are kept) because a variable or an environment value can put anything in it. The app already appends a millisecond stamp to every screenshot, so files do not collide. `compare screenshot` expands variables but not the built-ins, because its name is the baseline's key.
- **Studio network redaction bounds work on nested assignments.** If a field would require excessive rescanning, the entire field is hidden.
- **Studio has an Explain failures button.** Under the Results title, only while AI is on and the last run has a failed test with error text. It shows what would be sent, with the same `Sent to` line as the chat, and sends only after **Send**: before every Explain for a hosted or remote provider (no way to turn it off), once per session and when the destination changes for a model on this machine. The advice (at most 5 failed tests, one request each) appears in an **Advisory** card that names the model and whether it is local or hosted, is shown as text, and never changes a result. A setting that changes between the preview and **Send** sends nothing, a new run or a change of settings drops an Explain that is still waiting so its answer never shows, and Escape on the preview is Cancel. In a window narrower than 1150 px the card sits on the window itself, inside its edges, so it and its close button are reachable at any width. `ExplainAvailability` and `ExplainPreviewed` are the new Studio methods. The browser suite starts with a seeded failed run (`PROBE_STUDIO_SEED_RUN`, headless builds only, a file inside the workspace, strict JSON, at most 1 MB and 50 tests) so these screens are tested without a device.
- **Studio can read the requests and responses the app made during a run, redacted (Network tab).** `HTTPExchanges` lists the recorded exchanges (newest 500, with a filter on method, status class and text) and `HTTPExchange(epoch, seq)` returns one with its headers and bodies, both read with the same `probe.http_log` call the runner uses; a changed or missing epoch (the app restarted, or sends no restart marker) is refused. Everything shown goes through one redactor in Go, before it reaches the window, layered on the recorder's own (`internal/record`): the value of any header, query parameter, JSON key, form field, XML element (also CDATA), HTML input or meta tag, or multipart part whose name is in a secret family (the recorder's token rule plus authorization, cookie, token, secret, password, API key, session, any name that ends in the word key, `code`, `private` and similar, in any case, any separators, camelCase, full-width letters and percent-encoding) is replaced; user names and passwords in a URL (even with a `?` in the password), fragments and query values, bare query components, emails and home folders are removed; JWTs, base64url runs of 24 characters or more and card numbers that pass the Luhn check are redacted; hidden characters are dropped first; truncated JSON, JSON whose comments are removed (`/* */` and `//`, outside strings), one-value-per-line JSON, SOAP, GraphQL arguments, YAML (also nested, and the block form) and form fields that hold URL-encoded JSON are covered; header values and error texts are read for assignments like `sid=` or a Digest `response=` and for URLs. The raw text of every field is bounded before any pattern reads it (8 KB per header value, 4 KB per URL and error text, 64 KB per body) and one exchange has a budget of 256 KB for all of them. Bodies are cut to 4 KB, a binary body or one over 1 MB shows only its size, and nothing is shown as markup. It makes no request of its own, writes nothing to disk or to a log, and nothing it reads goes to an AI provider (checked on the syntax tree as a tripwire for accidents, not as proof). Not covered: a secret that has no name and no shape (a hex value under 32 characters, a short base64url token, a six-digit code under a name that is not in a family, a card number glued to letters, a Social Security number), a token in a path segment, UUIDs, IP addresses and phone numbers; a combining or glyphless character that is not named can still split a name. The recorder's own redactor (`internal/record`, used by `probe record`) gained the same-line fix for nested config text and the YAML block form, and keeps its previous rule as a floor: a recorder bundle is never weaker than before (a differential test against a copy of the previous redactor, 14,208 cases, finds no secret that the old one hid and this one shows). Also covered in the monitor: YAML block scalars, lists, tags and anchors under a secret key; XML elements and attributes that name a secret (`<add key="licenseKey" value=...>`); HTML textareas, selects, placeholders and data attributes of a secret input; name/value objects with short member names (`k`/`v`); forms inside forms; and error texts, which now take the same pass as a body. A header value that starts with a scheme word (`Basic`, `NTLM`, `Negotiate`, `Token`...) loses what follows it. A name over 256 characters is treated as a secret. Not covered: header values with no scheme word and no secret name, a JSON key that is itself a secret, a secret in a path segment, a secret in a positional array, a secret split in two parts, and names that are over-redacted on purpose (`primaryKey`, `publicKey`, `sortKey`, `cacheKey`, `tokenizer`, `secretary`, `sessionId`, `password_changed_at` and the like). The **Network** tab next to Results lists the exchanges of the last run (the newest 500; method, URL with query values hidden, status, time, size, whether it was mocked), refreshes every second while a run is going, filters by method, status class and text (on what is shown, so a search finds a parameter name and never confirms a value), and shows one request's headers (names, values hidden for credentials) and bodies (a text preview, a size for a binary or large body) as text. It is read-only, shows only what the agent recorded, stores nothing in the browser, and sends nothing anywhere but to Studio. What is shown is decided so that a slow answer never lands under a newer view: a new run, a connection or workspace change, a refused read or a changed row drops a detail or a list that was still on its way, and an app that restarted is read again from the start. Rows from a test seed (headless build) are marked `seeded (test)`.
- **`internal/httpcall` makes one HTTP request with the limits a request typed into a tool needs (a library for Studio's coming Send request panel; `probe test`'s `call` step is not changed).** Only `http` and `https`; no user name or password in the address; a method and header allow-list (no `Host`, `Content-Length`, `Transfer-Encoding`, `Connection`), at most 50 headers and 256 KB of request; a 30 second time limit (at most 120) and a 1 MB response cap (the body is marked cut, at most 8 MB); at most 5 redirects, and a redirect to another origin (scheme, host or port) is reported and never followed; no proxy from the environment, no cookies, no skipping of certificate checks, a fixed User-Agent; a response that ends early is an error and never a shorter response; and a policy `Policy(host, port, ip)` that is asked about every address of a name after it is resolved (one refusal refuses the name), so a name that resolves to this machine or a private or cloud metadata address cannot get around it. `ClassOf` names the kind of an address, reading IPv4 addresses inside NAT64, 6to4, IPv4-compatible and IPv4-mapped IPv6 addresses as the IPv4 address, and calls reserved ranges `Reserved`. It writes nothing, logs nothing, and its errors carry no address or header.
- **Studio has a Send request panel in the Network tab.** A method, an address, headers (a table that is not saved to anything) and a body; Studio makes the request from this machine with `internal/httpcall` and shows the status, time, redirects, response headers and a text preview of the body, all redacted by the Network tab's redactor, as text, with nothing stored and nothing sent to an AI provider. The first request to a host and port in a session asks first, naming where it leads (public, this machine, a private network, with a stronger warning for the last two); Allow lasts for the session, Cancel and Escape send nothing. `CheckTarget`, `AllowHost` and `SendRequest` are the new Studio methods; the rules are in Go, so an unallowed host is refused through the bridge too: only `http` and `https`, one request at a time, ten a minute, no cross-host redirect followed (it is shown), cloud metadata, unspecified and multicast addresses never, and in the headless build addresses on this machine or its network only for hosts in `PROBE_STUDIO_TEST_ALLOW_LOCAL`. An allow is for the kind of address that was shown (the internet, or this machine and private and reserved ranges): a name that is allowed as the internet and later leads to this machine is refused and asked about again, and a host cannot be allowed before it was checked. Every value typed in the panel is hidden wherever the answer echoes it. Hosts are kept in one form (punycode, IPv6 in brackets, no trailing dot), what is remembered is bounded, address checks are limited to 30 a minute, and all multicast addresses are never called. `internal/httpcall` classes all multicast as Multicast and has a `Dial` option for tests. The Go policy that `internal/httpcall` asks at dial time checks the allow by host and port again, so a name cannot reach a host that was not allowed; a refusal names the kind of address and never the address; a response that ends early is an error. Not covered: a live proxy and revealing a redacted value.
- **Studio can save the request typed in the Send panel as a `call` step (Save as step).** It inserts `call METHOD "address" [with body "..."]` into the open file under the cursor, using only the existing `call` grammar: GET, POST, PUT and DELETE (PATCH, HEAD and OPTIONS cannot be saved) and a text body, checked by reading the line back with the ProbeScript parser; no grammar changed. Headers are not saved (the panel says so). Every query value, and any path segment or body value that looks like a credential, is under a credential name, is a card number or is an email address, is written as `${PROBE_SAVED_NAME}` (the environment variable `probe test` expands and masks; the prefix keeps it from colliding with `USER`, `PATH` or `HOME`), and a comment line names what to set. **Values the save cannot recognise are not replaced** (a passphrase of letters only, a short value, a six-digit code, a shapeless value under a neutral name): read the step before saving the file. A `${NAME}` or `<name>` typed in the request is refused, because `probe test` would fill it in when the step runs. The step goes into the test the cursor is in, and nothing is inserted with no file open, a read-only file, a cursor outside a test or a step that would not parse there; a body with an invisible or control character, an address with a user name or password, or anything that does not read back exactly is not saved. `BuildCallStep` is the new Studio method; it reads the typed request only, makes no request and does not touch the allowed hosts or any other state.
- **Studio treats what a device announces over mDNS as untrusted.** A name is cleaned of control, invisible and direction-changing characters and cut at 64 characters (a name with nothing left is listed by its address), the version is one short word from the first `version` text record, a port outside 1 to 65535 or an address that is 0.0.0.0, multicast, broadcast or loopback makes no device, at most 256 devices are listed per search (the `version` key is matched without case, as RFC 6763 says), and an address and port is listed once. The list item is built with text only, and the new Studio Wi-Fi tests (hostile announcements, 10,000 records, the item as text) run in CI.

### Fixed
- **A startup connect failure still writes a JUnit report.** With `probe test --format junit -o results.xml`, a run that cannot reach the local agent (for example a closed port) used to exit with the error and write no file, so CI had nothing to publish. The file is now written with one `<error>` testcase per planned test file, carrying the connect error message; the exit code is unchanged. The local JUnit file stays unscrubbed (it is your own output); only the `--upload-report` payload is scrubbed.

### Changed
- **Provider error text, model names and connection details are scrubbed like an answer, before they are cut; a suggested fix that was changed says so.** An error body that echoed `jane@example.com at /Users/alice/x` came back in full from a suggested fix, Explain's errors, the model list and the connection check (a chat error is covered by the per-field chat path). They now take the answer's steps: escapes of hidden characters decoded, hidden characters dropped, bounded, key removed, families redacted, emails and this account's own home folder scrubbed, and only then cut to 200 bytes (before, the cut came first, and an address across it left a fragment, such as `jane.doe@example.c`; the Anthropic calls, the error field of an answer and the shared provider code all clean first now). The reason of a suggested fix was cut before it was scrubbed too; it is now scrubbed first and cut last, and its cap is 400 characters (it was 400 bytes). What a reply and an error may lose: an email address, this account's own home folder (the folder `os.UserHomeDir` names, `/Users/you`, `/home/you`, with a boundary, also percent-encoded as in a query value), a credential and the key. A reply is not scanned for other people's folders, so `/users/42`, `/home/settings`, `/root-cause` and `https://app.test/root/x` are shown as written. The text that is sent with a failure (Explain and the suggested fix) still has the generic home-folder pattern applied, which also replaces text that only looks like one (`/home/settings` becomes `~`): a stated limit. A suggestion that cleaning changed (`type "[email]" into "Email"`, or a reason with an address in it) shows a **Changed** row in the card, saying text was removed from the suggestion or its reason, so nobody runs a test that types the literal text. Not covered: a folder name with a space in it, and other people's folders in a reply.
- **Every DeviceContext on a run path is built by one helper that asks whether the device is physical.** The single-device run, the `--locale` language step, composite runs and parallel runs each set the physical flag themselves, and a builder could leave it unset. `runner.NewDeviceContext` asks the device once, and a test fails if a builder in the CLI or the runner builds the context some other way. The `--locale` step now asks an Android device too; the language step reads the flag on iOS only, so its result is the same. The single-device run has one more change, in the next entry.
- **In a single-device run, a platform the run does not know counts as physical.** When `--device` names no listed device, the platform stays at its configured default, which is `both` unless `probe.yaml` sets one. Before, the physical flag was false for such a run, so its steps took the emulator branch; now the device is asked the physical question, which answers yes for a platform it does not know. The run takes the HTTP fallback (`UseHTTP`) and the physical branch of each step that checks the flag, so `set airplane mode` is refused. This is the safe direction: an emulator-only step does not run on a device the run cannot identify. A named emulator or simulator in the device list is unchanged.
- **Studio's chat takes the same steps as Explain before the gate, for every field.** A token, a card, a JWT, an email, a home folder or the saved key written with a hidden character or a no-break space in it (`ghp_` + `\u200b` + ...) was whole to the provider in a chat turn and its preview, because the chat gate read the text as typed. Each field (the system prompt with the open file, and each message) now has hidden characters and no-break spaces dropped, is bounded to 150,000 characters (a cut field ends with `[truncated: N more characters not sent]`), has the exact key removed, emails replaced (`[email]`) and this account's own home folder replaced (`~`), the families redacted and a cap applied, in that order, in one function that the request and the preview both call; a suggested fix goes through it too, and the chat reply and a provider's error text are cleaned the same way (a reply that was changed carries a notice). Other people's folders are not guessed at (`/home/settings`, `/Users/list`, `https://app.test/home/list` are routes); this account's own folder is replaced wherever it appears: the folder `os.UserHomeDir` names (also with `/` for `\\` and with escaped slashes for JSON), and `/Users/you`, `/home/you` and `\\Users\\you` for the account's name, in any case, also behind a mount prefix (`/host_mnt/Users/you`, `/mnt/c/Users/you`, `/Volumes/Macintosh HD/Users/you`) and in a URL; only the folder part is replaced (`https://h/Users/you/x` is `https://h/~/x`), and a name that continues (`/home/users/42` for `/home/user`, in any script) is not replaced. The forms come from HOME and the login name (a Windows `DOMAIN\\name` gives `name`), and a HOME of fewer than two folders or four characters replaces nothing. The scan is one linear pass per form (about 0.1 s per MB of text, up to about 1 s per MB on text that is mostly matches). A zero-width non-joiner or joiner between two non-ASCII letters or symbols is kept (Persian, Indic, emoji) and dropped elsewhere. Full-width digits and typographic dashes are read as ASCII ones only inside a run that looks like a card number; other text is sent as it is. The open file is cut back to a space, a quote or a punctuation mark, so a key or token is not sent in part; a long line with none of those (one line of minified JSON, CJK text, a data URI over 64 KB) sends only the `[truncated]` marker. A provider error that spells a hidden character as `\\u200b`, `\\x{200b}`, `&#8203;` or `%E2%80%8B` (one or several in a row) is decoded before the drop. Only up to the newest 40 messages are sent, starting at one of yours; the preview and a notice with the reply say how many were left out. Changed on purpose: an email address in a chat message or the open file is now replaced before it is sent (it was sent as typed), a tab becomes a space, and a message over 150,000 characters is cut. A token split across two messages is not found. A reply is not decoded or folded before it is shown.
- **Composite and parallel runs ask each device whether it is physical, as a single-device run does.** Before, the Android devices of a `--device a,b` composite run and of a parallel run were never marked physical, and neither were the iOS devices of a parallel run (the iOS devices of a composite run already were). Now a physical device takes the physical branch of each step it has:
  - A physical Android device: `set location`, `travel`, `enroll biometric`, `biometric match` and `biometric no match` skip with a warning, as in a single-device run.
  - A physical iOS device in a parallel run: `set language` is now an error (`set language is not supported on physical iOS devices`), where it was a silent simulator command before. Clear app data, the permission steps (`allow`, `deny`, `grant all permissions`, `revoke all permissions` and `--grant`), `open link into the app` and `add media` skip with a warning. `restart the app`, `kill the app` and the launch use `devicectl` instead of `simctl`, and the relaunch after a permission change is not run.
  - The performance gate no longer applies the virtual-device default to a physical device in those runs, so frame metrics there are judged as they are in a single-device run.
  `set airplane mode` runs only on a device whose serial is `emulator-NNNN`, whatever the run marked it as.
- **Save as step finds a card number split by slashes in the address path.** `https://x/a/4111/1111/1111/1111/b` kept the card in the saved step because each path segment was read alone; the segments are now read as one text, and every segment a card run touches is replaced. A path of short numbers whose digits happen to pass the card check (such as a date) can be replaced too.
- **Studio's AI gate redacts a short JWT and a card number written with spaces or hyphens.** A JWT of three parts under 40 characters (the base64 rule starts at 40; the parts need at least 7, 4 and 0 characters, the first starting with `eyJ`, so `eyJab.cd.ef` is not caught) and a Luhn-valid card number of 13 to 19 digits written solid, as `4111 1111 1111 1111` or as `4111-1111-1111-1111`, no longer reach a prompt, a preview or a request; the Network tab already did this. A long number that passes the Luhn check by chance (about one in ten) is redacted too. Not covered: card numbers with other separators, or glued to letters or an underscore (`card_4111...`).
- **The MCP job-pruning test no longer depends on timing.** Its fake `--slow` run now lasts until the test releases it, instead of sleeping 30 seconds, so a slow or busy machine cannot let it finish and be pruned before the check. Test code only; no behaviour changed.
- **Docs: the Studio pages describe the Network tab, Send request and Explain failures, and the variables the browser suite reads (`PROBE_STUDIO_SEED_RUN`, `PROBE_STUDIO_SEED_EXCHANGES`, `PROBE_STUDIO_LOCAL_ENDPOINT`, `PROBE_STUDIO_TEST_SEND_RATE`, `PROBE_STUDIO_TEST_ALLOW_LOCAL`).** No code change.
- **Studio's exported `App` methods each need a recorded decision.** A test lists every method callable from the window and the headless bridge with a category, and fails for a new one without an entry, for a stale entry, for bindings that differ, and for any change to what the headless bridge blocks or what the automation endpoint can reach. No behaviour changed.
- **Studio's selector-fix request needs a preview (security).** `SuggestSelectorFix`, a bound method that sent the failed step, the last run, the open file and the device's widget tree to the AI provider in one call, with no preview and no gate, is gone. `PreviewSelectorFix` builds the request, sends nothing and returns it with an id; `SelectorFixPreviewed(id)` sends exactly that request, once, and only if the provider, model, endpoint, key, workspace, context switches and the failed step are unchanged since (a hosted destination needs a new preview for every request; a local one is confirmed once per session and again after a change). Every field is cleaned like an Explain field (hidden characters, the key, personal text, the credential gate), the screen is bounded, and the suggestion is cleaned before it is returned. No button uses it yet: the card on a failed step follows.
- **Studio's run tracker has direct tests.** The bounded step history, a clean start for every run, `finish`, snapshot copies, JSON lists that are never null and concurrent events under `-race` are now checked in `studio/runtrack_test.go`. No behaviour changed.
- **Save as step: a card number next to other digits, and a credential used as a JSON key, are no longer written as typed.** A fuzz test over random requests with a planted secret found `0 4111111111111111 0` and `{"<hex>":true}` saved unchanged. A card number is now searched for in every run of digit groups in any layout (4-4-4-4, 4-6-5, 4-4-4-1, 1-4-4-4-4 and the rest, separated by one or two spaces, hyphens, slashes, underscores or dots; there is no size limit: a card behind thousands of groups was once skipped) and the whole run that holds one is replaced, an object key that holds a run of 16 or more hex digits (with a letter among them) bounded by non-hex characters, a JWT or padded base64 stops the save (the whole key is read, and a key over 1,024 bytes stops the save as too long to check; a legitimate key with a 16-hex-digit run such as `id1700000000000abc` is refused too, where it was saved before), a generated placeholder name never carries a credential, and a step may replace at most 200 values. The fuzz test (`FuzzSaveStep`) runs its seed corpus with `go test`, so CI runs the seeds; exploratory fuzzing is not part of CI. Other separators, three or more characters between groups, and a card glued to letters are not covered.

### Fixed
- **A swipe over a scrollable did not move it, and a `drag` step did not drag.** The agent sent each move of a swipe or a drag with its position and no movement, and Flutter's drag recognizers take the movement from the event, so a swipe over a list, a page view or a dismissible row was recognized and moved nothing: the step passed, and the content stayed where it was. A `drag` step from a `Draggable` to a `DragTarget` was not accepted by the target, so its `onAccept` never ran; a drag over a `Slider` (a swipe or a `drag` step on it) did not move its value. Each move now carries its movement, so the moves add up to the swipe's or the drag's whole travel: a swipe up over a list scrolls it by the distance it travels, a horizontal swipe over a page view turns the page, a horizontal swipe over a dismissible row dismisses it, a `drag` step drops a `Draggable` on its `DragTarget`, and a drag over a slider moves its value. A swipe over something that does not scroll is unchanged: it still completes and releases where it did. What changes for existing tests: a swipe that used to do nothing now scrolls, turns or dismisses what is under it, so a test that relied on the content staying put (a `don't see` after a swipe, or a `see` of an item that the swipe would now reveal) may see different content; to keep the old behaviour, check before the swipe, or use `scroll` and `tap` steps for the navigation the test means. The wire format is unchanged. Measured on an Android emulator (API 35): a swipe up over 40 percent of a list about 830 logical pixels tall moved its offset from 0.0 to 332.0.
- **Studio connects to an iOS simulator with the token of the app the workspace names, and a simulator with two tokens is refused instead of guessed.** Studio read the agent token with no app id, so it took the first token file of any app container on the simulator, and when another app's token came first the agent answered HTTP 401 (`agent rejected token`). Studio now passes `project.app` from the workspace's `probe.yaml`; the CLI already did. With no app id, `ReadToken` counts the containers that hold a token and, when more than one does, returns `ErrAmbiguousToken` ("more than one app on this simulator has a Probe token (N): set app.id in probe.yaml ...") instead of the first match; one container is used as before, none is still "not found", and the physical-device fallback is not tried for a simulator that is ambiguous. With an app id nothing changes. The Studio fixture's `probe.yaml` named its app under a key (`app: id:`) that no setting reads; it now uses `project: app:`. The error never prints a token.
- **`type ... in webview`: a form control inside a content-editable element is refused, the refusal comes before the element is focused, and the template check is stricter.** Inside an editing host every descendant reports `isContentEditable`, so a checkbox, select or button there used to be treated as typeable; the tag and the input type are now tested first (a select, button, option, optgroup, meter, progress, output or fieldset is refused whatever is around it). A `type` step now does a first lookup that does not focus anything, decides from it, and only then runs the focusing lookup (and checks the element again, since the page can change in between), so a refused element is scrolled into view but never focused. The check that the lookup template only reads the page is now made on its tokens instead of a few text patterns: every name followed by `(` must be on the allow-list, an index must be a plain local name or number, nothing but a declared local may be assigned to or incremented, no `+` may touch a string or one of the template's own parameters, and a list of dangerous names and any character the tokenizer does not know is refused; over 30 ways of writing or building code, including the four that got through the first version, are tests that the check reports. The slice-3 proposal text about read-only inputs is corrected.
- **`type ... in webview` refuses an element that cannot take typed text, with the reason.** Before, the step only checked that the element held the focus afterwards, so a read-only input (which takes the focus and ignores `Input.insertText`) passed with nothing typed. The lookup now also reports the element's tag, the type of an input, and whether it is read-only, disabled (also through a disabled fieldset) or content-editable (the page's words are checked to be a plain tag or type name before they are used), and the step fails with "cannot be typed into (read-only)", "(disabled)", `(an input of type "checkbox")` or "(not a text field: select)" and types nothing. Text, search, email, url, tel, password and number inputs, textareas and content-editable elements are typed into. The lookup template changed, so its checks were extended: it only reads the page (a fixed list of calls, no property assignment), and nothing from the script is part of it. Checked once on a named Android emulator (API 35, WebView 124) with a page that has each kind of element: the four that take text were typed into and the text was found on the page afterwards, and the other seven were refused with their reason and nothing appeared. The documentation and the dictionary say so.
- **The diff image of `compare screenshot` is listed with the test's screenshots, and no path is listed for a file that was not written.** Before, a failed comparison named its diff image only in the error text, so the HTML report (which shows the test's image artifacts) never had it, and a comparison with no differing pixel listed a diff path for a file that does not exist. Now the diff image is added to the artifacts when a pixel differs, whether the comparison failed or passed under the threshold, and `visual.Comparator.Compare` leaves `DiffPath` empty when it wrote nothing (the failure message then says there is no diff). The visual regression page says so.
- **Report tags and properties: boundary tests, case-insensitive reserved keys, and plainer wording.** A `--property` key is now compared with the names the reports write themselves without regard to case (`Platform` and `RUN_LABEL` are refused like `platform` and `run_label`). New tests pin every bound (32 and 33 properties, a 64 and a 65 byte key, a 1024 and a 1025 byte value, 32 and 33 tags, a 64 and a 65 character tag), every reserved name in three cases, the key order of the properties in JUnit, JSON and the write cap, and masking of a tag on the HTML page. The documentation now says what masking covers: the secrets the run knows, and nothing else (a token, email address or card number typed into a value or a tag is written as it is); that the 1 KB limit is checked on the value as written, before masking; and that the report `--upload-report` sends to the hosted service carries no tags or properties yet. The doc comment of `Execute` is back above `Execute`.
- **Studio's device connections now ask whether the device is physical.** A connection made from Studio (a Wi-Fi iOS device or a listed device) built its device context without that answer, so a physical device was treated as an emulator or simulator by the steps that differ between the two. Both connections now use the same builder as `probe test`, and a test fails if any builder writes the context by hand.

- **Studio has a Suggest a fix button next to Explain failures.** One ProbeScript line from the AI provider for the failed step of the last run, on an advisory card. The window asks Studio for a preview first (`PreviewSelectorFix`), shows it when Studio says it must (a hosted or remote destination every time, one on this machine once per session and after a change of model, endpoint, key, workspace or settings) and only on **Send** asks Studio to send that preview (`SelectorFixPreviewed(id)`): Studio sends exactly what was shown and refuses when anything changed since. The card names the model and whether it is local or hosted, says in the CLI's `selector_proposal` wording that it is not applied or verified and does not change the result, and is set as text (markup in an answer stays text). A suggestion in flight is dropped by a new run or a change of workspace, settings or key, and a dialog is never shown for a dropped one. The seeded run of the browser suite may now carry the steps of the run.
- **Studio scrubs personal text from what a model answers (Explain and a selector fix).** An email address in an answer becomes `[email]` and the home-directory part of a path becomes `~`, as in what is sent, after hidden characters are dropped and the key and credentials are removed. An answer can repeat what it was sent, or invent a person's address or a home folder.

## [0.27.0] - 2026-11-11

### Added
- **`scripts/ci-local.sh` runs the CI checks locally, for example when hosted CI is unavailable.** `make ci-local` runs the five checks a pull request needs (Build, Go unit tests, probe-convert tests, Lint, Disclosure guard) with the commands of their workflow jobs, and `make ci-local-all` adds the Dart agent, Studio, VS Code extension and website checks. The run prints one line per check and a final block with the commit, the tool versions and a pass or fail line; `--report FILE` writes it as Markdown for a pull request. For a run of the checks the line says `PASS`, and the exit status is 0, only when all five ran and passed on a clean tree: a `--job` selection, a skip allowed with `--allow-skip`, a requested optional check skipped for a missing tool or a run with `GOFLAGS` set ends with `PARTIAL` and exit status 5; a run on a tree that is not the commit (a changed tracked file, an untracked file git does not ignore, a file hidden from `git status`, or a tracked file changed by a check) is marked `DIRTY TREE` and fails (`status.showUntrackedFiles`, submodule ignore settings and `GIT_*` variables do not hide anything; files that git ignores, such as through `.gitignore`, `.git/info/exclude` or `core.excludesFile`, do not count, by design); an interrupted run ends with `FAIL (interrupted)`. The Disclosure guard check reads the maintainers' pattern file from `PROBE_DISCLOSURE_DENYLIST` and reports `NOT RUN` without it, and also when the base already contains `HEAD`, so that there is nothing to scan. `--release-dry-run` builds the `probe` and `probe-mcp` release binaries into a temporary directory and lists their checksums; it publishes nothing, runs no checks and ends with `BUILT`, never `PASS`. A test fails when a workflow command and the script differ, or when a workflow step gains an `env:`, `if:` or `continue-on-error:`, or a job a `container:`, `strategy:`, `services:` or `timeout-minutes:`, the script does not know. For contributors; nothing changes for users of `probe`.
- **The chat header says where the next message would go.** It used to read "claude-sonnet-4-6 · BYOK" whatever the provider was. It now shows the provider and the model id, `on this machine` for a local endpoint on this machine, `remote endpoint` for a confirmed one, `Anthropic, key needed` when Anthropic is chosen and no key is saved, or that AI is off or needs a fix in Settings. It is filled when the chat pane opens and follows a Settings save, a saved or removed key and a workspace change while the pane is open, so a hidden pane never reads the keychain. The model id comes from `probe.yaml`, so it passes the same redaction gate and 200-byte cut as every provider string, and control, bidirectional, zero-width and separator characters and commas in it are shown as `?`. The header never shows an address, a key or a path.
- **`probe test --ai-heal` proposes a replacement selector for a failed step, in the JSON report.** When a `tap`, `type`, `see` or other step runs into its timeout on a text or key selector, the configured `ai:` model is asked for one replacement, and the failed result in the JSON report gets a `selector_proposal` object: the failed selector, the proposed one, the model, the provider, whether the model is `local`, on the `network` or `hosted`, and `applied: false` and `verified: false`. It is advice. The step stays failed, the test file is not touched, and the results and the exit code are the same when the model answers, fails or times out. A proposal is kept only when it is exactly one of the visible texts (as `"text"`) or exactly one of the visible keys (as `#key`) that the runner recorded when a step ran into its timeout. The lists are recorded as data, never read back out of a failure text: widget text can look like a list, and the agent's own "Widget not found" message, which also lists what was on screen, is not used for this. So a failure other than a step timeout gets no request and an entry with an `unavailable` reason, and a text longer than 40 characters is never proposed. A run makes at most 3 requests, at most one per failed step, each bounded by `ai.timeout`, and none after a failed request. Control characters in model text are replaced by spaces before it is printed or written to the report. With no `ai:` block the run makes no request. The model gets the failure text with resolved environment values masked, which is what `--ai-triage` sends. Proposals are not included in an uploaded report. The field is absent when there is no proposal, so existing report readers are unaffected.
- **`probe-mcp` can serve its tools over HTTP on your own machine.** `probe-mcp --http 127.0.0.1:7391` starts the MCP "Streamable HTTP" transport at `/mcp`, for clients that connect to a running server, such as an agent in VS Code. It is off by default and the stdio server is unchanged. The tools and their results are the same. Every request needs `Authorization: Bearer <token>`. A new 256-bit token is made on each start and written to `http-token` in `flutter-probe-mcp` under the user config directory, readable only by you, and removed on stop; the path is printed, the token never is. Set `PROBE_MCP_HTTP_TOKEN` (43 characters or more, at least 16 different ones, for example from `openssl rand -base64 32`) for a token that stays the same; the variable is removed from the server's environment at start, so the commands it runs do not inherit it. The server refuses an address that is not loopback unless `--dangerously-allow-non-loopback` is given; it has no TLS. It refuses requests from web pages (any `Origin` header, and any `Sec-Fetch-Site` value other than `none`), a `Host` that does not name the listening address, a body that is not JSON or is over 4 MiB, and more than 8 requests at a time. `probe-mcp --help` now prints its options.
- **Studio's AI chat can use a local model.** When `probe.yaml` has an `ai:` block with `provider: local`, the chat sends each turn to that OpenAI-compatible endpoint (Ollama, LM Studio, llama.cpp, or another server) with no key. A local endpoint must be on this machine. Another address is refused unless `studio.ai_allow_remote_endpoint: true` is set in `probe.yaml`. A key must be an environment variable named as `${NAME}`: a plain key in `probe.yaml` is not used (with a saved Anthropic key, the reply uses that key and says so), and the key is read when a request is sent. With no `ai:` block and no saved Anthropic key, AI stays off and nothing is sent. The same block lists the local endpoint's models. The Settings tab for these options comes in a separate change.
- **Studio runs part of a file.** Next to **Run**, a scope control picks the whole file (the default), the test at the cursor, the tests with one tag, or the tests that failed the last time the file ran in this session. Results group failures first when a run has more than one outcome. Automation's `run_file` takes `line`, `tag` or `failedOnly`, and the MCP tool `studio_run_file` takes `line`, `tag` or `failed_only`; give at most one; a line of 0 or an empty tag is refused rather than run as the whole file. The file must be inside an open workspace or an automation folder, the same check Run File makes. A scope that matches no test is an error and nothing runs. Two tests that share a name refuse a scope that names one of them. The failed tests are kept in memory, so a restart starts with none. The CLI has no per-test filter yet, so `probe test` is unchanged.
- **Failed selector steps get a suggested fix and a reason.** A failed `see` or `tap` on a text or an id shows, under the step in Results and in `studio_run_state`, a rule-based suggestion: for a text, the `wait until "<text>" appears` line to try before the step, plus the reason and the time the step ran. The rules run without AI and send nothing over the network. Steps that are not `see` or `tap` are not covered.
- **Settings has an AI provider section.** It sets the provider (Off writes `provider: none`, which also overrides a saved key), endpoint, model, key source (the keychain, or the name of an environment variable), vision, timeout and the remote-endpoint confirmation, and writes them into the `ai:` block of `probe.yaml`. The other keys are kept, and so is a plain `api_key` already in the file, because the CLI reads it too: Studio does not use a plain key and does not change it, and the section says so. Naming an environment variable there replaces it. A key is never typed or written in Settings: the key button in the AI panel stores it in the keychain. A form it can tell would not run is refused, such as OpenAI with the keychain as the key source, but only when the AI section was changed: an unchanged section is never refused or written, so a setup the CLI accepts (OpenAI with a plain key, say) cannot stop the rest of Settings from saving, and a refused section shows its reason and the other settings still save. A plain key already in the file is shown as its own key source, "In probe.yaml". **Test connection** runs the `probe ai doctor` checks against the saved settings, with a fixed prompt and a fixed image and never file text, and what it reports is cleaned of the key before it is shown, and every model id the list shows is cleaned and cut to 200 bytes. A new `probe.yaml` is created readable by the user only. Before a chat message goes to a destination you have not confirmed in this window, Studio shows what that message would send now, after redaction, and waits for **Send**. It asks again when the destination changes, and after the workspace, the provider settings or the saved key change. The check is in the Studio window only: the chat and the selector fix send without it through other callers, such as the headless bridge. The request builds its own turn when it runs, so a change after the preview, such as a new run result, can make it differ from what was shown. The regenerated Wails bindings also pick up `ListAIModels`, `SuggestSelectorFix`, `SelectorFix`, `StepTriage` and `ChatResponse.notice` from earlier changes.
- **Settings has a Find local models button.** It asks this machine's own model server, such as Ollama, which models it has, only on your click and only on a loopback address: the endpoint in the form when it passes the same check as every local endpoint, and `127.0.0.1:11434`. It sends a read-only request for the model list, with no key and no redirect followed, 2 seconds per address, and sets every id as text after the same redaction gate and 200-byte cut as every provider string, cleaned of control characters and capped at 100. It fills only empty fields and says what it filled, picks a model only when exactly one is listed, and saves nothing until you press Save. The headless build reads `PROBE_STUDIO_LOCAL_ENDPOINT` for its browser tests, and ignores it, with a note on stderr, unless it passes the same loopback check.
- **The chat header keeps the place in view when the model id is long, and hides more invisible characters.** A model id of any length is shortened with an ellipsis, the provider or the place after it (`remote endpoint`, `on this machine`) stays on screen, and the whole line is the header's title. Braille blank, Hangul fillers, variation selectors, tag characters, the soft hyphen and the other blank-looking or hidden characters in a model id are shown as `?`, after the redaction gate. The rule is now one for every code point: anything that is not a letter, mark, number, punctuation, symbol or space is shown as `?` too (controls, format characters, private-use, unassigned, line and paragraph separators), as is anything Unicode marks default-ignorable or a variation selector, and a few named glyphless characters.
- **Studio can explain the failed tests of the last run (backend only, no button yet).** `PreviewExplain` returns what an explanation would send and where, and `ExplainLastRun` asks the provider for short advice on each failed test, using the same `ai.Triage` as `probe triage` and the MCP tool `triage_failure`. It sends at most 5 failures, one request each, with a 45 second bound and no retries, stops after the first provider error, and never changes the run. The test name, the file and the error text each take one path: hidden characters are dropped first (anything that is not a letter, mark, number, punctuation, symbol or space, anything Unicode marks default-ignorable or a variation selector, and the blank-looking or glyphless ones found so far, by name; the rule the chat header uses, and a space other than the ASCII space becomes a space), then a text over 60,000 characters is cut with its unfinished last word and 200 more characters dropped, then the exact key is removed, email addresses and home folders are replaced, the redaction gate runs once and the text is cut. A dropped character in the middle of a token, the key, an address or a path therefore cannot get it past a step, and a token cut in two at the 60,000 limit does not reach the provider, except a key with spaces in it that is longer than about 200 characters and starts a few hundred characters before the limit, behind text the gate shrinks a great deal. Other glyphless joiners and fillers that Unicode keeps as graphic characters and that are not named can still split a token. A failure with no error text is left out and a repeated failure is sent once. A file is sent relative to the workspace, and no widget tree or screenshot is sent. Why it stopped is one fixed sentence, never the provider's own message, and the advice it returns is cleaned the same way before it is shown. With AI off, nothing is sent.

### Changed
- **Tests prove that the error-text filter runs on a verdict's reasoning and on the doctor's model ids and sample, and the filter's documentation says it must not panic.** No user-visible change.
- **Browser specs cover the Settings AI section.** Against a fake provider on 127.0.0.1, they check Test connection with AI off and with a local model, List models, the plain-key note and that Save keeps the key, that Save writes the form and the form reloads, that Off writes `provider: none`, and that a refused form shows its reason. No user-visible change.
- **`probe-mcp` refuses an option it does not know.** An argument that starts with a dash and is not `--http`, `--dangerously-allow-non-loopback` or `--help` (a mistyped `--htpp`, say) now ends with exit code 2 and a usage line. Before, it was ignored and the stdio server started. `probe-mcp` with no arguments, or with plain words, starts the stdio server as before. If your MCP client configuration passes a dash-prefixed argument to `probe-mcp`, remove it.
- **A test now proves that Studio's AI makes no request when it is off or refused.** A counting transport replaces the default one and records every request, and the chat, the selector fix and the model list are run in nine configurations. No user-visible change.
- **`ai.provider: openai` in `probe.yaml` now sends the open file to OpenAI with each message in Studio's chat.** Before this change, Studio's chat used only the Anthropic key, so this switch had no effect there. Set `provider: none` to keep Studio's AI off.
- **Studio's chat reads only the `ai:` and `studio:` sections of `probe.yaml`.** An error in an unrelated key, such as `agent.port`, no longer blocks the chat.
- **`provider: none` turns Studio's AI off,** even when an Anthropic key is saved.
- **Studio's redaction covers the Anthropic path and the selector fix, and more credential families.** The new families are AWS, Slack, Google, Stripe and GitHub fine-grained tokens, PEM private keys, `Authorization: Basic` values, assignments named secret, token, password or key, and secret query parameters. Emails and file paths are not redacted.
- **Studio's chat redacts credentials in the open file and in the conversation,** as it already did for the run results and the widget tree. Before this change, the open file was sent as written.
- **The system-dialog language claims say what is covered.** Permission buttons (allow, deny, allow once, allow while using) are matched by role in the 55 iOS 26.5 locales, and on Android for dialogs that use the stock permission controller ids. OK, cancel, not now, open and close are English only. The iOS label table writes its no-break spaces and zero-width non-joiners as escapes (the same bytes). Tests cover the ATT alert in either order and the invisible-character locales.
- **The embedded license key list has a test.** The test checks each embedded key's key id and Ed25519 public key, and that no key id is listed twice. It passes on the empty list in this release. A license signed with the matching private key verifies once its key is embedded, and one signed with another key is refused. Embedding the production public key before a client release lets customers upload reports is a manual release checklist item; no test enforces it. Verification itself is unchanged.
- **The AI helpers make no HTTP request when no `ai:` block is set.** A new test counts every request the AI code makes and fails if a helper that a test run or `probe ai doctor` reaches sends one with no AI configuration.
- **Docs describe the Studio AI surfaces and their tests.** The Studio and e2e-web docs cover the AI chat preview, the AI settings and the chat header tests, the headless keychain seam and `provider: none`; the `generate_test` tool description now says it follows the `ai:` block. No behaviour change.

### Removed
- **Studio: two leftovers nothing used.** The `discoveryContext` helper in `studio/wifi_discovery.go` was never called (WiFi discovery builds its own context), and `studio/frontend/src/app.css` held Wails starter-template styles that nothing imported. No behavior changes. The third item in the issue, a commented-out `replace` in `studio/go.mod`, was already removed in #477. (#401)

### Fixed
- **The agent reports its version at handshake, and the same version as its package.** It reported 0.26.0 in 0.26.1, so the CLI printed a version mismatch even when both were the same release. A test now fails when the agent's reported version and `probe_agent/pubspec.yaml` differ.
- **Studio gates a provider's error text before `internal/ai` cuts it, on the OpenAI-compatible and local paths.** `internal/ai` cuts every provider string to 200 bytes, and Studio's redaction ran after the cut, so a token that started near byte 190 could leave its first characters, such as `ghp_` and six more. `internal/ai` now has a documented hook, `SetErrorTextFilter`, that runs after the exact key is removed and before the cut. Studio installs its credential gate there at start. The command line installs nothing, so its error text is cut and nothing else, as before.
- **Studio's Anthropic chat gates a provider's error text, then cuts it to 200 bytes.** The cut used to run first, so a key that started near byte 190 could leave `sk-` and a few characters of itself. The text is now redacted before it is cut, for the body of a non-200 answer and for the message of an error object in a 200 answer.
- **`Generator.Complete` returns an error when there is neither an API key nor a completer.** Before this change, a direct call with no key sent the request anyway, to the hosted endpoint with an empty key header. No `probe` command reached that path, and the other entry points already refused it.
- **Studio's Anthropic chat cuts the message of an error object in a 200 answer to 200 bytes.** It already cut the body of a non-200 answer, so a provider error message now keeps only the first 200 bytes on every Studio chat path. Studio's redaction gate still runs over it; an exact key of a shape the gate does not know is not removed from this text.
- **Provider text in a CLI failure is cut, and the client's own key is removed from it.** In `probe generate`, the `with ai` screen steps, the text-only mode and `probe ai doctor`, text that comes from the provider and is shown in a failure or in the doctor report shows at most its first 200 bytes per string, after every exact occurrence of the API key sent in that request has been removed. The strings are the body of a non-200 answer, the type and message of an error object, an answer that does not parse as a verdict, the reasoning of a verdict (so a long reasoning in a failed `with ai` step is shortened), and, in the doctor, the model ids it lists and its sample of the model's answer. This hides an exact echo of the key only: a key that comes back masked, encoded or split is not found, the first 200 bytes of an echoed prompt still appear, and other secrets in that text are not touched. Text a provider returns as the result of a successful call, such as a generated test or an extracted value, is not changed. Studio's chat is unchanged: it redacts error text through its own gate.
- **Provider requests do not follow redirects, in Studio and in the CLI.** An endpoint that redirects is an error, so a prompt and any key are never sent to a second host. Provider error messages keep only the first 200 bytes of the reply, and Studio redacts them.
- **`dismiss system dialog` and button matching know the iOS system alerts in the 55 locales of the iOS 26.5 runtime.** On iOS, `dismiss system dialog` and `probe system-dialog dismiss` now tap the decline button of the App Tracking Transparency alert ("Ask App Not to Track" in English, with its translation in each of those 55 locales). `tap "Don't Allow"` on that alert taps the same decline button. `allow permission "notifications"` and `deny permission "notifications"` no longer answer the tracking alert: they skip it and wait for the notification alert. The standard permission alerts and the location alert (allow once, allow while using app) are recognised the same way. The labels are the system's own localization strings in that runtime; 27 of those languages were also measured on a device, and the table agrees with the measurements. On Android, "No, thanks" counts as a cancel button. The iOS driver's own English fallback list has the same ATT label; that part reaches users with the next `probe-ios-driver.zip`.
- **A provider's answer is read up to 1 MB.** The OpenAI-compatible, Claude, vision and doctor clients, Studio's Anthropic chat and Settings' model list read the whole answer into memory, so a server that streamed without end could fill it (a 300 MB stream took about 1.2 GB of heap within the time limit). An answer over 1 MB is now an error that names the limit and repeats none of the answer. Normal answers are unchanged.

## [0.26.1] - 2026-10-26

### Security
- Secret text is masked more thoroughly in warnings, run output and recordings. See the repository security advisories for details.

### Added
- **MCP: `start_tests`, `test_status`, `test_result` and `cancel_tests` run tests in the background.** `run_tests` blocks until `probe test` exits, so a multi-device run (`--parallel`, `composite_devices`, several devices) could outlast the MCP client's tool timeout and be reported as failed while it kept running. `start_tests` takes the same arguments, returns a `job_id` at once and runs the command in its own process group; `test_status` returns the state, elapsed time and the last lines of output; `test_result` returns the retained output (the last 1 MB) and the exit code; `cancel_tests` sends SIGTERM to the run and kills it after 5 seconds. Jobs live in the `probe-mcp` process and are killed when it exits. The hosted-upload flags are refused here exactly as in `run_tests`. `run_tests` is unchanged. The MCP server now has 40 tools.
- **`probe lint` checks the references a run would resolve.** Lint reports each step and recipe call that matches no built-in step and no recipe in the file, its `use` statements or the recipes folder (the check `probe test --dry-run` makes), each selector that can never match, and each `<name>` placeholder that no example column, recipe parameter or stored value defines. It runs without a device, gives the line of each finding, and checks hook bodies and the device blocks of composite tests too.
- **Recordings show the element each step acted on.** A step that taps, double taps, long presses, types, clears, toggles, or swipes or scrolls with a selector records its selector, and its bounds and tap point when the element is on screen as the step starts. The viewer shows them under "Element". Text typed into a field is never recorded, and a failed lookup is an advisory warning that does not change the step.
- **`probe test --record` writes a step-by-step recording of each test.** Each test attempt includes source files and lines, screenshots before and after steps, widget trees after steps, and the app's `dart:io` HTTP exchanges with credential redaction, plus an offline viewer. Bundles stay under `reports_folder/recordings` or `--record-dir`; nothing uploads, and recording warnings do not change test results or exit codes. Native SDK and WebView traffic and screenshot redaction rules are not yet recorded; screenshots are not redacted and widget trees are not captured before steps. Keep recordings out of version control because they can contain screens and request or response bodies.
- **Recording manifests name the platform and OS version.** `manifest.json` in each recording bundle now carries the run's `platform` (`ios` or `android`) and `os_version`, the same values the report shows, instead of leaving both empty.
- **Studio's AI chat can send the last run and the device's widget tree, but only when the workspace turns that on (off by default).** Workspace settings have an AI chat context section with two switches, stored in `probe.yaml` under `studio.ai_context`. When either is on, a line above the chat input says what is sent. The widget tree is redacted first: obscured fields and credential-looking text are removed, and each part is cut to a size limit. A new backend call, `SuggestSelectorFix`, asks the model for one ProbeScript line that replaces the last failed step. Nothing is applied, and the pane button is a follow-up.
- **Probe Cloud (preview): local runs, `probe test --upload-local`.** A run from a local simulator, emulator or device can be sent as a local run. It is opt-in for each run and never implied by `--upload-report`. The envelope carries the repository (GitHub remotes only), the branch or `HEAD`, the commit, a dirty flag, and a tree hash of the state that was tested: `HEAD^{tree}` for a clean tree, and the tracked and untracked, non-ignored files for a dirty one. It also carries a random per-installation signer and a sequence number that is never reused for a signer. No device serial, UDID, name, e-mail address or path is sent. The service keeps local runs as unverified evidence, shown beside the CI result; they never count toward a pull request's verdict. The team must enable local uploads and register or allow the repository. Limits are stricter than for CI: 256 KB and 2,000 results per upload. A retry resends the same bytes. `--upload-dry-run` sends nothing and reserves no sequence number. The hosted service is not live yet, so a real local upload gives a warning today. There is no signing yet.
- **Studio's ProbeScript editor offers completions, hover help and snippets** for every command in the ProbeScript dictionary and every reserved word in the grammar. Both come from the docs through a generator (`studio/frontend/scripts/gen-probescript-dictionary.mjs`), so the editor and the documentation do not drift apart. `npm run check:probescript` fails when the generated module is out of date.
- **VS Code: tests in the native Testing view.** Each `test "<name>"` block in a `.probe` file appears in VS Code's Testing view, with a run button next to it and in the editor gutter. A run executes the file with `probe test --yes --format json` and reports each selected test's pass or fail from the CLI's report. The CLI has no per-test filter yet, so a run executes the whole file. The Tests sidebar is unchanged.
- **Studio: a Settings panel in the device pane header** changes dark mode, font size, app language, orientation (Android) and opens a deep link on the connected emulator or simulator, through the same code as `probe device set`. It acts only on the connected device and says why it is unavailable for WiFi connections and physical iPhones. Browser tests 26, 27 and 37 cover it (37 on a named simulator, reading each setting back).
- **`probe device set` and the `set_device_setting` MCP tool change an OS setting of one named emulator or simulator**: appearance (dark mode), font scale, app language, orientation (Android) and opening a deep link, through `adb` and `simctl`. The device must be named (a serial, UDID or unique name, never `booted`), every value is checked before a command runs, and physical iOS devices are refused. Checked against an Android 15 emulator and an iOS simulator. The MCP server now has 36 tools.

### Changed
- **The disclosure guard reports positions only, and can check a path tree.** A match is reported by rule number and position: a changed file's number, the line, or a tree file's number. The matched text, the file path and the pattern are never printed. `scripts/disclosure-scan.sh` takes `--tree PATH` to check every file tracked under a path at the end of the range. A deleted file is not matched. Binary files, and file names that git quotes, are scanned too.
- **The MCP tools keep to the local tool surface.** `run_tests` and `run_script` refuse the upload flags (`--upload-report`, `--upload-local`, `--upload-dry-run`, and any other `--upload-` flag) with the message "hosted upload is not available through MCP". The MCP tool text no longer describes the upload flags. Use the CLI from a terminal for uploads.
- **Building from source needs Go 1.26.10 or newer (was 1.26.1).** The `go` line in `go.mod`, `studio/go.mod`, `tools/probe-convert/go.mod` and `go.work` is now 1.26.10, and the install docs say so. With the default `GOTOOLCHAIN=auto`, an older Go 1.26 downloads 1.26.10 on its own; with `GOTOOLCHAIN=local` the build stops with a version error until Go is upgraded.
- **Older evidence files removed.** The disclosure guard also scans `docs/evidence`.
- **`probe lint` fails on references a run cannot resolve.** A step or recipe call that resolves to nothing, or a selector that can never match, now makes the command exit with status 1, where it used to pass. A placeholder no value defines and a `use` file that does not exist are warnings and do not change the exit status. Lint also checks hook bodies and the device blocks of composite tests. `probe test --dry-run` checks only test bodies, so a bad step in those places fails at run time today, and a repository whose CI runs `probe lint` may see new failures there. Environment variables (`${NAME}`) are not checked, and the other limits are listed in the CLI reference. Within a file, errors print before warnings. Example output for a file with an error (colour codes omitted; the last line goes to stderr):

  ```text
    ✗  x.probe
       line 7: unknown step "send reset code": not a built-in step and no recipe with that name is defined (check the use statements and the recipes folder)
       line 8: <code> is not defined: no example column, recipe parameter or stored value has that name

  Error: 1 file(s) have errors
  ```

  A file with only warnings, such as a placeholder no value defines or a missing `use` file, prints them under a warning mark and still exits with status 0. The run then ends with `All files valid.`:

  ```text
    ⚠  y.probe
       line 3: <code> is not defined: no example column, recipe parameter or stored value has that name

    All files valid.
  ```
- **Recording manifests list what is redacted and what is not.** `manifest.json` replaces the `redaction` boolean with `redacted` and `not_redacted` lists, and the bundle schema is now version 2. Each entry says what its rule covers, and the `not_redacted` list names the kinds of content the rules do not reach, such as screenshots, free text, unusual encodings and values the run did not resolve from the environment. The viewer says that recordings may contain on-screen data, so review them before sharing. The recording feature is not released yet, so no released reader is affected.
- **`probe record` refuses an agent whose version does not match the CLI**, as `probe test` already does.
- **Recording redaction covers credential names by their words.** A name is treated as a credential when one of its words (split at separators, digits and camel-case boundaries) names a secret, token, password, key, card or session value, or when it is a known compound spelling. The value is replaced whatever its type, in JSON, forms, XML, multipart parts, URLs (query, fragment, path parameters and userinfo), text bodies and headers, and the agent masks custom credential headers by the same rule before it sends them. Resolved environment values are masked in their JSON-escaped and URL-encoded forms. A name that contains a credential word is masked whatever else it says, counters included. This is a stated trade-off.

### Docs
- **Recording evidence write-ups no longer name private apps.** The prose and commands in the evidence notes use neutral labels and placeholders. The raw run output that the notes cite is unchanged.
- **Run recording groundwork, internal only.** The CLI's new `internal/record` package can write a recording bundle, follow the agent's HTTP log without skipping requests still in flight, redact credentials from headers, URLs and bodies, and render a bundle as an offline viewer. Step events now carry their wall-clock time. The CLI now exposes this through `probe test --record`; the design is in `docs/prd/run-recording.md`.

### Fixed
- **Studio: the frontend builds with `bundler` module resolution.** TypeScript 7 removed `moduleResolution: "Node"`, so the Studio build failed on the TypeScript 7 and Monaco 0.57 upgrades. The Studio tsconfig now uses `bundler`.
- **CI: the website checks and the docs deploy run on Node 22.23.4.** Astro 7.4.2 needs Node 22.23.4 or newer, and the runner cache held 22.23.3. The Dependabot compatibility jobs and `docs.yml` now pin 22.23.4.
- **VS Code: the extension compiles again with TypeScript 7 and @types/node 26.** The tsconfig names the Node types, which TypeScript 7 no longer loads by default. Without it, `npm test` failed to compile after those two upgrades.
- **CI: the Dependabot auto-merge job adds approved pull requests to the merge queue again.** It passed `--squash` with `--auto`, so the queue ignored the request, nothing was queued, and the run still reported success. The job now passes no merge method and logs a warning for each pull request it cannot queue.
- **CI: the Studio Go tests and the frontend and VS Code test suites run on every change to `studio/` or `vscode/`.** They ran in no workflow before, and only the browser suite covered Studio. The frontend suites run on Node 26.
- **probe-convert writes Robot keyword recipes in lower case, and its calls match them.** The recipe kept its Robot spelling (`Login As User`), while the lexer writes every call word in lower case, so no converted call matched its recipe. The checkout golden is regenerated, and a test checks the names. A recipe name that keeps capital letters still cannot be called from a test; that is tracked separately.
- **CI: the CLI test package runs in seconds, not minutes.** The hosted-upload failure test waited out the 5-minute retry window; it now uses a short window and checks that the failure is still only a warning.
- **`probe test --device` finds the Android application ID for a camelCase `project.app`.** When the bundle ID as written is not installed on an Android device and its snake_case form is (`com.example.myApp` → `com.example.my_app`), the single-device run uses the snake_case ID, as `--devices` and parallel runs already did. Before, the restart relaunched the ID as written and failed. A run keeps the ID as written when that ID is installed, and when neither form is, so a project that runs today is unchanged. `--devices` and parallel runs still convert without this check.
- **Composite runs write the platform and OS version into each recording manifest.** `probe test --composite-device` now passes each device's platform and OS version to its recorder, as single-device runs already do, so `manifest.json` is no longer empty for those devices. Devices the CLI cannot read a platform for (a WiFi composite device) still write empty values. The recording ships with `probe test --record`, which is not released yet.
- **The recording viewer fits a 390 px phone screen.** Long step names, URLs and error text no longer push the page sideways: the single-column layout shrinks to the screen, long words wrap, and the 16 px side gutters stay. The desktop layout is unchanged. The viewer ships with `probe test --record`, which is not released yet.
- **VS Code: the Run Test commands no longer pass `--name` to `probe test`.** This version of the CLI defines no such flag, so those runs failed with an unknown-flag error. "FlutterProbe: Run Test File at Cursor", the CodeLens buttons and the Tests view now run the file that contains the test, and their titles and tooltips say so. Until the CLI has a per-test filter, the extension runs whole files.
- **Studio: the top bar is a window drag handle on macOS.** It used `-webkit-app-region`, which Wails does not read, so the full-size content view had no drag handle. It now sets `--wails-draggable: drag`; its buttons and device picker opt out so their clicks still work.
- **Studio: the macOS window has its native title bar again.** The title bar was transparent, and Wails then makes the web view cover it, so the window had no native drag area and some toolbar controls did not show. The title bar is now opaque and sits above the top bar, so the window can be dragged from it and every control shows.
- **Dev tooling: `@azure/msal-node` is pinned to 7.1.0 in the VS Code extension's development tree, which closes GHSA-xcx6-jx8p-x3h5.** The package reaches that tree through `@azure/identity`, and the packaged extension does not include it.
- **Dev tooling: the Studio frontend's npm tree resolves `dompurify` 3.4.17.** `monaco-editor` pins `dompurify` exactly to 3.4.15, so an override sets the npm package to 3.4.17. The editor bundles its own copy of DOMPurify, which is unchanged at 3.4.15, so the code Studio ships does not change. The advisories need options that the bundled sanitizer does not set.
- **Output masking covers more paths.** Resolved values are masked before a timeout hint is cut for display, and before a `with ai` assertion or a `read ... with ai` query is sent to the vision provider.
- **Short environment values are masked only when they are credential-shaped.** A value of eight characters or more is always masked. A shorter value is masked only when its variable's name has a secret word as a whole token: `PASS`, `PASSWORD`, `PASSWD`, `PWD`, `PW`, `SECRET`, `SECRETS`, `TOKEN`, `TOKENS`, `OTP`, `CRED`, `CREDENTIAL`, `CREDENTIALS`, `PIN`, `KEY`, `PASSPHRASE`, `PASSCODE`, `CSRF`, `SID`, `SIGNING` or `ENCRYPTION`. It is then masked from three characters, and a plain word such as `true` is never masked under eight characters. So `API_KEY=abc` and `PIN=1234` are masked, while `SPINNER`, `MAPPING`, `BYPASS_MODE` and `MONKEY_COUNT` are not, because the secret-looking part is not a whole token. A short value under any other name is printed as written, so `${COUNT}` set to `1` no longer hides that digit in unrelated output. A short password under an ordinary name, such as a six-character `DB_LOGIN`, is not masked; name the variable with one of those words to mask it. Values typed into a system dialog from an environment variable, and the sandbox sign-in credentials, follow the same rule.
- **A password field stays secret when the app swaps it for a plain field under the same key.** The agent remembers the key of a field that was seen obscured when the key is a `ValueKey` on a text field, so a field the app rebuilds under that key is not listed or read as text. Fields without such a key are tracked per controller only, and the agent keeps a bounded number of keys; past that bound every keyed field counts as secret. The recording's password-field map holds its fields weakly and starts fresh for each recording.
- **Connection error text is checked before it is printed.** HTTP connection errors and relay connect errors no longer include the agent's address query.
- **The recording agent no longer errors on a text field that leaves the tree or is rebuilt.** A change to a field that had left the tree was judged on its missing widget, which raised an error for each change, and a rebuilt field stayed bound to its old element, so its later text was lost. Text still pending in a field that leaves the tree is now recorded as obscured, with no value, and a rebuilt field is recorded through its new element. A pending text timer is also cancelled when recording stops, so it cannot emit into a later recording. Text typed into an ordinary field that leaves the tree before its 300 ms debounce fires is recorded as a `type` event with an obscured, empty value, because the field can no longer be read; this is a documented trade-off. The recording ships with `probe test --record`, which is not released yet.

### Security
- **Studio no longer returns the Anthropic API key to the window.** The key panel shows whether a key is saved and starts with an empty field. Saving an empty field no longer clears a saved key; use Remove key to delete it.
- **Studio keeps each WiFi token in the OS keychain, under its device, and never returns it to the window.** A device with a saved token shows a 🔑 saved tag, and connecting to it needs no paste. Its ✕ control forgets the token. Studio deletes the plain-text copy that an earlier version kept in local storage on first start.
- **Studio's Run File accepts only `.probe` files inside the open workspace or an automation folder.** Other paths are refused before anything runs, as the automation endpoint already refused them.
- **Studio opens only `.probe` files inside the open workspace or an automation folder.** The editor's open action uses the same check as Run File, and a path with `..` is refused.
- **Studio saves `.probe` files and lists folders only inside the open workspace or an automation folder.** Save and the file tree use the same rule as Read File and Run File, including the `..` and symlink checks. With no workspace open, the file tree is empty.

## [0.26.0] - 2026-10-14

### Added
- **Studio's device pane header says what the device view can do**: "view only" when the agent has no input calls (older `flutter_probe_agent`) and "no element select" when it cannot hit-test, with the reason in the tooltip. Before, a read-only view looked like a broken one.
- **Studio: "+ tap" in the device pane header writes a `tap` step for the element selected with Alt+click at the editor cursor** (shared device view, preview). The step uses the same escaped selector as the clipboard copy; a selection with control characters is refused. Browser test 36 covers it.
- **`probe lint` warns about count and zero-state text checks** (warning only; the exit status is unchanged). A `see` or `wait until ... appears` on text such as "0 items", "3 items" or "No items", with no `matching` pattern, also passes on longer text that contains it ("10 items"). The warning suggests the anchored form, `matching "^0 items$"`; a `matching` pattern anchored at both ends (`^...$`) silences it, and any other pattern is still flagged. The text-matching docs and the CLI reference say the same.
- **`probe doctor` checks that a first run can work** and says how to fix what is missing. It looks at `probe.yaml`, the `tests/` folder, the Flutter SDK, the platform tools (`adb` for Android, `xcrun` for iOS) and the devices they can see. It changes nothing, and it exits non-zero only when `probe.yaml` or the `tests/` folder is missing; warnings alone do not fail it.
- **`probe doctor --format json`** prints the same checks as a JSON report (schema `probe.doctor/1`, with `ok` and each check's `fix`), and the **`doctor` MCP tool** returns it, so an AI agent can check a machine without running tests. A required check that fails is a normal result, not an MCP error. The tool count is 35 in the development version.
- **Studio can show the shared device view** (preview, off by default; set `PROBE_STUDIO_DEVICE_VIEW=1`). Studio starts a loopback hub for the connected device in process (`internal/deviceview`) and the device pane draws it with the shared component: tap, swipe, typing, hover outline and Alt+click selection (the selector goes to the clipboard and the inspector search; text with control characters is refused). The old capture loop stays the default and is not started when the view is on. In Studio's browser test mode the hub also accepts the test page's own origin (`deviceview.Options.ExtraOrigins`, exact matches only). Browser test 36 covers it; tests 10 and 11 still pass with the flag off.
- **`probe device view`: an interactive device view over a local WebSocket** (preview, Flutter layer only, from the on-device agent). The command starts a hub on `127.0.0.1` with a one-time token, prints one JSON line (`url`, `token`, `device`) and streams frames to one client at a time; the client can tap, long press, swipe, type and ask which widget is under a point (`docs/studio-device-protocol.md`). The hub checks the token and the `Host` and `Origin` headers, limits message size and rate, keeps only the latest frame per client and writes no files. An agent without the new methods keeps working as a view-only mirror.
- **Agent methods for the device view** (`probe_agent`): `probe.tap_at`, `probe.long_press_at`, `probe.swipe_at`, `probe.type_text`, `probe.hit_test`, `probe.dump_tree` with `bounds:true` (nodes with logical-pixel bounds), and `scale` on `probe.screenshot` (the reply also carries `width`, `height`, `logicalWidth`, `logicalHeight` and `dpr`). All are additive; an older CLI never sends them. JPEG is not offered yet because the agent has no encoder without a new dependency.
- **A shared TypeScript device view component** (`studio/frontend/src/device-view/`, built to `dist-device-view/`): letterboxed canvas, gestures to taps, swipes and long presses, hover bounds, Alt+click to produce a selector, reconnect, pause when hidden. It runs in a browser and in a webview; a demo page and a mock hub are included.
- **VS Code: "FlutterProbe: Open Device View"** opens the device in an editor tab through `probe device view`, with a strict content security policy limited to the hub's origin, and inserts a `tap` step for an Alt+clicked element. Not available in remote windows yet.
- **Two read-only MCP tools, `studio_widget_tree` and `studio_ui_state`** (33 tools in all): the widget tree of the connected device as plain text, and the connection and run state Studio shows, in one call. Both go through the Studio automation endpoint, so no second connection to the agent is made. Every doc surface and the extension manifest list them.
- **The Studio UI shows fps, average and slowest capture time** over the last second (it showed the last frame's time only).
- **CI integrations, as sources to copy**: `actions/probe-test` (a composite GitHub Action that installs the CLI from a release, runs `probe test` on a device the job provides, attaches the JUnit report, and can pass `--upload-report`), `bitrise-step` (a Bitrise Step used with a `git::` reference), and a Codemagic recipe page (`/ci-cd/codemagic/`, iOS simulator). Both directories are MIT-licensed and self-contained: copy one to its own repository to publish it. They were written from each vendor's documentation and tested locally (asset mapping, argument assembly) but have not run on those services.
- **`probe_version` in probe.yaml pins the FlutterProbe CLI release a project is tested with**, as an exact release (`0.25.1`) or a release line (`0.25.x`). `probe test` stops before any device is touched or any test starts when the installed CLI does not meet the pin, and names the installed version, the required one and how to install it. A malformed value fails when probe.yaml loads. Without the field nothing changes.
- **`probe validate` checks probe.yaml and the `probe_version` pin without connecting to a device.** It reports each check as ok, warn or fail, with the fix for a failure, and exits non-zero when a check fails. `--format json` prints the same report with schema `probe.validate/1`. Run it in CI before `probe test`.
- **`validate_project` MCP tool** runs `probe validate` and returns its JSON report, so an AI agent can check the setup and the pin without running tests. A failed check is returned as a normal result. The tool count is 34 in the development version.
- **Run line and report version.** `probe test` prints `probe CLI X / agent A` on stderr once the first agent answers the handshake, so stdout (JSON, JUnit) is unchanged. The JSON report metadata and the JUnit suite properties gain `agent_version`, omitted when the agent does not report a version.

### Changed
- **`studio/README.md` has a manual checklist for the native Studio window and the VS Code device view** (what the browser tests cannot cover).
- **The Studio E2E workflow runs only when started by hand** (`gh workflow run "Studio E2E"`), no longer on version tags. With no self-hosted runner registered, every tag run queued for 24 hours and was cancelled; and a public repository should not hand a self-hosted runner to anything but a deliberate dispatch.
- **`probe test` says what to do when no device is usable.** The message names `probe doctor`, and a failed device listing shows its error instead of only "no connected devices found".
- **Cloud device farm status is stated honestly.** The README table, the new [Cloud device farms](https://flutterprobe.dev/tools/cloud-farms/) page, the VS Code README, `probe.yaml.example`, `--cloud-provider` help, `llms.txt` and the comparison pages now say the five providers are implemented but not verified on a live account, that cloud runs need a relay service (not shipped, and the hosted service is not live), and that Maestro Cloud runs Maestro flows only. The page lists what was compared with each vendor's documentation on 2026-10-14, with mismatches.
- **`--cloud-provider firebase` prints a deprecation notice.** Google's page says Firebase Test Lab "is deprecated and will be fully shut down on September 30, 2027" and points to the Developer Device Platform; FlutterProbe has no provider for it yet. The provider still works until then.
- **Shorter path to the Probe Cloud offer and to a first test on the site**: the landing page has Tutorials and Pricing in its header and footer and a design-partners band (free beta for the first 10 teams, with a link to the sample dashboard), the pricing Team card's button now applies as a design partner, the waitlist is a one-click prefilled email, and the install page opens with a four-command fast path.
- **The site no longer calls the whole project open source.** The CLI is source-available under BSL 1.1 and the Dart agent is MIT, so the landing page, its meta tags, the social image, the contact, privacy and terms pages and one comparison page now say "free to use, source-available CLI (BSL 1.1), MIT agent", and the landing page and pricing state what the BSL's use grant allows (any purpose, including production, except offering a competing hosted testing service).
- **The landing page's performance numbers are the measured ones.** "<50ms command round-trip", "<0.5% flake rate" and "<100ms CLI cold start" were design targets, not measurements, and are gone from the landing page; it now shows the replicated Maestro benchmark (about 2x faster median suite time, 0 flaky FlutterProbe runs of 20, with a link to the data).
- **The pricing page labels the planned Team price** ("Planned price, not final. Free for design partners.").
- **The landing page's install command is Homebrew** (`brew install AlphaWaveSystems/tap/probe`) instead of cloning the repository and running `make build`, the install step no longer shows a Linux-only download, and the page links to the first tutorial.

### Security
- **The GitHub Action and the Bitrise Step verify the downloaded CLI.** The install scripts compare the download with the SHA-256 digest that GitHub records for the release asset (the `digest` field of the Releases API; the releases publish no checksum file), install nothing on a mismatch or when no digest is published, and have an off-by-default escape hatch (`skip-verify` / `skip_verify`). The Action pins `actions/upload-artifact` by commit SHA (v7.0.2), takes a `github-token` for the API call, and has an `artifact-name` input so matrix legs do not clash. Tested offline with a good, a tampered and an unpublished download (`actions/probe-test/scripts/test-install.sh`, `bitrise-step/test-step.sh`). The Codemagic recipe checks the digest too.

### Fixed
- **The device view says why a connection is refused right after another window took over.** The hub answers a connection inside its 3 s takeover cool-down with HTTP 429, but a browser WebSocket does not expose the status of a refused handshake, so the VS Code panel and Studio showed only "reconnecting". A connection that fails before it opens, within 4 s of the page being taken over, now reads "another window just took over this device, retrying" and still retries with backoff. Studio runs the hub in process, so it needs nothing like VS Code's `--exit-on-stdin-close` (that flag only matters for a hub the editor starts as a subprocess).
- **`probe init` wrote a sample test that waited for "Welcome"**, which most apps do not show, so the first `probe test` of a new project timed out. The sample now only opens the app, so it passes on any app that starts with the agent enabled once `project.app` in `probe.yaml` is set. A test checks that the sample parses, lints clean and checks no text. The init output, the README and the install and quick-start guides now describe the same first run.
- **Two Maestro claims on the Flutter testing frameworks comparison are now sourced and worded to what Maestro documents**: the iOS docs describe simulators and do not describe local physical iOS devices (it was stated as "no physical iOS device support"), and the Cloud price is quoted from Maestro's pricing page with the date read. The CI paragraph no longer says built-in reporting requires Maestro Cloud.
- **AWS Device Farm sessions did not install the uploaded app**: `CreateRemoteAccessSession` was sent without `appArn` (and with `remoteDebugEnabled`, which the API reference does not list). The upload ARN is now sent as `appArn` and the undocumented field is dropped. Not yet verified on a live account.
- **Studio's live view no longer writes every frame to `reports/screenshots/`, and no longer fills the device with screenshots.** The screenshot request returned the image inline and the CLI also saved it, once per frame, in the working directory of the Studio process (several files a second, without bound); the agent also kept a copy of each frame in the device's temporary directory. The live view and `probe studio` now ask for the image inline (`probe.screenshot` with `inline: true`): nothing is written on the host and the agent deletes its copy. `probe test` screenshots (`take a screenshot`, failure screenshots) are unchanged. Compatible both ways: an older agent ignores `inline` and answers as before (the view still works and its device copies stay), and an older CLI never sends it. The agent change reaches pub.dev with the next release.
- **`probe studio`'s `/api/exec` answered `ok` without running the step, and `/api/tap` ignored the click's coordinates.** `/api/exec` now answers HTTP 501 "not implemented". A tap by text still works; a tap at a screen position answers 501 (the agent has no coordinate tap) and an empty request answers 400.
- **Studio's recorder wrote `type "hi" into the "hi" field`** for a typing event that named no target: the typed text was used as the selector. It now records `type "hi"`.

### Docs
- **The GitHub Actions page and the tutorial install the released CLI** instead of building it from source, enable KVM for the Android emulator action (which its documentation requires on Linux runners), and use the current `actions/checkout` major version.
- **README and pricing clarity**: the version badge shows the latest release (the tag badge could show a canary tag), the license badges say which part is BSL (CLI) and which is MIT (agent), the table of contents lists "Get started", and the pricing header says what is free and that Probe Cloud is a preview that is not live yet.
- **The unmeasured "sub-50ms" claim is removed everywhere.** "Sub-50ms command round-trips" (and the "<0.5% flake", "<100ms cold start" and "under 90 seconds" goals, and the "~100-200ms" and "10-40ms" figures for other tools and for the WebSocket) were never measured. The README, docs index, comparison and blog pages, architecture page, social image, `probe --help` text and the agent README now describe direct in-app control and cite the one measurement we have: the replicated benchmark against Maestro (about 2x faster median suite time, 0 flaky runs of 20). The `flutter_probe_agent` pubspec description changed too; pub.dev shows it from the next release.
- **A Probe Cloud design-partner guide** (`/tools/design-partner-guide/`): what a partner installs, how to inspect an upload with `--upload-dry-run` before the hosted service is live, what a GitHub Actions upload needs, and how to send feedback.
- **The sample dashboard pages at `/demo/` are refreshed** from their source (static pages with no form or sign-out button).
- **A "Why FlutterProbe" page** comparing FlutterProbe with Maestro, Patrol and the Appium Flutter driver. Every statement about another tool is taken from that tool's own documentation and linked; costs of choosing FlutterProbe and when each tool fits are listed.
- **Probe Cloud design-partner offer and a Probe Cloud preview page** on the docs site: pricing now has a design-partners section (free beta for the first 10 teams, 50% off for life when paid plans launch, apply by email or a public GitHub issue), and `/probe-cloud/` states what the preview does today (`--upload-report`, `--upload-dry-run`, the scrubber and its caps) and what is only planned.

## [0.25.2] - 2026-10-13

### Added
- **Report upload (Probe Cloud preview): `ci.head_sha`, the job's identity token and retries.** `ci.head_sha` is the commit under test (the push commit, or `pull_request.head.sha` from the event payload on a pull request); like `ci.base_branch` and `ci.default_branch` it is optional and a missing or garbled event file never fails the upload. When the job has `permissions: id-token: write`, the CLI requests the GitHub Actions identity token for the audience in `hosted.audience` / `PROBE_CLOUD_AUDIENCE` (default `probe-cloud`) and sends it as `X-Probe-Attestation` (at most 4,096 characters) to the configured endpoint only; it is never printed or logged, is redacted like the license key, and `--upload-dry-run` shows only `attestation: present|absent`. A busy service (HTTP 429 or 503) is retried with backoff for up to `hosted.retry_window` (default 5 minutes) and then gives a warning; the run is never failed. The endpoint can be set with `PROBE_CLOUD_URL`, and a successful upload prints one line naming the endpoint host. The hosted service is still not live. All additive: the report JSON and its `schema` value do not change.
- **Upload envelope: `ci.default_branch`, and `ci.base_branch` is documented.** A hosted report upload (`--upload-report`, or `--upload-dry-run` to preview it) gains an optional `ci.default_branch`, the repository's default branch, read from the GitHub event payload when it names one. `ci.base_branch`, the branch a pull request targets, is sent only for the `pull_request` and `pull_request_target` events. Both fields are optional: a missing or unreadable event file only adds a warning.

### Docs
- **Docs site**: tutorials (login test, GitHub Actions, flaky tests), a "Why FlutterProbe" comparison with sourced claims, the Probe Cloud design-partner offer and preview page (the hosted service is not live yet), a sample dashboard at `/demo/`, install and quick-start in first-run order, and recipe examples with quoted placeholders that `probe lint` accepts.

## [0.25.1] - 2026-10-12

### Changed
- **Studio device E2E checks the end of a live run**: test 15 (the native Test12 checks) now asserts that the verdict row heads its steps, the last step passes, no step is left running, and Cancel is hidden after the run. The native copy is removed with it (#331).
- **Studio native E2E runs a smoke set by default** (`make studio-e2e-native`; `make studio-e2e` is a deprecated alias for one release). The smoke set is launch and the recorder. The other native tests were removed once the browser suite covered them (#331). The suite refuses to start unless `STUDIO_E2E_DISPLAY` names an attached second display, and it no longer falls back to the main display; in CI an unset variable skips the job with exit 0.
- **Release process: one dogfood gate instead of two days.** A canary needs one green dogfood gate of about an hour or more on the named devices before a stable tag; the second-day gate and the 48-hour soak requirement are removed. The 24-hour auto-rollback watch after a stable tag stays, and the separate 14-day soak toward 1.0 (#325) is unchanged. Documented in `docs/ops/release-pipeline.md` and the dogfood-gate workflow.

### Security
- **Studio's automation endpoint also refuses `Sec-Fetch-Site: cross-site` and `same-site` requests**, matching the documentation and the headless test mode. The MCP server and other non-browser clients are unaffected.
- **Dependabot #157: `postcss-selector-parser` 6.1.4 to 7.1.6** (GHSA-rj75-hqrm-r3gf, CVE-2026-104844, medium: quadratic parsing of flat selectors). The docs site's build reached the 6.x parser through `@expressive-code/core` and `postcss-nested` 6.2.0. `@expressive-code/core` 0.44.2, the latest release, still pins `postcss-nested` 6.x, so the website overrides the parser to 7.1.6. The docs build output is byte-identical before and after the change, and `npm audit` reports 0 vulnerabilities for the website tree (10 moderate before, all along that chain). Dependabot lists the package only in `website/package-lock.json`: it is not part of the CLI or the Dart agent.

### Fixed
- **The Studio device picker keeps a device you pick while the list is refreshing.** The selection was read when a refresh started, so a pick made while the list was being fetched was reset to blank when the refresh finished. It is read when the list arrives now. Browser test 25 reproduces the race.
- **`probe init` scaffolded a recipe that its own `probe lint` and `probe test` rejected** (`type <email> into ...`: an unquoted placeholder is a parse error since PT-02, because it would be typed as literal text). The sample recipe now writes `type "<email>" ...`, and a test checks that what `probe init` writes parses. The same unquoted placeholders in the README, the VS Code README, the data-driven guide, the MCP guide and the AI test-generation prompt are corrected, so generated tests no longer start from an invalid example.
- **The agent install snippets said `flutter_probe_agent: ^0.5.3` (and `^0.9.6`, `^0.16.0`)**: for a 0.x package that constraint means "0.5.x only", so following the install guide fetched an agent far older than the CLI. They now say `^0.24.0`.
- **The report upload sent a retried test's absolute local path** in `envelope.retries[].file` (the report's own `results[].file` was already relative). That leaked the machine's path, and a server refuses a payload whose retry file is absolute, so every upload containing a retried test would have failed. The envelope's retries now go through the same scrubber as the report (relative path, secret patterns on the test name).
- **The upload scrubber missed Stripe-shaped secrets**: Stripe secret, restricted and webhook-signing key formats, live and test (only the hyphenated `sk-` form was covered), are now redacted, in text, error messages and nested fields.
- **`probe-convert` wrote unquoted placeholders for Gherkin steps whose outline variable has no quotes** (`I type <email> into "Email"` became `type <email> into "Email"`, `I tap on <button>` became `tap on <button>`): the converted file then failed `probe lint`, because an unquoted placeholder is a parse error. The converter now writes `type "<email>" into "Email"`, `tap on "<button>"` and `into "<field>"`; the catalog example is corrected, and a fixture (`examples/gherkin/unquoted_variables.feature` with its golden file) and a test cover it. Robot Framework, Maestro, Detox and Appium output was already quoted.

### Docs
- **Studio docs no longer call physical devices unsupported or the AI chat unbuilt**: `studio/README.md` says physical devices are supported (the USB, manual WiFi and Android paths are not yet verified on a real phone), and the Studio page's known limitations describe the AI chat as it is: an Anthropic key in the OS keychain, not part of automation or MCP.
- **Studio browser suite covers three checks that only the native suite had**: the manual WiFi empty-host explanation (test 23), the Record and Save toolbar states (test 24), and the platform tag in the device picker (test 22, for a simulator and an emulator). The native copies stay until the rest is ported (#331).
- **ProbeScript and EBNF code blocks are highlighted on the docs site.** The site registers the VS Code extension's ProbeScript grammar (`vscode/syntaxes/probe.tmGrammar.json`, read from there so the two stay in step) for the `probe` and `probescript` languages, and a small EBNF grammar (`website/grammars/ebnf.tmLanguage.json`) for `ebnf`. The 84 code blocks on four pages used to render as plain text, each with a build warning. Nothing else in the site's build output changes, and the docs deploy also runs when the grammar changes.
- **Studio testing docs**: `studio/e2e/README.md` covers the display rule, the smoke set and the three modes (headless browser, headed browser, native smoke set); the Studio page and `studio/README.md` name the new targets.
- **`ProbeAdvertiser` iOS requirements (#379)**: the Studio and iOS pages now state that `NSLocalNetworkUsageDescription` and `NSBonjourServices` are needed on iOS 14+, as bonsoir's own installation notes say. The iOS guide's WiFi-discovery sentence repeated a fragment; it reads once now.
- **Release PR checklist** in `docs/ops/release-pipeline.md`: every version-bearing file, the greps that find what a bump misses (including the `flutter_probe_agent: ^0.x` install snippets, which for a 0.x package pin one minor), canary examples, the MCP tool count, grammar and doc surfaces.

## [0.25.0] - 2026-10-12

### Added
- **Physical-device fixture and script** (phase 2 of the Studio physical-device plan): the Flutter fixture app advertises the agent with a `bonsoir` `ProbeAdvertiser` when built with `PROBE_WIFI=true`, and its `Info.plist` has the `NSLocalNetworkUsageDescription` and `NSBonjourServices` entries. `scripts/physical-ios-fixture.sh --ip <phone-ip>` prints the exact build, install, check and connect steps (CLI and Studio) for the connected iPhone; `--check` tests ping, the agent's status endpoint and the Bonjour service. It changes nothing.
- **Opt-in Studio E2E against a physical phone** (phase 4 of the physical-device plan): `scripts/studio-e2e-physical.sh --host <phone-ip> --token <token>` runs four browser-driven tests against the fixture app on a real phone (test connection, connect with the live screen and widget tree, a probe file runs, a wrong token is refused without echoing it). It refuses to run in CI and without the phone's address and token, and checks that an agent answers before starting. `studio/e2e-web/PHYSICAL.md` is the ten-minute manual checklist with the expected result of each step.
- **Studio's device picker lists physical iPhones that are paired over the network** (phase 3 of the physical-device plan): `xcrun devicectl list devices` is read next to `libimobiledevice`, so a phone that is not on a cable no longer silently disappears. Entries show the phone's name and model and how it can be reached (USB, or WiFi only). Selecting a physical device shows a line saying what it needs (the app running in the foreground with the agent: a profile build made with `PROBE_AGENT=true`, and `PROBE_WIFI=true` for WiFi); a WiFi-only phone has Connect disabled with a pointer to Connect manually, and an unauthorized or offline Android phone says what to do. `probe device list` gets the phone's real name too.
- **`--format junit` works with flaky-test and test-analytics services** (checked against their public docs only, nothing uploaded): suites are sorted, with `file`, `timestamp`, a real `time` and `errors`; paths are relative with forward slashes (no home directory); `<error>` marks infrastructure problems (agent connection, setup hooks) and `<failure>` assertions; `<properties>` carry probe version, platform, OS version, app id and version, device name and class; data-driven rows get a `row` property and a re-run test an `attempts` property. New flags: `--junit-attempts` (one `<testcase>` per attempt of a re-run test, off by default because the extra failed cases change the counts of a run that finally passed), `--junit-suffix` (appended to classname) and `--junit-label` (`run_label` property). The JSON report's metadata gains `device_class`. New page: Results in other tools.
- **Studio: manual WiFi entry** (phase 1 of the physical-device plan, `docs/proposals/studio-physical-devices.md`): the WiFi overlay has host, port (default 48686) and token fields with validation (IPv4, IPv6 or hostname; port range; token characters), a **Test connection** step that dials and pings without connecting, and error messages that say what to check (same network, app in the foreground and unlocked, profile build with `PROBE_AGENT=true` and `PROBE_WIFI=true`, port, fresh token). The automation endpoint has `test_wifi` next to `connect_wifi` (port defaults to 48686), and the MCP tool `studio_connect` accepts `host`, `port`, `token` (and `test_only`). Headless browser tests and a native E2E test cover it.
- **`perf.frame_budget_ms` in `probe.yaml`** sets the build+raster time above which a frame counts as slow (default 16.7 ms), so `slow_frames` is meaningful on 120 Hz displays and debug builds (#354). Sent as an optional `frameBudgetMs` parameter of `probe.perf_start` (agent 0.25+; older agents keep 16.7 ms and the CLI prints a note); the budget used is reported as `frame_budget_ms`, and `slow_frames` is not compared between windows measured with different budgets. Grammar unchanged.
- **Probe Cloud (preview): opt-in report upload.** `probe test --upload-report` (or `hosted.upload_report` in probe.yaml) sends a scrubbed copy of the JSON report plus a small envelope (Probe version, device class, retry outcomes, GitHub Actions run info) to a hosted service, with a license that has the `probe_cloud` plan. The scrubber drops artifacts, device ids and environment, makes paths relative, redacts credential-shaped strings and caps the size. `--upload-dry-run` prints the exact payload and sends nothing. One attempt with a 5 s timeout; every failure is a warning, never a failed or delayed run. The `hosted:` block is separate from the device-farm `cloud:` block. The default endpoint is a placeholder that is not live. New page: Probe Cloud (preview). In CI, supply the license key with the `PROBE_LICENSE_KEY` environment variable (read at run time, never printed or written). Redirects are never followed and server text is sanitized before it is shown. The CI facts follow the hosted service contract: the pull request source branch (`GITHUB_HEAD_REF`) is the branch and the target branch is `base_branch`, `ci.job` (`GITHUB_JOB`) and `run_label` (`--upload-label`, `hosted.label`; needed when one CI job uploads more than once), 40-character commit hashes, `attempts` capped at 20; a real upload needs a GitHub Actions run and a report with a platform, otherwise a clear warning says why nothing was sent.

### Changed
- **A timed-out `wait for system dialog appears` explains itself on Android** (#367): it adds the focused window, the grant state of the permission the dialog is about (or the app's denied runtime permissions) and a hint such as "no system dialog was requested: the app is in the foreground and the permission is denied", so it is clear at once when the app never asked. Also in `probe system-dialog wait`.
- **On emulators and simulators the baseline comparison gates `memory`, `cpu` and `data` by default**; `frame_time` and `slow_frames` gate only on physical devices (they flapped on same-build reruns on virtual devices, #356). `--perf-metrics` still names any set explicitly. In compare mode windows that span an app restart (or have no memory/frame data) are reported as skipped, counted once, and no longer also as "no entry". This relaxes the default of an existing flag and does not change any grammar, `probe.yaml` key or protocol meaning; restore the previous set with `--perf-metrics all` or an explicit list such as `--perf-metrics memory,cpu,slow_frames,frame_time,data`.
- **Canary releases attach `probe-ios-driver.zip`** (a non-blocking `ios-driver` job in `release-canary.yml`, mirroring the stable workflow), so `probe ios-driver install` works against a canary. Applies from the next canary; documented in the install guide and the release pipeline notes.
- **`--format junit` identities and error kinds changed (migration note).** `classname`, `name` and `file` are now relative to the working directory with forward slashes (they were the path as typed; a file outside the directory keeps only its name): consumers keyed on the old identities see new test identities once, which only matters if you passed absolute or `./`-prefixed paths. Infrastructure problems (agent connection, `before all`/`before each` hooks) are `<error>` instead of `<failure>`: consumers must count `failures` and `errors`. `probe test` still exits non-zero for both. Both are valid JUnit; see Results in other tools.

### Security
- **Studio's manual WiFi entry only dials local targets** (follow-up to the manual entry): loopback, link-local, RFC 1918 private IPv4, IPv6 unique-local and link-local, `.local` and single-label hostnames. Hostnames are resolved and every resolved address is checked, so a name that resolves to a public address is refused. A public address needs the owner of the Studio process to set `PROBE_STUDIO_ALLOW_PUBLIC_TARGETS=1`; automation and MCP callers cannot enable it. Attempts are limited to 6 per minute (`TestWiFi` and `ConnectWiFi` share the limit) and errors no longer contain the agent token (dial errors could carry the URL with the token in its query).

### Fixed
- **The `ProbeAdvertiser` example used an outdated `bonsoir` API** (`await broadcast.ready`, `bonsoir: ^5.1.10`); it now uses `initialize()` and `bonsoir` 6.0.2, and the docs say why 7.1.x is avoided for now (its Android plugin did not compile against the installed Android SDK platform 36).
- **The suite `time` attribute of `--format junit` was always empty** (`time=""`); it is now the sum of its cases.
- **A wedged iOS XCUITest driver is reported directly** (#357): when the runner log shows "Timed out waiting for AX loaded notification" (xcodebuild exit 65), starting the driver or a later driver call now says the simulator's accessibility service is not responding and how to restart only the test simulator, instead of surfacing as unrelated step timeouts. A driver process that stopped mid-run is reported as stopped.

### Docs
- **iOS guide: "Physical iPhone over WiFi (CLI and Studio): what must be true"**: build mode and flavor, `PROBE_WIFI`, the port, the token, network reachability, how the Mac finds the phone (mDNS only with a `ProbeAdvertiser`; Studio's WiFi overlay has no manual host/port field, the CLI `--host/--token` path always works), and a three-command check.

## [0.24.0] - 2026-10-11

> **Performance baselines on emulators and simulators:** the frame metrics (`frame_time`, `slow_frames`) vary a lot between runs of the same build there, even with the stable-metrics default. Gate with `--perf-metrics memory,data` (plus `cpu` for windows of 5 s or more) and keep frame metrics for physical devices in profile mode. In compare mode, windows that span an app restart are skipped.

### Fixed
- **`double tap` and `long press` on a covered target now warn like `tap`** (#265): when something unrelated is on top at the target's point and the screen does not change, the step returns the same "covered by another widget" warning, which `--fail-on-warning` turns into a failure. They used to succeed silently against a target hidden by an overlay.
- **`probe device start --platform ios --name ...` failed on a simulator that was already booted** on Xcode versions that word the error "Unable to boot device in current state: Booted". It now treats that as success, like the older wording. Found by the browser UI suite.
- **Studio's device picker listed every device twice** when two refreshes overlapped (start-up plus a click on refresh); only the newest refresh fills the list now. Found by the browser UI suite.
- **Pricing page no longer advertises what does not exist:** the pay-per-use card is now "Pay-per-use for AI agents: Planned" with no features listed, the Team card is labelled planned/not available yet (badge "Planned", "Planned features"), and the waitlist is a plain email link instead of a form with placeholder fields that collected nothing. The experimental `--pay` flag and the `wallet` config key stay but are described as experimental (the flag is hidden from `--help`).
- **JSON/HTML report metadata `probe_version` was empty**; it now carries the CLI version.
- **Go directive is `go 1.26.1` in every module**, matching `go.work`, so CI jobs that read the version from `go.mod` (CodeQL) install the toolchain the workspace needs.
- **CPU was reported as `0.0` when the sampler had no usable interval** (one sample, or a counter that went backwards after an app restart). It is now unavailable and skipped by comparisons.
- **A zero or negative baseline made the comparison meaningless** (`+0%` on a failing test). It now compares the absolute change against the noise floor and prints the real delta.
- **A measuring window that spans an app restart** warns, is marked `window_spans_restart` in the report and is not compared with the baseline.
- **`start measuring` with no app connected** says "no app is connected ... open the app first" instead of blaming the agent version.
- **A measuring window that contains a relaunch of the app no longer reports zero memory and zero frames.** The agent-side window is re-opened on the new process after `restart the app`, `clear app data` and the other steps that relaunch the process (a plain `open the app` that only brings the app to the front is not a restart), so memory and frames cover the time since the relaunch. Such a window is marked `window_spans_restart`, and `--perf-update-baseline` leaves it (and any window with no memory and no frames) out of the baseline, with a warning, instead of writing zeros.
- **A `start measuring` without a name** is reported as `measurement`, so its baseline key no longer ends in an empty name.

### Changed
- **License keys use format FP2** (`FP2.<kid>.<payload>.<signature>`): the Ed25519 signature covers the exact text `FP2.<kid>.<payload>`, so the key id and the format tag are signed. The CLI verifies in a fixed order (shape, key id lookup, signature, claims, expiry), against a built-in trusted key set and a revoked key id list. A key signed with an unknown key id asks to update the CLI, a revoked one says so, `seats` must be at least 1 (0 is rejected), claims need `v: 2`, a `kid` equal to the key's, at least one plan and an expiry after the issue time. The old FP1 format is dropped and reported as an old format. The built-in key set is empty in this change; `PROBE_LICENSE_PUBKEY` (now `kid=publicKey[,kid=publicKey]`) still adds keys for development and tests. A shared test vector is checked in the unit tests.
- **Baseline comparison gates only stable metrics by default** (`memory`, `cpu`, `slow_frames`, `frame_time`, `data`). `memory_growth`, `cpu_peak` and `frame_max` are single-sample extremes that differ a lot between two runs of the same build (a rerun of 106 unchanged tests failed 47 of them); they are still recorded and shown, and gate again with `--perf-metrics all` or by name. `cpu` is compared only for windows of 5 s or more. New flags `--perf-metrics`, `--perf-ignore`, `probe perf compare --metrics`.
- **Baseline keys are relative to the current directory** (or `--perf-key-base DIR`), so the same tests match from another checkout; keys written by earlier versions are still found, and the run says how many measurements had no baseline entry.

### Security
- Dependency updates for the open Dependabot alerts: website (astro 7.2.8+, sharp, devalue, svgo, js-yaml, smol-toml, ...), VS Code extension (`@vscode/vsce` 4, fast-uri, undici, ...) and Studio (`labstack/echo` 4.15.3, source-map-js). All are build-time or dev-server dependencies; none is part of the CLI or the Dart agent. Ten moderate alerts remain in the docs site's Starlight/expressive-code chain, whose only suggested fix is a downgrade.
- **Studio headless mode refuses `Sec-Fetch-Site: cross-site` and `same-site` requests**, on top of the existing loopback, Host and Origin checks. Test-only server; non-browser clients are unaffected.

### Added
- **Compatibility policy for 1.x** (docs): what is frozen (ProbeScript grammar, `probe.yaml`, the agent protocol, report JSON), what is additive-only, what is outside the promise (Studio, experimental features, human-readable output) and the deprecation process.
- **Studio UI E2E in a browser** (`make studio-e2e-web`, `studio/e2e-web/`): the real frontend and Go backend, served by Studio's new headless mode (`PROBE_STUDIO_HEADLESS=1`, test-only, loopback, per-launch token) and driven by Playwright in headless Chromium: no native window, cursor or focus. 9 tests need no device and run on any CI runner (`.github/workflows/studio-e2e-web.yml`); 9 more use a named simulator running the fixture app (`scripts/studio-e2e-web.sh`, macOS). The native-window suite remains for what needs a real window.
- **Studio automation endpoint** (opt-in `PROBE_STUDIO_AUTOMATION=1`; confined to the workspace a person opened plus `PROBE_STUDIO_AUTOMATION_ROOTS`, symlinks resolved; the MCP client only talks to loopback): loopback-only control endpoint with a per-launch token and a 0600 discovery file, exposing open_workspace, connect, run_file (async), cancel, run_state, results, screenshot and more, so Studio can be driven without a window, cursor or focus. Refuses requests from browsers, non-loopback hosts and without the token.
- **MCP tools `studio_open_workspace`, `studio_list_devices`, `studio_connect`, `studio_run_file`, `studio_cancel`, `studio_run_state`, `studio_results`, `studio_screenshot`** drive a running Studio through that endpoint.
- Studio tracks the current/last run (step, step history, results) for `RunState`; `RunFileAsync` starts a run in the background.
- **Studio: live step highlight.** While a file runs, the editor's gutter follows the executing step (▶, then ✓ / ✗ / – stays on the line; a failed line keeps its error on hover), step rows nest under each test in the results timeline with line, duration and attempt (click one to jump to its line), the toolbar shows `step N of M`, and a **Cancel** button stops the run. Backed by a new runner hook, `Runner.OnStep` / `Executor.OnStep`, that emits a `StepEvent` before and after every step (including recipe bodies and composite devices).
- Website: draft pages for terms of service, privacy policy, refund and cancellation policy and contact (`/terms`, `/privacy`, `/refunds`, `/contact`), linked from the footers and the pricing page. Marked "draft for review" until confirmed.
- **`probe license activate <key>` / `probe license status`:** stores and verifies an offline ed25519-signed license key in `~/.flutterprobe/license.key`. Only hosted features will ever need one; everything that runs locally stays free and works without a key.
- **Release pipeline:** canary channel (`vX.Y.Z-next.N` tags publish `probe@next` on Homebrew and pre-release Dart packages), a dogfood gate workflow, an automated review-agent check on pull requests, and auto-rollback of a stable release when a `prod-incident` is filed within 24 hours. `scripts/release.sh` accepts canary versions. See `docs/ops/release-pipeline.md`.
- **Studio E2E suite** (`make studio-e2e`, `studio/e2e/`): drives the real Studio window on macOS through the accessibility tree and screenshots — launch, device picker, connect, run, results, failures, performance lines, recorder, settings, AI-chat and WiFi overlays, error paths — against a new fixture app (`native-test-apps/studio-fixture/`). Optional vision assertions via `STUDIO_E2E_AI_PROVIDER`. Parameterised for dogfooding on any app (`studio/e2e/DOGFOOD.md`); self-hosted macOS workflow `studio-e2e.yml`.
- **`--measure-all`** (and `perf: {measure_all: true}` in probe.yaml): a measuring window named after the test around every test body, so an existing suite gets performance baselines with no script edits. Tests that use `start measuring` themselves are left alone.

### Security
- **Review-agent workflow:** reviews only same-repository branches from owners, members and collaborators, and fails (never skips) for anything else, since a skipped required check counts as passing; the model gets no tools and one prompt (conventions from the trusted base commit plus the diff between random markers, declared untrusted), so it cannot read files or the environment; nothing PR-controlled reaches a shell through `${{ }}`; the verdict counts only as the exact first line of the reply; an approval is bound to the reviewed commit; a branch behind the current tip of its base fails both before the review and again just before it is submitted; credential-shaped strings in the diff fail the check before any model sees it; the CLI version and actions are pinned. New `REVIEW_PROVIDER` variable (`oauth` default, `anthropic`, or `local` as a never-approving pre-screen).
- **Dogfood-gate workflow:** the `version` input is validated against `X.Y.Z[-next.N]` before it is used in paths, download URLs or issue titles.

### Fixed
- **License keys have exactly one valid spelling:** the verifier now decodes base64url strictly, so a key re-spelled with non-zero trailing bits is rejected instead of verifying as an alias (not a forgery: the signed bytes were unchanged). The tamper test is deterministic; it failed about once in a few hundred runs before.
- **Studio reads `probe.yaml` from the open workspace** (agent port, timeout, device ids) when connecting; it used to read the process working directory. `PROBE_STUDIO_WORKSPACE=/path` opens a workspace at launch.
- Studio file-browser rows and result rows carry accessible names (`role=button`, `pass:`/`fail:` labels).

### Docs
- **Installation: "Try a canary build"** covers `brew install AlphaWaveSystems/tap/probe@next`, the pre-release binaries and taking the agent from the tagged source (git dependency) because canaries are not published to pub.dev.
- Performance guide: what the numbers mean on simulators and emulators (every frame is "slow" in a debug build), what memory is, and that network assertions see `dart:io` traffic only.

## [0.23.0] - 2026-10-10

### Added
- **Performance testing:** `start measuring "name"` ... `stop measuring` measures, over those steps, the app's memory and frame timings (agent 0.23+, every device), its CPU (Android via /proc, iOS simulator via host ps; not a physical iPhone) and its HTTP traffic. Assertions: `see memory below N MB`, `see memory growth below N MB`, `see cpu below N percent`, `see cpu peak below N percent`, `see slow frames below N percent`, `see frame time below N ms`, `see slowest frame below N ms`, `see data transferred below N KB`, and `see response <ref> below N ms`. EBNF + conformance tests.
- **Baselines and trends:** `probe test --perf-baseline FILE` fails a test whose numbers got worse than the baseline by the tolerance (`--perf-tolerance`, default 20%, with noise floors); `--perf-update-baseline` writes it; every run appends to `reports/perf-history.jsonl`; `probe perf trend` and `probe perf compare`.
- **Reports:** a `perf` array per test in the JSON report, a Performance block in the HTML report, measurements under each result in Probe Studio.
- **MCP:** tools `perf_trend` and `perf_compare`; the guide covers performance, backend data, waiting, localization and the new flags.
- New guide "Performance testing"; proposal for the unified Studio UI (`docs/proposals/performance-testing.md`).

## [0.22.1] - 2026-10-09

### Security
- The agent's HTTP capture redacts more credentials: `x-goog-api-key`, `api-key`, `x-auth-token`, `x-access-token`, `x-amz-security-token`, `x-csrf-token`, `x-xsrf-token` headers and credential query parameters (`?key=`, `?access_token=`, `?token=`, ...) in recorded URLs. Bodies are unchanged and documented as such.

## [0.22.0] - 2026-10-09

### Fixed
- **`defaults.retry_failed_tests` now does something** (it was read but never applied) and its default is 0: a failing test is re-run up to that many extra times, also `probe test --retry-failed N`; a pass on a retry is reported (`passed on attempt N`, `attempts` in JSON).

### Added
- **`within N seconds|ms`** on a step (tap, type, see, wait, system dialog, response steps): its own timeout and implicit-wait window, e.g. `wait until "Report ready" appears within 90 seconds`. New guide "Timeouts, waiting and retries" explaining every layer (step timeout, implicit wait, `within`, `optional`/`if visible`, `retry N times`, `retry_failed_tests`).
- **Backend awareness (agent 0.22+):** `wait for response GET "/api/orders" [status 200]`, `see response <ref> status N | contains "x" | json "path" equals "v" | json "path" exists`, `store response <ref> json "path" as var`, `if response <ref> <check>` (+ `otherwise`), `see exactly N requests <ref>` / `see no requests <ref>`, `clear recorded requests`. References are `[METHOD] "path-with-*"` or a full URL. Failures list the app's recent requests. New guide "Testing against backend data". EBNF + conformance tests (`wait-response-step`, `see-response-step`, `store-response-step`, `see-requests-step`, `clear-requests-step`).
- **`when the app calls` mocks are now real** (they used to be recorded but never applied): the app's `dart:io` client receives the status/body, `after N seconds` delays it, `respond with network failure` drops the connection; `patch`/`head`/`options` methods; they last one test and are re-applied after `restart the app`.

## [0.21.0] - 2026-10-09

### Added
- **`l10n "key"`**: a string whose text comes from the app's ARB files in the current app language (`probe.yaml` `l10n: {dir, default}`; language fallback `de-AT` -> `de` -> default). Keys are validated for the run language, every `set language` literal and the default before any device work; placeholder/plural messages are rejected. EBNF + conformance test updated (`l10n-string`).
- **`set language "de"`** (ProbeScript) and **`probe test --locale de`**: run the app in another language (BCP-47 tag, `"system"` resets). Android 13+ per-app locales; iOS simulator launch arguments (also applied on `restart the app`/`clear app data`). Right-to-left languages flip the layout. Physical iOS devices are rejected with a clear message. Grammar page and conformance test updated.
- **`probe test --locales de,ja,ar`**: the suite once per language in separate runs; `-o r.json` becomes `r.de.json`, ...; per-language pass/fail table; reports record `locale`.

## [0.20.0] - 2026-10-09

### Fixed
- **Agent: `scroll down in "<anchor>" until X appears` regression (0.19.2+):** failed with `Widget not found: text("<anchor>")` once the anchor scrolled off screen (the scrollable was re-resolved from the anchor every step). Resolved once now.
- **Agent: "UI one tap behind":** the post-action settle (and `wait for idle`) now lets a scheduled frame run before declaring the UI idle, so `see` right after `tap` no longer reads the pre-tap state (bottom sheets, radio rows; intermittent).

### Added
- **iOS system-dialog labels for 27 languages:** allow / deny / allow once / allow while using measured on a real simulator (iOS 26.3) per language, raw output in `docs/evidence/i18n-ios-dialog-labels-2026-10/`. `tap "Allow" in system dialog` now works on a device set to any of them. Not measured (so exact label needed): `ok`, `cancel`, `open` outside English, photo/calendar/contacts alerts.
- **Named devices (hard rule):** `probe device start --platform ios --name <n>` (and MCP `start_device` `name`) boots the simulator with that name, or creates one (same device type/runtime as the default) when missing. Running Android emulators are listed and reported by their AVD name instead of the hardware model, and a run never has an unnamed device (falls back to the serial/UDID). Every result records device name and id.
- **Loose text matching (`--match-loose`, `defaults.match: loose`):** text selectors, `see`, `wait until` and tooltip/Semantics-label
  matching fold case, accents and diacritics, typographic apostrophes and dashes, full-width forms, invisible characters and
  whitespace on both sides (`Änderungen` ~ `anderungen`, `Don’t Allow` ~ `Don't allow`). Off by default. Ids are never folded.
  Marks that carry meaning (Indic vowel signs, Thai tone marks) are kept. A shared fixture file keeps the Go and Dart folding identical.
- The system-dialog button/title matching now uses the same folding (accents included).
- **System-dialog buttons resolve by role in any device language:** `tap "Don't Allow" in system dialog`, `deny permission "notifications"`,
  `--grant notifications` and `dismiss system dialog` find the button with the same role (allow, deny, allow once, allow while using,
  OK, cancel, not now, open, close) on a device set to another language. Android uses the permission dialog's resource ids; an exact
  label always wins over a role. The iOS notification alert is recognised by its buttons (Allow + Don't Allow) instead of its English title.
- **iOS `deny permission "notifications"` really resets the decision when the app is not running:** iOS keeps an earlier Allow (or
  `--grant notifications`) for good and simctl cannot change it, so a "first run" notification test only saw the alert once. The
  step now reinstalls the app from a copy of its own bundle (`SimCtl.Reinstall`), which brings notifications back to "not asked
  yet" (verified on a simulator: Allow, relaunch shows no alert; reinstall, launch shows the alert again). With the app running it
  still answers the alert. The reinstall leaves a new, empty data container.
- Docs: new guide "Testing localized apps"; README, agent README (pub.dev), `internal/README`, llms.txt, MCP guide, VS Code snippet.

## [0.19.5] - 2026-10-09

### Fixed
- **iOS system-dialog buttons match across apostrophes and case:** iOS writes "Don’t Allow" (U+2019); a script written for
  Android says "Don't allow". The iOS driver's button, title and dismiss matching now folds typographic apostrophes, case and
  whitespace, so `tap "Don't Allow" in system dialog` and `deny permission "notifications"` work on iOS (the latter always
  failed with `no button "Don't Allow" in the dialog (buttons: Don’t Allow, Allow)`). Checked on a real system alert.
- **Android `wait for system dialog appears` survives a failed UI dump:** `uiautomator dump` fails now and then while the screen
  animates or the device is loaded (the cause of "1-2 of 5 tests fail, a different one each run"). The dump is retried up to 4
  times with its own output kept in the error, and the wait keeps polling until its deadline instead of ending on the first
  failed dump; the dump error is reported only if the dialog never showed.
- **Quieter iOS `clear app data`:** the benign non-zero status of restarting the preferences daemon is no longer printed as a
  warning.

### Docs
- `--yes` cannot answer the iOS notification alert (simctl cannot grant notifications): use `--grant notifications`.

## [0.19.4] - 2026-10-08

### Fixed
- **iOS simulator `clear app data` deleted nothing (since the command was written):** the files were removed with `xcrun simctl spawn <udid> rm -rf`, but `simctl spawn` runs the command with the simulator's PATH where a bare `rm` is not found (ENOENT). The error was discarded and "Cleared data container" was printed anyway, so onboarding flags, Hive/sqlite files and preferences survived every clear. The container is now emptied from the host (`Documents`, `Library`, `tmp`, recreated empty with `Library/Caches`, `Library/Preferences`, `Library/Application Support`), and a failure to wipe is an error. The same bare-`rm` bug left the stale agent-token file in place on iOS reconnects; that is removed from the host too. The preferences-daemon restart from 0.19.1 and the running-app check now use `/bin/launchctl`, and a failed daemon restart is reported instead of hidden.

### Docs
- `tap`, `type` and `long press` scroll a target that exists in the widget tree into view; a target a lazy list has not built (beyond its cache extent) needs `scroll <direction> until "X" appears`.

## [0.19.3] - 2026-10-08

### Fixed
- **A bare `scroll down` / `swipe` picks a scrollable that can scroll.** It chose the largest scrollable under the touch point, which
  could be a `PageView` with `NeverScrollableScrollPhysics` (the biggest scrollable on its screen) and then moved nothing, while
  the page inside it needed the scroll (`scroll down until "..." appears` reported "not found after scrolling" on an onboarding
  flow). Scrollables with a scrollable extent that accept user input now rank first, then the one that receives touches, then
  the largest.
- **iOS `probe system-dialog tap` no longer reports a tap that did nothing as done:** after tapping, the driver checks that the
  dialog went away (it polls for about a second), retries once with a tap on the button's centre point, and otherwise
  fails with `tapped "Open" but the dialog is still showing` (seen with SpringBoard's "Open in <app>?" confirmation).
  Not verified against that dialog here; the driver builds and the other dialog paths are unchanged.

## [0.19.2] - 2026-10-08

### Fixed
- **Actions scroll an off-screen target into view:** since 0.19.0 `tap "Service"` failed on a chip that is built but beyond the
  viewport of a horizontal list (assertions rightly do not see it). `tap`, `long press`, `double tap`, `type` and `clear` now
  scroll such a target into view first (`Scrollable.ensureVisible`), like a user. `see` / `don't see` still ignore it.
- **iOS simulator, connection lost while the app is in the background** (a link that opens Safari): the reconnect brings the app
  back to the foreground first when its process is alive, instead of redialing a suspended socket. A crashed app is still not
  relaunched silently.
- **`probe migrate maestro`:** a text-entry step (`eraseText`, `inputText`) after a `tapOn` that became a `# TODO` is no longer
  emitted as a bare `clear` / `type` (nothing is focused); it becomes a TODO as well.

## [0.19.1] - 2026-10-08

### Fixed
- **`clear app data` on an iOS simulator now resets `shared_preferences` / `NSUserDefaults`:** the files were deleted, but the
  simulator's preferences daemon (cfprefsd) kept the old values in memory (and can write them back), so a relaunched app
  could still be "already onboarded". The daemon is restarted before and after the container is wiped. The mechanism
  (`launchctl kill` of the daemon through `simctl spawn`) was checked on a simulator, but the full effect on an app that
  persists a flag was not reproduced here.

## [0.19.0] - 2026-10-08

### Added
- **`see any of "A", "B"` / `don't see any of "A", "B"`:** an assertion over alternatives (passes when any one is on screen; the
  negated form when none is). `probe migrate maestro` converts a plain `assertVisible: "A|B"` / `assertNotVisible: "A|B"` to it.
  With implicit wait it retries like any other `see`. Grammar page, conformance test, dictionary and MCP guide updated.

### Fixed
- **`see` / `don't see` / `wait until` only count widgets that are on screen:** a widget laid out entirely outside the screen (the
  neighbouring page of a `PageView`, a tab kept alive to the side, list items in the cache area) no longer matches. `don't see X`
  used to fail on such off-screen copies ("found 2 element(s)") while Maestro's `assertNotVisible` passed. **Behavior change:** a
  `see` of a widget that is built but off screen now fails (it is not visible to the user).
- **Elements caught mid-rebuild no longer crash lookups:** `tap "Add Community"` right after a route change failed with a Flutter
  assertion (`'_renderObject != null'`); such elements are treated as not visible, and with `--implicit-wait` the step retries.
- **`screenshot` right after a navigation** no longer fails with `'!debugNeedsPaint'`: the capture waits for the pending frame
  and retries (bounded).
- **`probe migrate maestro`:** relative selectors (`tapOn: {below: ...}`, `childOf`, `index`, ...) become a `# TODO` with a warning
  instead of the garbage selector `tap "map[below:...]"`.

## [0.18.1] - 2026-10-08

### Fixed
- **`--disable-animations` / `defaults.disable_animations` set `timeDilation` to 0**, which Flutter does not accept: an assert in
  debug builds, and in profile and release builds a division by zero in every frame. A frame that throws can leave the UI a
  tap behind (the change appears only after the next input). The CLI now sends 0.001 (time runs about 1000x faster, so
  animations end within a frame) and the agent maps any non-positive factor to 0.001.

## [0.18.0] - 2026-10-08

### Added
- **Implicit waiting (`probe test --implicit-wait 7s`, `defaults.implicit_wait`):** a `tap`, `type`, `long press`, `double tap`,
  `clear`, `drag` or plain `see` whose target is not on screen yet is retried (every 0.3 s) for up to the given time before
  failing, like Maestro's implicit waiting. Off by default, so existing suites behave as before. Steps with `if visible` /
  `optional`, `don't see`, and the explicit `wait` steps are never retried. `probe migrate maestro` prints the recommended
  setting: migrated flows rely on it (a login step used to fail while the sign-in was still in flight, 55 of 75 migrated flows
  in one project).

## [0.17.2] - 2026-10-08

### Fixed
- **`probe migrate maestro`, `scrollUntilVisible`:** converts exactly to `scroll <direction> until <#id | "text"> appears`
  (it was approximated as one blind `scroll`, so the target was often not reached).
- **`probe migrate maestro`, soft keyboard:** after `inputText` a `close keyboard` is emitted when the next step acts on
  something that is not another text field (a button tap, a scroll toward a target). The keyboard otherwise stays open and
  covers the button; Maestro's own tap or scroll dismisses it implicitly. This was the cause of most migrated login helpers
  failing.

## [0.17.1] - 2026-10-08

### Fixed
- **`see #x is checked` / `don't see #x is checked` now read the real value.** The agent had no `checked` check at all:
  it fell through and passed for every widget, so `is checked` always succeeded and `don't see ... is checked` always
  failed. It now reads `Switch`, `SwitchListTile`, `CupertinoSwitch`, `Checkbox`, `CheckboxListTile`, `Radio`,
  `RadioListTile`, `FilterChip` and `ChoiceChip` (the matched widget, a control inside it, or the control enclosing the
  matched label text), and says so when the widget is not a checkable control. **Tests that passed only because
  `is checked` was always true will now fail where the control is off.**

## [0.17.0] - 2026-10-08

### Fixed
- **`see "Submit" is disabled` / `is enabled` read the control that labels the text:** the selector matches the `Text`
  inside a button, which was never itself a button, so every labelled button counted as enabled. The nearest enclosing
  button (`ElevatedButton`, `TextButton`, `OutlinedButton`, `FilledButton`), `IconButton`, `FloatingActionButton`,
  `TextField` (`enabled`), `Switch` or `Checkbox` decides.

### Added
- **`wait until "X" appears matching "<regex>"`:** the exact-text form of a wait (text selectors match substrings), the
  same suffix as `see ... matching`. Grammar page and conformance test updated.
- **`probe test --no-grant-on-clear`:** after `clear app data` keep runtime permissions revoked even with `--yes`
  (which implies granting), so a test can see the first-run permission dialog.
- **Tooltips and Semantics labels are matched** by text selectors when no visible text matches: `see "Show password"` finds
  an icon button whose only label is its tooltip. A visible text always wins, so counts do not double.
- **iOS share sheet in `probe system-dialog`:** `list` / `see` / `wait` / `tap` / `dismiss` (and the ProbeScript
  system-dialog steps) now handle the share (activity) sheet an app opens with `UIActivityViewController`, which
  previously reported "No system dialog is showing." The driver looks inside the app under test (`project.app`, or the
  new `--app <bundle-id>`); the sheet's title is the shared item's caption and `Share sheet` always matches; buttons are
  Close (where the OS draws one) plus the share targets and actions; `dismiss` taps Close, else taps outside the sheet,
  else swipes down. Alerts are unchanged. The driver host app gets a `-probeShowShareSheet` self-test argument.
- **Android `open link "scheme://..." in the app`** is restricted to the app under test (`am start -p <package>`; retried
  unrestricted if the app cannot handle the link) and the URL is quoted for the device shell. With two flavours of one app
  installed, the wrong one could start, and the CLI then never found an agent (`reconnect: read token`).
- **`--driver-port` / `agent.driver_port`:** choose the iOS system-dialog driver's loopback port (1024-65535) on
  `probe test`, `probe system-dialog` and `probe ios-driver status|stop`; the flag wins over probe.yaml; the default
  (derived from the simulator UDID) is unchanged.

## [0.16.9] - 2026-10-08

Fixes from the first full campaigns on 0.16.6-0.16.8.

### Fixed
- **`use` resolution:** a `use`d file's own `use` lines are followed, each relative to the file that contains it (nested
  helpers did not load). `probe test --dry-run` resolves each file on its own, so it matches running that file alone (it
  pooled all files' recipes and passed what a single run could not resolve). An unresolved call names `use` targets that do
  not exist.
- **Bare `type "x"` / `clear`** act on the text field that has focus. `type` went to the first text field on the screen
  (LIM-15), and a bare `clear` did nothing. With no focused field they fail with a clear message.
- **Texts under a dialog:** a SnackBar (or any content) of the page underneath a see-through route (dialog, bottom sheet,
  popup) is found by `see` / `wait until`; only routes under an opaque page are excluded.
- **Android `restart the app`:** the reconnect re-creates the `adb forward` while dials are refused. A forward removed by
  another tool on the same adb server (for example `adb forward --remove-all` from a parallel run) caused sporadic
  `agent not reachable within 30s` failures.
- **`clear app data` without a terminal** (CI, scripts) fails at once with "pass --yes" instead of blocking on a prompt;
  documented in the dictionary.
- **`probe migrate maestro`:** `optional: true` is kept (`tap ... if visible`, assertions get `optional`); `tapOn` of a
  field followed by `inputText` / `eraseText` becomes `type "x" into <field>` / `clear <field>`; `waitForAnimationToEnd`
  becomes `wait for idle` (it was `wait for the page to load`, which does not wait for animations).

### Added
- **Environment variables in steps:** `${NAME}` inside a quoted text is replaced from the environment when the step runs.
  The step line shows `${NAME}`, the value is scrubbed from errors and reports, an unset variable is left as written with a
  warning. Migrated Maestro flows that use `${TEST_EMAIL}` now run as they are.

## [0.16.8] - 2026-10-08

### Fixed
- **`see "X" matching "<regex>"` never checked the pattern:** the agent received it and only used it in the
  not-found message, so the assertion passed whenever the text selector found something. It now requires that some
  matched widget's text matches the regular expression (an invalid regex is an error). With text selectors matching
  substrings, `see "0 ml" matching "^0 ml$"` is the exact-text form.

### Added
- **`probe test --fail-on-warning`:** agent warnings (a tap that did nothing, `press enter` with no focused field,
  `go back` at the root) fail the step. Off by default.

### Docs
- Text selectors match substrings (`see "0 ml"` passes on "250 ml"); documented on the syntax, dictionary and grammar pages
  with the exact-match forms.

## [0.16.7] - 2026-10-08

### Added
- **New step `press enter`:** presses the keyboard action key (done / go / search / send / next) on the focused
  text field; warns when no field has focus. `probe migrate maestro` converts `pressKey: Enter` to it (it emitted
  `press key "Enter"`, which does not exist). Other `pressKey` values (home, volume, ...) become a `# TODO` comment.
  It is a compound keyword, so recipes whose names start with `press` or `submit` are unaffected.

### Fixed
- **`probe migrate maestro`, conditional `runFlow`:** `runFlow: {when: {visible: "X"}, commands: [...]}` becomes an
  `if "X" appears` block, and `notVisible` becomes `if "X" appears` / `otherwise` with the commands (a `file:` target works
  too). Conditions on platform or scripts, and regex conditions, stay `# TODO`.
- **`probe migrate maestro`, regex selectors that mean a literal:** `.*TEXT.*` becomes `TEXT` (text selectors already
  match substrings), escaped characters (`time\\.`) are unescaped, parentheses are literal, and plain alternations
  with those forms become `wait until any of`. Only real regex syntax (a wildcard in the middle, classes,
  quantifiers) keeps a `# TODO`.
- **`before each` / `after each` without `test`** (what the Hooks and Dictionary pages show) kept the keyword but
  silently dropped the hook body. They are now the same hook as `before each test` / `after each test`.
- **`probe migrate maestro` now warns when a `runFlow` target is outside the migrate root** (it was never converted, so
  the generated `use` pointed at a file that does not exist). Migrate the parent directory that holds both flows and
  helpers to convert them together.

## [0.16.6] - 2026-10-07

### Fixed
- **`probe migrate maestro` and `runFlow`:** a `runFlow` target is now converted to a recipe file
  (`recipe "flow <file name>"`, written at the mirrored path) and the caller gets `use "<file>.probe"` plus a call to the
  recipe. Before it emitted `use "<helper>.yaml"`, which is not ProbeScript, so every migrated flow with a helper failed.
  `runFlow` options (`env`, `when`) are dropped with a warning.
- **`probe migrate maestro`:** `evalScript` becomes a `# TODO` comment (it was emitted as a step line that failed as an
  unknown recipe call); Maestro regex selectors (`"A|B"`, `.*x.*`) get a `# TODO` and a warning because ProbeScript
  matches text literally; `${ENV}` placeholders are reported in a warning. Migrated tests are named after the file.
- **Complete ProbeScript grammar in EBNF** (`website/src/content/docs/probescript/grammar.md`): lexical rules, layout,
  every statement, selector, modifier and block, with a runnable example per production. A conformance test
  (`internal/parser/grammar_test.go`) parses every example and fails when a keyword is added to or removed from the
  lexer without the grammar page changing. The page also lists where the other docs and the parser disagree.
- **New step `wait until any of "A", "B", "C" appears`** (the first alternative on screen satisfies it; times out
  with the step timeout). `probe migrate maestro` converts a plain `"A|B|C"` wait to it; other regex selectors still get
  a `# TODO`. Documented in the dictionary, syntax page, VS Code snippet/grammar and the MCP guide.
- **A recipe call with a bare number argument resolves:** `increment counter "x" 3` for
  `recipe "increment counter" (identifier, times)` failed with `unknown recipe call "increment counter <arg> 3"`
  because only quoted values counted as arguments. Bare numbers are now arguments too (a number that is part of a
  recipe's name, `step 2 of onboarding`, still matches as before).
- **Converters no longer emit steps ProbeScript does not have:** `probe-convert` turned Maestro `openLink` into
  `open "<url>"` (the valid form is `open link "<url>"`) and `setAirplaneMode` into `toggle wifi off`/`on` (no such
  step); both converters now emit `open link` and a `# TODO` comment. The new dry-run check found these.
- **`--composite-device` flags replace `composite.devices` from probe.yaml** instead of merging with it: a configured
  device that was down used to skip the whole composite test even when the flags named other devices.
- **`probe test --dry-run` now resolves every step.** A step that is not a built-in and matches no recipe (a typo, a
  verb that does not exist, such as `hide keyboard`) fails the dry run, also when it sits inside a recipe the test calls
  or in a hook. Before, dry-run reported such tests as passed and they only failed at runtime.
- **A tap that nothing receives is no longer a silent success** (Android: a submit button under the on-screen
  keyboard, or clipped away). A hit that stops at a passive ancestor or the root view now counts as "covered", so the
  existing warning appears when the screen also did not change; it says when the keyboard is open and suggests
  `close keyboard`. A hit that stops at a parent that handles taps is still normal.

## [0.16.5] - 2026-10-06

### Fixed
- **A recipe whose name starts with `clear`** (for example `recipe "clear search"`, called as `clear search`) is called
  instead of being parsed as the built-in `clear` with a type selector. The built-in forms (`clear "Email"`,
  `clear #id`) are unchanged.
- **False "covered by another widget" warning** (seen on a SnackBar action): the hit test can disagree with what a
  real tap reaches on overlays. The warning now appears only when the hit test says covered *and* the screen did not
  change after the tap, which is the silent-tap case it exists for. Its text now includes the topmost hit render objects.

- **Stale Android token file after an app restart:** if the token read from the app's cache is rejected again on a
  re-read (the agent could not rewrite the file), the CLI now falls back to the newest `PROBE_TOKEN=` line in logcat
  instead of retrying the same file until it gives up. The port-holder hint no longer blames the CLI's own `adb forward`
  for a rejected token on Android.

- **`go back` at the root route no longer exits the app on Android.** It used to call the system Back, which killed
  the app and the agent: the step hung until its timeout and every later test failed with a broken pipe. It is now a
  no-op with a warning ("already at the root route"); use `close the app` to leave the app on purpose. **Behavior
  change:** a test that relied on `go back` to exit the app must use `close the app`.
- A lost Android connection now says when the app process is gone ("the app process ... is not running") instead of
  only "probe token not found".

### Changed
- When the agent port refuses connections for 5 s the CLI now says so (is the app running and built with
  `--dart-define=PROBE_AGENT=true`?) instead of waiting silently for the dial timeout.

## [0.16.4] - 2026-10-06

### Fixed
- **False "covered by another widget" warning on a SnackBar action (Android):** when several widgets match a
  selector (an exiting SnackBar or route keeps its widgets in the tree briefly) `tap` now picks the one a pointer can
  actually reach, and a tap only warns after the target has stayed covered for ~0.6 s, so overlays that are still
  sliding in or out are not reported. A really covered target still warns (and costs that extra 0.6 s).

## [0.16.3] - 2026-10-06

Follow-ups from a real project's gate on 0.16.2 (FP-20).

### Fixed
- **Silent tap after `scroll ... until`:** after a scroll the agent now waits (up to 2 s) until the target is actually
  hit-testable before tapping, and the "covered by another widget" warning only fires when the widget at the tap point
  is unrelated to the target (a descendant or ancestor hit, such as a button's inner text, is no longer reported).
- **`connection refused` after a cold launch (Android):** while dials are refused the CLI re-creates the `adb forward`
  every ~3 s, so a forward that vanished no longer turns into a full dial timeout. A refused timeout now says what
  it means (agent never started vs. forward gone).
- **iOS simulator port clashes:** `Address already in use` while forwarding now names the process holding the port.

### Docs
- `wait for idle` does not wait for application-level async loads (a list fetched in `initState`); use
  `wait until "<text>" appears` after navigation.

## [0.16.2] - 2026-10-06

Fixes for issues found by running a real project's full gate on 0.16.1 (FP-19).

### Fixed
- **Android failure screenshots were corrupt.** The screenshot is saved on the host from the RPC's base64 data, and
  `PullArtifacts` then re-read that *host* path through `adb exec-out run-as ... cat`; `cat` printed "No such file" on
  stdout (exit 0) and that text overwrote the PNG. A screenshot already on the host is now kept, and anything pulled from
  a device must start with a PNG/JPEG signature or it is reported as an error instead of written as an image.
- **Cold-launch race on Android (and iOS simulators):** dialing a few seconds after launch could end in
  `unexpected EOF` and then a 401, because the token readable on the device was still the previous app instance's. When
  the CLI auto-detected the token it now re-reads it on a 401 and retries for up to 12 seconds. An explicit `--token`
  still fails fast.
- **The "agent port is held by" hint now says what holds it:** an `adb` port forward (with `adb forward --remove`), an iOS
  simulator app (with its simulator UDID and `simctl terminate`), or any other process. It used to print the same text for
  the first two.

### Changed
- **A tap that cannot reach its target is no longer silent.** When something else is on top at the tap point, the agent
  returns a warning (`tap target #id is covered by another widget at (x, y) ...`, with the visible texts/keys) and the
  CLI prints it. The tap still happens, as for a real user.
- A tap now waits (up to 1.5 s) while a scrollable around its target is still scrolling: Flutter ignores pointer events
  during scroll activity, so a tap right after `scroll ... until ... appears` could be dropped. Costs nothing when
  nothing is scrolling.

### Not changed
- `--grant notifications` taps Allow as soon as the iOS notification alert is up, which can be right after a relaunch and
  before the app has visibly asked. That is intended (the alert is what is answered); it is harmless.
- Whether the silent tap after `scroll ... until` on Android is fully explained by the two changes above could not be
  reproduced without that app; the new warning will name the cause if it happens again.

## [0.16.1] - 2026-10-06

### Fixed
- **The Windows build of 0.16.0 failed**, so no 0.16.0 GitHub release or Homebrew update was published (the 0.16.0 agent
  did reach pub.dev; it is unaffected). The iOS system-dialog driver used `syscall.Setpgid` / `syscall.Kill`, which do
  not exist on Windows; process-group handling now lives in build-tagged files. 0.16.1 carries everything in 0.16.0
  below.
- CI now cross-compiles `probe` and `probe-mcp` for linux/amd64, darwin/amd64, darwin/arm64 and windows/amd64 on every
  PR, so a platform-specific API fails the PR instead of the release.

## [0.16.0] - 2026-10-06

### Added: system dialogs (FP-16)
OS-level dialogs (iOS permission alerts, the StoreKit "Sign in to Apple Account" sheet, Android permission dialogs)
live outside the Flutter widget tree and were invisible to probe. They can now be driven from ProbeScript and the
command line.

- **ProbeScript steps:** `tap "Allow" in system dialog`, `type "$ENV" into system field "Password"`,
  `see system dialog "Title"` / `don't see system dialog`, `wait for system dialog "Title" appears|disappears`,
  `dismiss system dialog`, `sign in sandbox tester` (idempotent; `PROBE_SANDBOX_USER` / `PROBE_SANDBOX_PASSWORD`). Any
  of them accepts a trailing `optional`.
- **iOS** (simulators; needs Xcode): a new XCUITest runner app (`ios-driver/`) serves a loopback HTTP API from inside the
  UI-test process and drives SpringBoard's accessibility tree. Built in CI on every release and attached as
  `probe-ios-driver.zip`; downloaded on first use (`probe ios-driver install|status|stop`). Physical devices are not
  supported (code signing).
- **Android:** `uiautomator` dumps scoped to system packages (permission controller, package installer, Play, GMS,
  systemui) plus `adb input`. Typed text is single-quoted for the remote shell, so `& ; $ ' "` in a password are safe
  (the existing `type native` only escaped spaces).
- **Secrets:** values come from environment variables only (`$NAME` / `${NAME}`), are **always masked** in step output,
  reports and error text (including literals), and are never accepted as command-line arguments.
- **`probe system-dialog list|see|wait|tap|type|dismiss|sign-in-sandbox`**: the same driver without any test file or
  Flutter app, for one-time provisioning such as signing a sandbox tester in on a CI simulator.
- **iOS notifications**, which `simctl privacy` cannot grant: `allow permission "notifications"` and
  `--grant notifications` now tap **Allow** on the system alert (a watcher for `--grant`).
- **Docs:** new System Dialogs, On-Device Agent and Using Probe from AI Agents pages, `llms.txt`, a reworked
  `flutter_probe_agent` README for pub.dev, and `ios-driver/README.md`.

### Fixed (reported from real-project runs of 0.15.0)
- A bare `scroll down` picked the largest scrollable in the tree, which could be a hidden tab kept alive by an
  `IndexedStack`, so the visible list never moved. It now prefers a scrollable that receives touches on screen.
- Agent errors showed an id selector as `id("#settings_screen")`; it is now `#settings_screen`.
- The CLI/agent version-mismatch warning compared raw strings, so a pre-release CLI (`0.15.0-fp13`) warned against its
  own release (`0.15.0`). Pre-release and build suffixes are now ignored.
- Android `uiautomator` failures now explain that only one UI-automation client can run per device.

### Verification
iOS driver, steps and CLI verified on an iOS 26.5 simulator (real notification alert, and a StoreKit-shaped sheet with a
secure field: the app received `tester+1@example.com|P@ss w0rd!&;$x`); Android verified on an API 35 emulator (real
camera/location permission dialogs: see, tap, dismiss, wait, apostrophe matching, wrong-button error) and the shell
quoting on a real device shell. **Not verified:** the genuine StoreKit "Sign in to Apple Account" sheet (no StoreKit app
available here; use `probe system-dialog list` and `PROBE_DRIVER_APPS` if a given iOS version hosts it in another
process), typing into an Android system dialog field, and the CI job that builds the iOS driver.

## [0.15.0] - 2026-10-06

### Changed (BREAKING for WiFi auto-discovery only) — FP-15
- **`flutter_probe_agent` no longer depends on the native `bonsoir` mDNS plugin.** A plugin in an app's
  dependencies is linked into every build of that app, release included, regardless of
  `--dart-define=PROBE_AGENT` (release size, privacy, possible local-network prompt).
  mDNS advertising is now an optional `ProbeAdvertiser` hook
  (`ProbeAgent.start(advertiser: ...)`) that the app implements with its own `bonsoir` dependency; a
  copy-paste example is in the Studio docs. No new pub.dev package is published. Without it WiFi testing is
  unchanged (`--host <ip> --token <token>`); Studio's WiFi auto-discovery needs an advertiser in the device build.
  The advertised port is now the actually bound one.

### Optional local-LLM support (FP-14)

AI stays strictly opt-in: no `ai:` block means no AI code runs, and none of the additions below can change a
test result, a report or the exit code (they run after results are final, are time-bounded, make no retries,
and stop after the first provider error).

- **`probe ai doctor`** — checks the configured `ai:` provider: endpoint, model listed, a text round trip, and
  whether the model accepts images. Verified live against LM Studio (a 0.5B text-only Qwen: text OK, vision
  not accepted) and Ollama (Gemma 4 31B: both OK).
- **Failure triage** — `probe test --ai-triage` (after the run) and `probe triage --input results.json`
  produce a short advisory probable cause and next step per failure, printed and written to
  `<reports>/triage.md`. MCP tool `triage_failure` wraps it (20 tools now; docs previously said 18).
- **`ai.vision: false`** — text-only models: `see "..." with ai` is judged from the screen's visible
  texts/keys instead of a screenshot. `assert no visual defects with ai` and `read ... with ai` report that
  they need a vision model; the mode is refused when `ai.redact` rules exist (they can't apply to text).
- **`probe generate` and AI selector suggestions honour `ai.provider`**, including `local` with no API key.
  With no `ai.provider` the original Claude-only behavior is unchanged.
- Docs: new [AI & Local Models](https://flutterprobe.dev/tools/ai/) page, plan in
  `docs/prd/optional-local-llm-plan.md`. A 0.5B model gives weak triage advice; 7B+ is recommended.
- READMEs added for `cmd/`, `internal/`, `scripts/`, `docker/`, `plugins/`, `mcpb/`, `tests/`,
  `tools/probe-convert/`.

### Fixed (docs)
- `scroll down until "X" appears` was documented in the VS Code README and snippets but not implemented by the
  parser; it is now (FP-13, below).

Fixes for limitations hit during a release-gate run (FP-13), reported against CLI 0.14.0.

### Added
- **`scroll [dir] until <target> appears`** — scrollIntoView for lazily built lists (Maestro's
  `scrollUntilVisible`). Resolved agent-side: scrolls half a viewport at a time, then brings the
  target fully on screen (`Scrollable.ensureVisible`), stops early when the list can't move, and
  fails with the visible texts if the target never shows up. Older agents fall back to a CLI-side
  loop (25 attempts).
- **`wait for idle`** (also `wait until idle`) — route transitions finished and no pending frames,
  animations or HTTP requests. `tap` also waits (max 2 s) for an in-flight dialog/sheet/page
  transition before firing.
- **`probe test --grant <list>`** — pre-grant OS permissions once before the first test
  (`--grant notifications,camera,location`). Android: `adb shell pm grant` incl. `POST_NOTIFICATIONS`;
  iOS simulator: every `simctl privacy` service. **iOS notifications cannot be pre-granted** (no simctl
  service; the prompt is a SpringBoard alert the agent cannot tap) — `--grant` warns instead of failing,
  and `allow permission "notifications"` on iOS now fails with that explanation instead of a
  contradictory "unknown permission — available: notifications".
- **`--agent-port <n>`** on `probe test` / `probe record` (alias of `--port`) and the matching
  **`--dart-define=PROBE_PORT=<n>`** for the app, so simulators can't fight over 48686.
- **Port-holder report:** a failed agent dial on loopback now names the process holding the port
  (`agent port 48686 is held by pid 4242 (Runner)`) and how to move off it.
- **`probe.visible_summary`** agent RPC (visible texts + string ValueKeys) used for failure diagnostics.

### Changed
- **Timeout errors say what timed out.** A step that hit its deadline used to fail with a bare
  `context deadline exceeded`. It now reports the line, the step, the timeout and the visible texts/keys
  at that moment. Agent-side `Widget not found` / `Timed out waiting for` errors carry the same
  suffix, and `wait` hands the agent a timeout 2 s shorter than the CLI's so the agent's descriptive
  error arrives first.
- **`type` and `clear` go through the platform text-input path**
  (`EditableTextState.userUpdateTextEditingValue`) instead of assigning `controller.text`, so
  `onChanged`, `inputFormatters` and form validation run as with real keystrokes.

### Fixed
- `dont see "X"` (no apostrophe) is accepted as an alias of `don't see "X"`; it previously failed
  with `unknown recipe call "dont see <arg>"`.

## [0.14.0] - 2026-08-31

### Added
- **GPS route simulation (`travel to ... over N seconds`, FP-6).** A new block-style ProbeScript
  construct — matching Maestro's `travel` command — that walks the device's GPS location through
  an ordered list of waypoints over a duration, simulating movement for maps/delivery/rideshare/
  fitness flows. It's pure orchestration on top of the existing single-point `set location`
  primitive: the executor linearly interpolates between consecutive waypoints and calls
  `SetLocation` repeatedly at ~1-second intervals (never tighter), rather than adding any new
  device-level GPS integration. Follows the same physical-device exclusion `set location` already
  has (skips with a warning on real devices) and the same block-parsing shape `retry N times`
  established. Covered by parser tests (waypoint parsing, negative coordinates, the optional `over`
  clause, malformed-waypoint errors) and executor tests (route interpolation math, cloud-mode skip,
  waypoint-count validation) — not yet verified against a real emulator/simulator.

### Fixed
- **Studio's docs claimed physical devices weren't supported — they have been since v0.7.0 (#85, #86).**
  `website/src/content/docs/tools/studio.md` still said "Physical devices are not supported (no
  iproxy management in Studio yet)" in its Known Limitations section, the Beta Preview banner, the
  System Requirements section, and the Frame Rate table, even though Studio's `Connect()`/
  `ConnectWiFi()` have wired `internal/device`'s `EnsureIProxy`, `EnsureADB`/`ForwardPort`, and mDNS
  discovery for two releases — the exact same device-management code the CLI uses. FP-7 audited the
  actual backend/frontend behavior (confirmed via a full `wails build`), found the code path already
  complete, and rewrote the doc to match reality: new "Physical devices" section covering USB
  (`iproxy` for iOS, `adb forward` for Android) and WiFi (mDNS-discovered, preferred for iOS per this
  project's own USB-C-instability findings), plus a corrected Known Limitations list (the one real
  gap: the WiFi overlay only connects to mDNS-discovered devices, no manual host/port entry field).
  Added `studio/app_test.go` — the studio module previously had zero test coverage — covering the
  pure/testable surface (`isDeviceReady`, `extractLineCol`, file-path guards, `Lint`, `ListDir`,
  `Connect`/`ConnectWiFi` input validation).
- **`tap` could invoke a Semantics-wrapped button's `onTap` even when something else covered it
  on screen (FP-10, #265).** `_tryDirectTap` walks the Element tree structurally to find a
  `GestureDetector`/`InkResponse` descendant and calls its `onTap` directly (added for PT-04/PT-05,
  since a real hit-tested pointer tap doesn't reliably reach focus/`onTap` through a `Semantics`
  wrapper) — but that walk has no relationship to paint order, so it could fire on a button hidden
  behind a modal barrier, loading overlay, or an unrelated `Stack` sibling that a real user's tap
  would hit instead. `tap` now runs a read-only hit test (the same `hitTestInView` call Flutter's
  own pointer dispatch makes internally) before taking the direct-invoke path, and only uses it
  when the target is genuinely the topmost thing at its own screen position — otherwise it falls
  through to the existing real hit-tested pointer tap, which already lands on whatever's actually
  on top. `probe_agent/lib/src/executor.dart`; `probe_agent/test/tap_occlusion_test.dart`.

## [0.13.0] - 2026-08-15

### Added
- **`migrate_maestro` MCP tool.** The MCP surface audit found `probe migrate maestro` was the one
  CLI capability with no MCP representation — an AI client couldn't run migrations
  conversationally. The new tool shares the exact discovery + conversion code the CLI uses
  (recursive directory walk, source-layout mirroring, the full G-3 2.x syntax hardening), and
  `DiscoverYAMLFiles` moved to `internal/migrate` as its natural shared home. Live-verified
  through the MCP protocol against a real 76-flow suite: 76/76 converted.

## [0.12.1] - 2026-08-15

### Fixed
- **`toggle` never actually toggled anything, and `toggle #id` didn't even parse.** Found by the
  full-feature test campaign: the agent's `toggle` DeviceAction has always been a no-op
  (`case 'toggle': break;`), so every `toggle` step "passed" without touching the device — and the
  parser only accepted a bare ident or quoted string, leaving `#id` selectors dangling to misparse
  as a junk recipe call (the PT-26/R-5 class). `toggle` now parses a full selector (text, `#id`,
  ordinal) and dispatches it as a real tap — which is how a Switch actually toggles.
- **A recipe whose own name contains a filler word (e.g. `recipe "add and verify"`) was
  unreachable by its exact written name.** Filler stripping was applied to the call name only —
  `add and verify "x"` stripped to "add verify", which matched nothing because the definition
  kept its "and". Both sides now normalize identically before matching.
- **Android `set location` had never worked.** It ran `adb shell emu geo fix ...` — but `emu` is
  an emulator *console* command (`adb emu ...`), not a device-shell binary, so every call failed
  with "/system/bin/sh: emu: inaccessible or not found". Live-verified fixed against a real
  emulator.
- **iOS permission verbs left the session hanging until the step timeout.** `simctl privacy
  grant/revoke/reset` silently terminates the target app (confirmed live via launchctl), but the
  WebSocket lingered half-open with no error, so the next RPC hung for the full step timeout
  instead of failing fast enough to auto-reconnect. All four permission verbs
  (`allow`/`deny`/`grant all`/`revoke all`) now eagerly relaunch and reconnect on iOS simulators,
  mirroring `restart the app`. Live-verified: grant → wait → assert now completes in ~4s.
- **`wait N seconds` round-tripped through the agent as an RPC**, so `kill the app` followed by
  any duration wait hit the dead connection and burned the whole step timeout in doomed reconnect
  attempts (through an adb forward, "nothing listening" surfaces as accept-then-EOF rather than
  ECONNREFUSED, so the PT-18 relaunch heuristic never fired either). Duration waits now sleep
  CLI-side — waiting for wall-clock time needs no device. Live-verified: kill → wait → open now
  completes in ~20s end-to-end.
- **`close keyboard` printed "close the app" in progress output** (cosmetic).

## [0.12.0] - 2026-08-15

### Added
- **`retry N times` block.** Wraps an indented block of steps: on failure, re-runs the whole
  block from the top, up to N total attempts, stopping at the first success. Distinct from
  `repeat N times`, which always runs every iteration regardless of failure. Matches Maestro's
  `retry` block.
- **`optional` step modifier**, on `tap`/`type`/`long press`/`double tap`/`clear`/`see`. Unlike
  `if visible` (a pre-check that skips the step entirely when the target isn't found), `optional`
  always attempts the step — a failure is logged as a warning and swallowed instead of failing the
  test. Matches Maestro's `optional: true` step property.
- **Element-scoped visual regression: `compare screenshot "x" of "Widget"`.** Crops the screenshot
  to the widget's on-screen bounds (via the existing `probe.selector_bounds` RPC) before comparing
  — both the first-run baseline and every later actual image are the widget's region, not the full
  screen. Matches Maestro's `assertScreenshot` + `cropOn`.
- **`open link "url" in the app` / `... into the app`.** Routes the URL through the OS's own
  intent/URL handling (`adb shell am start -a android.intent.action.VIEW`, `xcrun simctl openurl`)
  instead of the Dart agent's `url_launcher`, so it actually opens inside the target app via a
  registered custom scheme rather than always launching an external browser. The existing
  `open link "url"` (no suffix) is unchanged. CLI-side only — skipped with a warning in cloud mode.
  Matches Maestro's `openLink` with app-targeting. Note: on iOS Simulator, `simctl openurl` does
  not cold-launch a fully-terminated app (it surfaces an "Open in App?" confirmation dialog it
  can't dismiss); it reliably works on an app that's already running. No such caveat on Android.
  See `docs/evidence/e3-deep-links-2026-08-15/`.
- **`add media "path/to/file.jpg"`.** Seeds a local file into the device's camera roll/gallery —
  `adb push` + `MEDIA_SCANNER_SCAN_FILE` broadcast on Android (confirmed MediaStore-indexed, not
  just written to disk), `xcrun simctl addmedia` on iOS. CLI-side only — skipped with a warning
  in cloud mode. Unblocks image-picker-adjacent flows that need an existing photo available to
  pick. Matches Maestro's `addMedia`. Note: this seeds the media store only — it does not drive
  the app's own native image-picker UI to select the photo, a Phase 2 (N-1/N-2) gap.
  See `docs/evidence/e4-add-media-2026-08-15/`.
- **`tap native "..."` / `see native "..."` / `type native "text" into "..."` (Android only).**
  Reaches outside the Flutter widget tree into native (OS-owned) UI — pickers, share sheets, and
  anything else the Dart agent can never see — matched by uiautomator's text or resource-id.
  CLI-side only (`uiautomator dump` + `input tap`/`input text`), dispatched through
  `DeviceContext`, no Dart RPC — skipped with a warning in cloud mode. Fixes the exact gap
  `add media` (E-4) called out as unclosed: a picker's contents can now actually be selected, not
  just seeded into the media store. iOS has no `uiautomator` equivalent — `tap`/`type native`
  error clearly rather than silently no-op'ing; `see native` reports "not found" (the honest
  answer). Matches Maestro/Appium-style native selectors, scoped to Android per
  `docs/proposals/pt13-native-ui-bridging.md`'s recommendation; iOS is N-2's own proposal.
  See `docs/evidence/n1-native-ui-android-2026-08-15/`.

### Fixed
- **A single Android transport drop no longer cascades into failing every remaining test
  (issue #237).** Two real bugs in the connection-error recovery path, found by tracing #237's
  exact failure signature through the executor: (1) the post-reconnect retry switch had silently
  drifted out of sync with the main step-dispatch switch — it was missing recipe calls,
  conditionals, loops, retry blocks, and HTTP calls, so a connection error surfacing from any of
  those could reconnect successfully but never re-run the step, burning the whole retry budget
  re-closing the connection each attempt had just re-established (both paths now share one
  dispatch function and cannot drift again); (2) `if X appears` treated a dead connection as
  "condition not visible" and silently took the else branch — misrouting navigation mid-recipe —
  instead of propagating the connection error for reconnect the way `tap ... if visible` already
  did. The underlying WS drop trigger from #237 remains under investigation (needs the reporting
  project's live Firebase load); these fixes change its blast radius from "rest of the run" to
  "one visible reconnect cycle". Also ruled out empirically: neither side's ping/keepalive
  machinery kills a busy-but-alive connection (four load-shape experiments, kept in the evidence
  folder). See `docs/evidence/i237-ws-drop-investigation-2026-08-15/`.
- **`probe migrate maestro` hardened against 2.x syntax and two real bugs (G-3).** Audited the
  converter against a real-world 76-flow Maestro suite: `setPermissions`, `retry` (with recursive
  nested-command conversion, also fixed for `repeat`), and `assertScreenshot` are now supported,
  and `relativePoint`-style `{point: "x%,y%"}` selectors get a clear `# TODO` instead of a real bug
  the old code had — silently emitting `tap on "map[point:50%,10%]"`, confirmed against an actual
  occurrence in the corpus. The corpus's own real gaps (not named in the original roadmap item, but
  27% of all step invocations in this specific suite) — `extendedWaitUntil`, `scrollUntilVisible`,
  `eraseText` — are now supported too. Also fixed: a stale `setLocation` "not supported" comment
  (it's been a real ProbeScript verb since before this cycle), and `probe migrate maestro <dir>`
  silently finding zero files for any suite organized into feature subdirectories — the real-world
  norm, including the social app's own — because directory discovery was single-level, not recursive.
  Every one of the 76 real flows converted and parses as valid ProbeScript.
  See `docs/evidence/g3-migrate-maestro-hardening-2026-08-15/`.
- **`dump tree`, `dump the widget tree`, and `save device logs` always misparsed as an unknown
  recipe call (R-5).** Both verb parsers called `skipFillers()` then `consumeNewline()` without
  ever explicitly consuming the trailing non-filler words ("tree", "widget tree", "device logs"),
  so those words were left dangling and misparsed as a second, stray step on the same line — the
  same class of bug `close the app`'s parser avoids by explicitly checking for its trailing
  keyword. Fixed by mirroring that pattern. See `docs/evidence/r5-dump-tree-save-logs-2026-08-14/`.
- **Android: the ProbeAgent's reconnection token lived in an OS-clearable app-cache directory and
  was only written once, at startup (PT-27).** Confirmed reproducible on a real device: the file
  disappeared permanently, mid-session, right after visiting a screen that touches `ImagePicker`
  — every subsequent reconnect attempt then failed with `probe token not found within 30s`, not
  because of any retry-window issue, but because the file itself was simply gone. The existing
  every-3-second token re-print now also re-attempts the file write, so a cleared cache dir gets a
  fresh copy back within seconds. See `docs/evidence/r3-android-token-cache-clear-2026-08-14/`.
- **Tap-family verbs (`tap`, `double tap`, `long press`, `clear`, `swipe`/`scroll` targets, and
  `drag ... to ...`) never resolved `<var>` placeholders in their selector — the still-open half
  of the PT-02 addendum.** `type`/`see` selectors were already routed through variable resolution
  before dispatch; every other selector-taking verb sent its selector's raw, unresolved text
  straight to the device, so `tap "<button_label>"` searched for the literal text
  `"<button_label>"` instead of the stored variable's value. Fixed by adding a single
  `resolveSelector()` helper and routing every selector-taking verb through it. Confirmed against
  a real Android build: the pre-fix binary fails with `Widget not found: text("<undo_label>")`;
  the fix resolves it correctly. See `docs/evidence/r1-tap-var-resolve-2026-08-14/`.
- **`see #field contains "..."` always failed on `TextField`/`TextFormField` selectors.**
  `_textOf` (in the Dart agent) only read `Text`/`RichText` widgets and returned `''` for
  anything else, so the check reported `contains "", not "<expected>"` no matter what the field
  actually contained. Fixed by reusing `_findTextController`'s existing up/down search (already
  used for tap-to-focus and `clear()`) to find the underlying `EditableText`'s controller.
  Confirmed with widget tests that fail against the pre-fix code and pass against the fix; see
  `docs/evidence/r4-textof-editabletext-2026-08-14/` (including an honestly-documented
  inconclusive real-device attempt, unrelated to this change).

## [0.11.0] - 2026-08-14

### Added
- **`see "<assertion>" with ai` — AI-powered visual assertions (Phase 1 of
  `docs/prd/ai-visual-assertions-prd.md`).** For checks that are hard to
  express structurally (does this screen look right, dynamic/generated
  content). Requires an `ai:` block in `probe.yaml` (`provider: openai |
  anthropic`, your own API key) — there is no default provider, and every
  call goes directly from the CLI to the provider you configure, never
  through a FlutterProbe-operated relay. Fails fast, before any device
  connection, if `with ai` is used without `ai:` configured. Supports
  `ai.redact` to black out named widgets' on-screen regions (via a new
  `probe.selector_bounds` agent RPC) before any screenshot is sent to a
  provider. See `docs/research/maestro-ai-assertions-investigation.md` for
  why this is BYO-key/direct-to-vendor by design, in contrast to Maestro's
  `assertWithAI`.
- **`assert no visual defects with ai` — Phase 2 of the same PRD.** A fixed
  "does this screen look broken" smoke check (cut-off/overlapping/
  mis-centered elements), reusing Phase 1's `ai:` config, redaction, and
  provider clients as-is — no new config fields, no new agent RPC. Unlike
  `see ... with ai`, `with ai` is mandatory here (no non-AI form).
- **`ai.provider: local` — Phase 3 of the same PRD.** Both AI-assertion
  commands can now run against any OpenAI-compatible local inference server
  (e.g. Ollama, LM Studio) via `ai.endpoint` — nothing leaves the
  device/host at all, not even to a BYO-key cloud vendor. `ai.api_key` is
  not required for this provider. Native on-device model support (Apple
  Intelligence, Gemini Nano), named in the PRD's design sketch, is
  explicitly **not** implemented — see `docs/prd/ai-visual-assertions-prd.md`
  §8 for why that's out of scope here rather than silently dropped.
- **`read "<query>" with ai into <var>` — Phase 4 (final phase) of the same
  PRD.** Extracts a specific piece of text off the current screen (an OTP
  code, a dynamically-generated ID) into a ProbeScript variable for use in
  later steps, e.g. `read "the 6-digit OTP code" with ai into otp` then
  `type <otp> into the "Code" field`. Fails with a clear error, rather than
  storing an empty/wrong value, if the requested text isn't visible. Same
  `ai:` config, redaction, and all three providers (`openai`/`anthropic`/
  `local`) as the other two AI commands — no new config surface. `with ai`
  is mandatory (no non-AI form), same as `assert no visual defects with ai`.
- **`ai.timeout` — configurable AI provider HTTP timeout (default: 60s).**
  Found by manually testing `ai.provider: local` (Phase 3) against a real
  LM Studio instance running a 31B local reasoning model:
  `assert no visual defects with ai`'s richer prompt consistently took
  longer than the previously-hardcoded 60s, and raising the ProbeScript
  step timeout alone didn't help — the HTTP client's own timeout, not the
  step timeout, was the actual binding constraint. Applies to all three
  providers; matters most for slow local models.

### Fixed
- **Android connect race: `websocket: bad handshake` on first dial** — the CLI
  dialed immediately after creating the adb port forward, but adb accepts the
  host-side TCP connection before the device-side socket is plumbed, so the
  WebSocket upgrade could fail mid-handshake. `bad handshake` (and `EOF`) were
  treated as fatal protocol errors, bypassing the retry loop and ignoring
  `--dial-timeout`. Both are now retried within the dial-timeout window; a real
  HTTP 401/403 token rejection from the agent still fails immediately.

## [0.10.4] - 2026-07-06

### Fixed
- **A hung reconnect attempt could leave the CLI silently stuck with no way
  to recover other than manually interrupting it (part of PT-25).** The
  auto-reconnect retry loop passed the *outer*, per-run context into each
  reconnect attempt — that context has no deadline of its own (only
  Ctrl-C/SIGTERM cancels it), unlike the per-step timeout the CLI otherwise
  honors. If a device call during reconnect (e.g. an `adb` command) ever
  hung, there was nothing to force it to give up. Each reconnect attempt is
  now bounded by the same step timeout the CLI already uses everywhere
  else. This addresses one confirmed, independently-reproducible
  contributing factor reported alongside PT-25's WebSocket-drop
  investigation; the drop itself is still open — see PT-25 in
  `IMPROVEMENT_TASKS.md` for what's resolved vs. still outstanding.
- **`tap 1st #id` (an ordinal modifier combined with an id-selector) always
  misparsed (PT-26).** The parser's ordinal-selector branch only checked for
  quoted text or a bare identifier after the ordinal, never an `#id` token —
  so `tap 1st #post_list_card` left `#post_list_card` completely unconsumed,
  and it misparsed as a second, stray, unknown recipe call. Fixed by
  accepting an id-selector after an ordinal too. Since Flutter itself
  enforces unique `Key`s among direct siblings, disambiguating repeated
  same-id rows by position also required a matching fix on the Dart agent
  side: the `ordinal` selector kind was hardcoded to always match by
  displayed text, never by id — now it checks for the `#` prefix (matching
  the plain `id` selector kind's existing convention) and matches by key
  instead when present.

## [0.10.3] - 2026-07-06

### Fixed
- **A recipe named starting with the word "open" (e.g. `open most recent
  post`) always misparsed (PT-23).** `open` is a reserved keyword for the
  built-in `open the app`/`open link` verbs, and the parser claimed any
  step starting with it unconditionally — an undocumented fallback then
  swallowed the next bare word as a selector and left the rest of the line
  to misparse as a second, stray, unknown recipe call. Fixed by only
  treating `open` as the built-in verb when what follows actually matches
  one of its two documented forms (`the app`/`app`, or a link) via a
  backtracking lookahead; anything else now falls through to a normal
  recipe call, preserving the whole phrase as the call's name.
- **A recipe with a hyphenated name (e.g. `create looking-for post`)
  always misparsed (PT-24).** The lexer tokenized a word-internal hyphen
  as its own standalone token unconditionally — fine for the recipe
  *definition* (a quoted string, hyphen preserved as-is), but at the
  *call* site the recipe name is built by rejoining bare word tokens with
  spaces, so the hyphen came back out padded (`"looking - for"`), and the
  two representations could never match. Fixed by keeping a hyphen that's
  directly between two word characters part of the same identifier token.
  A standalone hyphen (e.g. the negative sign in `set location -33.8,
  151.2`) is unaffected, since it's only ever reached preceded by a space,
  never mid-identifier.

## [0.10.2] - 2026-07-06

A small release with no functional changes — a reported regression was
re-investigated and re-closed, with a regression test added to lock in the
verified-correct behavior going forward.

### Testing
- **Locked in sequential tap+type behavior when one field auto-focuses on
  page load (PT-21, reopened and re-closed).** A report claimed sequential
  multi-field text entry breaks specifically when a field requests focus
  during `initState`/first frame (e.g. a "remember last email" convenience
  feature) — text supposedly keeps landing in that auto-focused field
  regardless of which field is tapped afterward. Re-investigated with the
  exact discriminator described: a widget test with one field auto-focusing
  via a post-frame `requestFocus()`, plus real-device verification against
  the actual login screen with the same pattern temporarily added, in both
  tap orders. Both pass cleanly — focus correctly shifts to whichever field
  is tapped, and each field retains only its own typed value. Added a
  regression test locking this in.

## [0.10.1] - 2026-07-05

A quick patch release for regressions surfaced during v0.10.0's own
verification pass, plus an unrelated CI-automation fix found along the way.

### Fixed
- **The daily Dependabot auto-merge workflow never actually merged anything,
  for months.** Its skip condition treated `mergeStateStatus == "UNKNOWN"`
  as "already in the merge queue" — but `UNKNOWN` just means GitHub hasn't
  finished computing mergeability yet (the common case on a fresh poll), not
  "already queued." Every eligible PR was silently skipped every single day
  since at least mid-May, despite the workflow itself reporting "success."
  Separately, even a PR that had passed that check would have hit a second,
  compounding bug: the merge command itself (`gh pr merge --squash`, no
  `--auto`) doesn't work for a merge-queue-gated branch — it just prints a
  warning and does nothing, the same issue found and fixed earlier this
  release for this repo's own CI (see PT-19-adjacent commit history). Fixed
  both: only skip on `DIRTY`/`BLOCKED` (states that definitively block a
  merge), and let a merge attempt happen and fail on its own terms
  otherwise; added `--auto` to the merge command. Verified by manually
  invoking the corrected logic against the real backlog — 9 previously
  stuck PRs (some open since May) are now genuinely in the merge queue.
- **`wait for network idle`/`wait for the page to load`/`wait for page to
  load` always misparsed, silently splitting into a no-op plus a stray,
  broken statement (PT-20).** `"for"` (and `"the"`) are both global filler
  words that `parseWait`'s initial filler-strip consumes immediately after
  `"wait"` — by the time the code checked for `"for"` specifically to
  distinguish this verb family, it was already gone, so that check could
  never match. The parser fell through to a default case that produced a
  `WaitPageLoad` step (the right kind, by coincidence) but never consumed
  "network idle"/"page to load" — those leftover words became a second,
  separate statement that then failed at runtime as an unknown recipe
  call. Invisible to `probe lint`, since both halves parse as individually
  valid syntax; only surfaces when the test actually runs. This bug has
  existed since the project's very first commit — not a regression from
  any recent release, despite surfacing during this release's own
  verification pass. Fixed by detecting the network/page-load cases
  directly (mirroring the pattern `wait for animations to end` already
  used for the same underlying issue) and consuming the full phrase before
  the next token is checked.
- **`.github/workflows/e2e.yml`'s CI E2E suite failed `flutter pub get` outright
  on every run** (`"name" field doesn't match expected name "probe_agent"`).
  The workflow's `dependency_overrides` block still referenced the package's
  old pre-rename name (`probe_agent`) instead of `flutter_probe_agent` — a
  stale reference to a rename that happened well before this release. Found
  when the `v0.10.0` tag push triggered this workflow directly (it isn't
  gated on regular PRs, only tag pushes, which is why it went unnoticed
  through several prior releases). Unrelated to this release's own changes;
  fixed by updating the override key to the current package name.

## [0.10.0] - 2026-07-05

A hardening release working through a backlog of real E2E test issues
surfaced by driver projects (`IMPROVEMENT_TASKS.md`, PT-01 through PT-19).
Every fix was reproduced and verified against a real device before shipping,
and several turned out to have a different (or larger) root cause than
originally reported — noted inline below.

### Added
- **`agent.launch_timeout` config option (and `--launch-timeout` flag)
  (PT-10).** `restart the app`/`clear app data` used to be bounded by a
  hardcoded, unconfigurable 90s step timeout — no amount of raising
  `dial_timeout`/`token_read_timeout` could help an app whose actual
  cold-launch path does non-trivial async work (e.g. Firebase App Check
  re-initialization costing 90-100s) before becoming interactive. Defaults
  to 120s; raise it for apps with an expensive startup path.
- **Verbose connect diagnostics (PT-01).** `-v`/`--verbose` on `probe test` now
  traces every step of the connect handshake — ADB setup, port forward
  setup/teardown, each Android token-read source attempted (run-as,
  `/data/local/tmp`, logcat) with hit/miss detail, WebSocket dial attempts
  (including transient retries), and handshake accept/reject. Connect
  failures were previously a total black box with zero diagnostic output;
  see PT-01 in `IMPROVEMENT_TASKS.md`.
- **CLI↔agent version handshake (PT-07).** `probe.ping` now carries
  `client_version` (CLI → agent) and `agent_version` (agent → CLI) alongside
  the existing `{"ok":true}` response. Every connect path (WebSocket dial,
  HTTP dial, relay, and reconnect-after-restart) now logs a warning when the
  CLI and agent versions differ, and hard-fails with a clear error when they
  have different major versions. Older CLIs/agents that don't send or
  recognize these fields degrade gracefully — an empty/missing version is
  always treated as "unknown," never as a mismatch. Addresses PT-07 in
  `IMPROVEMENT_TASKS.md` — CLI/agent version drift was a standing suspect in
  unexplained connection failures with zero signal from the tool itself
  until now.

### Changed
- **ProbeScript now errors loudly instead of silently no-oping on several
  classes of malformed script (PT-02):**
  - An **unknown recipe call** (typo, or the recipe was never defined/isn't
    loaded from `recipes_folder`/`use`) is now a runtime error, not a silent
    skip. This was the single highest-leverage fix in `IMPROVEMENT_TASKS.md`
    — a silently-skipped call could mask a completely broken flow (e.g. a
    sign-in recipe that never actually signs anyone in) indefinitely, with
    every downstream test still reporting green.
  - An **unquoted `<placeholder>`** (e.g. `type <email>` instead of
    `type "<email>"`) is now a parse error. Angle brackets have no meaning
    outside a quoted string in ProbeScript's grammar; unquoted, both brackets
    were silently dropped by the lexer, leaving a bare identifier that gets
    typed/matched as literal text with no indication anything was wrong.
  - **`else` is now accepted as an alias for `otherwise`.** Previously `else`
    lexed as a plain identifier, was silently treated as an unknown recipe
    call (a sibling step of the `if`, not nested inside it), and its body ran
    **unconditionally** on every run regardless of the `if` condition —
    exactly the opposite of what the test author intended, with no error
    anywhere.
  - `resolve()`'s placeholder-substitution loop is now bounded. A variable
    bound to a value containing its own placeholder marker (e.g. passing the
    unquoted literal `<email>` as a recipe argument) previously looped
    forever substituting the same text for itself, hanging the CLI with no
    error; it now terminates, leaving the placeholder unresolved. (Full
    positional/named argument forwarding into nested recipe calls — a
    larger, separate redesign of how recipe calls are matched against
    definitions — is intentionally not part of this change; see PT-02's
    "Update" note in `IMPROVEMENT_TASKS.md`.)

### Fixed
- **`drag <selector> to <selector>` — the documented syntax — always failed
  to parse (PT-19).** `"to"` was lexed as its own token but was never
  actually consumable anywhere: it was missing from the parser's
  filler-word list and used nowhere else in the grammar. The parser choked
  on `to` where it expected the second selector to start, and the rest of
  the line got misparsed as an unrelated recipe call. Found during a final
  regression pass across the full e2e suite before this release — this bug
  has always existed; PT-02's error-loudly fix (above, same release) is what
  first surfaced it, since it previously failed silently instead.
- **`kill the app` followed by any step other than `open the app`/
  `restart the app` (a `wait`, `tap`, `see`, etc.) still hung and then
  permanently failed (PT-18, follow-on from PT-09's `open the app` fix).**
  The generic step-level auto-reconnect only re-dials, assuming the app
  process is already running — after a genuine `kill the app`, nothing is
  listening at all, so re-dialing could never succeed regardless of
  remaining attempts. Detects a connection-refused dial failure
  specifically (as opposed to a timeout/reset on a still-alive process,
  which a transient network drop would produce) and relaunches the app
  before retrying, the same way the `open the app` fix does, but from the
  generic reconnect path so it now covers every verb, not just `open`.
- **`open the app` after `kill the app` never actually relaunched the app
  (PT-09).** It always sent an RPC over the (now-closed) connection, which
  failed; the generic step-level auto-reconnect that kicked in afterward
  only re-dials assuming the app process is already running — it never
  relaunches one that was genuinely force-stopped. In practice this meant
  the documented `kill the app` → `open the app` pattern hung through the
  full reconnect-retry window and then failed with a connection-refused
  error. `open the app` now detects a dead connection and relaunches the
  app the same way `restart the app` does, before reconnecting. Found
  while documenting cross-`test`-block state behavior (see Documentation
  below) — not what that investigation originally set out to find, but
  blocking anyone who'd try to use `kill the app`/`open the app` for
  exactly the per-test isolation those docs describe as the supported way
  to opt out of the default shared-state behavior.
- **`take screenshot` could capture stale content from the previous route
  instead of the current screen (PT-16).** Found while investigating a
  scroll bug (PT-03): a screenshot taken right after navigating sometimes
  looked unchanged, even though `see`/`don't see` assertions confirmed real
  navigation had happened. Two independent causes:
  - Unlike every other verb, `screenshot` never called
    `_sync.waitForSettled()` before capturing — a capture taken right after
    navigation could land mid-route-transition instead of waiting for the
    push/pop animation to finish.
  - `_captureViaRepaintBoundary` picked the largest `RenderRepaintBoundary`
    in the *entire* element tree with no route-awareness — since `Navigator`
    keeps the previous route mounted underneath the current one, and both
    routes typically produce a same-size, screen-sized boundary, the strict
    `area > bestArea` comparison kept whichever one was visited first (the
    previous route, in `Overlay` insertion order), silently capturing the
    old screen instead. Same class of bug as PT-03/PT-15, just in the
    screenshot path instead of `ProbeFinder`/`scroll`. Fixed by skipping
    boundaries belonging to a non-current route, mirroring `ProbeFinder`'s
    existing fix.
- **`scroll` could lose the gesture arena to `Dismissible`-wrapped list rows
  and never actually scroll (PT-15).** `scroll` was a thin delegate to
  `swipe`'s pointer-gesture simulation, which has to *win* the gesture arena
  against any competing recognizer along the way — a `Dismissible` row's own
  `HorizontalDragGestureRecognizer` could still intercept it. Reproduced
  against a real iOS simulator: a 50-item list with `Dismissible` rows never
  scrolled past the first screen, while the identical verb worked fine on a
  plain list. `scroll`'s job is "reveal more content," unlike `swipe` (which
  tests a real gesture interaction like swipe-to-dismiss) — it doesn't need
  to enter the gesture arena at all, so it now drives the nearest
  `Scrollable`'s own `ScrollPosition` directly instead, sidestepping the
  competition entirely. When no selector is given, picks the Scrollable with
  the largest viewport (a `TextField`'s own internal cursor-scrolling
  `Scrollable` could otherwise be found first in tree order and silently
  "scroll" nothing visible).
- **`scroll`/`swipe` could report success while producing zero visible
  movement (PT-03).** Root-caused two independent bugs by reproducing
  against a real iOS simulator:
  - The synthetic drag gesture sent a single `PointerMoveEvent` covering the
    entire distance in one jump. Real touches (and Flutter's own gesture
    arena / scroll physics) expect a sequence of incremental moves — a
    single jump could fail to register as a scroll at all. Now split into
    10 incremental steps, matching a real drag.
  - The widget-tree finder had no concept of "which mounted route is
    actually the current one." Flutter's `Navigator` keeps previous routes
    mounted underneath the current one by default (no `Offstage` wrapper),
    so a screen reached via a stacked push could have several live
    `Scrollable`s (and other matching widgets) simultaneously — one per
    mounted route — and every selector-based verb (not just scroll/swipe)
    could silently resolve to a widget on a route the user can no longer
    see. Fixed by checking `ModalRoute.of(element)?.isCurrent` in the
    finder's core visibility check, so this also fixes false-positive
    `see`/`wait until` matches against stale content underneath the current
    screen — confirmed via a real-device repro where `don't see "<home page
    text>"` incorrectly found 2 elements after navigating away from Home,
    and now correctly finds none.

  A `scroll until #id visible` verb (combining corrected scroll targeting
  with polling) was in PT-03's original scope but is intentionally not part
  of this change — it's a larger, separate feature addition.
- **`close keyboard` and `close the app` were both complete no-ops (PT-12).**
  Both parse to the same `ActionStep` (`close`/`close keyboard`/`close the
  app` only differ in an argument name) and dispatch to
  `probe.device_action` with `action:"close"` — but the Dart agent's
  `_deviceAction` switch never had a `'close'` case at all, so neither did
  anything, silently. (The originally reported theory — an OS-level gesture
  colliding with iOS's Back-swipe — wasn't the actual cause: there was no
  gesture-based implementation in the first place.) Fixed by adding the
  missing case: `close keyboard` now calls
  `FocusManager.instance.primaryFocus?.unfocus()` directly in the Flutter
  widget tree (immune to any OS gesture collision, per the original PT-12
  suggestion), and `close the app` calls `SystemNavigator.pop()`.
- **The `focused` state check (`see`/`don't see #id is focused`) had a
  false-positive: it also matched *ancestors* of the selected element**, not
  just the element itself or its descendants. Once nothing more specific is
  focused (e.g. immediately after `FocusNode.unfocus()`), Flutter falls back
  to the enclosing `ModalRoute`'s own `FocusScopeNode` holding primary focus
  — and that scope is an ancestor of every widget on the current screen, so
  the old ancestor walk reported *all of them* as focused. Found while
  verifying the `close keyboard` fix above: `don't see #field is focused`
  kept failing right after a real, successful unfocus. Fixed by removing the
  ancestor walk, keeping only the direct match and the subtree walk (which
  already correctly handles a selector matching a composite widget like
  `TextField`, not the `EditableText` it builds internally).
- **`wait until #id appears`/`disappears` always searched for the literal
  text `"#my_button"` instead of resolving the id (PT-06).** `WaitStep`
  only carries a raw target string, not a selector kind (unlike
  `Selector`/`SelectorParam` used by `tap`/`type`), and the Dart agent's
  wait loop never checked for the `#` prefix before building a selector —
  it always built a *text* search, which can never match a non-text widget
  like an icon button. This meant `wait until #id appears` timed out on
  indisputably mounted, visible widgets every time, forcing real projects
  to fall back to hardcoded `wait N seconds` sleeps instead. Fixed by
  detecting the `#` prefix and dispatching an id selector, mirroring the
  same pattern `if`/`otherwise` conditionals already use. (The originally
  reported theory — that nesting inside `Material`/`Tooltip` breaks
  Semantics-based resolution — didn't independently reproduce: id
  resolution walks the full element tree unconditionally regardless of
  wrapper widgets, and a plain text search never touches Semantics at all,
  so nesting depth was never actually the cause.)
- **`tap #id`'s fast direct-invoke path only recognized `GestureDetector`/
  `InkWell`, missing `InkResponse`-based buttons (PT-05).** `InkWell` is
  just an `InkResponse` subclass with a fixed splash shape, and modern
  Material buttons (`IconButton`, `ElevatedButton`, etc.) commonly build an
  `InkResponse` directly — the old check missed them, always falling
  through to the slower synthetic-tap fallback. Verified (with new tests)
  that PT-05's actual reported symptom — `tap #id` on a button with no
  `onTap` `SemanticsAction`, or shadowed by an overlapping Semantics node —
  already worked correctly via that fallback: `Semantics` doesn't
  participate in hit-testing at all, so a real synthetic tap at the node's
  geometric center reaches whatever is actually rendered there regardless
  of Semantics-tree structure. No fix was needed for that path itself; this
  change only broadens the faster path to cover more cases before falling
  back to it.
- **`tap #id` could report success while leaving a text field genuinely
  unfocused (PT-04).** A real pointer tap on a text field requests focus as
  part of `EditableText`'s own internal tap handling; probe's tap paths
  (both the Semantics-direct-invoke fallback and the synthetic pointer tap)
  don't reliably reach that internal recognizer — a Semantics wrapper or a
  surrounding `GestureDetector`/`InkWell` can intercept the tap first. Now
  `tap #id` (and `type` when used without a preceding `tap`) explicitly
  requests focus on the field's real `FocusNode`, found by resolving down
  to the underlying `EditableText` the same way `type` already resolves its
  `TextEditingController`.
  - `see #id is focused` could never actually detect focus on a
    `TextField`/`TextFormField` for the same underlying reason: the check
    only walked *ancestors* looking for the focused widget, but the
    actually-focused widget (`EditableText`) is a *descendant*. Fixed to
    also walk down the subtree.
  - `don't see #id is focused` (and other negated state checks — `enabled`,
    `disabled`, `contains`) silently ignored the state entirely, reporting a
    failure as soon as the element existed at all. Now correctly passes when
    the element exists but isn't in that state.
- **Added regression test coverage confirming text selectors already resolve
  `ListTile` title/subtitle independently (PT-11).** The reported symptom —
  a title text selector failing on iOS 26.3+ because "the OS accessibility
  layer merges the title and subtitle into one combined node" — describes
  real platform (VoiceOver/XCUITest) behavior, but doesn't apply to probe:
  `_findByText` walks the live Flutter element tree directly and never
  touches the platform accessibility tree. `ListTile` builds title/subtitle
  as fully independent `Text` widgets with no merging anywhere in the
  element tree, so each already resolves correctly regardless of platform.
  No code fix was needed; added a test locking in this behavior since it
  wasn't previously covered.
- **`probe test` could fail with zero diagnostic output (PT-14).** `testCmd`
  sets `SilenceErrors: true` so a failed *test* (already reported in detail
  by the runner) doesn't print a redundant generic line — but this silenced
  every other kind of error too (token read, connect, handshake failures),
  leaving nothing printed beyond whatever progress lines ran before the
  failure. Now only `errTestFailed` (and errors wrapping it) stay silent;
  every other error is printed. Also fixed a related cosmetic bug this made
  visible: the "is the app running with probe_agent?" suggestion was
  appearing twice in token-read failure messages (once from the low-level
  error, once from the wrapping call site).
- **Android token read could pick up a stale token from a dead process,
  causing an immediate non-retryable connection failure (PT-01).** `logcat -d`
  dumps the entire ring buffer since it was last cleared/the device booted;
  on any device that's run more than one probe session it can contain
  `PROBE_TOKEN=` lines from multiple app-process generations, including
  already-exited ones. The token-read fallback took the *first* matching
  line, which could belong to a dead process — the live agent then rejects
  that token with a non-retryable "bad handshake." Fixed to take the *last*
  (most recent) matching line instead, since the agent reprints its token
  every ~3s. Reproduced and confirmed fixed against a real Android emulator
  — a leftover process from an earlier test run was still present in the
  log buffer, and the CLI connected successfully once the fix was applied.
  This is a confirmed root cause for at least one of PT-01's two reported
  failure shapes ("instant exit ~0.5-0.6s"); the other shape (hangs the
  full configured timeout) was not reproduced in this environment and
  remains open — the tracing above should make it diagnosable if it recurs.
- **Android `probe test` never passed the app's bundle ID into token
  reading (PT-01)**, so the fastest, most reliable token source (`run-as
  <appID> cat cache/probe/token`) was silently dead code in the main
  `probe test` path — only the slower `/data/local/tmp` and `logcat`
  fallbacks ever ran. Now threads `project.app` through.
- **`flutter_probe_agent`'s reported version had drifted to 0.7.0** while
  `pubspec.yaml` moved on to 0.9.9 across several releases — the agent's
  mDNS advertisement and `GET /probe/status` endpoint were both silently
  reporting a stale version. Fixed to match `pubspec.yaml`, and moved into
  its own `agent_version.dart` file with a comment documenting that it must
  be bumped by hand alongside `pubspec.yaml` going forward.

### Documentation
- **Clarified that all `test` blocks (and hooks) in a `.probe` file share
  one continuous app instance and connection by default — nothing resets
  app/session state between them (PT-09).** Investigated the reported
  symptom (a later test failing as if session state had been silently
  reset) and it doesn't reproduce: there is no code path that resets
  anything between blocks. Documented this explicitly in `hooks.md`, along
  with how to opt into per-test isolation (`restart the app`/
  `clear app data`/`kill the app` in `before each`) for projects that want
  it. Also corrected an inaccurate claim in `app-lifecycle.md` that
  `open the app` always launches via ADB/simctl "not through the Dart
  agent" — true only when there's no live connection yet; otherwise it's a
  no-op RPC to the already-running agent.
- **Documented an API stability/deprecation policy for `flutter_probe_agent`'s
  public Dart API (PT-08)**, in `CONTRIBUTING.md`. Prompted by a past
  breaking change (a minor bump silently removed `ProbePlugin`/
  `ProbePluginRegistry`, which at least one downstream project had a
  load-bearing test feature built on). Future removals/breaking changes now
  require a `@Deprecated` window of at least one minor version plus an
  explicit CHANGELOG migration note before actual removal. Whether
  `ProbePlugin`/`ProbePluginRegistry` should be reintroduced under this
  policy, or that removal should stand as a permanent scope decision, is
  left as an open product question rather than resolved here.
- **Documented the native (non-Flutter) UI boundary explicitly (PT-13)**:
  image/file pickers, share sheets, and the handful of permission prompts
  that can't be bypassed by an OS-level grant are invisible to every
  selector-based verb, by design — probe drives the Flutter widget tree via
  an in-process Dart agent, and native OS UI never enters it. Noted that
  `take screenshot`/video recording already capture this content (the full
  physical screen, not just the Flutter view) even though no verb can
  select or tap inside it yet, and documented the current workarounds
  (design around it, or a test-only in-app bypass). A design proposal for a
  full native-UI bridging mode was written up separately, not implemented
  in this release.

## [0.9.9] - 2026-05-13

### Added
- **`deliver signal "name" ["value"]`** — new ProbeScript step that resolves
  a pending `awaitSignal(name)` call in the Flutter app. Use to unblock any
  OS-level interaction that isn't in the Flutter widget tree: push permission
  dialogs, payment sheets, App Tracking Transparency, deep-link handlers, etc.
  The value defaults to `"true"` when omitted.
- **`awaitSignal(String name)`** — new public function exported from
  `flutter_probe_agent`. Returns a `Future<String>` that resolves with the
  value sent by the CLI. Generalises the `awaitBiometricResult()` pattern to
  any named signal.
- **`DeliverSignal(String name, {String value})`** — new annotation step class
  in `flutter_probe_annotation`. Emits `deliver signal "name"` or
  `deliver signal "name" "value"`.
- Parser: `TOKEN_DELIVER`, `TOKEN_SIGNAL`, `VerbDeliverSignal`.
- Agent: `probe.signal` JSON-RPC method, `ProbeMethods.signal` constant.

## [0.9.8] - 2026-05-12

### Fixed
- **Biometric no-match on iOS 26+ simulator** — `notifyutil` no-match notifications
  no longer resolve `LAContext.evaluatePolicy` on iOS 26 / Xcode 26.5. The CLI now
  sends a `probe.biometric_signal {result: bool}` JSON-RPC command to the agent
  after firing platform-level notifications. Test apps call `awaitBiometricResult()`
  from `flutter_probe_agent` instead of `local_auth.authenticate()` in PROBE_AGENT
  builds; the agent resolves a Dart `Completer` with the CLI-delivered result, making
  biometric no-match reliable on all iOS simulator versions.

### Added
- **`awaitBiometricResult()`** — new public function in `flutter_probe_agent`.
  Returns a `Future<bool>` that resolves when the CLI delivers
  `probe.biometric_signal`. Usage pattern in test apps:
  ```dart
  final ok = const bool.fromEnvironment('PROBE_AGENT')
      ? await awaitBiometricResult()
      : await localAuth.authenticate(...);
  ```
- **`probe.biometric_signal`** — new JSON-RPC method. Sent by the CLI after
  the platform-level biometric simulation commands so the result is always
  delivered regardless of simulator version behavior.

## [0.9.7] - 2026-05-12

### Added
- **Biometric authentication testing** — three new ProbeScript steps that drive Face ID / Touch ID / fingerprint flows on iOS Simulator and Android emulator without real hardware. Skipped on physical devices with a warning (same pattern as `set location` and other simulator-only ops).
  - `enroll biometric` — marks the simulator/emulator as having an enrolled face or finger. iOS posts the `com.apple.BiometricKit.enrollmentChanged` Darwin notification via `xcrun simctl spawn booted notifyutil`. Android requires the fingerprint to be pre-enrolled in Settings.
  - `biometric match` — simulates a successful capture, satisfying any pending biometric prompt. iOS posts `*_Sim.faceCapture.match` AND `*_Sim.fingerTouch.match` so the same step works on Face ID and Touch ID devices. Android runs `adb -s <serial> emu finger touch 1`.
  - `biometric no match` — simulates a failed capture so the app's "authentication failed" path can be tested. iOS posts the `.no-match` variants; Android runs `adb emu finger touch 9999` (an unregistered id).
  - **Annotation DSL**: matching `EnrollBiometric()`, `BiometricMatch()`, `BiometricNoMatch()` const Step classes in `flutter_probe_annotation`, with a new `biometric_auth` golden fixture in `flutter_probe_gen/test/fixtures/` that round-trips through the Go parser via the cross-language integration test.
  - **Parser**: 2 new tokens (`TOKEN_BIOMETRIC`, `TOKEN_ENROLL`), 3 new `ActionVerb` constants, 2 new parser dispatch cases. 3 new unit tests in `parser_test.go`.
  - **Runner**: `EnrollBiometric` / `BiometricMatch` / `BiometricNoMatch` methods on `DeviceContext`, dispatch cases in `Executor.runAction`, and human-readable strings in `stepDescription`.
  - **Docs**: new section in [annotations.md](https://flutterprobe.dev/probescript/annotations/#biometric-authentication-v097) and [syntax.md](https://flutterprobe.dev/probescript/syntax/#biometric-authentication) on the website. Per-package CHANGELOGs updated.

## [0.9.6] - 2026-05-12

### Fixed
- **`flutter_probe_gen`: `Mock` path silently truncated.** The emitter wrote the path unquoted (`when the app calls GET /api/products`), so the Go lexer split on `/` and the parser only recorded the first IDENT segment. Now emits the canonical quoted form. Caught by a new `mock_and_call` golden + the existing cross-language integration test.
- **`flutter_probe_gen`: `See` suffixes silently dropped.** When `state`, `containing`, and `matching` were all set on a single assertion, only the last branch's text reached the output. Now composes all three suffixes additively: `see "x" is enabled contains "y" matching "z"`. Caught by a new `see_states` golden covering the matrix.

### Added
- **`flutter_probe_annotation`: `@ProbeCompositeTest` annotation.** The flagship multi-device composite testing feature finally has a DSL surface. Pair with `Device(alias, target: …)`, `OnDevice(alias, steps: […])` per-device groups, and `Sync(label)` cross-device barriers. Emitter generates standard `composite test` / `devices` / `<alias>:` / `sync` blocks that the existing CLI runner picks up unchanged.
- **`flutter_probe_annotation`: `See.id` / `See.selector` factories** — assertions can now target by `ValueKey` or any rich selector (Ordinal, Below/Above/LeftOf/RightOf, InContainer, TypeSel) rather than only by literal visible text. Same factories on `DontSee`. The Go parser always supported this; the DSL just didn't expose it.
- **`flutter_probe_annotation`: `WaitUntil.idAppears` / `.idDisappears`** — emits unquoted `#key` selector form (Go parser's WaitSelector branch), which is more reliable than text matching for stable `ValueKey`-tagged widgets.
- **`flutter_probe_gen`: 6 new golden fixtures.** `mock_and_call`, `see_states`, `composite_chat`, `wait_variants`, `examples_inline`, `kitchen_sink`. The kitchen sink fixture exercises one of every step, selector kind, and control-flow construct. Every fixture round-trips through `internal/parser/golden_integration_test.go`. Total golden coverage went from 4 → 10 fixtures, builder tests from 5 → 11.

### Changed
- **`flutter_probe_annotation`: `Press` and `Pinch` are now `@Deprecated`.** The Go parser has no `press` or `pinch` case, so emitted text fell through to `parseRecipeCall` and was misinterpreted. Marked deprecated until runtime support lands. Use `GoBack()` in place of `Press('back')`.
- **`flutter_probe_gen`: emitter no longer coupled to enum declaration order.** `_direction`, `_httpMethod`, and the `See` state lookup now read the enum constant identifier (`_name` field) instead of indexing a hard-coded array by `.index`. Reordering `Direction`, `HttpMethod`, or `SeeState` no longer silently corrupts emitted ProbeScript.

### Docs
- New website page: [Annotation-driven Tests](https://flutterprobe.dev/probescript/annotations/) — full reference for the annotation DSL with every step class, selector kind, and the new composite test syntax.

## [0.9.5] - 2026-05-12

### Fixed
- **Dart agent: iOS / Impeller screenshots** — `take_screenshot` previously called `OffsetLayer.toImage()` on the root render view. On iOS with the Impeller renderer (Flutter's default on iOS 17+), that returns a GPU-backed texture whose `toByteData(ImageByteFormat.png)` is `null`, so capture silently produced nothing. The agent now primarily captures via the largest visible `RenderRepaintBoundary` in the widget tree (Impeller-supported); the legacy `OffsetLayer` path is only used as a fallback when no boundary is found (Skia). Also awaits `WidgetsBinding.instance.endOfFrame` before capture so the latest frame is always in the image, and uses the actual `View.devicePixelRatio` rather than a hard-coded `2.0`.

## [0.9.4] - 2026-05-09

### Added
- **Claude Desktop Extension (`.mcpb`)** — `probe-mcp` is now packaged as a one-click Claude Desktop Extension. Users drag `flutter-probe-<platform>-<arch>.mcpb` onto Claude Desktop's Extensions settings, pick their Flutter project directory, and all 18 MCP tools are available. No `brew install`, no JSON config editing, no PATH setup. CI builds and attaches `.mcpb` artifacts for darwin-arm64, darwin-amd64, linux-amd64, and windows-amd64 to every release.
  - Manifest declares `server.type: "binary"` and bundles the platform-specific `probe-mcp` executable.
  - `user_config.projectRoot` is a directory picker surfaced as the `PROBE_PROJECT_DIR` env var.
  - `probe-mcp` reads `PROBE_PROJECT_DIR` at startup and `os.Chdir`s into it so `run_tests`, `list_files`, `get_report`, and `init_project` resolve paths against the user's Flutter project rather than Claude Desktop's working directory.
  - `scripts/build-mcpb.sh` produces a bundle locally from any built `probe-mcp` binary; uses `npx @anthropic-ai/mcpb` for schema validation and packing.
  - New `build-mcpb` job in `.github/workflows/release.yml` runs after `build`, downloads the per-platform `probe-mcp` artifacts, packs them into `.mcpb` bundles, and attaches them to the GitHub release.

## [0.9.3] - 2026-05-09

### Added
- **Annotation-driven test generation** — two new Dart packages, `flutter_probe_annotation` and `flutter_probe_gen`, let you declare ProbeScript end-to-end tests as decorators on your Flutter screen classes. A `build_runner` builder reads the annotations at build time and emits matching `.probe` files into `tests/generated/`, picked up unchanged by `probe test`.
  - **Annotation API**: `@ProbeSuite`, `@ProbeTest`, `@ProbeRecipe` plus a fully type-checked step DSL covering all 31 ProbeScript action verbs (Tap, Type, See, Wait, Swipe, Scroll, Drag, Restart, Kill, ClearAppData, permissions, clipboard, location, screenshots, etc.), all 6 selector kinds (text, id, type, ordinal, positional, relational), hooks (`beforeEach`, `afterEach`, `beforeAll`, `afterAll`, `onFailure`), loops (`Repeat`), conditionals (`If`/`otherwise`), recipes with named parameters, data-driven `Examples`, HTTP mocks (`Mock`), and inline Dart blocks (`RunDart`).
  - **Builder**: declares `lib/{{}}.dart` → `tests/generated/{{}}.probe`. Cheap text pre-check skips files without annotations so it's safe to enable on a whole `lib/` tree. Each annotated class becomes a top-level `test "..."` block with optional tags and step body.
  - **Cross-language validation**: every Dart-emitted golden is parsed by the Go-side parser in CI (`internal/parser/golden_integration_test.go`), so a malformed emitter line is caught immediately rather than at user runtime.
  - **Docs**: see [`docs/wiki/Annotations.md`](docs/wiki/Annotations.md) for the full reference.

## [0.9.2] - 2026-05-09

### Added
- **Real-time step feedback** — the runner now emits progress during test execution instead of staying silent until a step completes:
  - **Pre-step indicator** (verbose mode): prints `→ step description` immediately before each step runs. On a TTY the line is overwritten in place by the `✓/✗` result when the step finishes (clean single-line-per-step output). On non-TTY (CI) both lines are appended.
  - **Progress ticker** (all modes): a goroutine fires every 5 seconds while a step is still running and prints `⏱ step... (Ns)`. Stops immediately when the step completes — fast steps produce no ticker output.
  - **Timeout warning** (all modes): when a step has consumed ≥ 80% of its context deadline, a one-time `⚠ step still running — Ns elapsed, Ns timeout` warning is printed. Gives time to react before the step times out.
  - **Non-verbose TTY status line**: even without `--verbose`, a faint `\r`-overwriting status line shows the current step name while it runs and is cleared when it finishes. No output on non-TTY so CI logs stay clean.

## [0.9.1] - 2026-05-09

### Added
- **MCP: 3 new tools** — `init_project` (scaffold `probe.yaml` + `tests/`), `generate_report` (HTML from JSON results), `record` (record interactions → `.probe` file with configurable timeout)
- **MCP: Android emulator shutdown** — `shutdown_device` now accepts `serial` for Android emulators in addition to `udid` for iOS simulators
- **MCP: composite test support** — `run_tests` exposes a `composite_devices` parameter (`"A=host:port/token B=udid"`) that maps directly to `--composite-device` flags; `write_test` description documents the full `composite test` syntax
- **MCP: `run_tests` flags documented** — tool description now enumerates key flags (`--timeout`, `--format`, `--parallel`, `--shard`, `--host/--token`, `--disable-animations`, `--video`, `--stream`)
- **MCP: `write_test` validates before writing** — content is parsed in-process before the file is created; syntax errors are returned immediately without touching the filesystem

### Fixed
- **MCP: `get_report` used alphabetical order instead of modification time** — `get_report` and `generate_report` now find the most recently *modified* JSON file in `reports/`, not the lexicographically last one

## [0.9.0] - 2026-05-09

### Added
- **Composite tests** — a new `composite test` keyword lets a single `.probe` file orchestrate multiple devices simultaneously. Declare device aliases (`A`, `B`, `C`, …), write per-device step blocks (`A: tap "Login"`), and use `sync "label"` as a cross-device barrier that all goroutines must reach before any proceeds. Supports N devices. Files with composite tests coexist with regular `test` blocks. Configure devices via `--composite-device A=host:port/token` (WiFi), `--composite-device B=<udid>` (iOS simulator), or `--composite-device C=emulator-5554` (Android). Probe.yaml can also set `composite.devices` for project-level defaults. If composite devices are not configured at runtime, composite tests are reported as SKIPPED.
- **Failure semantics**: if one device fails mid-test, the shared context is cancelled immediately; all other devices are unblocked from any `sync` barrier they are waiting at and their next step returns a cancelled error. The final result marks the root cause device as FAIL and secondary devices as CANCELLED.

## [0.8.0] - 2026-05-02

### Added
- **Studio: ProbeScript recorder** — new ● Record button (⌘⇧R) starts an interactive recording session. The agent streams `probe.recorded_event` notifications; Studio converts each interaction to a ProbeScript step in real time in the editor. Gaps >2 seconds between actions automatically insert `wait N seconds` steps. Requires a WebSocket connection (simulators and emulators); returns a clear error on physical-device (HTTP) connections.
- **Studio: AI chat pane (BYOK)** — ✦ button (⌘⇧A) opens a 260px chat panel at the bottom of the Studio window. Chat with `claude-sonnet-4-6` about the open `.probe` file — the current file contents are injected into the system prompt for context. Your Anthropic API key is stored in the **platform keychain** (macOS Keychain, Windows Credential Manager, Linux libsecret) via `zalando/go-keyring`. Key is never written to disk or sent anywhere except `api.anthropic.com`. Running cost counter (input tokens / output tokens / estimated USD at Sonnet 4.6 rates).
- **Studio: WiFi token memory** — Studio remembers the agent token per discovered device in localStorage. Subsequent mDNS sessions for that device prefill automatically with a "🔑 saved" tag and forget button.
- **Studio: workspace settings overlay** — ⚙ button opens a form for `agent.port`, `defaults.timeout`, iOS UDID, Android serial. Saves back to `probe.yaml` preserving all other keys.
- **Studio: diagnostics polish** — error toasts include actionable hints for missing `iproxy`, `adb`, or `PROBE_AGENT=true`. Status tooltip shows device ID and transport. Inspector search scrolls to first match.
- **Security**: bumped 8 vulnerable dependencies (vite, @vscode/vsce, golang.org/x/crypto, net, sys, text).

## [0.7.0] - 2026-05-02

### Added
- **MCP device lifecycle tools** — `probe-mcp` now exposes 5 new tools that let AI agents discover and manage simulators/emulators end-to-end without leaving chat: `list_devices` (booted/connected sims, emulators, physical devices), `list_simulators` (all iOS sims including shutdown), `list_avds` (Android Virtual Device names), `start_device` (boot Android emulator by AVD name or iOS simulator by UDID, blocks until ready), `shutdown_device` (iOS simulator only). Brings the total tool count from 10 to 15.
- **`device` argument on existing MCP tools** — `get_widget_tree`, `take_screenshot`, `run_script`, and `run_tests` accept an optional `device` (serial or UDID) so the agent can pin a target when multiple devices are connected. Previously the agent had to smuggle this through the undocumented `flags` string.
- **Studio: physical-device support over USB** — Studio's Connect flow now handles physical iOS (via `iproxy` tunnel + `idevicesyslog` token read) and physical Android (via `adb forward`, same path as emulators). The picker shows a `physical` tag so cabled devices are obvious next to sims and emulators. Requires `brew install libimobiledevice` for physical iOS.
- **Studio: WiFi physical-device discovery via mDNS** — Studio now browses for `_flutterprobe._tcp` on the LAN and lets you connect to discovered devices with one click + token paste. Requires `flutter_probe_agent` v0.7.0+ in your Flutter app. The token is intentionally NOT advertised over mDNS (anyone on the network would be able to read it) — the user pastes it from the app's `PROBE_TOKEN=...` log line.
- **`flutter_probe_agent` v0.7.0**: agent advertises itself over Bonjour/NSD when running in WiFi mode (`PROBE_WIFI=true`). New dependency: `bonsoir: ^5.1.10`. Localhost-bound agents skip mDNS entirely so simulator-only apps pay zero overhead.
- **Studio: new Wails methods** `StartWiFiDiscovery`, `StopWiFiDiscovery`, `ConnectWiFi(host, port, token)`. Backed by `github.com/grandcat/zeroconf`.

## [0.6.0] - 2026-04-26

### Added
- **Configurable auto-reconnect policy** — `agent.reconnect_attempts` (default 4) and `agent.reconnect_backoff` (default 1s base) in `probe.yaml`. Replaces the previous fixed 2-attempt, 1s-sleep policy with capped exponential backoff plus jitter (1s → 2s → 4s → 8s, ±20%, ~15s total budget). Slow devices and brief USB-C cable mode flips now recover transparently instead of failing the step.
- **iproxy tunnel TCP health check** — physical iOS startup now verifies the iproxy tunnel is actually forwarding via a 127.0.0.1 probe (up to 3s) before the first dial. Dead-tunnel-on-live-process is detected immediately instead of failing later as a 30s WebSocket handshake timeout.
- **`probe-mcp` standalone binary** — the MCP (Model Context Protocol) server now ships as its own binary alongside `probe`. Configure your MCP client (Claude Desktop, Cursor, etc.) to call `probe-mcp` directly. Same 10 tools, same protocol, smaller per-binary surface. Available via Homebrew (`brew install probe`) and GitHub release artifacts.
- **`probe test --stream`** — when combined with `--format json`, emits one ndjson line per test as it completes (`{"type":"test_result","result":{...}}`), in addition to the final report. Built for live consumption by Studio, CI dashboards, and other tooling that wants real-time progress.
- **FlutterProbe Studio (Beta Preview)** — new `studio/` directory containing a [Wails 2.12+](https://wails.io/) desktop app for visual ProbeScript test authoring. Cross-platform (macOS / Windows / Linux). Marked Beta Preview because the surface area (editor, lint, device pane, run integration) is feature-complete but stability work is ongoing. Features:
  - Monaco editor with ProbeScript syntax highlighting (keywords, strings, variables, tags, comments) and live lint markers driven by the runner's parser
  - File browser with workspace folder picker (persists across sessions in localStorage)
  - Device picker backed by `internal/device.Manager` (simulators and emulators)
  - **Live device view** at ~10 FPS via the existing `take_screenshot` RPC — no new agent code, works on all sim/emu platforms
  - **In-process test execution** by importing `internal/runner` directly — no subprocess shell-out, no JSON wire format
  - Live results timeline streamed via Wails event bus as tests complete
  - Widget tree inspector (refresh on demand)
  - Connection status indicator with semantic colors (connected / connecting / error / disconnected)
  - Toast notifications, keyboard shortcuts (⌘R run, ⌘S save, ⌘B connect, ⌘P workspace, ⌘K refresh devices, `?` help)
  - Native macOS dark appearance, draggable title bar, About panel
  Build with `cd studio && wails build`. Physical device support, scrcpy/simctl native video, multi-device side-by-side, time-travel debugging, and AI chat pane via MCP are deferred to follow-ups.

### Changed
- **`probe mcp-server` is deprecated** — the subcommand still works for backwards compatibility (runs the same server code embedded in `probe`) but prints a one-time deprecation notice on stderr. Migrate your MCP client config to `probe-mcp`. Will be removed in a future release.
- **MCP server reports binary version** — the `initialize` response's `serverInfo.version` now reflects the installed binary version (set at build time) instead of a hardcoded `0.5.7`.

### Fixed
- **Ctrl-C no longer leaks iproxy / `idevicesyslog` / ADB forwards.** The `probe test` command now installs a `SIGINT` / `SIGTERM` handler that cancels the run context so deferred cleanup actually runs. Press Ctrl-C twice to force-exit.
- **Reconnect serialization** — concurrent steps (loops, conditionals) that both observe a dropped connection no longer race on the executor's client reference. A generation counter ensures only one reconnect runs at a time and late callers reuse the new client.

## [0.5.7] - 2026-04-26

### Added
- **Relational selectors** — `tap "Submit" below "Username"`, `see "Price" right of "Label"` — spatial anchoring via Flutter `RenderBox` positions (`below`, `above`, `left of`, `right of`)
- **`open link "url"`** — opens a URL in the default external browser via `url_launcher` platform channel
- **`wait for animations to end`** — polls `SchedulerBinding.hasScheduledFrame` until animations complete
- **`see "Field" is focused`** — asserts that a widget holds keyboard focus via `FocusManager.primaryFocus`
- **`store "value" as varName`** — stores a literal or `${variable}` value for use in later steps
- **`probe mcp-server`** — stdio MCP server (10 tools) for AI agent integration with Claude Desktop, Cursor, etc.: `get_widget_tree`, `take_screenshot`, `read_test`, `write_test`, `run_script`, `run_tests`, `list_files`, `lint`, `get_report`, `generate_test`; see [MCP Server docs](https://flutterprobe.dev/tools/mcp/)
- **`--disable-animations`** flag (also `defaults.disable_animations` in `probe.yaml`) — sets Flutter `timeDilation = 0` after connecting to skip animations and speed up tests
- **`probe.open_link`** RPC — agent-side handler that invokes url_launcher or records the URL for `verify_browser`
- **`probe.set_time_dilation`** RPC — sets `timeDilation` on the agent at runtime
- **`probe.set_output` / `probe.drain_output`** RPCs — inter-step output variable exchange between Dart and CLI
- `device.ios_device_id` / `device.android_device_id` in `probe.yaml` — set a preferred simulator UDID or emulator serial without requiring `--device` on every run

### Fixed
- Token acquisition reliability: `simctl` token reader now globs all app data containers, resolving stale-container mismatches after reinstalls or clear-data operations
- WebSocket dial now retries on transient errors (`connection refused`, reset, timeout) within the configured `dial_timeout` window, eliminating the race between token file write and agent server startup

## [0.5.6] - 2026-04-02

### Added
- Homebrew tap: `brew tap AlphaWaveSystems/tap && brew install probe` (macOS + Linux)
- Homebrew formula auto-updates on every release tag via `HOMEBREW_TAP_TOKEN`

## [0.5.5] - 2026-04-02

### Changed
- `flutter_probe_agent` Dart package re-licensed from BSL 1.1 to MIT (Go CLI remains BSL 1.1)
- CI: added Dart agent validation job — `dart analyze`, `flutter test`, `dart pub publish --dry-run`, CHANGELOG enforcement
- CI: added PR template with pub.dev and docs checklist

## [0.5.3] - 2026-03-28

### Added

- Automated pub.dev publishing via GitHub Actions using official `dart-lang/setup-dart` reusable workflow
- FAQ section on landing page (WiFi testing, physical devices, CI/CD, setup)
- ProbeScript Dictionary — complete reference of all keywords, commands, and modifiers
- Comprehensive third-party tool requirements documentation

### Changed

- Renamed Dart package from `probe_agent` to `flutter_probe_agent` for pub.dev branding
- Publish workflow chains after Release workflow (prevents publishing broken versions)
- Version badge auto-updates from git tags (no more hardcoded versions)

### Fixed

- Broken wiki link on landing page (`AlphaWaveSystems/wiki` → `flutter-probe/wiki`)
- Old domain references (`flutterprobe.com` → `flutterprobe.dev`)
- Old package name references in vscode README and docs
- pub.dev score: shorter description, dartdoc warning, clean public API

## [0.5.1] - 2026-03-26

### Added

- Pre-shared restart token (`probe.set_next_token`) — CLI sends a token to the agent before `restart the app`; agent persists it and uses it after restart, enabling WiFi reconnection without `idevicesyslog`
- `--host` flag for WiFi testing — connect directly to device IP, no iproxy needed
- `--token` flag to skip USB-dependent token auto-detection
- `PROBE_WIFI=true` dart-define — binds agent to `0.0.0.0` for network access
- HTTP POST fallback transport (`POST /probe/rpc`) — stateless per-request communication for physical devices
- `ProbeClient` interface — both WebSocket and HTTP clients satisfy it for transport-agnostic execution
- `tap "X" if visible` ProbeScript syntax — silently skips when widget is not found; works with tap, type, clear, long press, double tap
- Direct `onTap` invocation fallback for `Semantics`-wrapped `GestureDetector` widgets on physical devices
- `take screenshot "name"` now accepts name directly (no `called` keyword needed)
- Physical device E2E test suite for FlutterProbe Test App (12 tests covering all 10 screens)

### Fixed

- `clear app data` on physical iOS now skips immediately (before confirmation prompt) to avoid killing the agent
- Connection error detection in `if visible` — propagates connection errors for auto-reconnect instead of silently swallowing them
- Screenshot parser accepts `take screenshot "name"` without requiring `called` keyword

## [0.5.0] - 2026-03-26

### Added

- Physical iOS device support: launch/terminate via `xcrun devicectl`, token reading via `idevicesyslog`, port forwarding via `iproxy`
- Physical Android device validation: `EnsureADB()` verifies binary, device reachability, and cleans stale port forwards
- Physical device detection: `IsPhysicalIOS` (simctl list check) and `IsPhysicalAndroid` (ro.hardware property check)
- Physical iOS devices listed in `probe device list` via `idevice_id`
- WebSocket ping/pong keepalive (5s interval) — prevents idle connection drops on physical devices via iproxy
- Auto-reconnect on WebSocket connection loss — up to 2 transparent retries per step with full re-dial
- `EnsureIProxy()` — automatic iproxy lifecycle management: checks installation, kills stale processes, starts fresh, defers cleanup
- Visibility filtering in widget finder — off-screen widgets (behind routes, Offstage, Visibility) no longer match `see`/`if appears`
- Unique pointer IDs for synthetic gestures — prevents collision with real touch events on physical devices
- ProbeAgent profile mode support — `ProbeAgent.start()` works in profile builds (required for physical iOS)
- ProbeAgent release mode safeguards — blocked by default, opt-in via `allowReleaseBuild: true` + `PROBE_AGENT_FORCE=true`
- Test files for all packages: `cmd/probe`, `internal/cli`, `internal/ios`, `internal/device` (manager tests)
- HTTP POST fallback transport (`POST /probe/rpc`) — stateless alternative to WebSocket for physical devices, eliminates persistent connection drops
- `ProbeClient` interface — both WebSocket `Client` and `HTTPClient` satisfy it, enabling transport-agnostic test execution
- WiFi testing mode (`--host <ip>` + `--token <token>` + `--dart-define=PROBE_WIFI=true`) — test physical devices without USB, no iproxy needed
- `tap "X" if visible` ProbeScript syntax — silently skips tap when widget is not found, replaces verbose dialog-dismissal recipes
- Direct `onTap` invocation fallback for `Semantics`-wrapped widgets — fixes tap failures on physical devices where synthetic gestures don't reach `GestureDetector`
- `take screenshot "name"` now accepts name directly (previously required `called` keyword)

### Changed

- Operations unsupported on physical devices now skip gracefully with warnings instead of crashing:
  - `clear app data` on physical iOS → warning + skip
  - `allow/deny permission` on physical iOS → warning + skip
  - `set location` on any physical device → warning + skip
- `restart the app` on physical iOS uses `xcrun devicectl` instead of `simctl`
- iOS connection setup now branches: simulator path uses simctl permissions + loopback; physical path uses iproxy + idevicesyslog
- Android connection setup validates ADB availability and device state before port forwarding

## [0.4.2] - 2026-03-25

### Added

- Cross-platform parallel E2E execution: `--parallel --devices emulator-5554,<iOS-UDID>` runs tests on iOS + Android simultaneously
- `ResolveAppID`: auto-converts camelCase iOS bundle IDs to snake_case Android package names for cross-platform runs
- Per-device `AppID` field in `DeviceRun` for mixed-platform parallel testing
- Retry logic for parallel device connections (up to 2 retries with 5s backoff)
- Graceful per-device error handling — one device failing doesn't stop others
- Custom domain: site now lives at [flutterprobe.dev](https://flutterprobe.dev)
- SEO overhaul: sitemap.xml, robots.txt, JSON-LD structured data, Twitter Cards, OG image
- 7 comparison pages targeting search intent (Flutter E2E testing, integration_test alternative, Patrol alternative, etc.)
- 3 blog posts (Flutter E2E testing guide, Why We Built FlutterProbe, honest comparison)
- Copilot Code Review configuration with path-specific review instructions for parser, runner, agent, website, and CI
- Dependabot compatibility workflow: security audit (`govulncheck`, `npm audit`), license compliance (rejects GPL/AGPL/SSPL), backward compatibility (.probe file parsing), auto-merge for patch/minor updates
- Headless E2E CI/CD: fully wired Android (ubuntu + emulator) and iOS (macOS + simulator) workflows with 3-way sharding, automated app build/install/launch, and HTML report generation

### Fixed

- Parallel port assignment: Android gets `portBase+1` via ADB forward, iOS uses `portBase` directly
- Landing page version badge updated to current release

## [0.4.1] - 2026-03-25

### Fixed

- Fix `set location` decimal parsing — coordinates like `37.7749, -122.4194` were stripped of decimals and negative signs
- Fix Android app launch — replace `adb shell monkey` with `am start -n {package}/.MainActivity` (monkey fails silently on many emulators)
- Fix Android token reading — file-based token via `adb shell run-as` instead of unreliable logcat scanning
- Fix variable resolution in `see` assertions — data-driven variables like `<expected>` were not substituted
- Fix Dart agent url_launcher interceptor — use proper `MethodChannel.setMethodCallHandler` instead of mock-only API
- Increase Android reconnect delay to 5s (emulators need more boot time than iOS simulators)

### Added

- `--parallel` flag — auto-discover all connected devices, distribute test files round-robin, run in parallel goroutines
- `--devices serial1,serial2` flag — explicit device list for parallel execution
- `--shard N/M` flag — deterministic file-based sharding for CI matrix jobs (e.g. `--shard 1/3`)
- `ParallelOrchestrator` with per-device goroutines, independent WebSocket connections, port allocation, and result merging
- Per-device test attribution — `TestResult` includes `DeviceID` and `DeviceName`
- JSON reporter includes `device_id` and `device_name` per result
- Terminal output shows per-device summary table in parallel mode
- Lexer support for float literals (e.g. `37.7749`) and negative sign tokens

## [0.4.0] - 2026-03-25

### Added

- `before all` / `after all` hooks for suite-level setup and teardown (run once per file)
- `kill the app` command — force-stop without relaunch (CLI-side via ADB/simctl)
- `open the app` now performs CLI-side launch + reconnect when device context is available
- `copy "text" to clipboard` and `paste from clipboard` commands (agent-side via Dart Clipboard API)
- `set location lat, lng` command — set device GPS coordinates (ADB geo fix / simctl location)
- `verify external browser opened` command — checks url_launcher platform channel for external launches
- `call GET/POST/PUT/DELETE "url"` command — execute real HTTP requests from tests (Go-side net/http)
- `call ... with body "json"` — HTTP calls with request body, response stored in `<response.status>` and `<response.body>` variables
- `<random.email>`, `<random.name>`, `<random.phone>`, `<random.uuid>`, `<random.number(min,max)>`, `<random.text(length)>` data generators for form-heavy tests
- `with examples from "file.csv"` — load data-driven test data from external CSV files
- Unit tests for random data generators, CSV loader, all new parser commands
- E2E test files for all new features: hooks, clipboard, app lifecycle, location, random data, HTTP calls, CSV-driven tests

## [0.3.0] - 2026-03-25

### Fixed

- Resolve all pre-existing staticcheck lint errors blocking CI
- Replace deprecated Go 1.26 crypto/ecdsa field access with ecdh+x509 round-trip in wallet signing
- Remove unused functions and variables across CLI, runner, and probe-convert packages
- Fix error string style violations (punctuation, numeric HTTP status codes, nil context)

### Added

- Unit tests for 6 previously untested packages: config, plugin, visual, report, device, cloud/wallet
- Test coverage for config loading/validation, plugin registry, visual regression comparison, HTML report generation, permission resolution, and wallet operations

### Changed

- Bump GitHub Actions: actions/checkout v5→v6, actions/upload-artifact v4→v7, actions/setup-node v4→v6, actions/upload-pages-artifact v3→v4, codecov/codecov-action v4→v5

## [0.2.0] - 2026-03-22

### Added

- Cloud device farm integrations: BrowserStack, Sauce Labs, AWS Device Farm, Firebase Test Lab, LambdaTest
- WebSocket relay mode for cloud device farms with session TTL and auto-connect
- x402 payment protocol support for cloud API billing (EIP-712 wallet signing)
- VS Code extension: Session Manager sidebar for multi-device parallel testing
- VS Code extension: Test Explorer sidebar with workspace-wide test discovery
- VS Code extension: CodeLens inline Run/Debug buttons above tests
- VS Code extension: real-time diagnostics (lint-on-save) and IntelliSense completions
- VS Code extension: Run Profile webview panel for configuring test options
- Physical iOS device support via iproxy (libimobiledevice)
- `probe studio` command for interactive widget-tree inspection
- `probe generate` command for AI-assisted test generation (Claude API)
- `probe heal` command for self-healing selector repair with AI analysis
- `probe migrate` command for converting tests from other frameworks
- Landing page and Astro/Starlight documentation website
- Cloud relay configuration in probe.yaml (TTL, connect timeout, auto-enable)
- AI configuration in probe.yaml (API key, model selection)

## [0.1.0] - 2026-03-16

### Added

- ProbeScript language with indent-based natural language test syntax
- Go CLI with commands: test, lint, init, device, record, report, migrate, generate
- Dart ProbeAgent with WebSocket JSON-RPC 2.0 protocol and direct widget-tree access
- iOS simulator support with token file fast path and log stream fallback
- Android emulator support with ADB port forwarding and logcat token extraction
- Sub-50ms command round-trip execution
- Recipe system with parameterized reusable steps and `use` imports
- Data-driven tests with `Examples:` blocks and variable substitution
- `before each`, `after each`, and `on failure` hooks
- Conditional execution with `if`/`else` blocks
- `repeat N times` loops
- Visual regression testing with configurable threshold and pixel delta
- Test recording mode capturing taps, swipes, long presses, and text input
- Custom plugin system via YAML definitions
- probe-convert tool supporting 7 source formats at 100% construct coverage
- Supported formats: Maestro, Gherkin, Robot Framework, Detox, Appium (Python/Java/JS)
- VS Code extension with syntax highlighting, snippets, and commands
- HTML, JSON, and JUnit XML report generation with relative artifact paths
- Self-healing selectors via fuzzy matching (text, key, type, semantic strategies)
- HTTP mocking with `when ... respond with` syntax
- App lifecycle commands: `clear app data`, `restart the app`
- OS-level permission handling via ADB and simctl
- Configurable tool paths for ADB and Flutter binaries
- Parallel testing support with per-platform config files
- Video recording on iOS (H.264) and Android (screenrecord/scrcpy)
- Dart escape hatch with `dart:` blocks
- probe.yaml configuration with full resolution order (CLI flag > YAML > default)
