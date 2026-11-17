---
title: FlutterProbe Studio
description: Visual ProbeScript test authoring with embedded device view, live widget tree inspector, and in-process test execution. Cross-platform desktop app built on Wails.
---

> **Beta Preview** — shipped in **v0.6.0**. Surface area is feature-complete, including physical iOS and Android devices (v0.7.0); expect rough edges around parallel multi-device authoring.

FlutterProbe Studio is a standalone desktop application for designing, recording, and executing ProbeScript tests. It complements the `probe` CLI rather than replacing it: same parser, same runner, same agent — just a visual front-end for the same underlying engine.

## What you get

- **Monaco editor** with ProbeScript syntax highlighting, live lint markers (parse errors and empty tests; `probe lint` also checks step and recipe references, selectors and placeholders), and the same TextMate grammar that powers the VS Code extension. Completions, hover help and snippets cover every command in the [ProbeScript dictionary](/probescript/dictionary/) and every reserved word in the [grammar](/probescript/grammar/); they are generated from those pages, so the editor and the docs agree.
- **Save a recording or a new buffer.** A recording (after Stop) or any buffer with no file is saved with **Save** (or Cmd/Ctrl+S): Studio asks for a file name in the page (`recorded.probe` or `untitled.probe` to start from), writes it in the folder the Workspace pane shows, and opens it as the current file. It never replaces a file that exists (names are compared without case, and the file is created exclusively, so one that appears between the check and the write is not replaced either), and refuses a path, a hidden or direction character, a leading dot and any extension other than `.probe`. A new file is at most 1 MiB. Saving an open file writes a new file beside it, flushes it and renames it over the old one, so a failed write (a full disk) leaves your file as it was. Studio does not write into a hidden folder of the workspace (a folder or file name that starts with a dot, such as `.git`), and a saved file is at most 1 MiB (1,048,576 bytes) of content, in place or new: a larger one is refused with the cap named.
- **Insert before the step.** Under a failed `see` or `tap` whose element was not found, the rule-based "Suggested fix" (`wait until "X" appears`) has an **Insert before the step** button. It adds the line above the failed step in the editor (one undo step; nothing is saved or run). Studio checks first that the editor holds the same text the run read, that the line is the step that failed, that the step is not inside a recipe, and that no such wait is already above it; otherwise it says why. After an insertion the marks of that run are cleared and its rows do not jump until you run again.
- **Workspace folder picker** — point Studio at any project root with a `tests/` folder. Choice persists across launches. **Close workspace** (the ✕ next to the folder button, shown while a workspace is open) goes back to the state of a first start: no workspace, no open file, the guidance in the panes, and the remembered folder is forgotten (with unsaved changes it asks first, like opening another file; it is refused while a run or a recording is going, by Studio itself, so a script cannot close it either). A connection that exists is not dropped by Close and keeps the `probe.yaml` configuration it was made with; disconnect it yourself if you want a different one. A folder named by `PROBE_STUDIO_WORKSPACE` still opens at the next start, and the folders named in `PROBE_STUDIO_WORKSPACE` and `PROBE_STUDIO_AUTOMATION_ROOTS` stay reachable by the automation endpoint after Close (they are the environment's, not the window's: Close forgets only the folder a person opened).
- **Device picker** that lists booted simulators, online emulators, and physical iOS/Android devices side by side, with a `physical` tag and a clear hint when nothing is connectable.
- **Physical device support** — the same `internal/device` code the `probe` CLI uses: `iproxy` tunneling for physical iOS over USB, `adb forward` for physical Android (same path as emulators), and mDNS auto-discovery (`_flutterprobe._tcp`) for WiFi-connected devices. See [Physical devices](#physical-devices) below.
- **Live device pane** — the connected simulator, emulator, or physical device's Flutter screen, shown read-only from repeated screenshot requests to the on-device agent (no video stream; system UI and native views are not included). The title shows `N fps · M ms avg · K ms max`.
- **Live widget tree inspector** — every device frame is bundled with a `dump_widget_tree` snapshot, so the inspector pane updates per frame. No refresh button.
- **In-process test execution** — Studio's Go backend imports `internal/runner` directly. No subprocess shell-out, no JSON wire format. Results stream into the timeline as `run:result` events fire.
- **Runs like `probe test`** — a Studio run builds its device context with the same code the CLI uses, from the `probe.yaml` of the open workspace: the app id, the reconnect, restart, token and dial timings, the driver port, the HTTP fallback of a physical phone and, on Android with `defaults.crash_evidence`, the line that says why the app died. So steps that restart the app (`restart the app`, `open the app`) work in Studio, and the live view, the widget tree and a recording that is on follow the run to the restarted app. **`clear app data` asks first**: it wipes the app's data, so Studio shows a dialog that names the app (the app id from `probe.yaml`, not read from the device) and the device, with Cancel selected (Escape also says no; Enter never presses **Clear data**, a click or Space does, after half a second). Connect is refused while a run is in progress, so the dialog never names one device while another is wiped. A second `clear app data` in the same run asks again. With no answer in 60 seconds, no window to ask, or a cancelled run, the step is not run and Studio says so. Studio never skips the question: there is no `--yes` here.
- **Live step highlight** — while a file runs, the editor gutter follows the executing step (▶ while running, then ✓ / ✗ / – stays on the line; a failed line shows its error on hover). Step rows nest under each test in the results timeline with line number, duration and attempt; click a failed step, or a failed test, to open the line where it failed, in that step's file (the rules are in [Quick start](#quick-start), step 5). The toolbar shows `step N of M` and a **Cancel** button stops the run.
- **Run part of a file** — next to **Run**, the scope control picks the whole file (the default), the test at the cursor, the tests with one tag, or the tests that failed the last time the file ran in this session. A scope that matches no test is reported and nothing runs. The failed tests are remembered in memory only, so a restart starts with none. When a run has more than one outcome, the results list groups its tests under Failed, Passed and Skipped, failures first.
- **Network tab** — the requests and responses the app made during a run, read from the agent's request log, with credentials hidden before anything reaches the window (see [The Network tab](#the-network-tab)).
- **Send request** — one HTTP request from the Network tab, to check a backend by hand, with the answer redacted the same way (see [Send request](#send-request)).
- **Save as step** — in the Send request panel, inserts the typed request into the open test as a `call` step (GET, POST, PUT or DELETE; no headers), with `${PROBE_SAVED_*}` placeholders for query values and anything that looks like a credential, a card number or an email address (see [Save as step](#save-as-step)).
- **Suggest a fix** — next to Explain failures: one ProbeScript line from the AI provider for the failed step of the last run, shown on an advisory card after the same preview (see [AI in Studio](/tools/ai/#in-studio)). It is not applied, and it changes no result. The card names the failed step's line (`L12`) and the same click as a failing row opens it. The card's **Replace the line** button puts the suggestion into the editor as one undo step (not saved, not run) after Studio checks the file, the line and that the new line is one step with the same verb and flags, from a closed list (tap, long press, double tap, type into, see, don't see, wait until appears or disappears). The card then offers **Re-run the failed test**: save the file yourself, then one click runs that test once and shows passed or still failing on the card. If it still fails, **Suggest a fix again** asks for another suggestion (the preview and confirmation as before), up to three for the same failed step in a session.
- **Explain failures** — under the Results title, while AI is on and the last run has a failed test with error text, the AI provider gives short advice on the failed tests. For a hosted or remote provider Studio shows what it would send before every Explain; for a model on this machine it shows that first time in a session, and again after the destination, the workspace, the provider settings or the saved key change. It sends only after **Send**. The advice is shown as text and never changes a result; each note has an `L12` button that opens the line where its test failed, in the same way as a failing row (see [AI in Studio](/tools/ai/#in-studio)).
- **Workspace settings write `probe.yaml`, and only the lines you changed.** The gear button opens the form. Besides the agent port, the default step timeout, the preferred device ids and the AI sections, it has three run knobs, written under `defaults:`: **Test time limit** (`test_timeout`, a duration such as `90s`, at least 1 s, empty = off), **Retry a failed test** (`retry_failed_tests`, 0 to 10, 0 = off) and **Wait for a target** (`implicit_wait`, a duration up to 5 m, empty = off). The same keys are read by `probe test` and by a Studio run. Save is the only thing that writes, and it never runs by itself. A field you did not change is not written, so a value you spelled your own way (or set by hand beyond the form's bounds) stays as it is. The edit is made on the file's own text: comments, blank lines, the order and indentation of your keys, and every key the form does not manage stay as they were, and the file is replaced atomically with its permissions kept (a new one is private to you). Studio refuses, with one line and without changing anything, a `probe.yaml` that does not parse or that the engine cannot load after the edit, one that is a link, a save for any folder but the workspace that is open, and a key whose shape it cannot edit in place (a flow mapping, a block scalar, a list where a map is expected). The bounds are checked by Studio, not only by the form, and only for a value you changed. Before anything is written Studio reads the edited text back with the engine's parser and refuses, writing nothing, unless it reads as your file plus exactly the keys you changed, so a layout it cannot edit safely (a value that spans lines in an unusual way, say) is refused rather than damaged. If `probe.yaml` changed on disk (an editor, git) after the form was opened and you change a field it touched, Studio asks you to close and reopen Settings instead of putting an old value back. The file is replaced by a rename, so a hard link to it is not kept.
- **Export the results of a run.** After a run has finished, **Export JSON** and **Export JUnit** (in the Results header) save its results as a report file: the same JSON and JUnit XML that `probe test --format json` and `--format junit` write, made by the same reporters from the results the engine returned for the run (including attempts, time limits and timeouts, errors and crash evidence), with the device and app of the run in the JSON, and file names relative to the workspace in the JUnit. A native save dialog opens and you choose where; the dialog asks before it replaces a file, and Studio refuses a name that does not end in `.json` (or `.xml`), a folder, a link and a folder that does not exist, writing a temporary file next to the name and renaming it into place so a failure leaves nothing half-written. The window cannot name a path, the export is not available to the automation endpoint, the MCP server or the headless build (there is no dialog there), and what it writes is never sent to an AI provider. The report has the same content as the CLI's: error messages are written as the engine produced them and can contain local paths, so check a report before you share it. A run you stopped leaves nothing to export (its results are partial), opening another workspace drops the results of the last one, and the buttons follow what Studio holds, also after the window is reloaded. Both buttons are off while a run goes and until one has finished with a result.
- **The build of the app under test.** When the agent reports it, the Device pane shows a line under its title: the Flutter version and channel and the Dart version the app was built with, and the framework revision (the short revision the Flutter tool reports), for example `App built with Flutter 3.44.8 (stable), Dart 3.12.2 · framework revision 1a2b3c4d5e`. The values are the app's own report, put into the app by its build; a fork or a custom embedder of Flutter can report values that are not the real ones, so Studio shows them and draws no conclusion. A part the app did not report is not shown, and an app that reports nothing (an older agent, or a build whose tool did not set the values) shows no line. It goes away when the device disconnects, and it is never sent to an AI provider.
- **Clear the app's data before every test.** The box **Clear the app's data before every test** in Workspace settings writes `defaults.clear_data_before_each_test: true` to `probe.yaml` (the same key `probe test` reads, and `--clear-data-before-each-test` overrides). Every test then starts after the app's data was wiped and the app relaunched. A run is asked once, in the window, before the first test; a no stops the run's tests. On a simulator or an emulator, **Run `clear app data` on simulators and emulators without asking** under **Studio on this computer** answers that question for you, and a note names the package and device; a physical phone always asks.
- **`clear app data` without asking on a simulator or emulator.** A step that wipes the app's data asks first, in the window, with a default of no. If you do not want to be asked on a simulator or an emulator, whose data is disposable, press **Run `clear app data` on simulators and emulators without asking** under **Studio on this computer** in Workspace settings (or run `probe prefs set clear-data-without-asking on`: the setting is shared with the command line). It is your own setting on this computer, kept in a file in your user configuration folder; it is never read from a project, so a `probe.yaml` or a cloned repository cannot turn it on, and the automation endpoint and the MCP server cannot press it. It applies only to a device that is verified to be a simulator or an emulator when the step asks (the device list says so, and an Android device also says so itself, because a serial can be given any name): a physical phone, a WiFi connection and any device Studio cannot tell always ask. When it applies, a note names the package and the device that were wiped. A preferences file that is a link or that other users can read or write is not used, and the form says so. Off by default.
- **`clear app data` without asking on a simulator or emulator.** A step that wipes the app's data asks first, in the window, with a default of no. On an iOS simulator the question also says that the simulator's keychain reset clears the keychain of every app on it. If you do not want to be asked on a simulator or an emulator, whose data is disposable, press **Run `clear app data` on simulators and emulators without asking** under **Studio on this computer** in Workspace settings (or run `probe prefs set clear-data-without-asking on`: the setting is shared with the command line). It is your own setting on this computer, kept in a file in your user configuration folder; it is never read from a project, so a `probe.yaml` or a cloned repository cannot turn it on, and the automation endpoint and the MCP server cannot press it. It applies only to a device that is verified to be a simulator or an emulator when the step asks (the device list says so, and an Android device also says so itself, because a serial can be given any name): a physical phone, a WiFi connection and any device Studio cannot tell always ask. When it applies, a note names the package and the device that were wiped. A preferences file that is a link or that other users can read or write is not used, and the form says so. Off by default.
- **Android devices and `adb`** — opening a folder never runs a program that the folder names. The device list, its refresh, the first-run line and the device settings use only the default `adb`: the one on `PATH`, or one in the standard Android SDK places (`ANDROID_HOME` or `ANDROID_SDK_ROOT` under `platform-tools`, `~/Library/Android/sdk` on macOS, `~/Android/Sdk` on Linux, `%LOCALAPPDATA%\Android\Sdk` on Windows), found as an executable file, so a Studio opened from the Dock or Finder, which has no shell `PATH`, can still list Android emulators and phones. The `tools.adb` of `probe.yaml` is used only when you press **Connect**, as `probe test` runs it only when you run tests there, and only if it is an absolute path to an executable file called `adb` outside the repository the workspace belongs to (the folder above it that holds a `.git`; with none, outside the workspace folder), symlinks resolved, because a program anywhere in that repository came with it; then Studio says "Using adb from probe.yaml: adb". `tools.flutter` is not used by Studio. When no `adb` is found and no device is usable, the line under the toolbar says what was searched and what to do ("adb was not found (looked in PATH and ...)") instead of the list being silently shorter.

- **A run that takes long** — the toolbar shows how long the running step has been going ("step 2 of 5 · 7 s", after three seconds), a step that has waited ten seconds adds a line under the toolbar ("Still waiting for ...: Cancel stops the run."), and "no signal" shows next to Cancel when the device sends no screen.
- **Workspace note** — under the folder path in the Workspace pane, one line when the workspace's `probe.yaml` will trip a first run: no `probe.yaml` (runs use the defaults, agent port 48686, and no app id), a `probe.yaml` that cannot be read (Connect and runs are refused with the reason until you fix or remove it; Studio never uses another folder's `probe.yaml` in its place), or no `project.app` (steps that restart or clear the app fail until it is set). It reads the same file runs read, and the refresh button next to the folder path reads it again after you edit it.
- **Result notes** — a test that a retry (`defaults.retry_failed_tests`) ran more than once says so: "Passed on attempt 3, after 2 failed attempts: the test is flaky" or "Failed on all 2 attempts", with the error of each earlier attempt (at most five). Under a failed test, one line that says why the app died (needs `defaults.crash_evidence: true` in `probe.yaml`, Android only; it is the line `probe test` prints), a note when the test ran past `defaults.test_timeout` with the limit, and a hint to turn crash evidence on when a test failed with a lost connection on Android without it. They are shown as text and are not sent to an AI provider.
- **Problems** — the Problems tab under the editor lists what the project's lint rules (`probe lint`) find in the open file: a recipe or a data file that is not there, a test with no steps, a count check. The editor marks each finding on its line, a click on a row (or Enter) goes to the line, and the tab shows the count. It checks the text in the editor, saved or not, against the `probe.yaml` of the open workspace, waits 400 ms after you stop typing, and never blocks Run; text that does not parse yet says it was not checked. A `${NAME}` in a step whose environment variable Studio does not have is a warning on that line (names only: Studio asks whether the variable exists and never reads or shows its value): `probe test` reads variables from the shell it runs in, but a Studio opened from the Dock or Finder does not have your shell's variables, so the step would use the text as written. Start Studio from a terminal where the variable is set (run `flutter-probe-studio.app/Contents/MacOS/flutter-probe-studio`); Studio has no place to set variables, because they often hold secrets. It reads only files inside the open workspace: a recipe, a data file or a recipes folder that points outside it is not read and gets one warning that names no file.
- **First-run guidance** — a line under the toolbar says the one thing to do next (open a workspace, open a `.probe` file, boot a device, pick one, press Connect) and goes away once it is done; an empty Workspace pane says what it needs, with a button for opening a workspace; and every disabled primary button (Run, Record, Save, Connect) says why, as its hover text and as a description a screen reader reads. A remembered folder that has moved or been deleted, and a folder with no `.probe` files, are named as the next step too (open another folder). Nothing is said while the workspace is still loading.
- **Toast notifications**, dark theme, draggable native title bar, About panel.
- **Keyboard shortcuts**: ⌘R run, ⌘S save, ⌘B connect/disconnect, ⌘P open workspace, ⌘K refresh devices, `?` help, `Esc` close help.

## The Network tab

The **Network** tab next to **Results** shows the requests and responses the app made while a test ran, as the agent in the app recorded them. It lists the newest 500 (method, URL, status, time, size, and `mocked` for a response that a `when the app calls` rule supplied), reads the log again every second while a run is going, and has filters for the method, the status class and a text.

Credentials are hidden before anything reaches the window: the value of a header, query parameter, JSON key, form field or XML element whose name says it is a credential (authorization, cookie, token, password, API key, session, any name ending in `key`, and more) is replaced and only the name is shown; every query value, the user name and password in a URL, fragments, email addresses, home folders, tokens and card numbers are hidden too. Bodies show the first 4 KB as text; a binary body, or one over 1 MB, shows only its size. The filter searches the text that is shown, so it can find a parameter name and can never confirm a secret value by finding it.

The list only reads the log of the connected app. It makes no request of its own (the **Send request** panel below does, only when you press Send), stores nothing in the browser or on disk, and nothing in it is sent to an AI provider. Some things are not recognised by name or shape and are shown as they are: a short secret under an ordinary name, a token that is part of a path, UUIDs, IP addresses.

## Send request

**Send** in the Network tab opens a small form: a method, an address, headers (one `Name: value` per line, not saved anywhere) and a body. Studio makes the request from this machine and shows the answer: the status, the time, the redirects that were followed, the response headers and a text preview of the body. The answer goes through the same redactor as the Network tab, so credentials, tokens and personal text in it are hidden; what you typed is hidden wherever the answer echoes it back (a header or body value under a credential name, any token that looks like a credential, query values and path segments that look like one, also in another case, split by invisible characters or percent-encoded), so a value with no name or shape of its own does not come back in clear. A value that is not a credential, such as `hello`, is left readable, and the address you typed is shown with its query values hidden. Hosts are kept in one form (an IPv6 address in brackets, a name written in unicode in punycode with the unicode name shown as a label, no trailing dot). Nothing is shown as markup and nothing is stored in the browser, on disk or in a log; nothing goes to an AI provider.

The first request to a host in a session asks first. The dialog names the host and port and says where the name leads (a public address, this machine, a private network); an address on this machine or a private network gets a stronger warning, because the request then comes from your machine into your own network. **Allow** lets Studio call that host and port, at the strength that was shown, for the rest of the session: an allow made while the name led to the internet does not cover it if it later leads to this machine or a private network (the dialog comes back), and an allow for those never covers a cloud metadata, unspecified or multicast address. A host that was not checked first cannot be allowed (at most 100 hosts are remembered, and at most 30 address checks a minute), **Cancel** (or Esc) sends nothing. The rules are enforced by Studio, not only by the window: an unallowed host is refused whatever calls it.

Limits: only `http` and `https`, no user name or password in the address, no `Host`, `Content-Length`, `User-Agent`, `Accept-Encoding` or connection headers (Studio sets them; the User-Agent is `FlutterProbe-Studio`), at most 50 headers, a 256 KB request, 30 seconds, a 1 MB response (a longer one is marked cut), at most 5 redirects on the same host, one request at a time and ten a minute. A response that ends early (a body shorter than it said, a broken or cut stream) is an error and none of it is shown. A redirect to another host is shown and not followed; send it as a new request to be asked about that host. Cloud metadata addresses and the unspecified and multicast addresses (all of them, also 224.0.0.1 and ff02::1) are never called, also when written as an IPv6 address that routes to one (NAT64, 6to4); reserved ranges get the same stronger warning as this machine. A name that answers with several addresses is called only when all of them pass. Nothing is kept: no cookies, no connection is reused, and the certificate is always checked. In the browser-driven (headless) build, an address on this machine or its network is refused unless it is named by the test allow-list (`PROBE_STUDIO_TEST_ALLOW_LOCAL`).

### Save as step

**Save as step** inserts the typed request into the open file as a [`call` step](/probescript/grammar/#http-calls): `call POST "https://host/path?id=${PROBE_SAVED_ID}" with body "{...}"`. It uses only what the grammar has: the methods GET, POST, PUT and DELETE (a PATCH, HEAD or OPTIONS request cannot be saved, and Studio says so), and a body of text. It sends nothing and asks nothing, and the file is saved with the editor's own save.

- **Where it goes.** Under the line the cursor is on, inside the test (or recipe or hook) the cursor is in, at the indent of the steps there. With no file open, a read-only file, a cursor that is not inside a test, or a step that would add a parse error at that place, nothing is inserted and Studio says why.
- **Headers are not saved.** The step has no way to carry them, and `probe test` sends a body as `application/json`.
- **Placeholders.** Every query value, and every path segment or JSON, form or text value that looks like a credential (eight characters or more with a letter and a digit), is under a credential name, is a card number or is an email address, is written as `${PROBE_SAVED_NAME}`: `probe test` fills it from the environment variable of that name and masks it in its output. The names all start with `PROBE_SAVED_`, so they never collide with a variable your shell already has (`USER`, `PATH`, `HOME`), and a comment line above the step names the ones to set.
- **A `${NAME}` or `<name>` you typed cannot be saved.** `probe test` fills in `${NAME}` from the environment and `<name>`, `<response.body>` or `<random.email>` from its variables whenever a step runs, and there is no way to write them literally, so the saved step would do something other than what you typed. Studio says so and saves nothing. (An XML body with plain tags such as `<password>x</password>` has the same problem; tags with attributes are fine.)
- **The limit.** Values the save cannot recognise are **not** replaced: a passphrase of letters only, a short value, a six-digit code, a value with no credential name and no shape, a value that is a name itself, and the words after the first one of a value in free text outside the credential-name lines. Read the generated step before you save the file; the panel and the status line say so.
- **What a step cannot say exactly is not saved:** an address with a user name or password, an address or body with an invisible or control character, a GET with a body, a query name that looks like a credential, a JSON object key that holds a run of 16 or more hex digits, a JWT or padded base64, or is over 1,024 bytes (hex in short groups, like an id with hyphens, is kept) (an email address or a card number used as a key is replaced), a request with more than 200 values to replace, or anything the ProbeScript parser does not read back as the same request.

## Automation (no window, no cursor)

Start Studio with `PROBE_STUDIO_AUTOMATION=1` and scripts or AI agents can drive it without touching the mouse or the keyboard focus. Studio then listens on **loopback only**, makes a random token for that launch and writes both to a private discovery file (`<user config dir>/flutter-probe-studio/automation.json`, mode 0600; `PROBE_STUDIO_AUTOMATION_FILE` moves it, `PROBE_STUDIO_AUTOMATION_PORT` fixes the port). Studio refuses to use a folder or file that is a link, that belongs to another user, or that others can reach (a file that is open to others, a folder that others can write to), and with no user config folder and no `PROBE_STUDIO_AUTOMATION_FILE` it does not start the endpoint: it never falls back to the shared temporary folder. Only the last component of the path is checked (a link in a folder above it, such as `~/.config`, is followed as you set it up), and on Windows the mode and owner checks are skipped. The endpoint is **off by default**.

`POST <url>/rpc` with `Authorization: Bearer <token>` and `{"method": "...", "params": {...}}` answers `{"result": ...}` or `{"error": "..."}`. Requests from a browser (any `Origin`, or `Sec-Fetch-Site: cross-site` or `same-site`), with a non-loopback `Host`, or without the token are refused. Methods: `open_workspace {path}`, `list_dir`, `read_file`, `list_devices`, `connect {deviceId}`, `connect_wifi {host, port, token}`, `test_wifi {host, port, token}`, `disconnect`, `status`, `run_file {path, line?, tag?, failedOnly?}` (returns at once; the optional fields pick part of the file as the Run control does, at most one of them), `cancel`, `run_state` (the running step, finished steps, plan, results), `results`, `ui_state`, `screenshot` (the connected device, base64 PNG), `widget_tree`.

**What the token grants.** Whoever holds the token can drive the connected device (taps, typing, screenshots, running tests), open workspaces, list folders and read and run `.probe` files, but **only inside the allowed folders**: the workspace a person opened in the window (or `PROBE_STUDIO_WORKSPACE`) plus the folders in `PROBE_STUDIO_AUTOMATION_ROOTS` (separated like `PATH`). With neither, `open_workspace`, `list_dir`, `read_file` and `run_file` are refused. Paths must be absolute; symlinks are resolved on both sides before the check, so a link that points out of an allowed folder does not get out of it, and `.probe` is checked on the resolved file. A workspace chosen through automation does not widen the allowed folders. Keep the discovery file at mode 0600 (Studio creates it that way) and treat the token like a password: anything that can read it can run tests on your device. The MCP client refuses to send the token to any address that is not loopback.

A workspace matters because Studio reads its `probe.yaml` (agent port, timeouts, device ids, and the AI and cloud endpoints) and runs its `.probe` files. Nothing in `probe.yaml` names a host command and Studio does not use the `tools:` paths, but a test file can make the host call a URL (`call GET ...`), copy a host file to the device (`add media`) or write screenshots and logs, which is why the folders are confined.

The MCP server exposes the same through `studio_open_workspace`, `studio_list_devices`, `studio_connect`, `studio_run_file`, `studio_cancel`, `studio_run_state`, `studio_results`, `studio_screenshot`, `studio_widget_tree` and `studio_ui_state` (the last two are read-only), so an AI client can open a workspace, connect, run a `.probe` file and read step-by-step progress.

## Testing Studio

Studio's own UI is tested two ways. `make studio-e2e-web` runs the real frontend and Go backend in headless Chromium (Studio's headless mode serves the UI over loopback; a small bridge stands in for the window runtime), so it needs no window, cursor or focus and its device-free half runs on a normal CI runner. `make studio-e2e-native` drives the real window through macOS accessibility for what needs one (window placement, focus, recording); it needs a second display named by `STUDIO_E2E_DISPLAY`. Headless mode is for tests only; to drive Studio from a script or an AI client use the [automation endpoint](#automation-no-window-no-cursor), which is confined to the project.

## Architecture

```
┌─────────────────────────────────────────────┐
│ Studio (Wails 2.12, single binary per OS)    │
│ ┌───────────────────────────────────────┐   │
│ │ WebView frontend (Vite + TypeScript)  │   │
│ │  - Monaco editor + ProbeScript grammar│   │
│ │  - Device pane (img stream)           │   │
│ │  - Inspector / results timeline       │   │
│ └─────────┬─────────────────────────────┘   │
│           │  Wails bindings + EventsEmit    │
│ ┌─────────▼─────────────────────────────┐   │
│ │ Go backend (App struct)               │   │
│ │  imports internal/runner              │   │
│ │  imports internal/parser              │   │
│ │  imports internal/probelink           │   │
│ │  imports internal/device              │   │
│ └─────────┬─────────────────────────────┘   │
└───────────┼─────────────────────────────────┘
            │  WebSocket / HTTP (existing transports)
            ▼
┌──────────────────────────────────┐
│ Flutter app under test            │
│  + flutter_probe_agent package    │
└──────────────────────────────────┘
```

The runner is a library, not a subprocess. The connection-stability work in v0.6.0 (configurable reconnect, signal handler, iproxy probe, serialization) applies to Studio without any extra wiring.

## System requirements

- macOS 11+ (Apple Silicon or Intel), Windows 10/11, or a recent Linux distribution
- A running Flutter app on a booted simulator, online emulator, or physical device, launched with `--dart-define=PROBE_AGENT=true`
- Linux only: WebKitGTK 4.1+ (`apt install libwebkit2gtk-4.1-dev`)
- Physical iOS over USB: `libimobiledevice` for `iproxy` (`brew install libimobiledevice` on macOS)
- Physical Android: `adb` on `PATH` (Android SDK platform-tools) — same requirement as emulators
- Physical devices over WiFi: `flutter_probe_agent` built with `--dart-define=PROBE_WIFI=true`, plus a `ProbeAdvertiser` in the app (agent 0.15.0+; before that mDNS shipped inside the agent) for mDNS discovery — see [WiFi auto-discovery](#wifi-auto-discovery)

## Physical devices

Studio connects to physical iOS and Android devices through the exact same `internal/device` package the `probe` CLI uses — there is no separate device-management code path for Studio.

- **USB** — plug in the device, launch the app with `--dart-define=PROBE_AGENT=true`, click ↻ to refresh the device picker, and pick the entry tagged `physical`. Physical iOS gets an `iproxy` tunnel automatically before Studio dials the agent; physical Android goes through `adb forward`, the same path as emulators. Connection errors for a missing `iproxy` or `adb` binary surface inline with an actionable fix (e.g. "install via: brew install libimobiledevice").
- **WiFi (preferred for iOS)** — matching the CLI's own documented preference, WiFi is more stable than USB-C for physical iOS: a USB-C cable's charge/data mode-flipping can drop the `iproxy` tunnel mid-run, where WiFi has no such failure mode. Build the Flutter app with `--dart-define=PROBE_WIFI=true` and pass an advertiser to `ProbeAgent.start` (see [WiFi auto-discovery](#wifi-auto-discovery)), click the 📡 button to open the WiFi discovery overlay, and pick the device once it appears via mDNS (`_flutterprobe._tcp`). Paste the token printed in the app's `PROBE_TOKEN=...` log line. Studio keeps it in the OS keychain for that device, so the next connection needs no paste. A device with a saved token shows a 🔑 saved tag, and its ✕ control forgets the token.
- **A saved WiFi token is tied to an address.** Studio keeps the token of each WiFi device in the OS keychain, together with the address (host and port) of the connection that accepted it, and sends it only there. When a device with that name shows up at a different address, or a token was saved before addresses were recorded, Studio asks first ("Send the saved token?", with both addresses; Cancel is the default, and no answer means no), because any device on the network can announce any name. Paste the token to connect without the question.
- The exact conditions for a physical iPhone over WiFi, with a three-command check, are in [Physical iPhone over WiFi](/platform/ios/#physical-iphone-over-wifi-cli-and-studio-what-must-be-true).
- Devices that don't advertise over mDNS (no advertiser in the build, older agent versions, or networks that block mDNS): use **Connect manually** in the WiFi overlay (host, port, token) — see [Manual WiFi entry](#manual-wifi-entry) — or the CLI's `--host <ip> --token <token>`.

### The device picker and physical phones

Physical iPhones come from two sources and are labelled by how Studio can reach them: **USB** (seen by `libimobiledevice`, reached through `iproxy`) and **WiFi only** (paired and reachable over the local network, seen by `xcrun devicectl list devices`, Xcode 15 or later). Before this, a phone that was paired over the network but not on a cable simply did not appear. Each entry shows the name you gave the phone and its model (for example `Jane's phone (iPhone 15) · physical · WiFi only`).

Selecting a physical device shows what it needs in a line under the toolbar, instead of leaving you with a Connect button that fails: the app must be **running in the foreground with the agent** (a profile build made with `--dart-define=PROBE_AGENT=true`; add `--dart-define=PROBE_WIFI=true` for WiFi). A WiFi-only phone has Connect disabled, and the line and the button's tooltip say to use the 📡 button and **Connect manually**. An Android phone that is `unauthorized` or `offline` says to unlock it and accept the USB debugging prompt. When nothing is found at all, the message points at the 📡 button too.

## Quick start

1. **Boot a simulator/emulator, or connect a physical device.** Xcode → Devices, or `xcrun simctl boot <udid>` for iOS sim; Android Studio → Device Manager for Android emu; or plug in a physical iOS/Android device over USB (see [Physical devices](#physical-devices) for WiFi).
2. **Launch your Flutter app** with the agent enabled:
   ```bash
   flutter run --dart-define=PROBE_AGENT=true
   ```
3. **Open Studio.** Click the 📂 button in the Workspace pane and pick your project root (the folder containing `probe.yaml` and `tests/`).
4. **Pick the device** in the toolbar dropdown (or use the 📡 WiFi button for a WiFi-connected physical device) and click **Connect**. The device pane should fill with a live mirror; the inspector pane gets a green pulsing **LIVE** badge.
5. **Open a `.probe` file** from the Workspace pane (or write one inline) and click **Run**. Per-test results stream into the Results pane below. To run less than the file, pick the test at the cursor, the tagged tests or the failed tests in the scope control beside **Run**. Click a failed step, or a failed test, to open the line where it failed, in that step's file: another file of the workspace is opened first (after the usual question about unsaved changes, and nothing moves if you cancel), a file name with `..` in it, one outside the workspace, a link out of it, or a file that is gone is refused with a sentence and leaves the editor as it was, and a line past the end of the file shows the last one and says so. Only a click moves the editor. The Explain failures notes and the Suggest a fix card also name the place of the failure (`L12`, with the file's name on hover and never its folder) and the same click opens it, in the same way. A failed `see` or `tap` on a text or an id shows, under the failed step, a suggested fix (for a text, a `wait until` line to try before the step) and the reason, from rules that run without AI.

## What the live view is, and its frame rate

The device pane is a **read-only** view. Studio asks the on-device agent for a screenshot and for the widget tree, over the same connection a test run uses, in a loop with no fixed delay: the next request goes out as soon as the previous answer arrives. There is no click-to-tap and no typing into the pane (the agent has no coordinate tap); use the widget tree and the recorder to build steps.

Because the image comes from the app's own renderer (`RenderRepaintBoundary.toImage`), the pane shows **only the Flutter layer**: the status bar, system dialogs, keyboards drawn by the OS and native views are not in it. The frames are PNG, sent as text, and the agent waits for the app to settle before capturing, so a screen with a running animation can capture slower than an idle one. The toolbar shows the frames per second and, per frame, the average and slowest capture time over the last second (`frameMs`, measured on the host around one screenshot and widget-tree pair; it does not include decoding and painting the image in the window).

Measured on one machine (Apple M5 Max, idle fixture app on a static screen, 150 frames each, the host also running other work, so treat the numbers as a rough guide):

| Device | Frames per second | Capture time per frame (median / 95th percentile) | PNG size |
|---|---|---|---|
| iOS simulator `ProbeBaseline-iPhone17` (iOS 26.5) | 14.1 | 68 ms / 86 ms | 52 KiB |
| Android emulator `ProbeRepro-API35` (Android 15) | 13.4 | 72 ms / 84 ms | 44 KiB |

A physical phone (USB or WiFi) uses the same agent request, so expect similar or lower rates depending on the tunnel or network; it has not been measured. A busier screen means larger frames and a lower rate. Faster, video-based views are a possible future change, not something Studio does today.

## Shared device view (preview, off by default)

Start Studio with `PROBE_STUDIO_DEVICE_VIEW=1` and the device pane uses the same interactive view as `probe device view` and the VS Code extension instead of the screenshot loop above. Studio starts a loopback-only hub for the connected device (a random token for this connection, never written to disk) and the pane draws its frames on a canvas. With it you can:

- **tap, swipe and type** in the app by clicking, dragging and typing in the pane (the agent must support the input calls; an older agent gives a read-only view);
- **hover** to outline the widget under the pointer, and **Alt+click** to select it: the selector (`#key`, else the text) is shown, copied to the clipboard and put in the inspector's search box;
- press **+ tap** (device pane header) to write a `tap` step for the selected element at the editor cursor: on a blank line it fills the line, otherwise it goes on a new line below with the same indentation. The selector is built by the same escaping rule as the clipboard copy, so text on the device screen cannot add a second statement;
- see frames at a lower cost: the pane asks the agent for the frame size it needs, and the agent captures at that size.

The pane header says what the device supports: "view only" when the agent has no input calls (an older `flutter_probe_agent`; taps, swipes and typing are off) and "no element select" when it cannot hit-test (hover and Alt+click are off); hover the label for the reason. The inspector text refreshes every 1.5 seconds in this mode (it is no longer bundled with each frame). If another window takes the view over (a second connection replaces the first), the pane shows "taken over" and a Reconnect button; it never reconnects by itself. Without the variable nothing changes: the old loop runs and no hub is started. The protocol is in `docs/studio-device-protocol.md`; this mode is a preview until it has run on a physical phone.

## Device settings

The **Settings** button in the device pane header opens a panel that changes OS-level settings of the connected emulator or simulator, so you can see a screen in dark mode, with a large font or in another language without leaving Studio:

- **Appearance** (light, dark; auto on Android), **font size** (an iOS content size category, or an Android factor such as 1.3), **app language** (needs the app id, which defaults to `project.app` in `probe.yaml`; the app restarts on iOS), **orientation** (Android only; iOS simulators have no command-line rotate) and **open link** (a deep link, delivered to the app when it claims the scheme).
- It always acts on the device Studio is connected to; there is no way to name another one. WiFi connections and physical iPhones are not supported (the panel says why), because the changes go through `adb` and `simctl`.
- Settings stay on the device until you change them back. The same operations are available from the command line (`probe device set`) and to AI clients (`set_device_setting`).

## Build from source

```bash
# Prerequisites
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor   # verify your toolchain

# Build the desktop binary for your current OS
cd studio
wails build

# Output:
#   studio/build/bin/flutter-probe-studio.app   (macOS)
#   studio/build/bin/flutter-probe-studio.exe   (Windows)
#   studio/build/bin/flutter-probe-studio       (Linux)
```

`wails dev` provides a live-reload development loop with browser dev tools available at `http://localhost:34115`.

## Known limitations

- No multi-device side-by-side view.
- `clear app data` in a Studio run always asks in a dialog (see *Runs like `probe test`*); a run that nobody is watching (the automation endpoint with no window) does not run it. Use `probe test --yes` for an unattended run.
- The toolbar needs about 1120 px; the window's minimum width is 1040 px, so between 1040 and 1120 px the page scrolls sideways a little. The default window is wider.
- Problems (lint) checks the text in the editor up to 1 MiB; a longer text says it was not checked.
- No per-step replay / time-travel debugging — see the Results pane for pass/fail, but stack inspection requires re-running.
- The AI chat pane talks to the Anthropic API with your own key stored in the OS keychain, to the OpenAI API when `probe.yaml` has an `ai:` block with `provider: openai`, or to a local model with `provider: local`. A local endpoint must be on this machine, unless `studio.ai_allow_remote_endpoint: true` confirms another address. A key in `probe.yaml` must be an environment variable named as `${NAME}`. **Workspace settings** has an **AI provider** section for these options, with a model list and **Test connection**. **Find local models** asks this machine's own model server, such as Ollama, which models it has: only on your click, only a loopback address (the endpoint in the form when it is this machine, and `127.0.0.1:11434`), with a read-only request for the model list and no key. It fills only fields that are empty and says what it filled, picks a model only when exactly one is listed, and saves nothing until you press Save. The model list and the test use the saved settings, so save before you test. Before a message goes to a destination you have not confirmed in this window, Studio shows what that message would send now, after redaction, and waits for **Send**. It asks again when the destination changes, and after the workspace, the provider settings or the saved key change. This check is in the Studio window only. **Explain failures** (under the Results title, shown only while AI is on and the last run has a failed test with error text) asks the provider for short advice on each failed test, at most 5, using the same triage as `probe triage`. It shows what it would send first, with the same **Sent to** line, and sends only after **Send** (or Cancel, or Escape, which sends nothing). For a hosted or remote provider the preview is shown before every Explain and cannot be turned off. For a model on this machine it is shown the first time in a session, and again after the destination, the workspace, the provider settings or the saved key change. A new run, or a change of workspace, settings or key, drops an Explain that is still waiting for its answer, so an answer for an older state never shows. What it sends is the test name, the file (relative to the workspace) and the error text, each cleaned: hidden characters dropped, the key removed, emails and home folders replaced, credentials redacted, and a size limit. No widget tree and no screenshot is sent. The answer appears in an **Advisory** card that names the model and whether it is local or hosted, is shown as text, and never changes a result. The same settings live in the `ai:` block of `probe.yaml`, which the CLI reads too. Settings never takes a key: the key button in the AI panel stores it in the keychain, and the panel shows only whether one is saved. **Save** keeps a plain `api_key` that is already in `probe.yaml`, because the CLI reads it, and Studio says it does not use it; naming an environment variable replaces it. A form you did not change is not validated or written, and a refused AI section shows its reason while the other settings still save. The AI section writes only the fields you changed, and asks you to reopen Settings if `probe.yaml` changed under the form for a field you changed, so an edit made elsewhere is never put back. The chat header names where the next message would go: the provider and the model (`on this machine` for a local endpoint on this machine, `remote endpoint` for a confirmed one), `Anthropic, key needed` when Anthropic is chosen and no key is saved, or that AI is off or needs a fix in Settings. It is filled when the chat pane opens. It never shows an address or a key. Each message sends the open file. The failures of the last run and the connected device's widget tree are sent only when you turn them on, under **AI chat context** in Workspace settings, or in `probe.yaml` under `studio.ai_context` (`run_results` and `widget_tree`). **Both default to off**: until you change them, the chat sends only the open file. When either is on, a line above the chat input says what is sent. Before the widget tree is sent, obscured fields and credential-looking text (API keys such as `sk-…`, GitHub tokens such as `ghp_…`, `Bearer` values, and long hex or base64 runs) are redacted. Each part is cut to a size limit and marked when it is cut. The chat is not exposed through the automation endpoint or the MCP server; use the standalone `probe-mcp` binary with Claude Desktop or Cursor for MCP-driven work.

## Configuration

Studio reuses `probe.yaml` from the workspace root. Connection-related fields (`agent.dial_timeout`, `agent.reconnect_attempts`, `agent.reconnect_backoff`) are honored exactly as the CLI uses them. See [Configuration Reference](/advanced/configuration/).

## Reporting issues

Studio bugs go to the same tracker as the CLI: [github.com/AlphaWaveSystems/flutter-probe/issues](https://github.com/AlphaWaveSystems/flutter-probe/issues). Please include the OS, Wails version (`wails version`), the connected device platform and OS version, and any relevant `studio/build/bin/.../*.log` output.

## Manual WiFi entry

The 📡 overlay has a **Connect manually (host, port, token)** section for a phone that does not show up in the discovered list.

- **Host**: an IPv4 address, an IPv6 address or a hostname (such as `iphone.local`): no `ws://`, path or port. **Port**: 1 to 65535, default 48686 (the port the app printed as `PROBE_PORT=` if it moved). **Token**: the value after `PROBE_TOKEN=` in the app's console (8 to 128 letters, digits and `_ . ~ -`; without the `PROBE_TOKEN=` part).
- **Test connection** dials the agent and pings it without making it the active connection, and says whether the agent is reachable and the token accepted. **Connect** does the same and then connects. The host and port are remembered for next time. A manually entered device does not save its token, so paste it each time; a device found over mDNS keeps its token in the keychain.
- When it fails, the message says what to check: the phone and this computer on the same network (and `curl http://<host>:<port>/probe/status` from this computer), the app in the foreground with the screen unlocked, a **profile build made with `--dart-define=PROBE_AGENT=true` and `--dart-define=PROBE_WIFI=true`** (a production or debug build does not listen), the port, and a fresh token (a new one is created whenever the app restarts).
- An agent can do the same through the automation endpoint (`test_wifi`, `connect_wifi`) or the MCP tool `studio_connect` with `host`, `port` and `token` (and `test_only` to only check).

**Where Studio will connect.** The manual entry sends the agent token to the address you type, so it only accepts a target on this machine or a local network: loopback, link-local, private IPv4 (10.x, 172.16 to 31.x, 192.168.x), IPv6 unique-local, and hostnames such as `iphone.local` or a single-label name. A hostname is resolved and every address it resolves to must be local too (a name that resolves to a public address is refused like the address itself). To connect to a public address on purpose, start Studio with `PROBE_STUDIO_ALLOW_PUBLIC_TARGETS=1`; automation and MCP callers cannot switch this on. Test and connect attempts are limited to 6 per minute, and error messages never contain the token.

The full list of conditions on the phone and the network is in [Physical iPhone over WiFi](/platform/ios/#physical-iphone-over-wifi-cli-and-studio-what-must-be-true).

## WiFi auto-discovery

Studio treats an announcement as untrusted text from any machine on the network: a device name is cleaned of control, invisible and direction-changing characters and shown at most 64 characters long, the version must be one short word, an address that is not a place on the network (0.0.0.0, multicast, broadcast, loopback) or a port outside 1 to 65535 is not listed, at most 256 devices are listed per search, and you still enter the token yourself; the token is never announced.

The core agent has **no native dependency**: a native plugin in your app's dependencies is linked into every
build of that app, release included. mDNS advertising needs one (Bonjour/NSD), so the agent only defines a hook,
`ProbeAdvertiser`, and you implement it in the build you test on devices. No extra FlutterProbe package is needed.

Add `bonsoir` to your app (ideally only in a dedicated flavor/target):

```yaml
dependencies:
  bonsoir: 6.0.2
```

```dart
import 'package:bonsoir/bonsoir.dart';
import 'package:flutter_probe_agent/flutter_probe_agent.dart';

class BonsoirAdvertiser extends ProbeAdvertiser {
  BonsoirBroadcast? _broadcast;

  @override
  Future<void> start({required String name, required int port, required String agentVersion}) async {
    try {
      final broadcast = BonsoirBroadcast(
        service: BonsoirService(
          name: name,
          type: mdnsServiceType, // _flutterprobe._tcp
          port: port,
          // The auth token is deliberately NOT advertised.
          attributes: {'version': agentVersion, 'port': '$port'},
        ),
      );
      await broadcast.initialize();
      await broadcast.start();
      _broadcast = broadcast;
    } catch (e) {
      print('ProbeAgent: mDNS advertise failed: $e'); // never block the agent
    }
  }

  @override
  Future<void> stop() async {
    try {
      await _broadcast?.stop();
    } catch (_) {}
    _broadcast = null;
  }
}

Future<void> main() async {
  await ProbeAgent.start(advertiser: BonsoirAdvertiser());
  runApp(const MyApp());
}
```

This is the code of the repository's own fixture (`native-test-apps/flutter/lib/probe_advertiser.dart`), which builds for iOS and Android.
Use `bonsoir` 6.0.2 for now: its 7.1.x Android plugin (`bonsoir_android` 7.1.3) uses `NsdServiceInfo.hostname`, which did not compile against the
Android SDK platform 36 that was installed when this was checked (2026-10-11) (7.1.x was not tried further). On iOS, bonsoir needs a deployment target
of at least 13.0 ([its installation notes](https://github.com/Skyost/Bonsoir/blob/main/docs/content/docs.md)).

Run with `--dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true`. On iOS 14+, add `_flutterprobe._tcp` to
`NSBonjourServices` and a `NSLocalNetworkUsageDescription` in that build's `Info.plist`. It only advertises in
WiFi mode (agent bound to `0.0.0.0`). Without an advertiser, WiFi testing still works with
`probe test --host <ip> --token <token>`; the agent logs `PROBE_MDNS=off`.
