# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- **`probe license activate <key>` / `probe license status`:** stores and verifies an offline ed25519-signed license key in `~/.flutterprobe/license.key`. Only hosted features will ever need one; everything that runs locally stays free and works without a key.
- **Release pipeline:** canary channel (`vX.Y.Z-next.N` tags publish `probe@next` on Homebrew and pre-release Dart packages), a dogfood gate workflow, an automated review-agent check on pull requests, and auto-rollback of a stable release when a `prod-incident` is filed within 24 hours. `scripts/release.sh` accepts canary versions. See `docs/ops/release-pipeline.md`.
- **Studio E2E suite** (`make studio-e2e`, `studio/e2e/`): drives the real Studio window on macOS through the accessibility tree and screenshots — launch, device picker, connect, run, results, failures, performance lines, recorder, settings, AI-chat and WiFi overlays, error paths — against a new fixture app (`native-test-apps/studio-fixture/`). Optional vision assertions via `STUDIO_E2E_AI_PROVIDER`. Parameterised for dogfooding on any app (`studio/e2e/DOGFOOD.md`); self-hosted macOS workflow `studio-e2e.yml`.

### Security
- **Review-agent workflow:** reviews only same-repository branches from owners, members and collaborators, and fails (never skips) for anything else, since a skipped required check counts as passing; the model gets no tools and one prompt (conventions from the trusted base commit plus the diff between random markers, declared untrusted), so it cannot read files or the environment; nothing PR-controlled reaches a shell through `${{ }}`; the verdict counts only as the exact first line of the reply; an approval is bound to the reviewed commit; a branch behind the current tip of its base fails both before the review and again just before it is submitted; credential-shaped strings in the diff fail the check before any model sees it; the CLI version and actions are pinned. New `REVIEW_PROVIDER` variable (`oauth` default, `anthropic`, or `local` as a never-approving pre-screen).
- **Dogfood-gate workflow:** the `version` input is validated against `X.Y.Z[-next.N]` before it is used in paths, download URLs or issue titles.

### Fixed
- **License keys have exactly one valid spelling:** the verifier now decodes base64url strictly, so a key re-spelled with non-zero trailing bits is rejected instead of verifying as an alias (not a forgery: the signed bytes were unchanged). The tamper test is deterministic; it failed about once in a few hundred runs before.
- **Studio reads `probe.yaml` from the open workspace** (agent port, timeout, device ids) when connecting; it used to read the process working directory. `PROBE_STUDIO_WORKSPACE=/path` opens a workspace at launch.
- Studio file-browser rows and result rows carry accessible names (`role=button`, `pass:`/`fail:` labels).

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
  converter against nect-flutter's real 76-flow suite: `setPermissions`, `retry` (with recursive
  nested-command conversion, also fixed for `repeat`), and `assertScreenshot` are now supported,
  and `relativePoint`-style `{point: "x%,y%"}` selectors get a clear `# TODO` instead of a real bug
  the old code had — silently emitting `tap on "map[point:50%,10%]"`, confirmed against an actual
  occurrence in the corpus. The corpus's own real gaps (not named in the original roadmap item, but
  27% of all step invocations in this specific suite) — `extendedWaitUntil`, `scrollUntilVisible`,
  `eraseText` — are now supported too. Also fixed: a stale `setLocation` "not supported" comment
  (it's been a real ProbeScript verb since before this cycle), and `probe migrate maestro <dir>`
  silently finding zero files for any suite organized into feature subdirectories — the real-world
  norm, including nect-flutter's own — because directory discovery was single-level, not recursive.
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
