# Studio E2E results

Run of the matrix in `MATRIX.md`. Newest first.

## 2026-11-16 — mutants of the confirmation dialog, run by hand

A manual run (not committed evidence: nothing in CI mutates the code), on `origin/main` after #672, with the headless suite without a device (`STUDIO_E2E_NO_DEVICE=1 scripts/studio-e2e-web.sh confirm-dialog`, the node suite `npm run test:confirm` for the first node row, and `go test -race` in `studio/` for the Go rows). Each row changes one line, runs the named spec, and puts the line back. The browser rows exist because the dialog's DOM behaviour is only seen by the browser specs.

| Mutant (one line changed) | Spec that fails | Result |
|---|---|---|
| Disconnect leaves the question open | Go TestDisconnectDeniesAPendingQuestion... | killed |
| a run keeps the run slot | Go TestARunReleasesTheRunSlot | killed |
| no note for a Studio-side deny | Go TestDisconnectDenies... | killed |
| focus on Clear data instead of Cancel | cd-01 | killed |
| Enter presses Clear data | node keyAction + cd-03 | killed |
| a key-made click answers yes | cd-03 | killed |
| app id as html | cd-05 | killed |
| an unknown kind is not refused | cd-05 | killed |
| no key trap | cd-07 | killed |
| no focus guard | cd-07 | killed |
| a press on the dark area takes the focus | cd-07 | killed |
| Clear data is never disabled | cd-08 | killed |
| Clear data is never armed | cd-08 | killed |
| the app label loses 'from probe.yaml' | cd-09 | killed |

## 2026-11-15 — window run on a named simulator, then the full suite

- **Build under test:** `origin/main` at 07aeb43 (after #643, #645, #647), plus the two fixes below as they were made.
- **Window:** the real desktop app (`wails build`), driven through the screen.
- **Device:** simulator `ProbeStudioParity-iPhone17`, iOS 26, own agent port 48694.
- **App:** `native-test-apps/studio-fixture`, rebuilt from the same commit (`--dart-define=PROBE_AGENT=true PROBE_PORT=48694`).
- **Workspace:** a copy of the fixture's `probe-tests` plus `restart.probe`, opened through the native folder dialog.

### Failures found, fixed, and re-run

| # | Where | What was seen | Cause | Fix | Re-run |
|---|---|---|---|---|---|
| 1 | W-01 start-up | Studio reopened the last workspace, which no longer existed: the pane was blank and the line under the toolbar said "Open a .probe file from the Workspace pane" | the folder listing failed (a remembered folder that is gone is outside the folders Studio may read) and nothing drew a line for a failed or empty listing | the pane says the folder cannot be read, with an Open workspace button; the next step names the folder; an empty folder says it has no `.probe` files (`first-run.spec.ts` fr-06, node tests, 5 mutants killed) | window: the same start now shows the pane text and the next step ✅ |
| 2 | W-10 after `restart the app` | the live view said "no signal" for good, and the widget tree stopped updating | the run reconnects to the new agent after a restart step, but Studio kept the old connection (`use of closed network connection`) | `runner.RunOptions.OnClient` reports each new client; Studio takes it, restarts the live view and closes the old one (`restart-stream-device.spec.ts` rs-01, Go tests, 7 mutants killed) | window: after the restart run the label shows "11 fps" and the run passes ✅; device spec rs-01 passes (it failed before the fix with the same error text) |

| 3 | full suite, device specs before the seeded specs | 25 Suggest-a-fix specs failed (`#btn-fix` hidden) when the device specs ran first | a real run on the device replaces the seeded failed run that those specs read; reproduced on code without any of today's changes, so it was in the suite already | `playwright.config.ts`: the device specs are their own project that runs after the no-device project | the full run below |
| 4 | full suite, after fix 3 | the first device spec could not connect (`dial ... :48686 ... HTTP 401`) | `ui.spec.ts` 04 wrote a hard-coded `probe.yaml` (old `app: id:` key, port 48686) over the workspace file the script had prepared, and restored that | it reads the file and puts it back byte for byte | the full run below |

### Window run (W rows)

| ID | Result | Evidence |
|---|---|---|
| W-01 start, first screen | ✅ after fix 1 | pane text, next step, scratch-buffer placeholder |
| W-02 move the window | ✅ | dragged by the title bar; the window moved, the panes kept their layout |
| W-03 pick a workspace (native dialog) | ✅ | dialog opened from the pane button; folder chosen with Go to Folder; files listed; next step changed to "Open a .probe file" |
| W-04 navigate, open a file | ✅ | probe-tests opened, `counter.probe` and `restart.probe` opened with their content |
| C-01 device list | ✅ | booted simulators listed by name, shut-down ones marked, a physical phone marked "WiFi only"; picking a shut-down device says "That device is shut down: boot it, then press the refresh button" and keeps Connect disabled |
| W-07 connect | ✅ | status shows the device name, live view and widget tree load, the next-step line goes away |
| W-09 run a passing file (`counter.probe`) | ✅ | tick on each line as it runs, "1 PASSED", Run re-enabled |
| W-10 run with a restart step (`restart.probe`) | ✅ after fix 2 | restart step 2.8 s, "1 PASSED"; the live view keeps running |
| W-11 run a failing file (`failing.probe`) | ✅ | red line 5, failed step row with the error, a suggested fix and its reason |
| W-08 disconnect | ✅ | status "disconnected", Run and Record disabled, next step "Press Connect…" |
| W-15 help overlay | ✅ | opens with `?`, shows the shortcuts, closes with Escape |
| Network tab without a device | ✅ | "connect to a device first: the request log is read from the app" |
| Layout at the default window size | ➖ cosmetic | when connected the status text wraps to two lines and the "BETA PREVIEW" badge wraps; nothing overlaps or is cut. Not changed here. |
| Sideways scroll below about 1175 px | ➖ known, not ours | measured on the CSS before and after #658: the toolbar's own content is 828 px wide and its buttons and selects do not shrink, so the page scrolls sideways at any viewport narrower than 1150 px before #658 and 1117 px after. The window's minimum width is 1040 px, so between 1040 and 1117 px the bar scrolls by up to 77 px (110 px before #658). Letting the two selects shrink removes the scroll but leaves the device picker 34 px wide. A real fix is a design change (for example hiding the Run/Record/Save labels below about 1180 px); not done. |
| W-05 save, W-12 cancel, W-13 record, W-14 inspector refresh, W-16 keyboard reach | ➖ not run in the window | covered by the headless specs in the full-suite row below; not repeated by hand |

### Full suite (headless + device) at the end

A **manual run** on 2026-11-15 on the simulator `ProbeStudioParity-iPhone17` (not committed evidence: nothing in CI reproduces it, the logs are not kept): `scripts/studio-e2e-web.sh` on the same simulator, fixture app and ports, on the branch with fixes 2, 3 and 4 (fix 1 is its own PR, with its own spec fr-06): **141 passed, 5 skipped, 0 failed** (4.2 min). The 5 skipped are the physical-phone specs (`physical.spec.ts`), which need a phone and were not run. Before fix 3 and 4 the same command gave 25 failures, all from the two order problems above (fixes 3 and 4).
