# Studio E2E matrix

Every Studio behaviour that is tested, with its variants and edge cases, and where it is tested. Three layers:

- **B** — browser spec (`e2e-web/tests/*.spec.ts`, headless Studio + Playwright, no device): `STUDIO_E2E_NO_DEVICE=1 scripts/studio-e2e-web.sh`.
- **D** — device spec (same suite, on a named simulator with the fixture app): `scripts/studio-e2e-web.sh`.
- **W** — the real desktop window (the Wails app), driven by hand through the screen on a named simulator with the fixture app. These rows are the
  *window run*: they check what the headless build cannot (window chrome and moving it, the native folder dialog, real keyboard focus, layout).

Status column (filled by a run; see `RESULTS.md` for the dated record): ✅ pass · ❌ fail (fixed in the same run, then re-run) · ➖ not run (reason).

## 1. Start-up and first run

| ID | Scenario | Variants / edge cases | Layer | Spec / procedure |
|---|---|---|---|---|
| S-01 | Start with no workspace | next step names the folder; empty pane says "No workspace open" + button; Run/Record/Connect/Save each give a reason (title = description) | B | first-run fr-01 |
| S-02 | Open-workspace button | keyboard focus + Enter makes the same call as the folder button | B | fr-02 |
| S-03 | Workspace still loading | nothing said, strip hidden, then the pane line exactly once | B | fr-03 |
| S-04 | Workspace with files | no pane hint; strip asks for a file; goes once a file is open; Save's reason goes when edited | B | fr-04 |
| S-05 | Guidance is text only | no child element in strip or reasons | B | fr-05 |
| S-06 | Empty workspace folder | "No .probe files here" | B | fr-03 |
| S-07 | Run while a run is in flight | Run disabled with "A run is in progress."; typing keeps it; goes at the end | D | rd-01 |
| S-08 | Fresh start of the desktop app | window opens, toolbar visible, no error toast, editor placeholder is the scratch note | W | W-01 |
| S-09 | Move and resize the window | drag by the title bar; resize keeps layout; panes do not overlap | W | W-02 |

## 2. Workspace and files

| ID | Scenario | Variants / edge cases | Layer | Spec / procedure |
|---|---|---|---|---|
| F-01 | Pick a workspace | native dialog opens; the chosen folder is listed; probe.yaml is read | W | W-03 |
| F-02 | Navigate folders | open a folder, go up, refresh; the path label follows | B/W | ui.spec, W-04 |
| F-03 | Open a .probe file | name shown; editor shows content; not dirty | B/W | ui.spec, W-04 |
| F-04 | Unsaved-change prompt | switching file with edits asks once; Cancel keeps; OK discards | B | goto-line gl-03 |
| F-05 | Save | disabled without edits; enabled after typing; writes once; Cmd+S | B/W | ui 24, W-05 |
| F-06 | ReadFile/WriteFile refusals | outside workspace, `..`, link out, non-.probe | B | goto-line gl-04, Go readfile_roots_test |

## 3. Devices and connection

| ID | Scenario | Variants / edge cases | Layer | Spec / procedure |
|---|---|---|---|---|
| C-01 | Device list | refresh; booted simulators listed by name; shut-down ones listed with a hint | B/W | ui 22/25, W-06 |
| C-02 | Pick during refresh | a device picked while the list refreshes stays selected | B | ui 25 |
| C-03 | Connect to a named simulator | status shows the device name; Run/Record enable | D/W | device.spec, W-07 |
| C-04 | Connect failure | no agent: sentence says what to check; error toast | D | device.spec |
| C-05 | Disconnect | status resets; Run/Record disabled again with reasons | W | W-08 |
| C-06 | Shared device context | restart/kill/permissions steps work in a Studio run (app id, timings from probe.yaml) | D/W | Go device_context_test, parity run, W-10 |
| C-07 | Wi-Fi overlay | opens/closes; manual entry refuses a malformed host, empty host; unreachable agent says what to check; token not kept on reopen | B | ui 06/19/20/21/23 |
| C-08 | Physical device | list shows physical phones with what they need; HTTP transport; semantics-wrapped taps | D (physical) | physical.spec (needs a phone) |

## 4. Run, results, cancel

