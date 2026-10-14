# Studio E2E suite

End-to-end tests for FlutterProbe Studio that drive the **real desktop app**
the way a person does — no test hooks compiled into Studio.

This is the native smoke set: the two checks that need the real window. It needs
macOS, a second display and an unlocked session with Accessibility and Screen
Recording permission. Every other UI check runs in the headless browser suite
(`studio/e2e-web`); see [Modes](#modes).

- **Launch**: the built `.app` binary, with `PROBE_STUDIO_WORKSPACE` pointing at a workspace.
- **Find**: the macOS accessibility tree (`System Events`) — buttons by label, texts by content, the device picker as a pop-up.
- **Act**: clicks at element centres, keystrokes, pop-up selection.
- **Assert**: accessibility tree first; window screenshots on every test. The optional vision provider in `vision.go` (`STUDIO_E2E_AI_PROVIDER`) is not used by any test now: its only user was the live-screen check, which the browser suite covers.
- **Device**: a named iOS simulator (`StudioE2E-iPhone17` by default), created and booted by the suite; the fixture app under `native-test-apps/studio-fixture/` is installed and relaunched before every test.

## Run

```bash
STUDIO_E2E_DISPLAY=2 make studio-e2e-native        # builds Studio + fixture, runs the smoke set, writes reports/studio-e2e/
SKIP_BUILD=1 STUDIO_E2E_DISPLAY=2 scripts/studio-e2e.sh -run Test07   # reuse builds, one test
```

`make studio-e2e` still works for one release as a deprecated alias of
`make studio-e2e-native`.

Reports: `reports/studio-e2e/report.json`, `report.html`, `screenshots/`, `logs/`.

## The display rule

The suite never runs on the main display. It needs `STUDIO_E2E_DISPLAY=n`, with
n 2 or more, naming a display that is attached:

- **Unset**: the script refuses (exit 2) and says how many displays are attached.
  There is no default display.
- **1, zero, a negative number, or anything that is not a number**: refused. 1 is the main display.
- **Above the number of attached displays**: refused, with the count.
- **In CI** (`CI` is set, as on GitHub Actions) an unset variable skips the run
  with exit 0 and prints `studio-e2e: skipped: no second display configured`.
  A variable that is set but wrong still refuses.

The suite checks the same rule again before its first test. After moving a
window, the driver reads its position back and fails the test if the window did
not land on that display. A window is created on the main display for a moment
before it is moved.

The suite needs the machine's keyboard focus: typing into another window while a
test runs makes it fail.

## Modes

| Mode | Command | Window | Input |
|---|---|---|---|
| Headless browser (CI default) | `make studio-e2e-web`, or `STUDIO_E2E_NO_DEVICE=1 make studio-e2e-web` for the device-free tests | none | none |
| Headed browser (watching a run) | `STUDIO_E2E_HEADED=1 make studio-e2e-web` | a Chromium window, never a Studio window | Playwright |
| Native smoke set (opt-in) | `STUDIO_E2E_DISPLAY=2 make studio-e2e-native` | the real Studio and Simulator windows on display 2 | macOS accessibility and clicks |

A headed run of the real Studio window, driven by the automation endpoint, is
not implemented yet. It is tracked in #389.

## Tests

The smoke set is the whole native suite:

| # | Covers |
|---|---|
| 01 | Launch, workspace file list, toolbar, initial status |
| 07 | Recorder: three taps on the device pane → saved `.probe` with a tap step |

### Removed native tests (covered by the browser suite)

These native tests were removed once their browser ports were merged (#331,
phase 2). The browser test numbers are in `studio/e2e-web/README.md`; the
device tests need a booted named simulator (`STUDIO_E2E_UDID`).

| Removed native test | Covered by |
|---|---|
| 02 Device picker lists the named simulator with its platform tag | device 10; ui 22 (the platform tag) |
| 03 Connect → live device frame and widget tree; disconnect | device 11 |
| 04 Open a file, Run, pass rows and summary | device 12 |
| 05 Failing file → fail row, error row, failed summary | device 13 |
| 06 Performance lines under a result | device 14 |
| 08 Workspace settings overlay (probe.yaml form) | ui 04 |
| 09 AI chat pane toggle and API-key overlay | ui 05 |
| 10 Connect with the app not running → error state | device 18 |
| 11 WiFi discovery overlay | ui 06 |
| 12 Live steps, verdict row heads its steps, last step passes, Cancel hidden after the run | device 15, 17 |
| 13 Failing step keeps a `step fail` row naming its line, with the error | device 13, 16 |
| 14 Manual WiFi entry explains an empty host | ui 19 (malformed host) and 23 (empty host) |

Each test launches its own Studio, closes it, and relaunches the fixture app
(self-contained; explicit cleanup).

## Findings the suite documents (as of the first run)

- Studio's device pane does **not** forward clicks to the app (README still lists tap
  forwarding as a target). The recorder test therefore taps the Simulator window
  directly; `FindDevicePane` is kept for the live-frame assertion.
- `Stop` replaces the editor with the agent's assembled steps only — the
  `test "recorded flow"` header typed at `Record` is dropped. The test asserts the
  recorded tap lines, not the header.
- Studio did not honour the workspace `probe.yaml` (fixed in this change).

## Requirements

- macOS with Xcode (simulators), Flutter, Go, Wails CLI.
- A second display, named with `STUDIO_E2E_DISPLAY` (see [The display rule](#the-display-rule)),
  and an unlocked session.
- The terminal running the suite needs **Accessibility** and **Screen Recording**
  permission (System Settings → Privacy & Security) — `osascript` drives the UI
  and `screencapture` takes the window shots.
- A self-hosted macOS runner for CI (`.github/workflows/studio-e2e.yml`); hosted
  runners cannot grant those permissions. The workflow is manual only (no tag or
  pull-request trigger, so outside code never reaches the runner): start it with
  `gh workflow run "Studio E2E" --ref <branch>` once the runner is online.

## Dogfooding on your own app

See [DOGFOOD.md](DOGFOOD.md).