| ID | Scenario | Variants / edge cases | Layer | Spec / procedure |
|---|---|---|---|---|
| R-01 | Run a passing file | steps stream with ✓; verdict row passes; Run re-enables | D/W | device.spec, W-09 |
| R-02 | Run a failing file | failed step row, error text, red verdict; failing line highlighted | D/W | device.spec, W-11 |
| R-03 | Run with a restart step | `restart the app` works (app id, delays) | D/W | parity run, W-10 |
| R-04 | Run scope | whole file / test at cursor / by tag / failed only; empty scope refused with a sentence | B/D | ui, device.spec |
| R-05 | Cancel a run | Cancel stops the run; Run re-enables; no stuck state | D/W | device.spec, W-12 |
| R-06 | Run without a device | explains what to do | B | ui 08 |
| R-07 | Open the failing line | click a failing step → file + line; relative name; other file; unsaved prompt; refused names; past-the-end line; keyboard Enter; passing row is no jump | B | goto-line gl-01..07 |
| R-08 | Live step highlight | gutter glyph follows the runner; verdict stays | W | W-09 |

## 5. Editor, Record, Insert tap

| ID | Scenario | Variants / edge cases | Layer | Spec / procedure |
|---|---|---|---|---|
| E-01 | Syntax and lint | ProbeScript highlighting; lint marks an unknown step | B | ui, probescript tests |
| E-06 | Problems tab (lint) | rows + markers with lines; click/Enter goes to the line without typing; lints unsaved text; 400 ms debounce; stale answer dropped; lint failure is one line; unparsable text "Not checked"; findings are text | B | lint-problems lp-01..08 |
| E-07 | Confirm before `clear app data` | dialog names app + device; Cancel focused; Escape/Cancel deny; Enter never presses Clear data; second step asks again; timeout / no window / cancelled run deny; unknown kind refused; ids single-use and bound to the run; automation cannot answer | B/D | confirm-dialog cd-01..06, confirm-device cd-10/11, Go confirm_test |
| R-09 | Result notes (crash line, timeout, hint) | crash line under the error; timeout + limit; hint only with a lost connection on Android and evidence off; none for passed/skipped; markup is text; not sent to a provider | B | result-notes rn-01..06, Go run_result_test |
| E-02 | Record | disabled without device (reason shown); records taps into the editor | B/W | ui 24, W-13 |
| E-03 | Insert tap | + tap writes a selector; refuses control characters (U+2028/2029 and hidden set tracked, see PR queue) | B | device-stream tests |
| E-04 | Inspector / widget tree | loads after connect; refresh | D/W | device.spec, W-14 |
| E-05 | Device settings panel | says to connect first; ApplyDeviceSetting refused without connection | B | ui 26/27 |

## 6. Network and Send

| ID | Scenario | Variants / edge cases | Layer | Spec / procedure |
|---|---|---|---|---|
| N-01 | Request log | seeded exchanges listed; headers/bodies redacted (cards, JWTs, tokens, URL query) | B | network.spec |
| N-02 | Send a request | method/url/headers/body; allow-list prompt; response shown | B | send.spec |
| N-03 | Send refusals | local/private addresses refused except the test allow; rate limit; redirects | B | send.spec, Go send tests |

## 7. AI (Explain, Suggest a fix, Chat)

| ID | Scenario | Variants / edge cases | Layer | Spec / procedure |
|---|---|---|---|---|
| A-01 | AI settings | provider, endpoint, model; key save/delete (memory keychain in tests); local model discovery | B | ai-settings, ai-local-discovery |
| A-02 | Explain | preview before send; send; note shows where the failure is; refused places | B | ai-explain, ai-preview, selector-fix sf-21 |
| A-03 | Suggest a fix | preview; card; Replace the line; refuses comments/`;`/`${ENV}`/composite/containers; attempts cap of 3; profile equality | B | selector-fix sf-01..21 |
| A-04 | Re-run after Replace | needs Save + a device; verdict; card survives the Explain offer going away | B/D | selector-fix rr-01..05, fix-roundtrip dg-01..06 |
| A-05 | Chat | label, per-field gate, clear, key prompt | B | ai-chat-label |
| A-06 | Gate | credentials, cards, JWTs, hidden characters, home paths never leave | Go + B | ai_*_test.go |

## 8. Chrome, accessibility, keyboard

| ID | Scenario | Variants / edge cases | Layer | Spec / procedure |
|---|---|---|---|---|
| U-01 | Help overlay | `?` opens, Escape closes | B/W | ui 07, W-15 |
| U-02 | Every disabled primary button has a reason | title = description (`aria-describedby`); enabled has none | B/D | first-run, rd-01 |
| U-03 | Keyboard reach | Tab order reaches toolbar, pane button, rows with Enter | B/W | goto-line gl-06, W-16 |
| U-04 | Toasts | appear, stack, auto-close | B | ui |
| U-05 | Bridge | event stream connected, Go methods answer | B | ui 09 |
